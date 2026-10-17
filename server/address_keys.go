package mail

import (
	"fmt"
	"slices"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// addressKeysMaxLen is the max of mail_threads.address_keys and
// mail_thread_state.address_keys (migration 2040000014).
const addressKeysMaxLen = 2000

// parseAddressKeys splits a stored address_keys value into its ids.
func parseAddressKeys(s string) []string {
	return strings.Fields(s)
}

// formatAddressKeys is the stored form of an id set: sorted, without
// duplicates, joined with one space.
func formatAddressKeys(ids []string) string {
	sorted := slices.Clone(ids)
	slices.Sort(sorted)
	return strings.Join(slices.Compact(sorted), " ")
}

// addAddressKeys adds the given address ids to the thread's address_keys and
// saves the thread only when an id was added, so most messages on a thread
// cost no extra save. The thread after-update hook (thread_state_sync.go)
// copies the new value to the thread's state rows.
//
// A set that would pass the field's max is kept as it is and logged: a failed
// thread save would fail the delivery or send that called this, and a
// missing key only leaves the thread out of one address view.
func addAddressKeys(app core.App, threadID string, ids ...string) error {
	thread, err := app.FindRecordById("mail_threads", threadID)
	if err != nil {
		return fmt.Errorf("thread for address keys: %w", err)
	}
	keys := parseAddressKeys(thread.GetString("address_keys"))
	added := false
	for _, id := range ids {
		if id != "" && !slices.Contains(keys, id) {
			keys = append(keys, id)
			added = true
		}
	}
	if !added {
		return nil
	}
	next := formatAddressKeys(keys)
	if len(next) > addressKeysMaxLen {
		storeLog.Warn("thread address list is full; new addresses not recorded",
			"thread", threadID, "length", len(thread.GetString("address_keys")), "adding", ids)
		return nil
	}
	thread.Set("address_keys", next)
	return app.Save(thread)
}
