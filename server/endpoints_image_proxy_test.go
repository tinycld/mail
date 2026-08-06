package mail

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/router"
)

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		host    string
		private bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"169.254.1.1", true},
		{"100.64.0.1", true},
		{"::1", true},
		{"::", true},
		{"fe80::1", true},
		{"fc00::1", true},
		{"fd12:3456::1", true},
		{"0.0.0.0", true},
		{"8.8.8.8", false},
		{"2001:4860:4860::8888", false},
	}

	for _, tt := range tests {
		got := isPrivateHost(tt.host)
		if got != tt.private {
			t.Errorf("isPrivateHost(%q) = %v, want %v", tt.host, got, tt.private)
		}
	}
}

func TestProxyRejectsNoURL(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/mail/image-proxy", nil)
	rr := httptest.NewRecorder()
	handleImageProxy(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestProxyRejectsFTPScheme(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/mail/image-proxy?url=ftp://example.com/file.jpg", nil)
	rr := httptest.NewRecorder()
	handleImageProxy(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestProxyRejectsPrivateIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/mail/image-proxy?url=http://127.0.0.1/secret.jpg", nil)
	rr := httptest.NewRecorder()
	handleImageProxy(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestProxyRejectsIPv6Loopback(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/mail/image-proxy?url=http://[::1]/secret.jpg", nil)
	rr := httptest.NewRecorder()
	handleImageProxy(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestProxyBlocksRedirectToPrivate(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data", http.StatusFound)
	}))
	defer ts.Close()

	// Allow the loopback test server but keep every other private range
	// blocked, so the redirect target is still rejected.
	orig := checkPrivateIP
	checkPrivateIP = func(ip net.IP) bool { return !ip.IsLoopback() && isPrivateIP(ip) }
	defer func() { checkPrivateIP = orig }()

	req := httptest.NewRequest("GET", "/api/mail/image-proxy?url="+ts.URL+"/img.png", nil)
	rr := httptest.NewRecorder()
	handleImageProxy(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("expected 502 for redirect to private address, got %d", rr.Code)
	}
}

func TestProxyFetchesExternalImage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte{0x89, 0x50, 0x4E, 0x47})
	}))
	defer ts.Close()

	// Allow loopback for this test since httptest binds to 127.0.0.1
	orig := checkPrivateIP
	checkPrivateIP = func(ip net.IP) bool { return false }
	defer func() { checkPrivateIP = orig }()

	req := httptest.NewRequest("GET", "/api/mail/image-proxy?url="+ts.URL+"/img.png", nil)
	rr := httptest.NewRecorder()
	handleImageProxy(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("expected Content-Type image/png, got %s", ct)
	}
	if cc := rr.Header().Get("Cache-Control"); cc == "" {
		t.Error("expected Cache-Control header")
	}
}

func TestImageProxyRequestRejectsNonAuthToken(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	defer app.Cleanup()

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("find users: %v", err)
	}
	u := core.NewRecord(users)
	u.Set("email", "proxy@example.com")
	u.Set("password", "s3cret-password")
	if err := app.Save(u); err != nil {
		t.Fatalf("save user: %v", err)
	}

	// A verification token authorizes email verification, not identity.
	// Before line 105's core.TokenTypeAuth constraint, the proxy accepted it.
	verificationToken, err := u.NewVerificationToken()
	if err != nil {
		t.Fatalf("NewVerificationToken: %v", err)
	}

	req := httptest.NewRequest("GET",
		"/api/mail/image-proxy?token="+verificationToken+"&url=http://example.com/x.png", nil)
	rr := httptest.NewRecorder()

	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rr

	err = handleImageProxyRequest(app, re)
	if err == nil {
		t.Fatalf("handleImageProxyRequest should reject verification token, got no error")
	}

	var apiErr *router.ApiError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected ApiError, got %T: %v", err, err)
	}
	if apiErr.Status != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d: %s", apiErr.Status, apiErr.Message)
	}
}
