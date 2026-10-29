package mail

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// findMailboxState returns the mail_imap_mailbox_state record for a mailbox,
// or nil when it has none yet.
func findMailboxState(app core.App, mailboxID string) (*core.Record, error) {
	records, err := app.FindRecordsByFilter(
		"mail_imap_mailbox_state",
		"mailbox = {:mailbox}",
		"",
		1,
		0,
		map[string]any{"mailbox": mailboxID},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to read mailbox state: %w", err)
	}
	if len(records) == 0 {
		return nil, nil
	}
	return records[0], nil
}

// createMailboxState writes the row a mailbox gets with its first UID
// (uid_validity=1, uid_next=1). The unique index on mailbox rejects a second
// row, so a concurrent first use fails here and then reads the winner's row.
func createMailboxState(app core.App, mailboxID string) error {
	collection, err := app.FindCollectionByNameOrId("mail_imap_mailbox_state")
	if err != nil {
		return fmt.Errorf("mail_imap_mailbox_state collection not found: %w", err)
	}
	record := core.NewRecord(collection)
	record.Set("mailbox", mailboxID)
	record.Set("uid_validity", 1)
	record.Set("uid_next", 1)
	return app.Save(record)
}

// bumpUIDNext reserves the next UID of a mailbox in one statement and returns
// it, or sql.ErrNoRows when the mailbox has no state row yet. A raw update,
// not a record save: the counter must move in one step, and the row has no
// hooks to run.
func bumpUIDNext(app core.App, mailboxID string) (uint32, error) {
	var uid int
	err := app.NonconcurrentDB().
		NewQuery("UPDATE {{mail_imap_mailbox_state}} SET uid_next = uid_next + 1 WHERE mailbox = {:mailbox} RETURNING uid_next - 1").
		Bind(dbx.Params{"mailbox": mailboxID}).
		Row(&uid)
	if err != nil {
		return 0, err
	}
	return uint32(uid), nil
}

// nextUID allocates the next UID for a mailbox.
//
// The single UPDATE is the whole allocation, so allocators cannot interleave:
// SQLite's one write connection already serializes them, inside or outside a
// transaction. No Go lock is held across it. A lock held while waiting for
// that connection deadlocked the server: inbound delivery and IMAP APPEND
// allocate inside a transaction that owns the connection, while a draft, a
// send or SMTP allocates outside one. The outside allocator took the lock and
// waited for the connection; the transaction then waited for the lock, and
// every write on the server, sign-in included, queued behind them until a
// restart.
func nextUID(app core.App, mailboxID string) (uint32, error) {
	uid, err := bumpUIDNext(app, mailboxID)
	if !errors.Is(err, sql.ErrNoRows) {
		return uid, err
	}
	if createErr := createMailboxState(app, mailboxID); createErr != nil {
		// Either a concurrent first use won the unique index, in which case
		// its row is there to bump, or the create failed for real.
		uid, err = bumpUIDNext(app, mailboxID)
		if err != nil {
			return 0, fmt.Errorf("failed to create mailbox state: %w", createErr)
		}
		return uid, nil
	}
	return bumpUIDNext(app, mailboxID)
}

// assignUID allocates the next UID for a message within a mailbox and sets
// imap_uid on the message record.
func assignUID(app core.App, mailboxID string, messageRecord *core.Record) (uint32, error) {
	uid, err := nextUID(app, mailboxID)
	if err != nil {
		return 0, fmt.Errorf("failed to allocate imap_uid: %w", err)
	}
	messageRecord.Set("imap_uid", int(uid))
	if err := app.Save(messageRecord); err != nil {
		return 0, fmt.Errorf("failed to set imap_uid on message: %w", err)
	}
	return uid, nil
}

// ensureMessageUID assigns a UID to a message if it doesn't have one yet.
// Called on message creation paths (inbound, send, draft).
func ensureMessageUID(app core.App, mailboxID string, messageRecord *core.Record) (uint32, error) {
	existing := messageRecord.GetInt("imap_uid")
	if existing > 0 {
		return uint32(existing), nil
	}
	return assignUID(app, mailboxID, messageRecord)
}

// lookupMessagesByUID queries mail_messages by imap_uid range within a mailbox's threads.
func lookupMessagesByUID(app core.App, mailboxID string, uidLow, uidHigh uint32) ([]*core.Record, error) {
	// Find all threads for this mailbox
	threads, err := app.FindRecordsByFilter(
		"mail_threads",
		"mailbox = {:mailbox}",
		"",
		0, // no limit
		0,
		map[string]any{"mailbox": mailboxID},
	)
	if err != nil || len(threads) == 0 {
		return nil, nil
	}

	threadIDs := make([]any, len(threads))
	for i, t := range threads {
		threadIDs[i] = t.Id
	}

	// Build filter for messages in these threads with UIDs in range
	messages, err := app.FindRecordsByFilter(
		"mail_messages",
		"imap_uid >= {:uidLow} && imap_uid <= {:uidHigh} && thread.mailbox = {:mailbox}",
		"imap_uid",
		0,
		0,
		map[string]any{"uidLow": int(uidLow), "uidHigh": int(uidHigh), "mailbox": mailboxID},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup messages by UID: %w", err)
	}
	return messages, nil
}

// mailboxUIDState reads a mailbox's UIDVALIDITY and UIDNEXT without creating
// its state row. SELECT and STATUS only read, so they must not write — the
// server may be read-only. A mailbox with no row yet has the values the row
// is created with (see createMailboxState) once its first UID is
// assigned.
func mailboxUIDState(app core.App, mailboxID string) (validity, next uint32, err error) {
	state, err := findMailboxState(app, mailboxID)
	if err != nil {
		return 0, 0, err
	}
	if state == nil {
		return 1, 1, nil
	}
	return uint32(state.GetInt("uid_validity")), uint32(state.GetInt("uid_next")), nil
}

// getMailboxUIDValidity returns the UIDVALIDITY for a mailbox.
func getMailboxUIDValidity(app core.App, mailboxID string) (uint32, error) {
	validity, _, err := mailboxUIDState(app, mailboxID)
	return validity, err
}

// getMailboxUIDNext returns the next UID that will be assigned for a mailbox.
func getMailboxUIDNext(app core.App, mailboxID string) (uint32, error) {
	_, next, err := mailboxUIDState(app, mailboxID)
	return next, err
}
