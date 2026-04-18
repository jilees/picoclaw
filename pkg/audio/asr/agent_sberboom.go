//go:build sberboom

package asr

import (
	"context"

	"github.com/sipeed/picoclaw/pkg/bus"
)

type Agent struct{}

func NewAgent(mb *bus.MessageBus, t Transcriber) *Agent { return &Agent{} }
func (a *Agent) Start(ctx context.Context)              {}
