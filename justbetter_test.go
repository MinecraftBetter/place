package place

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// fakeLLDAP answers like LLDAP: tiago/bon-mdp (admin group), lucie/secret (no group).
func fakeLLDAP(t *testing.T, down *bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if *down {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		switch r.URL.Path {
		case "/auth/simple/login":
			var b struct{ Username, Password string }
			json.NewDecoder(r.Body).Decode(&b)
			groups := map[string][]string{"tiago": {"Admins", "lldap_admin"}, "lucie": {"joueurs"}}
			pw := map[string]string{"tiago": "bon-mdp", "lucie": "secret"}
			if pw[b.Username] == "" || pw[b.Username] != b.Password {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			claims, _ := json.Marshal(map[string]any{"user": b.Username, "groups": groups[b.Username], "exp": time.Now().Add(time.Hour).Unix()})
			json.NewEncoder(w).Encode(map[string]string{"token": "e30." + base64.RawURLEncoding.EncodeToString(claims) + ".sig"})
		case "/api/graphql":
			if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer e30.") {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			names := map[string]string{"tiago": "Tiago", "lucie": "Lucie Pixel"}
			var q struct{ Variables map[string]string }
			json.NewDecoder(r.Body).Decode(&q)
			json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"user": map[string]string{"id": q.Variables["id"], "displayName": names[q.Variables["id"]]}}})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestJustBetterLogin(t *testing.T) {
	down := false
	ldap := fakeLLDAP(t, &down)
	st := newTestStore(t)
	a := NewAuth(st, NewJustBetterProvider(ldap.URL, []string{"admins", "lldap_admin"}))
	srv := httptest.NewServer(a)
	t.Cleanup(srv.Close)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	login := func(user, pw string) *http.Response {
		t.Helper()
		res, err := client.PostForm(srv.URL+"/auth/justbetter", url.Values{"identifiant": {user}, "motdepasse": {pw}, "next": {"/musee"}})
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res
	}

	res := login("tiago", "bon-mdp")
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/musee" || len(res.Cookies()) == 0 {
		t.Fatalf("login: %d %s", res.StatusCode, res.Header.Get("Location"))
	}
	u, _ := st.LoginUser(NewUser{Provider: "justbetter", ExternalID: "tiago"})
	if u.Pseudo != "Tiago" || u.Role != "admin" {
		t.Fatalf("admin from the LLDAP group: %+v", u)
	}
	login("lucie", "secret")
	if u, _ := st.LoginUser(NewUser{Provider: "justbetter", ExternalID: "lucie"}); u.Pseudo != "Lucie Pixel" || u.Role == "admin" {
		t.Fatalf("player: %+v", u)
	}

	if loc := login("lucie", "faux").Header.Get("Location"); !strings.Contains(loc, "erreur=identifiants") || !strings.HasPrefix(loc, "/login?next=%2Fmusee") {
		t.Fatalf("wrong password: %s", loc)
	}
	if loc := login("lucie@example.com", "secret").Header.Get("Location"); !strings.Contains(loc, "erreur=email") {
		t.Fatalf("e-mail instead of the username: %s", loc)
	}
	// too many failures from the same address: blocked, even with the right password
	for i := 0; i < loginMaxFailures; i++ {
		login("lucie", "faux")
	}
	if loc := login("lucie", "secret").Header.Get("Location"); !strings.Contains(loc, "erreur=trop") {
		t.Fatalf("no slow-down after failures: %s", loc)
	}

	a2 := NewAuth(newTestStore(t), NewJustBetterProvider(ldap.URL, nil))
	srv2 := httptest.NewServer(a2)
	t.Cleanup(srv2.Close)
	down = true
	res, _ = client.PostForm(srv2.URL+"/auth/justbetter", url.Values{"identifiant": {"tiago"}, "motdepasse": {"bon-mdp"}})
	if loc := res.Header.Get("Location"); !strings.Contains(loc, "erreur=indisponible") {
		t.Fatalf("LLDAP down: %s", loc)
	}
	// cross-site form posts are refused
	req, _ := http.NewRequest("POST", srv2.URL+"/auth/justbetter", strings.NewReader("identifiant=tiago&motdepasse=bon-mdp"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://evil.example")
	if res, _ := client.Do(req); res.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-site login accepted: %d", res.StatusCode)
	}
}

func TestJustBetterClientIP(t *testing.T) {
	r := httptest.NewRequest("POST", "/auth/justbetter", nil)
	r.RemoteAddr = "172.18.0.1:5555" // the proxy, for a visitor on the server's network
	r.Header.Set("X-Real-IP", "192.168.1.42")
	if ip := clientIP(r); ip != "192.168.1.42" {
		t.Fatalf("got %s", ip)
	}
	r.Header.Del("X-Real-IP")
	if ip := clientIP(r); ip != "172.18.0.1" {
		t.Fatalf("got %s", ip)
	}
}
