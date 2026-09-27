package place

import (
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := OpenStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func mustUser(t *testing.T, st *Store, pseudo string) *User {
	t.Helper()
	u, err := st.LoginUser(NewUser{Provider: "dev", ExternalID: Slugify(pseudo), Pseudo: pseudo})
	if err != nil || u == nil {
		t.Fatalf("LoginUser(%q) = %v, %v", pseudo, u, err)
	}
	return u
}

func mustRecord(t *testing.T, st *Store, e PixelEvent) []string {
	t.Helper()
	b, err := st.RecordPixel(e)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestStoreFileMigratesOnce(t *testing.T) {
	path := t.TempDir() + "/bp.db"
	st, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	mustUser(t, st, "Zozo42")
	st.Close()
	st, err = OpenStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer st.Close()
	if u, _ := st.UserByID(1); u == nil || u.Pseudo != "Zozo42" {
		t.Fatalf("user lost after reopen: %+v", u)
	}
}

func TestLoginUserCreatesOnceWithUniqueSlug(t *testing.T) {
	st := newTestStore(t)
	a := mustUser(t, st, "Mamie Pixel")
	again, _ := st.LoginUser(NewUser{Provider: "dev", ExternalID: "mamie-pixel", Pseudo: "Autre pseudo"})
	if again.ID != a.ID {
		t.Fatalf("second login created a new account")
	}
	if a.Slug != "mamie-pixel" || a.Role != "joueur" || a.Status != "actif" || a.CreatedAt == 0 {
		t.Fatalf("defaults: %+v", a)
	}
	b, err := st.LoginUser(NewUser{Provider: "justbetter", ExternalID: "sub-123", Pseudo: "Mamie  Pixel!"})
	if err != nil {
		t.Fatal(err)
	}
	if b.ID == a.ID || b.Slug != "mamie-pixel-2" {
		t.Fatalf("same pseudo on another provider: id %d slug %q", b.ID, b.Slug)
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Brindille":        "brindille",
		"Mamie Pixel":      "mamie-pixel",
		"  Élodie_Œuvre  ": "elodie-ouvre",
		"Zozo42":           "zozo42",
		"🤠🤠":               "joueur",
		"a---b":            "a-b",
		"Lucie_px":         "lucie-px",
		"x0123456789012345678901234567890123456789": "x0123456789012345678901234567890",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUsersByIDs(t *testing.T) {
	st := newTestStore(t)
	a, b := mustUser(t, st, "Evan"), mustUser(t, st, "Tiago")
	got, err := st.UsersByIDs([]uint32{a.ID, b.ID, 999})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[a.ID].Pseudo != "Evan" || got[b.ID].Pseudo != "Tiago" {
		t.Fatalf("got %v", got)
	}
	if empty, err := st.UsersByIDs(nil); err != nil || len(empty) != 0 {
		t.Fatal("empty list")
	}
}

func TestSessions(t *testing.T) {
	st := newTestStore(t)
	u := mustUser(t, st, "Nyx")
	token, err := st.CreateSession(u.ID, time.Hour)
	if err != nil || len(token) < 40 {
		t.Fatalf("token %q, %v", token, err)
	}
	got, err := st.SessionUser(token)
	if err != nil || got == nil || got.ID != u.ID {
		t.Fatalf("SessionUser = %+v, %v", got, err)
	}
	if nobody, _ := st.SessionUser("forged"); nobody != nil {
		t.Fatal("forged token accepted")
	}
	if nobody, _ := st.SessionUser(""); nobody != nil {
		t.Fatal("empty token accepted")
	}

	st.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if expired, _ := st.SessionUser(token); expired != nil {
		t.Fatal("expired session accepted")
	}
	st.now = time.Now

	st.DeleteSession(token)
	if gone, _ := st.SessionUser(token); gone != nil {
		t.Fatal("deleted session accepted")
	}
}

func TestRecordPixelCounters(t *testing.T) {
	st := newTestStore(t)
	a, b := mustUser(t, st, "Brindille"), mustUser(t, st, "Zozo42")
	rec := func(e PixelEvent) {
		t.Helper()
		if _, err := st.RecordPixel(e); err != nil {
			t.Fatal(err)
		}
	}
	rec(PixelEvent{X: 1, Y: 1, Color: 0x5eb3ff, UserID: a.ID, PrevColor: 0xffffff, PrevUserID: 0, TS: 1})
	rec(PixelEvent{X: 2, Y: 1, Color: 0x5eb3ff, UserID: a.ID, PrevColor: 0xffffff, PrevUserID: 0, TS: 2})
	rec(PixelEvent{X: 1, Y: 1, Color: 0x000000, UserID: b.ID, PrevColor: 0x5eb3ff, PrevUserID: a.ID, TS: 3}) // Zozo42 retouches
	rec(PixelEvent{X: 2, Y: 1, Color: 0xff63aa, UserID: a.ID, PrevColor: 0x5eb3ff, PrevUserID: a.ID, TS: 4}) // over her own pixel

	ga, _ := st.UserByID(a.ID)
	gb, _ := st.UserByID(b.ID)
	if ga.PixelsPlaced != 3 || ga.PixelsVisible != 1 {
		t.Errorf("Brindille placed %d visible %d, want 3 and 1", ga.PixelsPlaced, ga.PixelsVisible)
	}
	if gb.PixelsPlaced != 1 || gb.PixelsVisible != 1 {
		t.Errorf("Zozo42 placed %d visible %d, want 1 and 1", gb.PixelsPlaced, gb.PixelsVisible)
	}

	hist, err := st.PixelHistory(1, 1, 3)
	if err != nil || len(hist) != 2 || hist[0].UserID != b.ID || hist[1].UserID != a.ID || hist[1].PrevUserID != 0 {
		t.Fatalf("history %+v %v", hist, err)
	}

	var since []int64
	st.EachPixelEvent(3, func(e PixelEvent) error { since = append(since, e.TS); return nil })
	if len(since) != 2 || since[0] != 3 || since[1] != 4 {
		t.Fatalf("EachPixelEvent(3) = %v", since)
	}
}

func TestWriteBlock(t *testing.T) {
	now := time.UnixMilli(10_000)
	cases := []struct {
		u    User
		want string
	}{
		{User{Status: "actif"}, ""},
		{User{Status: "banni"}, "banned"},
		{User{Status: "suspendu", SuspendedUntil: 20_000}, "suspended"},
		{User{Status: "suspendu", SuspendedUntil: 5_000}, ""},
	}
	for _, c := range cases {
		if got := c.u.WriteBlock(now); got != c.want {
			t.Errorf("%+v: %q, want %q", c.u, got, c.want)
		}
	}
}

func TestBackfillBadges(t *testing.T) {
	st := newTestStore(t)
	u := mustUser(t, st, "Ancien")
	st.db.Exec(`UPDATE users SET pixels_poses = 1200 WHERE id = ?`, u.ID)
	if n, err := st.BackfillBadges(); err != nil || n != 2 {
		t.Fatalf("backfill = %d, %v; want premier-pixel + millier", n, err)
	}
	if n, _ := st.BackfillBadges(); n != 0 {
		t.Fatal("backfill not idempotent")
	}
}
