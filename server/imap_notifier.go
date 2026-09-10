package mail

import "tinycld.org/core/mailproto"

// globalNotifier is this process's IMAP IDLE pub/sub. A self-hosted app is
// one org, so one notifier is correct; core exposes the type (rather than a
// package global) so a supervisor can hold one per deployment.
var globalNotifier = mailproto.NewIdleNotifier()
