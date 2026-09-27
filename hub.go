package place

import (
	"bytes"
	"encoding/json"
	"image/color"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 120 * time.Second // the client also sends "ping" every 30 s
	pingPeriod = 50 * time.Second
	sendQueue  = 256
	statDelay  = 500 * time.Millisecond
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Cross-site sockets are allowed but stay read-only: the session is only
	// honoured when the Origin matches (see ServeWS).
	CheckOrigin: func(r *http.Request) bool { return true },
	Error: func(w http.ResponseWriter, r *http.Request, status int, err error) {
		log.WithField("ip", r.RemoteAddr).WithField("endpoint", "Socket").WithField("action", "Get").Error(err)
		http.Error(w, "Error while trying to make websocket connection.", status)
	},
}

// PixelColor is a pixel sent by a client, in the historical format.
type PixelColor struct {
	X     int         `json:"x"`
	Y     int         `json:"y"`
	Color color.NRGBA `json:"color"`
}

// pixelMsg is a broadcast pixel: the historical format plus its author and time.
type pixelMsg struct {
	X     int         `json:"x"`
	Y     int         `json:"y"`
	Color color.NRGBA `json:"color"`
	U     uint32      `json:"u"`
	T     int64       `json:"t"`
}

type errorMsg struct {
	Type  string  `json:"type"`
	Err   string  `json:"err"`
	Retry float64 `json:"retry,omitempty"`
	X     int     `json:"x"`
	Y     int     `json:"y"`
}

type statMsg struct {
	Type   string `json:"type"`
	Online int    `json:"online"`
	Slots  int    `json:"slots"`
}

// Hub keeps the WebSocket clients, applies the cooldown and broadcasts pixels.
type Hub struct {
	canvas   *Canvas
	store    *Store
	auth     *Auth
	cooldown time.Duration
	maxConns int
	now      func() time.Time

	maxRate     float64       // anti-flood: pixels per second per player (0 = unlimited)
	cooldownNew time.Duration // cooldown for accounts of less than an hour
	mode        string        // normal | readonly | maintenance
	mu          sync.Mutex
	clients     map[*client]struct{}
	nextAt      map[uint32]time.Time
	buckets     map[uint32]*bucket
	statTimer   *time.Timer
	pixelHooks  []func(u *User, e PixelEvent)
	badgeHooks  []func(u *User, badge string)
}

type client struct {
	conn   *websocket.Conn
	user   *User // nil for guests
	ip     string
	send   chan []byte
	closed bool // guarded by Hub.mu
}

func NewHub(c *Canvas, st *Store, a *Auth, maxConns int, cooldown time.Duration) *Hub {
	return &Hub{
		canvas: c, store: st, auth: a, cooldown: cooldown, maxConns: maxConns, now: time.Now,
		clients: map[*client]struct{}{}, nextAt: map[uint32]time.Time{}, buckets: map[uint32]*bucket{},
		maxRate: 30,
	}
}

// bucket is a token bucket: maxRate tokens per second, up to twice that in a burst.
type bucket struct {
	tokens float64
	at     time.Time
}

// SetMaxRate sets the anti-flood limit (pixels per second per player, 0 = unlimited).
// Invisible to people drawing by hand, it stops scripts from flooding the canvas.
func (h *Hub) SetMaxRate(r float64) { h.maxRate = r }

// SetCooldown changes the delay between two pixels (admin settings).
func (h *Hub) SetCooldown(d time.Duration) {
	h.mu.Lock()
	h.cooldown = d
	h.mu.Unlock()
}

// allowRate takes a token for the player; the second value is the wait when refused.
// Called with h.mu held.
func (h *Hub) allowRate(uid uint32, now time.Time) (bool, float64) {
	if h.maxRate <= 0 {
		return true, 0
	}
	b := h.buckets[uid]
	burst := h.maxRate * 2
	if b == nil {
		b = &bucket{tokens: burst, at: now}
		h.buckets[uid] = b
	}
	b.tokens = math.Min(burst, b.tokens+now.Sub(b.at).Seconds()*h.maxRate)
	b.at = now
	if b.tokens < 1 {
		return false, (1 - b.tokens) / h.maxRate
	}
	b.tokens--
	return true, 0
}

// Online returns the number of connections and the maximum.
func (h *Hub) Online() (count, slots int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients), h.maxConns
}

func (h *Hub) Cooldown() time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cooldown
}

// CooldownFor returns the delay that applies to a player (new accounts may wait longer).
func (h *Hub) CooldownFor(u *User) time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cooldownForLocked(u, h.now())
}

func (h *Hub) cooldownForLocked(u *User, now time.Time) time.Duration {
	if u != nil && h.cooldownNew > h.cooldown && now.UnixMilli()-u.CreatedAt < int64(time.Hour/time.Millisecond) {
		return h.cooldownNew
	}
	return h.cooldown
}

// UpdateUser refreshes the player attached to open sockets (suspension, role…).
func (h *Hub) UpdateUser(u *User) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		if c.user != nil && c.user.ID == u.ID {
			c.user = u
		}
	}
}

// ReadyIn returns how long the player must still wait before drawing.
func (h *Hub) ReadyIn(uid uint32) time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()
	if d := h.nextAt[uid].Sub(h.now()); d > 0 {
		return d
	}
	return 0
}

func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	logger := log.WithField("ip", r.RemoteAddr).WithField("endpoint", "Socket").WithField("action", "Get")
	var user *User
	if sameOrigin(r) {
		user = h.auth.User(r)
	}
	c := &client{user: user, ip: r.RemoteAddr, send: make(chan []byte, sendQueue)}

	h.mu.Lock()
	if len(h.clients) >= h.maxConns {
		h.mu.Unlock()
		logger.Warning("Server full")
		http.Error(w, "Server full", 509)
		return
	}
	h.clients[c] = struct{}{} // reserve the slot before upgrading
	h.mu.Unlock()

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.remove(c)
		return
	}
	c.conn = conn
	who := "guest"
	if user != nil {
		who = user.Pseudo
	}
	logger.Info("Connected (", who, ")")

	online, slots := h.Online()
	h.sendTo(c, mustJSON(statMsg{"stat", online, slots}))
	h.scheduleStat()
	go h.writePump(c)
	go h.readPump(c)
}

func (h *Hub) readPump(c *client) {
	logger := log.WithField("ip", c.ip).WithField("endpoint", "Socket").WithField("action", "Read")
	defer func() {
		h.remove(c)
		c.conn.Close()
		logger.Info("Disconnected")
	}()
	c.conn.SetReadLimit(1024)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { return c.conn.SetReadDeadline(time.Now().Add(pongWait)) })
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNoStatusReceived) {
				logger.Error("Unexpected close error, ", err)
			}
			return
		}
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		if bytes.Equal(msg, []byte("ping")) {
			h.sendTo(c, []byte("pong"))
			continue
		}
		var p PixelColor
		if err := json.Unmarshal(msg, &p); err != nil {
			logger.Error("Client kicked for bad message (", string(msg), "), ", err)
			return
		}
		if p == (PixelColor{}) { // p is "nil"
			continue
		}
		h.handlePixel(c, p)
	}
}

func (h *Hub) writePump(c *client) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				log.WithField("ip", c.ip).WithField("endpoint", "Socket").WithField("action", "Write").Error("Write error ", err)
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (h *Hub) handlePixel(c *client, p PixelColor) {
	fail := func(code string, retry float64) {
		h.sendTo(c, mustJSON(errorMsg{Type: "error", Err: code, Retry: retry, X: p.X, Y: p.Y}))
	}
	if c.user == nil {
		fail("auth", 0)
		return
	}
	now := h.now()
	if code := c.user.WriteBlock(now); code != "" {
		fail(code, 0)
		return
	}
	if _, _, ok := h.canvas.At(p.X, p.Y); !ok {
		fail("bounds", 0)
		return
	}
	uid := c.user.ID
	h.mu.Lock()
	if (h.mode == "readonly" || h.mode == "maintenance") && c.user.Role != "admin" {
		mode := h.mode
		h.mu.Unlock()
		fail(mode, 0)
		return
	}
	if next := h.nextAt[uid]; now.Before(next) {
		h.mu.Unlock()
		fail("cooldown", math.Ceil(next.Sub(now).Seconds()*10)/10)
		return
	}
	if ok, wait := h.allowRate(uid, now); !ok {
		h.mu.Unlock()
		fail("flood", math.Ceil(wait*10)/10)
		return
	}
	if cd := h.cooldownForLocked(c.user, now); cd > 0 {
		h.nextAt[uid] = now.Add(cd)
	}
	h.mu.Unlock()

	p.Color.A = 255
	prev, prevOwner, ts, _ := h.canvas.Set(p.X, p.Y, p.Color, uid)
	ev := PixelEvent{
		X: p.X, Y: p.Y, Color: nrgbaToRGB(p.Color), UserID: uid,
		PrevColor: nrgbaToRGB(prev), PrevUserID: prevOwner, TS: ts,
	}
	earned, err := h.store.RecordPixel(ev)
	if err != nil {
		log.WithField("endpoint", "Socket").Error("Recording pixel: ", err)
	}
	log.WithField("ip", c.ip).WithField("endpoint", "Socket").WithField("action", "Read").
		Debugf("Pixel (%d, %d) changed to %s by %s", p.X, p.Y, toHex(p.Color), c.user.Pseudo)
	h.broadcast(mustJSON(pixelMsg{p.X, p.Y, p.Color, uid, ts}))
	for _, b := range earned {
		if badge := BadgeByID(b); badge != nil {
			h.SendToUser(uid, mustJSON(badgeMsg{"badge", badge.ID, badge.Name, badge.Sprite}))
		}
		for _, f := range h.badgeHooks {
			f(c.user, b)
		}
	}
	for _, f := range h.pixelHooks {
		f(c.user, ev)
	}
}

type badgeMsg struct {
	Type   string `json:"type"`
	Badge  string `json:"badge"`
	Name   string `json:"name"`
	Sprite string `json:"sprite"`
}

// OnPixel registers a function called after each accepted pixel (activity, alerts…).
// Hooks run on the placing client's goroutine: keep them quick.
func (h *Hub) OnPixel(f func(u *User, e PixelEvent)) { h.pixelHooks = append(h.pixelHooks, f) }

// OnBadge registers a function called when a player earns a badge while drawing.
func (h *Hub) OnBadge(f func(u *User, badge string)) { h.badgeHooks = append(h.badgeHooks, f) }

// SendToUser queues a message for every socket of a player.
func (h *Hub) SendToUser(uid uint32, msg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		if c.user != nil && c.user.ID == uid && !c.closed {
			select {
			case c.send <- msg:
			default:
			}
		}
	}
}

// Broadcast queues a message for every client.
func (h *Hub) Broadcast(msg []byte) { h.broadcast(msg) }

// broadcast queues a message for every client; a client whose queue is full is dropped
// instead of blocking everyone else.
func (h *Hub) broadcast(msg []byte) {
	h.mu.Lock()
	dropped := false
	for c := range h.clients {
		select {
		case c.send <- msg:
		default:
			h.removeLocked(c)
			dropped = true
			log.WithField("ip", c.ip).WithField("endpoint", "Socket").Warning("Client too slow, dropped")
		}
	}
	h.mu.Unlock()
	if dropped {
		h.scheduleStat()
	}
}

func (h *Hub) sendTo(c *client, msg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if c.closed {
		return
	}
	select {
	case c.send <- msg:
	default:
	}
}

func (h *Hub) remove(c *client) {
	h.mu.Lock()
	removed := h.removeLocked(c)
	h.mu.Unlock()
	if removed {
		h.scheduleStat()
	}
}

func (h *Hub) removeLocked(c *client) bool {
	if c.closed {
		return false
	}
	c.closed = true
	delete(h.clients, c)
	close(c.send)
	return true
}

// scheduleStat broadcasts the number of connections, at most every statDelay.
func (h *Hub) scheduleStat() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.statTimer != nil {
		return
	}
	h.statTimer = time.AfterFunc(statDelay, func() {
		h.mu.Lock()
		h.statTimer = nil
		msg := mustJSON(statMsg{"stat", len(h.clients), h.maxConns})
		h.mu.Unlock()
		h.broadcast(msg)
	})
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
