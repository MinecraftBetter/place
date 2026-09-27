package place

// Release day: until the date set in /admin/reglages, visitors land on the countdown
// (/lancement), which plays the trailer at zero. « tout » closes the whole site but
// the countdown (admins still get in to check everything), « accueil » only replaces
// the home page. At the hour the gate opens by itself.

import (
	"fmt"
	"html"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const launchLayout = "2006-01-02T15:04" // Paris time, as an <input type="datetime-local">

// launchTime is the release instant, if one is set.
func (st Settings) launchTime() (time.Time, bool) {
	if st.LaunchAt == "" {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation(launchLayout, st.LaunchAt, Paris)
	return t, err == nil
}

// countdown tells whether the site is waiting for its release.
func (st Settings) countdown(now time.Time) bool {
	t, ok := st.launchTime()
	return st.LaunchGate != "off" && st.LaunchGate != "" && ok && now.Before(t)
}

func (st Settings) launchInfo(now time.Time) map[string]any {
	res := map[string]any{"gate": st.LaunchGate, "active": st.countdown(now), "trailer": st.LaunchTrailer}
	if t, ok := st.launchTime(); ok {
		res["at"] = t.UnixMilli()
		res["label"] = frenchLaunchDate(t)
	}
	return res
}

var frenchDays = []string{"dimanche", "lundi", "mardi", "mercredi", "jeudi", "vendredi", "samedi"}
var frenchMonthsLong = []string{"janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"}

// frenchLaunchDate: « samedi 24 octobre à 20 h », « … à 20 h 30 », the year when it isn't this one.
func frenchLaunchDate(t time.Time) string {
	t = t.In(Paris)
	s := fmt.Sprintf("%s %d %s", frenchDays[t.Weekday()], t.Day(), frenchMonthsLong[t.Month()-1])
	if t.Year() != time.Now().In(Paris).Year() {
		s += fmt.Sprintf(" %d", t.Year())
	}
	s += fmt.Sprintf(" à %d h", t.Hour())
	if t.Minute() != 0 {
		s += fmt.Sprintf(" %02d", t.Minute())
	}
	return s
}

// trailerMedia: the trailer is a file of the media folder (sent by deploy/justbetter/stage.sh),
// with a picture shown before it plays. The version in the URL follows the file.
func (api *API) trailerMedia() (video, poster string) {
	if api.mediaDir == "" {
		return "", ""
	}
	if st, err := os.Stat(filepath.Join(api.mediaDir, "bande-annonce.mp4")); err == nil && !st.IsDir() {
		video = fmt.Sprintf("/media/bande-annonce.mp4?v=%d", st.ModTime().Unix())
	}
	if st, err := os.Stat(filepath.Join(api.mediaDir, "bande-annonce.jpg")); err == nil && !st.IsDir() {
		poster = fmt.Sprintf("/media/bande-annonce.jpg?v=%d", st.ModTime().Unix())
	}
	return video, poster
}

// launchStatus: the release as the pages see it, with the trailer.
func (api *API) launchStatus(st Settings) map[string]any {
	res := st.launchInfo(api.store.now())
	if v, p := api.trailerMedia(); v != "" {
		res["video"] = v
		res["poster"] = p
	}
	return res
}

// pages always reachable during the countdown
var launchOpen = []string{"/lancement", "/bande-annonce", "/login", "/auth/", "/api/", "/ws", "/media/", "/stat"}

// LaunchGate sends visitors to the countdown until the release.
func (api *API) LaunchGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st := api.Settings()
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) && st.countdown(api.store.now()) && api.gated(st, r) {
			http.Redirect(w, r, "/lancement", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (api *API) gated(st Settings, r *http.Request) bool {
	p := r.URL.Path
	if st.LaunchGate == "accueil" {
		return p == "/" || p == "/home" || p == "/home/"
	}
	for _, o := range launchOpen {
		if p == o || strings.HasSuffix(o, "/") && strings.HasPrefix(p, o) {
			return false
		}
	}
	if path.Ext(p) != "" { // scripts, styles, images, place.png, archives…
		return false
	}
	if u := api.auth.User(r); u != nil && u.Role == "admin" {
		return false
	}
	return true
}

// RewritePage fills the date of the release in the countdown page (Discord and
// other link previews read these meta tags without running any script).
func (api *API) RewritePage(r *http.Request, page string) string {
	if !strings.Contains(page, "{{") {
		return page
	}
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") == "" && strings.HasPrefix(r.Host, "localhost") {
		scheme = "http"
	}
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = h
	}
	st := api.Settings()
	desc := "Le nouveau place de JustBetter arrive bientôt. Prépare tes pixels !"
	if t, ok := st.launchTime(); ok {
		if api.store.now().Before(t) {
			desc = "Le nouveau place de JustBetter ouvre " + frenchLaunchDate(t) + ". Profils, musée, revendications… prépare tes pixels !"
		} else {
			desc = "Le nouveau place de JustBetter est ouvert ! Profils, musée, revendications : viens poser tes pixels."
		}
	}
	video := ""
	if v, _ := api.trailerMedia(); v != "" {
		video = scheme + "://" + host + v
	}
	return strings.NewReplacer("{{DESCRIPTION}}", html.EscapeString(desc), "{{ORIGIN}}", html.EscapeString(scheme+"://"+host),
		"{{VIDEO}}", html.EscapeString(video)).Replace(page)
}

// WatchLaunch tells the open pages when the countdown ends (the canvas itself opens
// on time without it: the hub compares each pixel with the release instant).
func (api *API) WatchLaunch() {
	last := api.Settings().countdown(api.store.now())
	go func() {
		for {
			time.Sleep(time.Second)
			st := api.Settings()
			now := api.store.now()
			if c := st.countdown(now); c != last {
				last = c
				api.hub.Broadcast(mustJSON(map[string]any{"type": "launch", "launch": api.launchStatus(st)}))
				if !c {
					log.WithField("endpoint", "Launch").Info("Release time: the place is open!")
				}
			}
		}
	}()
}

// SeedLaunch sets the release from the command line when no date is set yet, so the
// site shows the countdown from its very first start. Admins' changes then win.
func (api *API) SeedLaunch(at, gate string) error {
	if _, err := time.ParseInLocation(launchLayout, at, Paris); err != nil {
		return fmt.Errorf("date %q : attendu 2006-01-02T15:04", at)
	}
	st := api.Settings()
	if st.LaunchAt != "" {
		return nil
	}
	st.LaunchAt = at
	if gate != "accueil" {
		gate = "tout"
	}
	st.LaunchGate = gate
	if err := api.store.SaveSettings(st); err != nil {
		return err
	}
	api.applySettings(st)
	log.WithField("endpoint", "Launch").Info("Release set to ", frenchLaunchDate(func() time.Time { t, _ := st.launchTime(); return t }()), " (", gate, ")")
	return nil
}
