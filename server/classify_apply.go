package mail

import (
	"context"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/classifier"
)

const (
	actionNone         = "none"
	actionLabel        = "label"
	actionLabelArchive = "label_archive"
)

// decideAction is the only place mail decides to archive. Every guard must
// hold: the user asked for archiving, the label allows it, the model is sure
// and its choice is its own top probability, nothing else has filed the
// thread, and the message started the thread (a reply in a conversation is
// never archived).
func decideAction(app core.App, userID, mode, threadID string, r classifier.Result, threshold float64) string {
	if mode == modePreview || mode == modeOff {
		return actionNone
	}
	if mode != modeLabelsArchive || !r.Label.AutoAction || r.ArgmaxMismatch || r.Confidence < threshold {
		return actionLabel
	}
	state := findThreadState(app, threadID, userID)
	if state == nil || state.GetString("folder") != "inbox" {
		return actionLabel
	}
	thread, err := app.FindRecordById("mail_threads", threadID)
	if err != nil || thread.GetInt("message_count") != 1 {
		return actionLabel
	}
	return actionLabelArchive
}

// applyAction returns the action it completed, which is what the saved row
// must say: "label" when the label landed but the archive failed.
func applyAction(app core.App, userID, threadID, action string, r classifier.Result) (string, error) {
	if action == actionNone {
		return actionNone, nil
	}
	if err := addThreadLabel(app, threadID, userID, r.Label.LabelID); err != nil {
		return actionNone, err
	}
	if action == actionLabelArchive {
		archived := false
		// The inbox check runs inside the state transaction: a rule or a user
		// that filed the thread after decideAction must win.
		err := modifyThreadState(context.Background(), app, threadID, userID, false, func(_ core.App, state *core.Record) error {
			if state.GetString("folder") != "inbox" {
				return errThreadStateUnchanged
			}
			state.Set("folder", "archive")
			archived = true
			return nil
		})
		if err != nil {
			return actionLabel, err
		}
		if !archived {
			return actionLabel, nil
		}
	}
	return action, nil
}
