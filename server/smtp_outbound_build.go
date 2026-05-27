package mail

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"time"

	gomail "github.com/emersion/go-message/mail"
)

// buildOutgoingRFC5322 serializes a SendRequest into RFC 5322 bytes for direct
// SMTP delivery. Threading headers (In-Reply-To, References) are emitted when
// present. Attachments are emitted as a multipart/mixed wrapper around the
// text/html alternative. The Message-ID is injected (the caller generates it
// up-front so the same value can be stored locally for thread matching).
func buildOutgoingRFC5322(req *SendRequest, messageID, helo string) ([]byte, error) {
	var buf bytes.Buffer

	var h gomail.Header
	h.SetDate(time.Now().UTC())
	h.SetSubject(req.Subject)
	h.SetMessageID(messageID)

	if req.From != "" {
		fromAddrs, err := gomail.ParseAddressList(req.From)
		if err == nil && len(fromAddrs) > 0 {
			h.SetAddressList("From", fromAddrs)
		} else {
			// Fall back: treat as a bare address with no display name.
			h.SetAddressList("From", []*gomail.Address{{Address: req.From}})
		}
	}
	if req.ReplyTo != "" {
		if addrs, err := gomail.ParseAddressList(req.ReplyTo); err == nil && len(addrs) > 0 {
			h.SetAddressList("Reply-To", addrs)
		}
	}
	if len(req.To) > 0 {
		h.SetAddressList("To", recipientsToAddrs(req.To))
	}
	if len(req.Cc) > 0 {
		h.SetAddressList("Cc", recipientsToAddrs(req.Cc))
	}
	// Bcc is deliberately omitted from headers — it lives only in the SMTP
	// envelope, per RFC 5322 §3.6.3.

	if req.InReplyTo != "" {
		h.Set("In-Reply-To", req.InReplyTo)
	}
	if req.References != "" {
		h.Set("References", req.References)
	}
	for _, hdr := range req.Headers {
		// Custom headers from the caller are appended verbatim. We do not
		// overwrite headers we've already set (Subject, From, etc.).
		if hdr.Name == "" {
			continue
		}
		h.Set(hdr.Name, hdr.Value)
	}

	if len(req.Attachments) > 0 {
		mw, err := gomail.CreateWriter(&buf, h)
		if err != nil {
			return nil, fmt.Errorf("create writer: %w", err)
		}
		if err := writeAlternativePart(mw, req.TextBody, req.HTMLBody); err != nil {
			mw.Close()
			return nil, err
		}
		for _, att := range req.Attachments {
			if err := writeAttachmentFromBase64(mw, att); err != nil {
				return nil, err
			}
		}
		mw.Close()
		return buf.Bytes(), nil
	}

	if req.HTMLBody != "" {
		mw, err := gomail.CreateWriter(&buf, h)
		if err != nil {
			return nil, fmt.Errorf("create writer: %w", err)
		}
		if err := writeAlternativePart(mw, req.TextBody, req.HTMLBody); err != nil {
			mw.Close()
			return nil, err
		}
		mw.Close()
		return buf.Bytes(), nil
	}

	h.SetContentType("text/plain", map[string]string{"charset": "utf-8"})
	w, err := gomail.CreateSingleInlineWriter(&buf, h)
	if err != nil {
		return nil, fmt.Errorf("create plain writer: %w", err)
	}
	io.WriteString(w, req.TextBody)
	w.Close()
	return buf.Bytes(), nil
}

func recipientsToAddrs(rs []Recipient) []*gomail.Address {
	out := make([]*gomail.Address, 0, len(rs))
	for _, r := range rs {
		out = append(out, &gomail.Address{Name: r.Name, Address: r.Email})
	}
	return out
}

func writeAttachmentFromBase64(mw *gomail.Writer, att Attachment) error {
	data, err := base64.StdEncoding.DecodeString(att.Content)
	if err != nil {
		return fmt.Errorf("decode attachment %q: %w", att.Name, err)
	}
	contentType := att.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	var ah gomail.InlineHeader
	ah.SetContentType(contentType, map[string]string{"name": att.Name})
	ah.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", att.Name))
	ah.Set("Content-Transfer-Encoding", "base64")
	if att.ContentID != "" {
		ah.Set("Content-ID", "<"+att.ContentID+">")
	}

	aw, err := mw.CreateSingleInline(ah)
	if err != nil {
		return fmt.Errorf("create attachment part: %w", err)
	}
	encoder := base64.NewEncoder(base64.StdEncoding, aw)
	encoder.Write(data)
	encoder.Close()
	aw.Close()
	return nil
}
