package mail

import "tinycld.org/core/mailproto"

// globalNotifier is this process's IMAP IDLE pub/sub. The single-tenant app is
// one org, so one notifier is correct; core exposes the type (rather than a
// package global) so a multi-org host can hold one per tenant.
var globalNotifier = mailproto.NewIdleNotifier()
