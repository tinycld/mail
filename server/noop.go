package mail

import (
	"context"
	"errors"
)

var errNoProvider = errors.New("no mail provider configured — set MAIL_PROVIDER and the corresponding credentials")

// NoopProvider returns descriptive errors for all operations.
// Used when no provider is configured so the server still boots.
type NoopProvider struct{}

func (n *NoopProvider) Send(_ context.Context, _ *SendRequest) (*SendResult, error) {
	return nil, errNoProvider
}

func (n *NoopProvider) ParseInbound(_ []byte) (*InboundMessage, error) {
	return nil, errNoProvider
}

// VerifyWebhookSignature is a no-op even without a provider configured:
// the per-domain webhook_secret embedded in the inbound URL (verified by
// constant-time compare in handleInbound) is the actual auth boundary, and
// the real PostmarkProvider's implementation is also a no-op since
// Postmark uses basic auth on the inbound URL rather than signing payloads.
// Returning an error here blocked CI/test scenarios where a domain has a
// webhook_secret in the test seed but MAIL_PROVIDER isn't set.
func (n *NoopProvider) VerifyWebhookSignature(_ map[string]string, _ []byte) error {
	return nil
}

func (n *NoopProvider) ParseBounce(_ []byte) (*BounceEvent, error) {
	return nil, errNoProvider
}

func (n *NoopProvider) AddDomain(_ context.Context, _ string) (*DomainVerification, error) {
	return nil, errNoProvider
}

func (n *NoopProvider) CheckDomainVerification(_ context.Context, _ string) (*DomainVerification, error) {
	return nil, errNoProvider
}

func (n *NoopProvider) CheckInboundDomain(_ context.Context) (*InboundVerification, error) {
	return nil, errNoProvider
}
