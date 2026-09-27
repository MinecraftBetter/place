package place

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

func TestMain(m *testing.M) {
	log.SetLevel(log.WarnLevel)
	os.Exit(m.Run())
}

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type testEnv struct {
	store  *Store
	canvas *Canvas
	auth   *Auth
	hub    *Hub
	api    *API
	clock  *fakeClock
	srv    *httptest.Server
}

func newEnv(t *testing.T, maxConns int) *testEnv {
	t.Helper()
	st := newTestStore(t)
	c := NewBlankCanvas(16, 16)
	a := NewAuth(st, DevProvider{})
	h := NewHub(c, st, a, maxConns, 5*time.Second)
	clock := &fakeClock{t: time.UnixMilli(1_700_000_000_000)}
	h.now = clock.Now
	api := NewAPI(c, st, a, h)
	mux := http.NewServeMux()
	mux.Handle("/api/", api)
	mux.Handle("/auth/", a)
	mux.Handle("/", NewServer(c, h))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &testEnv{st, c, a, h, api, clock, srv}
}

func (e *testEnv) login(t *testing.T, pseudo string) *http.Cookie {
	t.Helper()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.PostForm(e.srv.URL+"/auth/dev", url.Values{"pseudo": {pseudo}})
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return sessionFrom(t, res)
}

func (e *testEnv) dial(t *testing.T, ck *http.Cookie, origin string) *websocket.Conn {
	t.Helper()
	hdr := http.Header{}
	if origin == "" {
		origin = e.srv.URL
	}
	hdr.Set("Origin", origin)
	if ck != nil {
		hdr.Set("Cookie", ck.String())
	}
	conn, res, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(e.srv.URL, "http")+"/ws", hdr)
	if err != nil {
		status := 0
		if res != nil {
			status = res.StatusCode
		}
		t.Fatalf("dial: %v (status %d)", err, status)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// next reads messages until one matches, skipping the others (e.g. stat updates).
func next(t *testing.T, conn *websocket.Conn, match func(map[string]any) bool) map[string]any {
	t.Helper()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		_, b, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("waiting for message: %v", err)
		}
		m := map[string]any{}
		if json.Unmarshal(b, &m) != nil {
			m = map[string]any{"raw": string(b)}
		}
		if match(m) {
			return m
		}
	}
}

func isType(typ string) func(map[string]any) bool {
	return func(m map[string]any) bool { return m["type"] == typ }
}

func isPixel(m map[string]any) bool { _, ok := m["color"]; return ok && m["type"] == nil }

func sendPixel(t *testing.T, conn *websocket.Conn, x, y int) {
	t.Helper()
	msg := `{"x":` + itoa(x) + `,"y":` + itoa(y) + `,"color":{"R":94,"G":179,"B":255,"A":255}}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
		t.Fatal(err)
	}
}

func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }

func TestGuestIsReadOnly(t *testing.T) {
	e := newEnv(t, 8)
	conn := e.dial(t, nil, "")
	sendPixel(t, conn, 3, 4)
	m := next(t, conn, isType("error"))
	if m["err"] != "auth" || m["x"] != 3.0 || m["y"] != 4.0 {
		t.Fatalf("got %v", m)
	}
	if _, owner, _ := e.canvas.At(3, 4); owner != 0 {
		t.Fatal("guest pixel applied")
	}
}

func TestPlayerPixelIsBroadcastWithAuthorAndTime(t *testing.T) {
	e := newEnv(t, 8)
	ck := e.login(t, "Brindille")
	author := e.dial(t, ck, "")
	watcher := e.dial(t, nil, "")
	next(t, watcher, isType("stat"))

	sendPixel(t, author, 5, 6)
	for _, conn := range []*websocket.Conn{author, watcher} {
		m := next(t, conn, isPixel)
		col := m["color"].(map[string]any)
		if m["x"] != 5.0 || m["y"] != 6.0 || col["R"] != 94.0 || col["A"] != 255.0 || m["u"] != 1.0 || m["t"].(float64) <= 0 {
			t.Fatalf("broadcast %v", m)
		}
	}
	if _, owner, _ := e.canvas.At(5, 6); owner != 1 {
		t.Fatalf("owner %d", owner)
	}
	hist, _ := e.store.PixelHistory(5, 6, 3)
	if len(hist) != 1 || hist[0].Color != 0x5eb3ff || hist[0].PrevColor != 0xffffff {
		t.Fatalf("journal %+v", hist)
	}
}

func TestCooldownIsEnforcedByServer(t *testing.T) {
	e := newEnv(t, 8)
	conn := e.dial(t, e.login(t, "Zozo42"), "")
	sendPixel(t, conn, 1, 1)
	next(t, conn, isPixel)

	e.clock.Add(1800 * time.Millisecond)
	sendPixel(t, conn, 2, 2)
	m := next(t, conn, isType("error"))
	if m["err"] != "cooldown" || m["retry"] != 3.2 || m["x"] != 2.0 {
		t.Fatalf("got %v", m)
	}
	if _, owner, _ := e.canvas.At(2, 2); owner != 0 {
		t.Fatal("pixel applied during cooldown")
	}

	e.clock.Add(3200 * time.Millisecond)
	sendPixel(t, conn, 2, 2)
	if m := next(t, conn, func(m map[string]any) bool { return isPixel(m) || m["type"] == "error" }); m["type"] == "error" {
		t.Fatalf("still refused after the cooldown: %v", m)
	}
}

func TestOutOfBoundsPixelIsRefusedWithoutKick(t *testing.T) {
	e := newEnv(t, 8)
	conn := e.dial(t, e.login(t, "Kaelen"), "")
	sendPixel(t, conn, 16, 0)
	if m := next(t, conn, isType("error")); m["err"] != "bounds" {
		t.Fatalf("got %v", m)
	}
	conn.WriteMessage(websocket.TextMessage, []byte("ping"))
	next(t, conn, func(m map[string]any) bool { return m["raw"] == "pong" })
}

func TestPingPongAndStat(t *testing.T) {
	e := newEnv(t, 8)
	conn := e.dial(t, nil, "")
	m := next(t, conn, isType("stat"))
	if m["online"] != 1.0 || m["slots"] != 8.0 {
		t.Fatalf("stat %v", m)
	}
	conn.WriteMessage(websocket.TextMessage, []byte("ping"))
	next(t, conn, func(m map[string]any) bool { return m["raw"] == "pong" })

	e.dial(t, nil, "")
	next(t, conn, func(m map[string]any) bool { return m["type"] == "stat" && m["online"] == 2.0 })

	res, err := http.Get(e.srv.URL + "/stat")
	if err != nil {
		t.Fatal(err)
	}
	var stat map[string]int
	json.NewDecoder(res.Body).Decode(&stat)
	res.Body.Close()
	if stat["connections"] != 2 || stat["slots"] != 8 {
		t.Fatalf("/stat %v", stat)
	}
}

func TestCrossSiteSocketIsReadOnly(t *testing.T) {
	e := newEnv(t, 8)
	conn := e.dial(t, e.login(t, "Brindille"), "https://evil.example")
	sendPixel(t, conn, 1, 1)
	if m := next(t, conn, isType("error")); m["err"] != "auth" {
		t.Fatalf("got %v", m)
	}
}

func TestBadMessageKicks(t *testing.T) {
	e := newEnv(t, 8)
	conn := e.dial(t, nil, "")
	conn.WriteMessage(websocket.TextMessage, []byte("{not json"))
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			if strings.Contains(err.Error(), "timeout") {
				t.Fatal("connection still open after a bad message")
			}
			return
		}
	}
}

func TestServerFull(t *testing.T) {
	e := newEnv(t, 1)
	e.dial(t, nil, "")
	_, res, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(e.srv.URL, "http")+"/ws", nil)
	if err == nil || res == nil || res.StatusCode != 509 {
		t.Fatalf("second connection: %v %v", err, res)
	}
}

func TestSlowClientDoesNotBlockBroadcast(t *testing.T) {
	h := NewHub(NewBlankCanvas(2, 2), nil, nil, 8, time.Second)
	slow := &client{send: make(chan []byte, 1)}
	fast := &client{send: make(chan []byte, 4)}
	h.clients[slow], h.clients[fast] = struct{}{}, struct{}{}
	slow.send <- []byte("full")

	done := make(chan struct{})
	go func() { h.broadcast([]byte("x")); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("broadcast blocked on a slow client")
	}
	if _, ok := h.clients[slow]; ok || !slow.closed {
		t.Fatal("slow client not dropped")
	}
	if len(fast.send) != 1 {
		t.Fatal("fast client missed the message")
	}
}

func TestNoCooldownButFloodGuard(t *testing.T) {
	e := newEnv(t, 8)
	e.hub.SetCooldown(0)
	e.hub.SetMaxRate(2) // burst of 4
	conn := e.dial(t, e.login(t, "Rapide"), "")
	for i := 0; i < 4; i++ {
		sendPixel(t, conn, i, 0)
		if m := next(t, conn, func(m map[string]any) bool { return isPixel(m) || m["type"] == "error" }); m["type"] == "error" {
			t.Fatalf("pixel %d refused without cooldown: %v", i, m)
		}
	}
	sendPixel(t, conn, 5, 0)
	m := next(t, conn, isType("error"))
	if m["err"] != "flood" || m["retry"].(float64) <= 0 {
		t.Fatalf("flood guard: %v", m)
	}
	e.clock.Add(time.Second)
	sendPixel(t, conn, 6, 0)
	if m := next(t, conn, func(m map[string]any) bool { return isPixel(m) || m["type"] == "error" }); m["type"] == "error" {
		t.Fatalf("refused after waiting: %v", m)
	}
	if me := e.getJSON(t, "/api/me", nil); me["cooldown"] != 0.0 {
		t.Fatalf("cooldown %v", me["cooldown"])
	}
}
