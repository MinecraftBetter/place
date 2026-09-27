package place

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	_ "modernc.org/sqlite"
)

// Store is the SQLite database: accounts, sessions and the pixel journal.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

var schema = []string{
	// version 1 — phase 1
	`CREATE TABLE users (
		id              INTEGER PRIMARY KEY,
		fournisseur     TEXT NOT NULL,
		id_externe      TEXT NOT NULL,
		pseudo          TEXT NOT NULL,
		slug            TEXT NOT NULL UNIQUE,
		avatar_url      TEXT NOT NULL DEFAULT '',
		banner          TEXT NOT NULL DEFAULT '',
		bio             TEXT NOT NULL DEFAULT '',
		accent          TEXT NOT NULL DEFAULT '',
		couleur_pref    TEXT NOT NULL DEFAULT '',
		role            TEXT NOT NULL DEFAULT 'joueur',
		statut          TEXT NOT NULL DEFAULT 'actif',
		suspendu_jusqua INTEGER,
		pixels_poses    INTEGER NOT NULL DEFAULT 0,
		pixels_visibles INTEGER NOT NULL DEFAULT 0,
		cree_le         INTEGER NOT NULL,
		UNIQUE (fournisseur, id_externe)
	);
	CREATE TABLE sessions (
		token_hash BLOB PRIMARY KEY,
		user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		cree_le    INTEGER NOT NULL,
		expire_le  INTEGER NOT NULL
	);
	CREATE TABLE pixel_events (
		id           INTEGER PRIMARY KEY,
		x            INTEGER NOT NULL,
		y            INTEGER NOT NULL,
		color        INTEGER NOT NULL,
		user_id      INTEGER NOT NULL,
		prev_color   INTEGER NOT NULL,
		prev_user_id INTEGER NOT NULL,
		ts           INTEGER NOT NULL
	);
	CREATE INDEX pixel_events_xy   ON pixel_events (x, y, id);
	CREATE INDEX pixel_events_user ON pixel_events (user_id, id);
	CREATE INDEX pixel_events_ts   ON pixel_events (ts);`,

	// version 2 — phase 2: profiles and badges
	`ALTER TABLE users ADD COLUMN pixels_restaures INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE users ADD COLUMN pixels_nuit INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE users ADD COLUMN pixels_pionniers INTEGER NOT NULL DEFAULT 0;
	ALTER TABLE users ADD COLUMN badges_epingles TEXT NOT NULL DEFAULT '';
	ALTER TABLE users ADD COLUMN carte_publique INTEGER NOT NULL DEFAULT 1;
	ALTER TABLE users ADD COLUMN alertes_retouche INTEGER NOT NULL DEFAULT 1;
	CREATE TABLE user_badges (
		user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		badge   TEXT NOT NULL,
		source  TEXT NOT NULL DEFAULT 'auto',
		detail  TEXT NOT NULL DEFAULT '',
		ts      INTEGER NOT NULL,
		PRIMARY KEY (user_id, badge)
	);`,
}

// OpenStore opens (and creates or migrates) the database. Use ":memory:" in tests.
func OpenStore(path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	if path != ":memory:" {
		dsn += "&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection: no SQLITE_BUSY between our own goroutines, and ":memory:" stays one database.
	db.SetMaxOpenConns(1)
	st := &Store{db: db, now: time.Now}
	if err := st.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return st, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`); err != nil {
		return err
	}
	var v int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`).Scan(&v); err != nil {
		return err
	}
	for ; v < len(schema); v++ {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(schema[v]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", v+1, err)
		}
		if _, err := tx.Exec(`DELETE FROM schema_version`); err != nil {
			tx.Rollback()
			return err
		}
		if _, err := tx.Exec(`INSERT INTO schema_version VALUES (?)`, v+1); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// ------------------------------------------------------------------
// Users

type User struct {
	ID             uint32
	Provider       string
	ExternalID     string
	Pseudo         string
	Slug           string
	AvatarURL      string
	Accent         string
	Role           string
	Status         string // actif | suspendu | banni
	SuspendedUntil int64  // ms, 0 when not suspended
	PixelsPlaced   int64
	PixelsVisible  int64
	CreatedAt      int64
	Bio            string
	Banner         string // desert | nuit | uni
	FavColor       string // a palette colour, "#RRGGBB"
	PixelsRestored int64  // pixels put back to their previous colour (badge Restaurateur)
	PixelsNight    int64  // pixels placed between 2 and 5 am, Paris time (badge Noctambule)
	PixelsPioneer  int64  // pixels of validated pre-account artworks (claims)
	PinnedBadges   []string
	MapPublic      bool
	RetouchAlerts  bool
}

// PublicUser is what other players can see.
type PublicUser struct {
	ID     uint32 `json:"id"`
	Pseudo string `json:"pseudo"`
	Slug   string `json:"slug"`
	Avatar string `json:"avatar"`
	Accent string `json:"accent"`
}

func (u *User) Public() *PublicUser {
	if u == nil {
		return nil
	}
	return &PublicUser{u.ID, u.Pseudo, u.Slug, u.AvatarURL, u.Accent}
}

// WriteBlock returns why the player cannot draw right now ("" if they can).
func (u *User) WriteBlock(now time.Time) string {
	switch {
	case u.Status == "banni":
		return "banned"
	case u.Status == "suspendu" && (u.SuspendedUntil == 0 || now.UnixMilli() < u.SuspendedUntil):
		return "suspended"
	}
	return ""
}

// userCols selects a user from "users u".
const userCols = `u.id, u.fournisseur, u.id_externe, u.pseudo, u.slug, u.avatar_url, u.accent, u.role, u.statut,
	COALESCE(u.suspendu_jusqua, 0), u.pixels_poses, u.pixels_visibles, u.cree_le, u.bio, u.banner, u.couleur_pref,
	u.pixels_restaures, u.pixels_nuit, u.pixels_pionniers, u.badges_epingles, u.carte_publique, u.alertes_retouche`

type scanner interface{ Scan(dest ...any) error }

func scanUser(row scanner) (*User, error) {
	u := &User{}
	var pinned string
	err := row.Scan(&u.ID, &u.Provider, &u.ExternalID, &u.Pseudo, &u.Slug, &u.AvatarURL, &u.Accent, &u.Role,
		&u.Status, &u.SuspendedUntil, &u.PixelsPlaced, &u.PixelsVisible, &u.CreatedAt, &u.Bio, &u.Banner, &u.FavColor,
		&u.PixelsRestored, &u.PixelsNight, &u.PixelsPioneer, &pinned, &u.MapPublic, &u.RetouchAlerts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if pinned != "" {
		u.PinnedBadges = strings.Split(pinned, ",")
	}
	return u, err
}

// NewUser describes an account coming from a login provider.
type NewUser struct {
	Provider   string
	ExternalID string
	Pseudo     string
	AvatarURL  string
	Accent     string
}

// LoginUser returns the account for a provider identity, creating it on first login.
func (s *Store) LoginUser(n NewUser) (*User, error) {
	u, err := scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM users u WHERE fournisseur = ? AND id_externe = ?`, n.Provider, n.ExternalID))
	if u != nil || err != nil {
		return u, err
	}
	base := Slugify(n.Pseudo)
	for i := 1; ; i++ {
		slug := base
		if i > 1 {
			slug = fmt.Sprintf("%s-%d", base, i)
		}
		_, err = s.db.Exec(`INSERT INTO users (fournisseur, id_externe, pseudo, slug, avatar_url, accent, cree_le) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			n.Provider, n.ExternalID, n.Pseudo, slug, n.AvatarURL, n.Accent, s.now().UnixMilli())
		if err == nil {
			break
		}
		if !strings.Contains(err.Error(), "UNIQUE") || !strings.Contains(err.Error(), "slug") || i > 1000 {
			return nil, err
		}
	}
	return scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM users u WHERE fournisseur = ? AND id_externe = ?`, n.Provider, n.ExternalID))
}

func (s *Store) UserByID(id uint32) (*User, error) {
	return scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM users u WHERE id = ?`, id))
}

func (s *Store) UsersByIDs(ids []uint32) (map[uint32]*User, error) {
	res := map[uint32]*User{}
	if len(ids) == 0 {
		return res, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.Query(`SELECT `+userCols+` FROM users u WHERE id IN (?`+strings.Repeat(",?", len(ids)-1)+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		res[u.ID] = u
	}
	return res, rows.Err()
}

// Slugify turns a pseudo into a URL-safe slug: "Mamie Pixel" → "mamie-pixel".
func Slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if a, ok := accents[r]; ok {
			r = a
		}
		switch {
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
			dash = false
		case b.Len() > 0 && !dash:
			b.WriteByte('-')
			dash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if len(slug) > 32 {
		slug = strings.Trim(slug[:32], "-")
	}
	if slug == "" {
		slug = "joueur"
	}
	return slug
}

var accents = map[rune]rune{
	'à': 'a', 'â': 'a', 'ä': 'a', 'á': 'a', 'ã': 'a', 'ç': 'c', 'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e',
	'î': 'i', 'ï': 'i', 'í': 'i', 'ô': 'o', 'ö': 'o', 'ó': 'o', 'õ': 'o', 'ù': 'u', 'û': 'u', 'ü': 'u',
	'ú': 'u', 'ÿ': 'y', 'ñ': 'n', 'œ': 'o', 'æ': 'a',
}

// ------------------------------------------------------------------
// Sessions

func hashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// CreateSession returns a new random session token for the player.
func (s *Store) CreateSession(uid uint32, ttl time.Duration) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	now := s.now()
	_, err := s.db.Exec(`INSERT INTO sessions (token_hash, user_id, cree_le, expire_le) VALUES (?, ?, ?, ?)`,
		hashToken(token), uid, now.UnixMilli(), now.Add(ttl).UnixMilli())
	return token, err
}

// SessionUser returns the player of a valid session, or nil.
func (s *Store) SessionUser(token string) (*User, error) {
	if token == "" {
		return nil, nil
	}
	return scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expire_le > ?`, hashToken(token), s.now().UnixMilli()))
}

func (s *Store) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, hashToken(token))
	return err
}

// DeleteExpiredSessions removes stale sessions; called at startup.
func (s *Store) DeleteExpiredSessions() error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE expire_le <= ?`, s.now().UnixMilli())
	return err
}

// ------------------------------------------------------------------
// Pixel journal

type PixelEvent struct {
	ID         int64
	X, Y       int
	Color      uint32 // 0xRRGGBB
	UserID     uint32
	PrevColor  uint32
	PrevUserID uint32
	TS         int64 // ms
}

// RecordPixel appends to the journal, updates the players' counters and returns the
// badges the player has just earned.
func (s *Store) RecordPixel(e PixelEvent) ([]string, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Restoring: putting back the colour a pixel had before someone else changed it.
	restored := 0
	var lastColor, lastPrev, lastUser int64
	err = tx.QueryRow(`SELECT color, prev_color, user_id FROM pixel_events WHERE x = ? AND y = ? ORDER BY id DESC LIMIT 1`, e.X, e.Y).
		Scan(&lastColor, &lastPrev, &lastUser)
	if err == nil && uint32(lastUser) != e.UserID && int64(e.Color) == lastPrev && int64(e.Color) != lastColor {
		restored = 1
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	night := 0
	if h := time.UnixMilli(e.TS).In(Paris).Hour(); h >= 2 && h < 5 {
		night = 1
	}

	if _, err := tx.Exec(`INSERT INTO pixel_events (x, y, color, user_id, prev_color, prev_user_id, ts) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.X, e.Y, e.Color, e.UserID, e.PrevColor, e.PrevUserID, e.TS); err != nil {
		return nil, err
	}
	visible := 0
	if e.PrevUserID != e.UserID {
		visible = 1
		if e.PrevUserID != 0 {
			if _, err := tx.Exec(`UPDATE users SET pixels_visibles = MAX(pixels_visibles - 1, 0) WHERE id = ?`, e.PrevUserID); err != nil {
				return nil, err
			}
		}
	}
	if _, err := tx.Exec(`UPDATE users SET pixels_poses = pixels_poses + 1, pixels_visibles = pixels_visibles + ?,
		pixels_restaures = pixels_restaures + ?, pixels_nuit = pixels_nuit + ? WHERE id = ?`, visible, restored, night, e.UserID); err != nil {
		return nil, err
	}
	earned, err := awardCounterBadges(tx, e.UserID, e.TS)
	if err != nil {
		return nil, err
	}
	return earned, tx.Commit()
}

const eventCols = `id, x, y, color, user_id, prev_color, prev_user_id, ts`

func scanEvent(row scanner) (PixelEvent, error) {
	var e PixelEvent
	err := row.Scan(&e.ID, &e.X, &e.Y, &e.Color, &e.UserID, &e.PrevColor, &e.PrevUserID, &e.TS)
	return e, err
}

// PixelHistory returns the latest changes of a pixel, newest first.
func (s *Store) PixelHistory(x, y, limit int) ([]PixelEvent, error) {
	rows, err := s.db.Query(`SELECT `+eventCols+` FROM pixel_events WHERE x = ? AND y = ? ORDER BY id DESC LIMIT ?`, x, y, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []PixelEvent
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		res = append(res, e)
	}
	return res, rows.Err()
}

// EachPixelEvent calls fn for every event at or after sinceTS, in order.
// fn must not use the store (the only connection is busy).
func (s *Store) EachPixelEvent(sinceTS int64, fn func(PixelEvent) error) error {
	rows, err := s.db.Query(`SELECT `+eventCols+` FROM pixel_events WHERE ts >= ? ORDER BY ts, id`, sinceTS)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return err
		}
		if err := fn(e); err != nil {
			return err
		}
	}
	return rows.Err()
}

// SetVisibleCounts overwrites every player's count of visible pixels.
func (s *Store) SetVisibleCounts(counts map[uint32]int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE users SET pixels_visibles = 0`); err != nil {
		return err
	}
	for id, n := range counts {
		if _, err := tx.Exec(`UPDATE users SET pixels_visibles = ? WHERE id = ?`, n, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
