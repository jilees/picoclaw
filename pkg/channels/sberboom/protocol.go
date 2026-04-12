package sberboom

import "time"

// Message types sent from PicoClaw → Skill Backend.
const (
	TypeRegister   = "register"    // initial identification after connect
	TypeEndSession = "end_session" // PicoClaw requests dialog termination
	TypePing       = "ping"
)

// Message types sent from Skill Backend → PicoClaw.
const (
	TypeMessage    = "message"     // transcribed user utterance from Sber platform
	TypeSessionEnd = "session_end" // backend signals session closed
	TypeError      = "error"
	TypePong       = "pong"
)

// SberMessage is the wire format for all WebSocket messages.
type SberMessage struct {
	Type      string `json:"type"`
	ID        string `json:"id,omitempty"`
	Text      string `json:"text,omitempty"`       // type=message: transcribed user speech
	UserID    string `json:"user_id,omitempty"`    // type=register: Sber user ID
	Token     string `json:"token,omitempty"`      // type=register: optional auth token
	SessionID string `json:"session_id,omitempty"` // type=message: Sber session ID
	Code      string `json:"code,omitempty"`       // type=error: error code
	Message   string `json:"message,omitempty"`    // type=error: human-readable description
	Timestamp int64  `json:"timestamp,omitempty"`
}

func newMessage(msgType string) SberMessage {
	return SberMessage{
		Type:      msgType,
		Timestamp: time.Now().UnixMilli(),
	}
}
