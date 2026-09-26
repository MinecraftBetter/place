package place

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func devLogin(t *testing.T, h http.Handler, pseudo, next string) *http.Response {
	t.Helper()
	form := url.Values{"pseudo": {pseudo}, "next": {next}}
	req := httptest.NewRequest("POST", "/auth/dev", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

func sessionFrom(t *testing.T, res *http.Response) *http.Cookie {
	t.Helper()
	for _, c := range res.Cookies() {
		if c.Name == sessionCookie {
			return c
		}
	}
	t.Fatalf("no session cookie (status %d)", res.StatusCode)
	return nil
}

func TestDevLoginSetsSessionCookie(t *testing.T) {
	st := newTestStore(t)
	a := NewAuth(st, DevProvider{})
	res := devLogin(t, a, "  Brindille ", "/ace?x=470&y=300&z=6")
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/ace?x=470&y=300&z=6" {
		t.Fatalf("status %d location %q", res.StatusCode, res.Header.Get("Location"))
	}
	ck := sessionFrom(t, res)
	if !ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode || ck.Path != "/" {
		t.Fatalf("cookie flags %+v", ck)
	}

	req := httptest.NewRequest("GET", "/api/me", nil)
	req.AddCookie(ck)
	u := a.User(req)
	if u == nil || u.Pseudo != "Brindille" || u.AvatarURL != "/img/avatars/brindille.png" || u.Accent == "" {
		t.Fatalf("user %+v", u)
	}
}

func TestDevLoginRejectsBadInput(t *testing.T) {
	a := NewAuth(newTestStore(t), DevProvider{})
	for _, p := range []string{"", "a", strings.Repeat("x", 25), "bad\x00name"} {
		if res := devLogin(t, a, p, ""); res.StatusCode != http.StatusBadRequest {
			t.Errorf("pseudo %q: status %d", p, res.StatusCode)
		}
	}
	req := httptest.NewRequest("POST", "/auth/dev", strings.NewReader("pseudo=Brindille"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("cross-site login: status %d", rec.Code)
	}
}

func TestSafeNext(t *testing.T) {
	cases := map[string]string{
		"":                   "/ace",
		"/ace?x=1":           "/ace?x=1",
		"//evil.example":     "/ace",
		"https://evil.com":   "/ace",
		"/\\evil.example":    "/ace",
		"/home":              "/home",
		"javascript:alert()": "/ace",
	}
	for in, want := range cases {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLogout(t *testing.T) {
	st := newTestStore(t)
	a := NewAuth(st, DevProvider{})
	ck := sessionFrom(t, devLogin(t, a, "Kaelen", ""))

	req := httptest.NewRequest("POST", "/auth/logout", nil)
	req.AddCookie(ck)
	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d", rec.Code)
	}
	if u, _ := st.SessionUser(ck.Value); u != nil {
		t.Fatal("session still valid after logout")
	}

	get := httptest.NewRequest("GET", "/auth/logout", nil)
	rec = httptest.NewRecorder()
	a.ServeHTTP(rec, get)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET logout: status %d", rec.Code)
	}
}

func TestDevProviderDisabled(t *testing.T) {
	a := NewAuth(newTestStore(t))
	if res := devLogin(t, a, "Brindille", ""); res.StatusCode != http.StatusNotFound {
		t.Fatalf("status %d without -devAuth", res.StatusCode)
	}
	if len(a.ProviderNames()) != 0 {
		t.Fatal("providers listed")
	}
}

func TestSameOrigin(t *testing.T) {
	req := httptest.NewRequest("GET", "http://place.local/ws", nil)
	if !sameOrigin(req) {
		t.Error("no Origin must pass")
	}
	req.Header.Set("Origin", "http://place.local")
	if !sameOrigin(req) {
		t.Error("same host must pass")
	}
	req.Header.Set("Origin", "http://evil.example")
	if sameOrigin(req) {
		t.Error("other host must fail")
	}
	req.Header.Set("X-Forwarded-Host", "evil.example")
	if !sameOrigin(req) {
		t.Error("forwarded host must pass")
	}
}
