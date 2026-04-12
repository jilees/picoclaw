package sberboom

import (
	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/channels"
	"github.com/sipeed/picoclaw/pkg/config"
)

func init() {
	channels.RegisterFactory("sberboom", func(cfg *config.Config, b *bus.MessageBus) (channels.Channel, error) {
		return NewSberBoomChannel(cfg.Channels.SberBoom, b)
	})
}
