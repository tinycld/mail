package mail

import (
	"context"
	"strings"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/appstate"
	"tinycld.org/core/background"
	"tinycld.org/core/classifier"
)

// classifyInput holds only what classification reads. The InboundMessage it
// is built from carries attachment bytes, and the background job outlives the
// delivery request, so the job must not keep that message reachable.
type classifyInput struct {
	MessageID, ThreadID, MailboxID string
	FromEmail                      string
	// evidence is the metadata-stage payload; groups copy it before adding facts.
	evidence map[string]any
	// Bodies feed the second stage only; stripping HTML waits until it runs.
	strippedReply, textBody, htmlBody string
}

func newClassifyInput(messageID, threadID, mailboxID string, msg *InboundMessage) classifyInput {
	return classifyInput{
		MessageID: messageID, ThreadID: threadID, MailboxID: mailboxID,
		FromEmail:     msg.From.Email,
		evidence:      metadataEvidence(msg),
		strippedReply: msg.StrippedReply, textBody: msg.TextBody, htmlBody: msg.HTMLBody,
	}
}

type classifyGroup struct {
	taxonomy []classifier.TaxonomyLabel
	facts    senderFacts
	users    []string
	modes    map[string]string
}

// enqueueClassificationFn is the delivery path's seam: a test swaps it to see
// which deliveries queue a classification, because the job itself runs in the
// background and offers no barrier.
var enqueueClassificationFn = enqueueClassification

// maxConcurrentClassifications bounds simultaneous model calls per app, so a
// burst of deliveries queues instead of opening one request each.
const maxConcurrentClassifications = 4

var classifySlotsKey = appstate.NewKey[chan struct{}]("mail.classifySlots")

// acquireClassifySlot waits for a free slot or for ctx to end.
func acquireClassifySlot(ctx context.Context, app core.App) (release func(), err error) {
	slots := classifySlotsKey.GetOrInit(app, func() chan struct{} {
		return make(chan struct{}, maxConcurrentClassifications)
	})
	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// enqueueClassification runs after the delivery transaction commits, with the
// input's own copy of the message data.
func enqueueClassification(app core.App, in classifyInput) {
	if !classifier.Available(app) {
		return
	}
	background.For(app, "mail").Run("classify:"+in.MessageID, func(ctx context.Context) {
		release, err := acquireClassifySlot(ctx, app)
		if err != nil {
			return
		}
		defer release()
		if err := classifyMessage(ctx, app, in); err != nil {
			notifyLog.WarnContext(ctx, "classification failed", "messageID", in.MessageID, "err", err)
		}
	})
}

func classifyMessage(ctx context.Context, app core.App, in classifyInput) error {
	if !classifier.Available(app) {
		return nil
	}
	members, err := getMailboxMembers(app, in.MailboxID)
	if err != nil {
		return err
	}
	groups := map[string]*classifyGroup{}
	for _, m := range members {
		userID := m.GetString("user")
		mode, taxScope := userClassifierPrefs(app, userID)
		if mode == modeOff {
			continue
		}
		scopeUser := ""
		if taxScope == "personal" {
			scopeUser = userID
		}
		tax := loadTaxonomy(app, scopeUser)
		if len(tax) == 0 && scopeUser != "" {
			scopeUser, tax = "", loadTaxonomy(app, "")
		}
		if len(tax) == 0 {
			continue
		}
		facts := factsFor(ctx, app, userID, in.FromEmail)
		key := scopeUser + "|" + facts.key()
		g := groups[key]
		if g == nil {
			g = &classifyGroup{taxonomy: tax, facts: facts, modes: map[string]string{}}
			groups[key] = g
		}
		g.users = append(g.users, userID)
		g.modes[userID] = mode
	}
	threshold := archiveThreshold(app)
	for _, g := range groups {
		runGroup(ctx, app, in, g, threshold)
	}
	return nil
}

func runGroup(ctx context.Context, app core.App, in classifyInput, g *classifyGroup, threshold float64) {
	anyArchive := false
	for _, mode := range g.modes {
		anyArchive = anyArchive || mode == modeLabelsArchive
	}
	evidence := make(map[string]any, len(in.evidence))
	for k, v := range in.evidence {
		evidence[k] = v
	}
	for k, v := range g.facts.evidence() {
		evidence[k] = v
	}
	res, err := classifier.Classify(ctx, app, classifier.Request{
		Taxonomy: g.taxonomy,
		Evidence: evidence,
		FullEvidence: func(context.Context) (map[string]any, error) {
			return map[string]any{"body": in.bodyText()}, nil
		},
		NeedsFull: func(r classifier.Result) bool {
			return anyArchive && r.Label.AutoAction && r.Confidence < threshold
		},
	})
	for _, userID := range g.users {
		row := classifier.Row{
			App: classifierApp, Collection: "mail_messages", RecordID: in.MessageID, User: userID,
			Preview: g.modes[userID] == modePreview, Error: classifier.ErrorCode(err),
		}
		if err == nil {
			action := decideAction(app, userID, g.modes[userID], in.ThreadID, res, threshold)
			done, aerr := applyAction(app, userID, in.ThreadID, action, res)
			if aerr != nil {
				notifyLog.WarnContext(ctx, "applying classification failed", "messageID", in.MessageID, "userID", userID, "err", aerr)
			}
			action = done
			row.ClassifierLabelID, row.LabelName = res.Label.ID, res.Label.Name
			row.Confidence, row.Probabilities = res.Confidence, res.Probabilities
			row.Stage, row.Model, row.Action = res.Stage, res.Model, action
			// Cost is per call; charge it once, to the first row of the group.
			if userID == g.users[0] {
				row.CostUSD = res.CostUSD
			}
		}
		if serr := classifier.SaveRow(app, row); serr != nil {
			notifyLog.WarnContext(ctx, "saving classification failed", "messageID", in.MessageID, "userID", userID, "err", serr)
		}
	}
}

func headerValue(msg *InboundMessage, name string) string {
	for _, h := range msg.Headers {
		if strings.EqualFold(h.Name, name) {
			return h.Value
		}
	}
	return ""
}

func addressList(rs []Recipient) string {
	parts := make([]string, 0, len(rs))
	for _, r := range rs {
		parts = append(parts, r.Email)
	}
	return strings.Join(parts, ", ")
}

func metadataEvidence(msg *InboundMessage) map[string]any {
	snippet := msg.StrippedReply
	if snippet == "" {
		snippet = msg.TextBody
	}
	size := len(msg.TextBody) + len(msg.HTMLBody)
	for _, a := range msg.Attachments {
		size += int(a.Size)
	}
	return map[string]any{
		"size_bytes":            size,
		"from":                  strings.TrimSpace(msg.From.Name + " <" + msg.From.Email + ">"),
		"to":                    addressList(msg.To),
		"cc":                    addressList(msg.Cc),
		"reply_to":              headerValue(msg, "Reply-To"),
		"subject":               msg.Subject,
		"date":                  msg.Date,
		"snippet":               snippet,
		"list_id":               headerValue(msg, "List-Id"),
		"list_unsubscribe":      headerValue(msg, "List-Unsubscribe"),
		"list_unsubscribe_post": headerValue(msg, "List-Unsubscribe-Post"),
		"auto_submitted":        headerValue(msg, "Auto-Submitted"),
		"precedence":            headerValue(msg, "Precedence"),
		"in_reply_to":           msg.InReplyTo,
		"references":            msg.References,
	}
}

func (in classifyInput) bodyText() string {
	switch {
	case in.strippedReply != "":
		return in.strippedReply
	case in.textBody != "":
		return in.textBody
	default:
		return stripHTMLToText(in.htmlBody)
	}
}
