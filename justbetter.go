package place

// JustBetter login: the player types their JustBetter username and password, which
// are checked against LLDAP exactly like justbetter.fr (auth.php) and JUST-TCG do:
// POST auth/simple/login, then the profile read with the player's own token.
// Members of the admin groups become admins of the place.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

type JustBetterProvider struct {
	URL         string   // LLDAP base URL, e.g. http://192.168.1.84:1390/
	AdminGroups []string // lowercase LLDAP groups whose members are admins
	Client      *http.Client

	mu       sync.Mutex
	failures map[string][]time.Time // by IP, to slow down password guessing
}

const (
	loginMaxFailures = 10
	loginWindow      = 10 * time.Minute
)

var errLLDAPDown = errors.New("LLDAP injoignable")

type lldapIdentity struct {
	ID, DisplayName string
	Groups          []string
}

func NewJustBetterProvider(baseURL string, adminGroups []string) *JustBetterProvider {
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	groups := []string{}
	for _, g := range adminGroups {
		if g = strings.ToLower(strings.TrimSpace(g)); g != "" {
			groups = append(groups, g)
		}
	}
	return &JustBetterProvider{URL: baseURL, AdminGroups: groups, Client: &http.Client{Timeout: 8 * time.Second}, failures: map[string][]time.Time{}}
}

func (p *JustBetterProvider) Name() string { return "justbetter" }

func (p *JustBetterProvider) post(path string, body any, token string) (*http.Response, error) {
	b, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", p.URL+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := p.Client.Do(req)
	if err != nil {
		return nil, errLLDAPDown
	}
	return res, nil
}

// authenticate returns nil (no error) when the credentials are refused.
func (p *JustBetterProvider) authenticate(username, password string) (*lldapIdentity, error) {
	res, err := p.post("auth/simple/login", map[string]string{"username": username, "password": password}, "")
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 500 {
		return nil, errLLDAPDown
	}
	if res.StatusCode != http.StatusOK {
		return nil, nil
	}
	var body struct {
		Token string `json:"token"`
	}
	if json.NewDecoder(res.Body).Decode(&body) != nil || body.Token == "" {
		return nil, errLLDAPDown
	}
	// claims of the JWT LLDAP just gave us: { user, groups, exp, iat }
	parts := strings.Split(body.Token, ".")
	if len(parts) < 2 {
		return nil, errLLDAPDown
	}
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return nil, errLLDAPDown
	}
	var claims struct {
		User   string   `json:"user"`
		Groups []string `json:"groups"`
	}
	if json.Unmarshal(raw, &claims) != nil || claims.User == "" {
		return nil, errLLDAPDown
	}
	id := &lldapIdentity{ID: claims.User, DisplayName: claims.User, Groups: claims.Groups}
	// the display name is a nicety: the login stays valid without it
	q, err := p.post("api/graphql", map[string]any{"query": "query($id: String!) { user(userId: $id) { id displayName } }", "variables": map[string]string{"id": claims.User}}, body.Token)
	if err == nil {
		defer q.Body.Close()
		var g struct {
			Data struct {
				User struct {
					DisplayName string `json:"displayName"`
				} `json:"user"`
			} `json:"data"`
		}
		if q.StatusCode == http.StatusOK && json.NewDecoder(q.Body).Decode(&g) == nil && strings.TrimSpace(g.Data.User.DisplayName) != "" {
			id.DisplayName = g.Data.User.DisplayName
		}
	}
	return id, nil
}

func (p *JustBetterProvider) isAdmin(groups []string) bool {
	for _, g := range groups {
		for _, a := range p.AdminGroups {
			if strings.EqualFold(g, a) {
				return true
			}
		}
	}
	return false
}

// tooMany tells whether an IP failed too often recently; failed records one more failure.
func (p *JustBetterProvider) tooMany(ip string, failed bool) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	recent := p.failures[ip][:0]
	for _, t := range p.failures[ip] {
		if now.Sub(t) < loginWindow {
			recent = append(recent, t)
		}
	}
	if failed {
		recent = append(recent, now)
	}
	if len(recent) == 0 {
		delete(p.failures, ip)
	} else {
		p.failures[ip] = recent
	}
	return len(recent) >= loginMaxFailures
}

// clientIP is the address the failed-login limit counts. X-Real-IP (set by nginx to the
// address it really sees) is only believed when the TCP peer is a private address — the
// proxy; the port can also be reached directly on the local network, where the header
// could be forged. The peer is the one kept by KeepPeer, before the xff middleware
// rewrites RemoteAddr from X-Forwarded-For (whose first public address can be forged too).
func clientIP(r *http.Request) string {
	peer := r.RemoteAddr
	if p, ok := r.Context().Value(peerKey{}).(string); ok {
		peer = p
	}
	host := peer
	if h, _, err := net.SplitHostPort(peer); err == nil {
		host = h
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsPrivate() || ip.IsLoopback()) {
		if real := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); real != nil {
			return real.String()
		}
	}
	return host
}

type peerKey struct{}

// KeepPeer remembers the TCP peer of a request before any header rewrites it (main
// wraps the whole server with it, outside the xff middleware).
func KeepPeer(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), peerKey{}, r.RemoteAddr)))
	})
}

func (p *JustBetterProvider) Mount(mux *http.ServeMux, a *Auth) {
	mux.HandleFunc("/auth/justbetter", func(w http.ResponseWriter, r *http.Request) {
		next := safeNext(r.FormValue("next"))
		back := func(code string) {
			http.Redirect(w, r, "/login?next="+url.QueryEscape(next)+"&erreur="+code, http.StatusSeeOther)
		}
		switch r.Method {
		case http.MethodGet:
			http.Redirect(w, r, "/login?next="+url.QueryEscape(next), http.StatusSeeOther)
		case http.MethodPost:
			if !sameOrigin(r) {
				http.Error(w, "Origine refusée.", http.StatusForbidden)
				return
			}
			ip := clientIP(r)
			if p.tooMany(ip, false) {
				back("trop")
				return
			}
			username := strings.TrimSpace(r.FormValue("identifiant"))
			password := r.FormValue("motdepasse")
			if username == "" || password == "" || len(username) > 64 || len(password) > 256 {
				back("identifiants")
				return
			}
			if strings.Contains(username, "@") {
				back("email") // LLDAP wants the username, like justbetter.fr
				return
			}
			id, err := p.authenticate(username, password)
			if err != nil {
				log.WithField("endpoint", "Auth").Warning("JustBetter login: ", err)
				back("indisponible")
				return
			}
			if id == nil {
				p.tooMany(ip, true)
				log.WithField("ip", ip).WithField("endpoint", "Auth").Info("JustBetter login refused for ", username)
				back("identifiants")
				return
			}
			pseudo, ok := cleanPseudo(id.DisplayName)
			if !ok {
				if pseudo, ok = cleanPseudo(id.ID); !ok {
					pseudo = "Joueur " + id.ID
				}
			}
			slug := Slugify(pseudo)
			a.Login(w, r, NewUser{Provider: "justbetter", ExternalID: id.ID, Pseudo: pseudo, Accent: accentFor(slug), Admin: p.isAdmin(id.Groups)}, next)
		default:
			http.Error(w, "Méthode non autorisée.", http.StatusMethodNotAllowed)
		}
	})
}
