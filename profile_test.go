package place

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestFirstPixelAndCounterBadges(t *testing.T) {
	st := newTestStore(t)
	u := mustUser(t, st, "Brindille")
	noon := time.Date(2026, 10, 14, 12, 0, 0, 0, Paris).UnixMilli()
	if got := mustRecord(t, st, PixelEvent{X: 1, Y: 1, Color: 1, UserID: u.ID, PrevColor: 0xffffff, TS: noon}); len(got) != 1 || got[0] != "premier-pixel" {
		t.Fatalf("first pixel badges = %v", got)
	}
	if got := mustRecord(t, st, PixelEvent{X: 2, Y: 1, Color: 1, UserID: u.ID, PrevColor: 0xffffff, TS: noon + 1}); len(got) != 0 {
		t.Fatalf("badge given twice: %v", got)
	}

	st.db.Exec(`UPDATE users SET pixels_poses = 998 WHERE id = ?`, u.ID)
	if got := mustRecord(t, st, PixelEvent{X: 3, Y: 1, Color: 1, UserID: u.ID, PrevColor: 0xffffff, TS: noon + 2}); len(got) != 0 {
		t.Fatalf("millier too early: %v", got)
	}
	if got := mustRecord(t, st, PixelEvent{X: 4, Y: 1, Color: 1, UserID: u.ID, PrevColor: 0xffffff, TS: noon + 3}); len(got) != 1 || got[0] != "millier" {
		t.Fatalf("millier at 1000: %v", got)
	}
}

func TestPioneerPixelsCountForMillier(t *testing.T) {
	st := newTestStore(t)
	u := mustUser(t, st, "Kaelen")
	st.db.Exec(`UPDATE users SET pixels_pionniers = 999 WHERE id = ?`, u.ID)
	got := mustRecord(t, st, PixelEvent{X: 1, Y: 1, Color: 1, UserID: u.ID, PrevColor: 0xffffff, TS: 1})
	if len(got) != 2 {
		t.Fatalf("badges %v, want premier-pixel and millier", got)
	}
}

func TestRestorationIsCounted(t *testing.T) {
	st := newTestStore(t)
	a, b := mustUser(t, st, "Brindille"), mustUser(t, st, "Zozo42")
	mustRecord(t, st, PixelEvent{X: 5, Y: 5, Color: 0x5eb3ff, UserID: a.ID, PrevColor: 0xffffff, TS: 1})
	mustRecord(t, st, PixelEvent{X: 5, Y: 5, Color: 0x000000, UserID: b.ID, PrevColor: 0x5eb3ff, PrevUserID: a.ID, TS: 2}) // pizza
	mustRecord(t, st, PixelEvent{X: 5, Y: 5, Color: 0x5eb3ff, UserID: a.ID, PrevColor: 0x000000, PrevUserID: b.ID, TS: 3}) // restored
	mustRecord(t, st, PixelEvent{X: 6, Y: 5, Color: 0x5eb3ff, UserID: a.ID, PrevColor: 0xffffff, TS: 4})                   // new pixel
	got, _ := st.UserByID(a.ID)
	if got.PixelsRestored != 1 {
		t.Fatalf("restored = %d, want 1", got.PixelsRestored)
	}
	// Undoing your own change is not a restoration.
	mustRecord(t, st, PixelEvent{X: 6, Y: 5, Color: 0xffffff, UserID: a.ID, PrevColor: 0x5eb3ff, PrevUserID: a.ID, TS: 5})
	got, _ = st.UserByID(a.ID)
	if got.PixelsRestored != 1 {
		t.Fatalf("own undo counted: %d", got.PixelsRestored)
	}

	st.db.Exec(`UPDATE users SET pixels_restaures = 49 WHERE id = ?`, b.ID)
	mustRecord(t, st, PixelEvent{X: 5, Y: 5, Color: 0x000000, UserID: b.ID, PrevColor: 0x5eb3ff, PrevUserID: a.ID, TS: 6})
	badges, _ := st.UserBadges(b.ID)
	if _, ok := badges["restaurateur"]; !ok {
		t.Fatalf("restaurateur not given at 50: %v", badges)
	}
}

func TestNightPixels(t *testing.T) {
	st := newTestStore(t)
	u := mustUser(t, st, "Nyx")
	for i, h := range []int{1, 2, 4, 5} {
		ts := time.Date(2026, 1, 10, h, 30, 0, 0, Paris).UnixMilli()
		mustRecord(t, st, PixelEvent{X: i, Y: 0, Color: 1, UserID: u.ID, PrevColor: 0xffffff, TS: ts})
	}
	got, _ := st.UserByID(u.ID)
	if got.PixelsNight != 2 {
		t.Fatalf("night pixels = %d, want 2 (2:30 and 4:30)", got.PixelsNight)
	}
}

func TestRanksAndWeeklyTop10(t *testing.T) {
	st := newTestStore(t)
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, Paris) // a Wednesday
	lastWeek := WeekStart(now).AddDate(0, 0, -3).UnixMilli()
	thisWeek := WeekStart(now).Add(time.Hour).UnixMilli()
	a, b, c := mustUser(t, st, "A1"), mustUser(t, st, "B2"), mustUser(t, st, "C3")
	for i := 0; i < 3; i++ {
		mustRecord(t, st, PixelEvent{X: i, Y: 0, Color: 1, UserID: a.ID, PrevColor: 0xffffff, TS: lastWeek + int64(i)})
	}
	mustRecord(t, st, PixelEvent{X: 9, Y: 0, Color: 1, UserID: b.ID, PrevColor: 0xffffff, TS: lastWeek})
	for i := 0; i < 2; i++ {
		mustRecord(t, st, PixelEvent{X: i, Y: 1, Color: 1, UserID: b.ID, PrevColor: 0xffffff, TS: thisWeek + int64(i)})
	}

	if r, _ := st.UserRank(b.ID, WeekStart(now).UnixMilli()); r != 1 {
		t.Errorf("B2 weekly rank %d, want 1", r)
	}
	if r, _ := st.UserRank(a.ID, WeekStart(now).UnixMilli()); r != 0 {
		t.Errorf("A1 has no pixel this week, rank %d", r)
	}
	if r, _ := st.UserRankAllTime(a.ID); r != 1 {
		t.Errorf("A1 all-time rank %d, want 1 (3 px vs 3 px, older account)", r)
	}
	if r, _ := st.UserRankAllTime(c.ID); r != 0 {
		t.Errorf("C3 without pixels ranked %d", r)
	}

	n, err := st.AwardWeeklyTop10(now)
	if err != nil || n != 2 {
		t.Fatalf("top10 awarded %d (%v), want 2 for last week", n, err)
	}
	if n, _ := st.AwardWeeklyTop10(now); n != 0 {
		t.Fatal("top10 not idempotent")
	}
	badges, _ := st.UserBadges(a.ID)
	if !strings.HasPrefix(badges["top10"].Detail, "semaine du 5 oct. 2026") {
		t.Fatalf("detail %q", badges["top10"].Detail)
	}
}

func TestWeekStartIsMondayInParis(t *testing.T) {
	sunday := time.Date(2026, 10, 18, 23, 30, 0, 0, Paris)
	if ws := WeekStart(sunday); ws.Weekday() != time.Monday || ws.Day() != 12 {
		t.Fatalf("WeekStart(%v) = %v", sunday, ws)
	}
}

func TestUpdateProfileValidation(t *testing.T) {
	st := newTestStore(t)
	u := mustUser(t, st, "Brindille")
	mustUser(t, st, "Kaelen")
	str := func(s string) *string { return &s }

	bad := []ProfileUpdate{
		{Pseudo: str("a")},
		{Bio: str(strings.Repeat("é", 161))},
		{Banner: str("plage")},
		{Accent: str("#123456")},
		{FavColor: str("#123456")},
		{PinnedBadges: &[]string{"millier"}}, // not earned
	}
	for i, p := range bad {
		if _, err := st.UpdateProfile(u, p); err == nil {
			t.Errorf("case %d accepted", i)
		}
	}

	st.GrantBadge(u.ID, "millier", "auto", "")
	yes := true
	got, err := st.UpdateProfile(u, ProfileUpdate{
		Pseudo: str("Kaelen"), Bio: str(" Je peins des couchers de soleil. "), Banner: str("nuit"), Accent: str("#5EB3FF"),
		FavColor: str("#FF63AA"), PinnedBadges: &[]string{"millier"}, MapPublic: &yes,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Pseudo != "Kaelen" || got.Slug != "kaelen-2" || got.Bio != "Je peins des couchers de soleil." || got.Banner != "nuit" ||
		got.Accent != "#5eb3ff" || got.FavColor != "#ff63aa" || len(got.PinnedBadges) != 1 {
		t.Fatalf("updated %+v", got)
	}
	// Same pseudo again keeps the slug.
	again, _ := st.UpdateProfile(got, ProfileUpdate{Pseudo: str("Kaelen")})
	if again.Slug != "kaelen-2" {
		t.Fatalf("slug changed to %q", again.Slug)
	}
}

func TestProfileAPI(t *testing.T) {
	e := newEnv(t, 8)
	ck := e.login(t, "Brindille")
	conn := e.dial(t, ck, "")
	sendPixel(t, conn, 2, 2)
	m := next(t, conn, isType("badge"))
	if m["badge"] != "premier-pixel" || m["name"] != "Premier pixel" {
		t.Fatalf("badge message %v", m)
	}

	p := e.getJSON(t, "/api/users/brindille", nil)
	if p["pseudo"] != "Brindille" || p["pixels_poses"] != 1.0 || p["rang_semaine"] != 1.0 || p["moi"] != false || p["alertes_retouche"] != nil {
		t.Fatalf("public profile %v", p)
	}
	earned := 0
	for _, b := range p["badges"].([]any) {
		if b.(map[string]any)["earned"] == true {
			earned++
		}
	}
	if earned != 1 || len(p["badges"].([]any)) != len(Badges) {
		t.Fatalf("badges %v", p["badges"])
	}
	if mine := e.getJSON(t, "/api/users/brindille", ck); mine["moi"] != true || mine["alertes_retouche"] != true {
		t.Fatalf("own profile %v", mine)
	}
	if status, _ := e.get(t, "/api/users/nobody", nil); status != 404 {
		t.Fatalf("unknown player: %d", status)
	}

	status, b := e.get(t, "/api/users/brindille/contributions.png", nil)
	if status != 200 {
		t.Fatalf("contributions: %d", status)
	}
	img, _ := png.Decode(bytes.NewReader(b))
	if r, g, bl, _ := img.At(2, 2).RGBA(); r>>8 != 94 || g>>8 != 179 || bl>>8 != 255 {
		t.Fatal("own pixel must keep its colour")
	}
	if r, _, _, _ := img.At(10, 10).RGBA(); r>>8 > 80 {
		t.Fatal("other pixels must be darkened")
	}

	// PATCH /api/me/profile
	req, _ := http.NewRequest("PATCH", e.srv.URL+"/api/me/profile", strings.NewReader(`{"bio":"Salut","carte_publique":false}`))
	req.AddCookie(ck)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil || res.StatusCode != 200 {
		t.Fatalf("patch: %v %v", err, res.StatusCode)
	}
	res.Body.Close()
	if status, _ := e.get(t, "/api/users/brindille/contributions.png", nil); status != 403 {
		t.Fatalf("hidden map still public: %d", status)
	}
	if status, _ := e.get(t, "/api/users/brindille/contributions.png", ck); status != 200 {
		t.Fatalf("own hidden map: %d", status)
	}

	req, _ = http.NewRequest("PATCH", e.srv.URL+"/api/me/profile", strings.NewReader(`{"bio":"x"}`))
	req.AddCookie(ck)
	req.Header.Set("Origin", "https://evil.example")
	res, _ = http.DefaultClient.Do(req)
	res.Body.Close()
	if res.StatusCode != 403 {
		t.Fatalf("cross-site patch: %d", res.StatusCode)
	}
	req, _ = http.NewRequest("PATCH", e.srv.URL+"/api/me/profile", strings.NewReader(`{"accent":"#000001"}`))
	req.AddCookie(ck)
	res, _ = http.DefaultClient.Do(req)
	var fe map[string]string
	json.NewDecoder(res.Body).Decode(&fe)
	res.Body.Close()
	if res.StatusCode != 400 || fe["field"] != "accent" {
		t.Fatalf("invalid accent: %d %v", res.StatusCode, fe)
	}
}

func TestAvatarUpload(t *testing.T) {
	e := newEnv(t, 8)
	dir := t.TempDir()
	e.api.SetMediaDir(dir)
	ck := e.login(t, "Brindille")

	src := image.NewNRGBA(image.Rect(0, 0, 12, 9)) // tiny pixel art, not square
	for i := range src.Pix {
		src.Pix[i] = 255
	}
	src.SetNRGBA(6, 4, color.NRGBA{255, 0, 0, 255})
	var pngBuf bytes.Buffer
	png.Encode(&pngBuf, src)

	upload := func(data []byte) (*http.Response, map[string]string) {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		fw, _ := mw.CreateFormFile("avatar", "a.png")
		fw.Write(data)
		mw.Close()
		req, _ := http.NewRequest("POST", e.srv.URL+"/api/me/avatar", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.AddCookie(ck)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]string{}
		json.NewDecoder(res.Body).Decode(&m)
		res.Body.Close()
		return res, m
	}
	res, m := upload(pngBuf.Bytes())
	if res.StatusCode != 200 || !strings.HasPrefix(m["avatar"], "/media/avatars/1-") {
		t.Fatalf("upload: %d %v", res.StatusCode, m)
	}
	h := MediaHandler(dir)
	rec := &recorder{header: http.Header{}}
	h.ServeHTTP(rec, httptestRequest("GET", m["avatar"]))
	if rec.status != 0 && rec.status != 200 {
		t.Fatalf("serving avatar: %d", rec.status)
	}
	w, hh, err := decodePNGSize(rec.body.Bytes())
	if err != nil || w != avatarSize || hh != avatarSize {
		t.Fatalf("avatar %dx%d %v", w, hh, err)
	}
	if u := e.getJSON(t, "/api/users/brindille", nil); u["avatar"] != m["avatar"] {
		t.Fatalf("profile avatar %v", u["avatar"])
	}
	if res, _ := upload([]byte("not an image")); res.StatusCode != 400 {
		t.Fatalf("garbage upload: %d", res.StatusCode)
	}

	rec = &recorder{header: http.Header{}}
	h.ServeHTTP(rec, httptestRequest("GET", "/media/avatars/"))
	if rec.status != 404 {
		t.Fatalf("directory listing: %d", rec.status)
	}
	rec = &recorder{header: http.Header{}}
	h.ServeHTTP(rec, httptestRequest("GET", "/media/../profile.go"))
	if rec.status != 404 {
		t.Fatalf("path traversal: %d", rec.status)
	}
}

type recorder struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (r *recorder) Header() http.Header         { return r.header }
func (r *recorder) Write(b []byte) (int, error) { return r.body.Write(b) }
func (r *recorder) WriteHeader(s int)           { r.status = s }

func httptestRequest(method, path string) *http.Request {
	req, _ := http.NewRequest(method, "http://x"+path, io.NopCloser(strings.NewReader("")))
	return req
}

func TestDrawnBanner(t *testing.T) {
	e := newEnv(t, 8)
	e.api.SetMediaDir(t.TempDir())
	ck := e.login(t, "Brindille")
	str := func(s string) *string { return &s }
	u, _ := e.store.UserBySlug("brindille")
	if _, err := e.store.UpdateProfile(u, ProfileUpdate{Banner: str("custom")}); err == nil {
		t.Fatal("custom banner accepted before drawing one")
	}
	post := func(img image.Image) (int, map[string]string) {
		var png1 bytes.Buffer
		png.Encode(&png1, img)
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		fw, _ := mw.CreateFormFile("banner", "b.png")
		fw.Write(png1.Bytes())
		mw.Close()
		req, _ := http.NewRequest("POST", e.srv.URL+"/api/me/banner", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.AddCookie(ck)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		m := map[string]string{}
		json.NewDecoder(res.Body).Decode(&m)
		res.Body.Close()
		return res.StatusCode, m
	}
	if status, _ := post(image.NewNRGBA(image.Rect(0, 0, 48, 48))); status != 400 {
		t.Fatalf("square banner: %d", status)
	}
	status, m := post(image.NewNRGBA(image.Rect(0, 0, 48, 16)))
	if status != 200 || !strings.HasPrefix(m["banner_url"], "/media/banners/1-") {
		t.Fatalf("banner: %d %v", status, m)
	}
	p := e.getJSON(t, "/api/users/brindille", nil)
	if p["banner"] != "custom" || p["banner_url"] != m["banner_url"] {
		t.Fatalf("profile banner %v %v", p["banner"], p["banner_url"])
	}
}
