package place

import (
	"database/sql"
	"fmt"
	"time"
	_ "time/tzdata" // the Docker image (busybox) has no time zone database
)

// Paris is the time zone of the community: nights, weeks and days are counted in it.
var Paris = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		return time.Local
	}
	return loc
}()

// Badge describes one of the badges of HANDOFF §7.3.
type Badge struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Rule   string `json:"rule"`
	Sprite string `json:"sprite"`
	Gold   bool   `json:"gold"` // earned through claims (pre-account artworks)
}

// Badges is the catalogue, in display order.
var Badges = []Badge{
	{"premier-pixel", "Premier pixel", "Poser son premier pixel", "/img/badges/premier-pixel.png", false},
	{"millier", "Millier", "1 000 pixels posés (pixels pionniers compris)", "/img/badges/millier.png", false},
	{"restaurateur", "Restaurateur", "50 pixels remis à l'identique d'une version précédente", "/img/badges/restaurateur.png", false},
	{"noctambule", "Noctambule", "100 pixels posés entre 2 h et 5 h", "/img/badges/noctambule.png", false},
	{"top10", "Top 10", "Dans le top 10 d'une semaine", "/img/badges/top10.png", false},
	{"temoin", "Témoin", "10 confirmations sur des revendications validées", "/img/badges/temoin.png", false},
	{"pionnier", "Pionnier", "Une œuvre d'avant les comptes, revendiquée et validée", "/img/badges/pionnier.png", true},
	{"veteran", "Vétéran 2022", "Une œuvre apparue en 2022", "/img/badges/veteran.png", true},
	{"batisseur", "Bâtisseur", "Une œuvre de plus de 1 000 pixels", "/img/badges/batisseur.png", true},
	{"indemodable", "Indémodable", "Une œuvre intacte à 90 % pendant plus d'un an", "/img/badges/indemodable.png", true},
}

func BadgeByID(id string) *Badge {
	for i := range Badges {
		if Badges[i].ID == id {
			return &Badges[i]
		}
	}
	return nil
}

const (
	millierThreshold      = 1000
	restaurateurThreshold = 50
	noctambuleThreshold   = 100
	temoinThreshold       = 10
)

type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}

// grant inserts a badge if the player does not have it yet; true when it is new.
func grant(db execer, uid uint32, badge, source, detail string, ts int64) (bool, error) {
	res, err := db.Exec(`INSERT OR IGNORE INTO user_badges (user_id, badge, source, detail, ts) VALUES (?, ?, ?, ?, ?)`, uid, badge, source, detail, ts)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// awardCounterBadges checks the badges computed from the player's counters.
func awardCounterBadges(db execer, uid uint32, ts int64) ([]string, error) {
	var placed, pioneer, restored, night int64
	if err := db.QueryRow(`SELECT pixels_poses, pixels_pionniers, pixels_restaures, pixels_nuit FROM users WHERE id = ?`, uid).
		Scan(&placed, &pioneer, &restored, &night); err != nil {
		return nil, err
	}
	checks := []struct {
		ok     bool
		badge  string
		detail string
	}{
		{placed >= 1, "premier-pixel", time.UnixMilli(ts).In(Paris).Format("02/01/2006")},
		{placed+pioneer >= millierThreshold, "millier", fmt.Sprintf("%d px", placed+pioneer)},
		{restored >= restaurateurThreshold, "restaurateur", fmt.Sprintf("%d px restaurés", restored)},
		{night >= noctambuleThreshold, "noctambule", fmt.Sprintf("%d px de nuit", night)},
	}
	var earned []string
	for _, c := range checks {
		if !c.ok {
			continue
		}
		isNew, err := grant(db, uid, c.badge, "auto", c.detail, ts)
		if err != nil {
			return nil, err
		}
		if isNew {
			earned = append(earned, c.badge)
		}
	}
	return earned, nil
}

// BackfillBadges re-checks the counter badges of every player (at startup, so that
// players who drew before a badge existed get it too).
func (s *Store) BackfillBadges() (int, error) {
	rows, err := s.db.Query(`SELECT id FROM users WHERE pixels_poses + pixels_pionniers > 0`)
	if err != nil {
		return 0, err
	}
	var ids []uint32
	for rows.Next() {
		var id uint32
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	n := 0
	for _, id := range ids {
		earned, err := awardCounterBadges(s.db, id, s.now().UnixMilli())
		if err != nil {
			return n, err
		}
		n += len(earned)
	}
	return n, nil
}

// GrantBadge gives a badge (claims, admin). Returns true if it is new.
func (s *Store) GrantBadge(uid uint32, badge, source, detail string) (bool, error) {
	return grant(s.db, uid, badge, source, detail, s.now().UnixMilli())
}

// RevokeBadge removes a badge (admin undo).
func (s *Store) RevokeBadge(uid uint32, badge string) error {
	_, err := s.db.Exec(`DELETE FROM user_badges WHERE user_id = ? AND badge = ?`, uid, badge)
	return err
}

type UserBadge struct {
	Badge  string `json:"id"`
	Source string `json:"source"`
	Detail string `json:"detail"`
	TS     int64  `json:"ts"`
}

func (s *Store) UserBadges(uid uint32) (map[string]UserBadge, error) {
	rows, err := s.db.Query(`SELECT badge, source, detail, ts FROM user_badges WHERE user_id = ?`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	res := map[string]UserBadge{}
	for rows.Next() {
		var b UserBadge
		if err := rows.Scan(&b.Badge, &b.Source, &b.Detail, &b.TS); err != nil {
			return nil, err
		}
		res[b.Badge] = b
	}
	return res, rows.Err()
}

// WeekStart returns Monday 00:00 (Paris) of the week containing t.
func WeekStart(t time.Time) time.Time {
	t = t.In(Paris)
	d := (int(t.Weekday()) + 6) % 7 // Monday = 0
	return time.Date(t.Year(), t.Month(), t.Day()-d, 0, 0, 0, 0, Paris)
}

// DayStart returns 00:00 (Paris) of the day containing t.
func DayStart(t time.Time) time.Time {
	t = t.In(Paris)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, Paris)
}

// RankEntry is one line of a ranking by pixels placed.
type RankEntry struct {
	UserID uint32 `json:"-"`
	Pixels int64  `json:"pixels"`
	Rank   int    `json:"rank"`
}

// RankSince ranks players by pixels placed in [from, to) (to = 0: until now).
func (s *Store) RankSince(from, to int64, limit int) ([]RankEntry, error) {
	if to == 0 {
		to = 1 << 62
	}
	rows, err := s.db.Query(`SELECT user_id, COUNT(*) AS n FROM pixel_events WHERE ts >= ? AND ts < ?
		GROUP BY user_id ORDER BY n DESC, MIN(ts) LIMIT ?`, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []RankEntry
	for rows.Next() {
		var e RankEntry
		if err := rows.Scan(&e.UserID, &e.Pixels); err != nil {
			return nil, err
		}
		e.Rank = len(res) + 1
		res = append(res, e)
	}
	return res, rows.Err()
}

// RankAllTime ranks players by pixels placed plus pioneer pixels.
func (s *Store) RankAllTime(limit int) ([]RankEntry, error) {
	rows, err := s.db.Query(`SELECT id, pixels_poses + pixels_pionniers AS n FROM users WHERE n > 0 AND statut != 'banni'
		ORDER BY n DESC, cree_le LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res []RankEntry
	for rows.Next() {
		var e RankEntry
		if err := rows.Scan(&e.UserID, &e.Pixels); err != nil {
			return nil, err
		}
		e.Rank = len(res) + 1
		res = append(res, e)
	}
	return res, rows.Err()
}

// UserRank returns the rank of a player in a period, 0 if they placed nothing.
func (s *Store) UserRank(uid uint32, from int64) (int, error) {
	var mine int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM pixel_events WHERE user_id = ? AND ts >= ?`, uid, from).Scan(&mine); err != nil || mine == 0 {
		return 0, err
	}
	var above int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM (SELECT user_id, COUNT(*) AS n FROM pixel_events WHERE ts >= ? GROUP BY user_id) WHERE n > ?`, from, mine).Scan(&above)
	return above + 1, err
}

// UserRankAllTime returns the all-time rank of a player, 0 if they have no pixel.
func (s *Store) UserRankAllTime(uid uint32) (int, error) {
	var mine int64
	if err := s.db.QueryRow(`SELECT pixels_poses + pixels_pionniers FROM users WHERE id = ?`, uid).Scan(&mine); err != nil || mine == 0 {
		return 0, err
	}
	var above int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users WHERE pixels_poses + pixels_pionniers > ? AND statut != 'banni'`, mine).Scan(&above)
	return above + 1, err
}

// AwardWeeklyTop10 gives the Top 10 badge for the last complete week. Idempotent.
func (s *Store) AwardWeeklyTop10(now time.Time) (int, error) {
	end := WeekStart(now)
	start := end.AddDate(0, 0, -7)
	top, err := s.RankSince(start.UnixMilli(), end.UnixMilli(), 10)
	if err != nil {
		return 0, err
	}
	detail := "semaine du " + frenchDate(start)
	n := 0
	for _, e := range top {
		isNew, err := grant(s.db, e.UserID, "top10", "auto", detail, end.UnixMilli())
		if err != nil {
			return n, err
		}
		if isNew {
			n++
		}
	}
	return n, nil
}

var frenchMonths = []string{"janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."}

func frenchDate(t time.Time) string {
	t = t.In(Paris)
	return fmt.Sprintf("%d %s %d", t.Day(), frenchMonths[t.Month()-1], t.Year())
}
