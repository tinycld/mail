package mail

import (
	"testing"

	"github.com/pocketbase/pocketbase"

	"tinycld.org/core/rlstest"
)

// Tripwire for host/tenant drift inside this package, mirroring
// coreserver/composition_parity_test.go one level down: Register (single-org
// app) may bind more than RegisterTenant (multi-org tenant) ONLY where the
// map below records the divergence with a reason. Adding a registration to
// one entry point without deciding whether the other gets it fails here with
// the offending hook name. See multi-org/docs/FINDING-tenant-composition-gap.md
// for what silent divergence cost.
func TestTenantCompositionMatchesHostMinusRecordedExceptions(t *testing.T) {
	host := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	Register(host)

	tenant := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	RegisterTenant(tenant)

	rlstest.AssertCompositionDiff(t,
		rlstest.HookHandlerCounts(t, host),
		rlstest.HookHandlerCounts(t, tenant),
		map[string]int{
			// registerMailListeners: the IMAP / SMTP-submission / inbound-SMTP
			// listeners bind fixed TCP ports, and the multi-org router owns
			// every listening socket — a tenant must not start them (its
			// OnTerminate shutdown binds happen at serve time, so only the
			// one OnServe handler shows at composition time).
			"OnServe": 1,
		})
}
