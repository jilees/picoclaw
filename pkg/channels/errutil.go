package channels

import (
	"errors"
	"fmt"
	"net"
	"net/http"
)

// ClassifySendError wraps a raw error with the appropriate sentinel based on
// an HTTP status code. Channels that perform HTTP API calls should use this
// in their Send path.
func ClassifySendError(statusCode int, rawErr error) error {
	switch {
	case statusCode == http.StatusTooManyRequests:
		return fmt.Errorf("%w: %v", ErrRateLimit, rawErr)
	case statusCode >= 500:
		return fmt.Errorf("%w: %v", ErrTemporary, rawErr)
	case statusCode >= 400:
		return fmt.Errorf("%w: %v", ErrSendFailed, rawErr)
	default:
		return rawErr
	}
}

// ClassifyNetError wraps a network/timeout error as ErrTemporary.
func ClassifyNetError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", ErrTemporary, err)
}

// IsNetworkUnavailable reports whether err is a low-level network error
// indicating the interface is not reachable yet (e.g. "network is unreachable",
// DNS lookup failure). Returns true for *net.OpError and *net.DNSError.
// Returns false for TLS errors, HTTP handshake failures, and application errors.
//
// Use in channel Start() methods to distinguish "no WiFi yet" from "bad config"
// so the channel can defer its connection rather than aborting.
func IsNetworkUnavailable(err error) bool {
	if err == nil {
		return false
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr)
}
