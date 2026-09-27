package place

import (
	"hash/fnv"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	log "github.com/sirupsen/logrus"
)

const (
	sessionCookie = "bp_session"
	sessionTTL    = 30 * 24 * time.Hour
)

// Provider is a way to log in: "justbetter" (LLDAP, justbetter.go) in production,
// "dev" (any pseudo, no password) locally.
type Provider interface {
	Name() string
	Mount(mux *http.ServeMux, a *Auth)
}

// Auth owns sessions and the login providers, under /auth/.
type Auth struct {
	store     *Store
	providers []Provider
	mux       *http.ServeMux
	admins    map[string]bool // slugs that get the admin role when they log in
}

// SetAdmins lists the players (slugs) who are admins, e.g. -admins tiago,evan.
func (a *Auth) SetAdmins(slugs []string) {
	a.admins = map[string]bool{}
	for _, s := range slugs {
		if s = strings.TrimSpace(strings.ToLower(s)); s != "" {
			a.admins[s] = true
		}
	}
}

func NewAuth(st *Store, providers ...Provider) *Auth {
	a := &Auth{store: st, providers: providers, mux: http.NewServeMux()}
	for _, p := range providers {
		p.Mount(a.mux, a)
	}
	a.mux.HandleFunc("/auth/logout", a.handleLogout)
	return a
}

func (a *Auth) ServeHTTP(w http.ResponseWriter, r *http.Request) { a.mux.ServeHTTP(w, r) }

// ProviderNames lists the enabled providers, for the login page.
func (a *Auth) ProviderNames() []string {
	names := make([]string, 0, len(a.providers))
	for _, p := range a.providers {
		names = append(names, p.Name())
	}
	return names
}

// User returns the logged-in player of a request, or nil for a guest.
func (a *Auth) User(r *http.Request) *User {
	ck, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil
	}
	u, err := a.store.SessionUser(ck.Value)
	if err != nil {
		log.WithField("endpoint", "Auth").Error("Session lookup: ", err)
		return nil
	}
	return u
}

// Login opens a session for a provider identity and sends the player back to next.
func (a *Auth) Login(w http.ResponseWriter, r *http.Request, n NewUser, next string) {
	u, err := a.store.LoginUser(n)
	if err != nil {
		log.WithField("endpoint", "Auth").Error("Login: ", err)
		http.Error(w, "Connexion impossible.", http.StatusInternalServerError)
		return
	}
	if (n.Admin || a.admins[u.Slug]) && u.Role != "admin" {
		if err := a.store.SetRole(u.ID, "admin"); err == nil {
			u.Role = "admin"
		}
	}
	token, err := a.store.CreateSession(u.ID, sessionTTL)
	if err != nil {
		log.WithField("endpoint", "Auth").Error("Session: ", err)
		http.Error(w, "Connexion impossible.", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", MaxAge: int(sessionTTL.Seconds()),
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: isHTTPS(r),
	})
	log.WithField("ip", r.RemoteAddr).WithField("endpoint", "Auth").Info("Logged in: ", u.Pseudo, " (", u.Provider, ")")
	http.Redirect(w, r, safeNext(next), http.StatusSeeOther)
}

func (a *Auth) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Méthode non autorisée.", http.StatusMethodNotAllowed)
		return
	}
	if !sameOrigin(r) {
		http.Error(w, "Origine refusée.", http.StatusForbidden)
		return
	}
	if ck, err := r.Cookie(sessionCookie); err == nil {
		if err := a.store.DeleteSession(ck.Value); err != nil {
			log.WithField("endpoint", "Auth").Error("Logout: ", err)
		}
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: isHTTPS(r)})
	http.Redirect(w, r, safeNext(r.FormValue("next")), http.StatusSeeOther)
}

func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

// sameOrigin is true when the request has no Origin header (not a cross-site
// browser request) or when it comes from this very host.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	fwd := r.Header.Get("X-Forwarded-Host")
	return fwd != "" && strings.EqualFold(u.Host, fwd)
}

// safeNext only allows local paths, so a login link cannot redirect elsewhere.
func safeNext(next string) string {
	if next == "" || next[0] != '/' || strings.HasPrefix(next, "//") || strings.ContainsAny(next, "\\\r\n") {
		return "/ace"
	}
	return next
}

// ------------------------------------------------------------------
// Dev provider: pick any pseudo, no password. Only enabled with -devAuth.

type DevProvider struct{}

func (DevProvider) Name() string { return "dev" }

// Avatars of the sample accounts from the design (design/assets/avatars).
var devAvatars = map[string]string{
	"brindille": "brindille", "evan": "evan", "kaelen": "kaelen", "lucie-px": "lucie", "mamie-pixel": "mamie",
	"nyx": "nyx", "poulpe": "poulpe", "sam": "sam", "tiago": "tiago", "zozo42": "zozo42",
}

// Accent colours, taken from the canvas palette.
var accentPalette = []string{"#ff63aa", "#5eb3ff", "#ffa800", "#7eed38", "#00ccc0", "#6a5cff", "#ff2651", "#ffd623", "#b44ac0", "#00a344"}

func (DevProvider) Mount(mux *http.ServeMux, a *Auth) {
	mux.HandleFunc("/auth/dev", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			http.Redirect(w, r, "/login?next="+url.QueryEscape(safeNext(r.FormValue("next"))), http.StatusSeeOther)
		case http.MethodPost:
			if !sameOrigin(r) {
				http.Error(w, "Origine refusée.", http.StatusForbidden)
				return
			}
			pseudo, ok := cleanPseudo(r.FormValue("pseudo"))
			if !ok {
				http.Error(w, "Pseudo invalide : entre 2 et 24 caractères.", http.StatusBadRequest)
				return
			}
			slug := Slugify(pseudo)
			n := NewUser{Provider: "dev", ExternalID: slug, Pseudo: pseudo, Accent: accentFor(slug)}
			if file, ok := devAvatars[slug]; ok {
				n.AvatarURL = "/img/avatars/" + file + ".png"
			}
			a.Login(w, r, n, r.FormValue("next"))
		default:
			http.Error(w, "Méthode non autorisée.", http.StatusMethodNotAllowed)
		}
	})
}

func accentFor(slug string) string {
	h := fnv.New32a()
	h.Write([]byte(slug))
	return accentPalette[h.Sum32()%uint32(len(accentPalette))]
}

// cleanPseudo trims a pseudo and checks it is 2–24 printable characters.
func cleanPseudo(s string) (string, bool) {
	s = strings.Join(strings.Fields(s), " ")
	n := utf8.RuneCountInString(s)
	if n < 2 || n > 24 {
		return "", false
	}
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return "", false
		}
	}
	return s, true
}
