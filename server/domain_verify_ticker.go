package mail

import (
	"context"
	"time"

	"github.com/pocketbase/pocketbase"
)

const (
	domainReverifyInterval = 1 * time.Hour
	domainReverifyDelay    = 30 * time.Second
	domainReverifyTimeout  = 30 * time.Second
)

// startDomainReverifyLoop runs an hourly re-check of any mail_domains rows
// that are not yet fully verified. Verified rows are skipped — a user clicks
// Verify again if they change DNS after success.
func startDomainReverifyLoop(app *pocketbase.PocketBase) {
	time.Sleep(domainReverifyDelay)
	reverifyUnconfirmedDomains(app)

	ticker := time.NewTicker(domainReverifyInterval)
	defer ticker.Stop()
	for range ticker.C {
		reverifyUnconfirmedDomains(app)
	}
}

func reverifyUnconfirmedDomains(app *pocketbase.PocketBase) {
	records, err := app.FindRecordsByFilter(
		"mail_domains",
		"verified = false",
		"",
		0,
		0,
	)
	if err != nil {
		app.Logger().Warn("mail: failed to list unverified domains for reverify", "error", err)
		return
	}
	if len(records) == 0 {
		return
	}

	app.Logger().Info("mail: reverifying unconfirmed domains", "count", len(records))
	for _, record := range records {
		ctx, cancel := context.WithTimeout(context.Background(), domainReverifyTimeout)
		if _, err := verifyDomainRecord(ctx, app, record); err != nil {
			app.Logger().Warn("mail: reverify failed",
				"domain", record.GetString("domain"),
				"error", err)
		}
		cancel()
	}
}
