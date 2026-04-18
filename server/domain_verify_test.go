package mail

import (
	"context"
	"errors"
	"net"
	"testing"
)

func withMXLookup(t *testing.T, fn func(ctx context.Context, name string) ([]*net.MX, error)) {
	t.Helper()
	orig := mxLookup
	mxLookup = fn
	t.Cleanup(func() { mxLookup = orig })
}

func TestCheckMX_MatchesPostmarkHost(t *testing.T) {
	withMXLookup(t, func(_ context.Context, _ string) ([]*net.MX, error) {
		return []*net.MX{{Host: "inbound.postmarkapp.com.", Pref: 10}}, nil
	})
	got := checkMX(context.Background(), "example.com")
	if !got.OK {
		t.Fatalf("expected MX check OK, got %+v", got)
	}
	if len(got.Actual) != 1 || got.Actual[0] != "inbound.postmarkapp.com" {
		t.Fatalf("expected normalized actual host, got %+v", got.Actual)
	}
}

func TestCheckMX_RejectsWrongHost(t *testing.T) {
	withMXLookup(t, func(_ context.Context, _ string) ([]*net.MX, error) {
		return []*net.MX{{Host: "mail.google.com.", Pref: 10}}, nil
	})
	got := checkMX(context.Background(), "example.com")
	if got.OK {
		t.Fatalf("expected MX check to fail, got OK")
	}
}

func TestCheckMX_EmptyRecords(t *testing.T) {
	withMXLookup(t, func(_ context.Context, _ string) ([]*net.MX, error) {
		return nil, nil
	})
	got := checkMX(context.Background(), "example.com")
	if got.OK {
		t.Fatalf("expected failure on empty MX set")
	}
	if got.Error == "" {
		t.Fatalf("expected error message for empty MX set")
	}
}

func TestCheckMX_LookupError(t *testing.T) {
	withMXLookup(t, func(_ context.Context, _ string) ([]*net.MX, error) {
		return nil, errors.New("boom")
	})
	got := checkMX(context.Background(), "example.com")
	if got.OK || got.Error != "boom" {
		t.Fatalf("expected error passthrough, got %+v", got)
	}
}

func TestCheckMX_MultipleRecordsOneMatches(t *testing.T) {
	withMXLookup(t, func(_ context.Context, _ string) ([]*net.MX, error) {
		return []*net.MX{
			{Host: "alt.example.com.", Pref: 20},
			{Host: "INBOUND.POSTMARKAPP.COM.", Pref: 10},
		}, nil
	})
	got := checkMX(context.Background(), "example.com")
	if !got.OK {
		t.Fatalf("expected match on case-insensitive host; got %+v", got)
	}
}
