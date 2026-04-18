//go:build !sberboom

package gateway

import (
	"strings"

	"github.com/sipeed/picoclaw/pkg/channels/pico"
	"github.com/sipeed/picoclaw/pkg/config"
)

func overridePicoToken(cfg *config.Config, token string) {
	if !cfg.Channels.Pico.Enabled {
		return
	}
	picoToken := cfg.Channels.Pico.Token.String()
	if picoToken == "" || strings.HasPrefix(picoToken, pico.PicoTokenPrefix) {
		return
	}
	cfg.Channels.Pico.SetToken(pico.PicoTokenPrefix + token + picoToken)
}
