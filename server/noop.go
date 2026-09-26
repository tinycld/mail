package mail

import (
	"context"
	"errors"
)

var errNoProvider = errors.New("no mail provider configured — configure one in Settings › System › Mail — Provider")

// NoopProvider returns descriptive errors for all operations.
// Used when no provider is configured so the server still boots.
type NoopProvider struct{}

func (n *NoopProvider) Configured() bool { return false }

func (n *NoopProvider) Send(_ context.Context, _ *SendRequest) (*SendResult, error) {
	return nil, errNoProvider
}

func (n *NoopProvider) ParseInbound(_ []byte) (*InboundMessage, error) {
	return nil, errNoProvider
}

func (n *NoopProvider) VerifyWebhookSignature(_ map[string]string, _ []byte) error {
	return errNoProvider
}

func (n *NoopProvider) ParseBounce(_ []byte) (*BounceEvent, error) {
	return nil, errNoProvider
}

func (n *NoopProvider) CheckInboundDomain(_ context.Context) (*InboundVerification, error) {
	return nil, errNoProvider
}
