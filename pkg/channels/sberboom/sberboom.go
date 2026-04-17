package sberboom

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/identity"
	"github.com/sipeed/picoclaw/pkg/isolation"
	"github.com/sipeed/picoclaw/pkg/logger"
)

const (
	channelName = "sberboom"

	defaultPingInterval = 30 * time.Second
	defaultReadTimeout  = 60 * time.Second
	reconnectBackoff    = 5 * time.Second

	// voiceInstruction is appended to every inbound message so the LLM replies
	// in a concise, TTS-friendly style (no markdown, short sentences).
	voiceInstruction = "\n\n[SYSTEM]: The user spoke this via voice on a smart speaker. " +
		"Reply in 1–3 short sentences. No markdown, asterisks, code blocks, or emojis. " +
		"Natural spoken style only."
)

// sberConn wraps a single WebSocket connection with write serialization and
// a closed flag so concurrent goroutines can detect disconnection safely.
type sberConn struct {
	ws     *websocket.Conn
	writeMu sync.Mutex
	closed  atomic.Bool
	cancel  context.CancelFunc
}

func (sc *sberConn) writeJSON(v any) error {
	if sc.closed.Load() {
		return fmt.Errorf("connection closed")
	}
	sc.writeMu.Lock()
	defer sc.writeMu.Unlock()
	return sc.ws.WriteJSON(v)
}

func (sc *sberConn) close() {
	if sc.closed.CompareAndSwap(false, true) {
		if sc.cancel != nil {
			sc.cancel()
		}
		sc.ws.Close()
	}
}

// SberBoomChannel connects PicoClaw to a SberBoom skill backend over WebSocket.
// PicoClaw acts as a WebSocket client: it dials the backend, registers its Sber
// user ID, then listens for transcribed user utterances. Responses are played
// locally via a configurable TTS shell script.
type SberBoomChannel struct {
	*channels.BaseChannel
	config config.SberBoomConfig
	conn   *sberConn
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	userID string
}

// NewSberBoomChannel creates a new SberBoom channel.
func NewSberBoomChannel(cfg config.SberBoomConfig, messageBus *bus.MessageBus) (*SberBoomChannel, error) {
	if cfg.BackendURL == "" {
		return nil, fmt.Errorf("sberboom: backend_url is required")
	}
	if cfg.TTSScript == "" {
		return nil, fmt.Errorf("sberboom: tts_script is required")
	}

	base := channels.NewBaseChannel(channelName, cfg, messageBus, cfg.AllowFrom)

	return &SberBoomChannel{
		BaseChannel: base,
		config:      cfg,
	}, nil
}

// Start resolves the user ID, dials the backend, and launches background goroutines.
func (c *SberBoomChannel) Start(ctx context.Context) error {
	logger.InfoC(channelName, "Starting SberBoom channel")

	c.userID = strings.TrimSpace(c.config.UserID)
	if c.userID == "" {
		id, err := c.detectDeviceID(ctx)
		if err != nil || strings.TrimSpace(id) == "" {
			if err == nil {
				err = fmt.Errorf("command returned empty output")
			}
			return fmt.Errorf("sberboom: user_id not set and device ID detection failed: %w", err)
		}
		c.userID = strings.TrimSpace(id)
		logger.InfoCF(channelName, "Auto-detected device ID", map[string]any{"user_id": c.userID})
	}

	c.ctx, c.cancel = context.WithCancel(ctx)

	if err := c.dial(); err != nil {
		c.cancel()
		return fmt.Errorf("sberboom: initial connect failed: %w", err)
	}

	c.SetRunning(true)
	go c.reconnectLoop()

	logger.InfoCF(channelName, "Connected to skill backend", map[string]any{"url": c.config.BackendURL})
	return nil
}

// Stop gracefully shuts down the channel.
func (c *SberBoomChannel) Stop(_ context.Context) error {
	logger.InfoC(channelName, "Stopping SberBoom channel")
	c.SetRunning(false)
	if c.cancel != nil {
		c.cancel()
	}
	c.mu.Lock()
	if c.conn != nil {
		c.conn.close()
	}
	c.mu.Unlock()
	logger.InfoC(channelName, "SberBoom channel stopped")
	return nil
}

// Send plays the agent response through the local TTS shell script.
// It runs the script in a goroutine so the agent loop is not blocked.
func (c *SberBoomChannel) Send(ctx context.Context, msg bus.OutboundMessage) ([]string, error) {
	if !c.IsRunning() {
		return nil, channels.ErrNotRunning
	}
	text := msg.Content
	go func() {
		if err := c.runTTS(text); err != nil {
			logger.WarnCF(channelName, "TTS script failed", map[string]any{"error": err.Error()})
		}
	}()
	return nil, nil
}

// VoiceCapabilities declares that this channel uses TTS (handled locally via script).
// ASR is false because the Sber platform performs speech recognition itself.
func (c *SberBoomChannel) VoiceCapabilities() channels.VoiceCapabilities {
	return channels.VoiceCapabilities{ASR: false, TTS: true}
}

// dial establishes a WebSocket connection to the skill backend and launches
// the read/ping loops for that connection.
func (c *SberBoomChannel) dial() error {
	header := http.Header{}
	if tok := c.config.Token.String(); tok != "" {
		header.Set("Authorization", "Bearer "+tok)
	}

	ws, resp, err := websocket.DefaultDialer.DialContext(c.ctx, c.config.BackendURL, header)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err != nil {
		return err
	}

	connCtx, connCancel := context.WithCancel(c.ctx)
	sc := &sberConn{ws: ws, cancel: connCancel}

	// Register this device with the backend.
	reg := newMessage(TypeRegister)
	reg.UserID = c.userID
	if tok := c.config.Token.String(); tok != "" {
		reg.Token = tok
	}
	if err := sc.writeJSON(reg); err != nil {
		connCancel()
		ws.Close()
		return fmt.Errorf("register send failed: %w", err)
	}

	c.mu.Lock()
	c.conn = sc
	c.mu.Unlock()

	go c.readLoop(connCtx, sc)
	return nil
}

// reconnectLoop re-dials when the connection drops.
// Pattern mirrors pkg/channels/pico/client.go reconnectLoop.
func (c *SberBoomChannel) reconnectLoop() {
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		c.mu.Lock()
		sc := c.conn
		c.mu.Unlock()

		if sc == nil || sc.closed.Load() {
			logger.InfoC(channelName, "Reconnecting to skill backend...")
			if err := c.dial(); err != nil {
				logger.WarnCF(channelName, "Reconnect failed", map[string]any{"error": err.Error()})
				select {
				case <-c.ctx.Done():
					return
				case <-time.After(reconnectBackoff):
				}
				continue
			}
			logger.InfoC(channelName, "Reconnected to skill backend")
		}

		select {
		case <-c.ctx.Done():
			return
		case <-time.After(1 * time.Second):
		}
	}
}

// readLoop reads messages from the backend for one connection lifetime.
func (c *SberBoomChannel) readLoop(connCtx context.Context, sc *sberConn) {
	defer sc.close()

	readTimeout := time.Duration(c.config.ReadTimeout) * time.Second
	if readTimeout <= 0 {
		readTimeout = defaultReadTimeout
	}
	_ = sc.ws.SetReadDeadline(time.Now().Add(readTimeout))
	sc.ws.SetPongHandler(func(string) error {
		return sc.ws.SetReadDeadline(time.Now().Add(readTimeout))
	})

	pingInterval := time.Duration(c.config.PingInterval) * time.Second
	if pingInterval <= 0 {
		pingInterval = defaultPingInterval
	}
	go c.pingLoop(connCtx, sc, pingInterval)

	for {
		select {
		case <-connCtx.Done():
			return
		default:
		}

		_, raw, err := sc.ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				logger.DebugCF(channelName, "Read error", map[string]any{"error": err.Error()})
			}
			return
		}
		_ = sc.ws.SetReadDeadline(time.Now().Add(readTimeout))

		var msg SberMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		c.handleInbound(msg)
	}
}

// pingLoop sends WebSocket ping frames to keep the connection alive.
func (c *SberBoomChannel) pingLoop(connCtx context.Context, sc *sberConn, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-connCtx.Done():
			return
		case <-ticker.C:
			if sc.closed.Load() {
				return
			}
			sc.writeMu.Lock()
			err := sc.ws.WriteMessage(websocket.PingMessage, nil)
			sc.writeMu.Unlock()
			if err != nil {
				return
			}
		}
	}
}

// handleInbound dispatches a message received from the skill backend.
func (c *SberBoomChannel) handleInbound(msg SberMessage) {
	switch msg.Type {
	case TypeMessage:
		c.handleUserMessage(msg)
	case TypeSessionEnd:
		logger.InfoC(channelName, "Session ended by backend")
	case TypeError:
		logger.WarnCF(channelName, "Backend error", map[string]any{
			"code":    msg.Code,
			"message": msg.Message,
		})
	case TypePong:
		// handled by pong handler on the ws connection
	default:
		logger.DebugCF(channelName, "Ignoring unknown message type", map[string]any{"type": msg.Type})
	}
}

// handleUserMessage publishes a transcribed user utterance to the agent bus.
func (c *SberBoomChannel) handleUserMessage(msg SberMessage) {
	text := strings.TrimSpace(msg.Text)
	if text == "" {
		return
	}

	// Append voice instruction so the LLM replies in a TTS-friendly style.
	// This follows the same pattern as pkg/audio/asr/agent.go.
	content := text + voiceInstruction

	chatID := channelName + ":" + c.userID
	peer := bus.Peer{Kind: "direct", ID: c.userID}
	sender := bus.SenderInfo{
		Platform:    channelName,
		PlatformID:  c.userID,
		CanonicalID: identity.BuildCanonicalID(channelName, c.userID),
	}

	c.HandleMessage(
		c.ctx,
		peer,
		msg.ID,
		c.userID,
		chatID,
		content,
		nil,
		map[string]string{
			"is_voice":   "true",
			"session_id": msg.SessionID,
		},
		sender,
	)
}

// detectDeviceID reads the SberBoom device ID from the system database.
func (c *SberBoomChannel) detectDeviceID(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c",
		`cat /proc/cmdline | grep -o 'androidboot.serialno=[^ ]*' | cut -d= -f2`)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := isolation.Run(cmd); err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

// runTTS executes the configured TTS shell script with the response text.
func (c *SberBoomChannel) runTTS(text string) error {
	cmd := exec.Command(c.config.TTSScript, text)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := isolation.Run(cmd); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return err
	}
	return nil
}

