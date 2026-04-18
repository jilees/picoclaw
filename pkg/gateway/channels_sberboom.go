//go:build sberboom

package gateway

import (
	_ "github.com/sipeed/picoclaw/pkg/channels/mattermost"
	_ "github.com/sipeed/picoclaw/pkg/channels/sberboom"
	_ "github.com/sipeed/picoclaw/pkg/channels/telegram"
)
