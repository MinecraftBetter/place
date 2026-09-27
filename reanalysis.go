package place

// When new captures reach the backup index (monthly zips, the server's bak/ folder…),
// the artworks are analysed again: dates, construction, stability. A gold badge that the
// new captures make true (typically « Indémodable » after a year intact) is given to the
// authors, like the ones given at validation. Badges the previous analysis already
// proposed stay the admins' decision.

import (
	"encoding/json"
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
)

type ReanalysisReport struct {
	Frames  int              `json:"sauvegardes"`
	Oeuvres int              `json:"oeuvres"`
	Claims  int              `json:"revendications"`
	Badges  []ReanalysisGain `json:"badges"`
	At      int64            `json:"le"`
}

type ReanalysisGain struct {
	User   *PublicUser `json:"user"`
	Badge  string      `json:"badge"`
	Oeuvre int64       `json:"oeuvre"`
	Titre  string      `json:"titre"`
}

// Reanalyse re-runs the zone analysis of every artwork and pending claim on the current index.
func (api *API) Reanalyse() (*ReanalysisReport, error) {
	idx := api.index()
	if idx == nil {
		return nil, fmt.Errorf("pas de sauvegardes")
	}
	api.reanalysing.Lock()
	defer api.reanalysing.Unlock()
	now := api.store.now()
	migration := api.store.MigrationTime()
	rep := &ReanalysisReport{Frames: len(idx.Frames), At: now.UnixMilli(), Badges: []ReanalysisGain{}}

	os, err := api.store.Oeuvres(0, false)
	if err != nil {
		return nil, err
	}
	for _, o := range os {
		pos, err := o.Mask.Positions(idx.W, idx.H)
		if err != nil || len(pos) == 0 {
			continue
		}
		before := o.Analysis()
		a := AnalyzeZone(idx, pos, migration, now)
		aj, _ := json.Marshal(a)
		var apparue, terminee any
		if a.Appeared != 0 {
			apparue = a.Appeared
		}
		if a.Finished != 0 {
			terminee = a.Finished
		}
		if _, err := api.store.db.Exec(`UPDATE oeuvres SET analyse_json = ?, apparue_le = COALESCE(?, apparue_le), terminee_le = COALESCE(?, terminee_le) WHERE id = ?`,
			string(aj), apparue, terminee, o.ID); err != nil {
			return nil, err
		}
		rep.Oeuvres++
		for _, b := range a.Badges {
			if !contains(goldBadges, b) || before != nil && contains(before.Badges, b) {
				continue // not new: validation already decided about it
			}
			if b == "pionnier" && o.Origine != "avant_migration" {
				continue
			}
			for _, au := range o.Auteurs {
				fresh, err := grant(api.store.db, au.userID, b, "sauvegardes", fmt.Sprintf("oeuvre:%d", o.ID), now.UnixMilli())
				if err != nil || !fresh {
					continue
				}
				rep.Badges = append(rep.Badges, ReanalysisGain{User: au.User, Badge: b, Oeuvre: o.ID, Titre: o.Titre})
				api.announceBadge(au.userID, b, o)
			}
		}
	}

	claims, err := api.store.Claims("attente", 0, 0)
	if err == nil {
		for _, c := range claims {
			if a, err := api.analyseFresh(c.Mask); err == nil && a != nil {
				api.store.SetClaimAnalysis(c.ID, a)
				rep.Claims++
			}
		}
	}
	rj, _ := json.Marshal(rep)
	api.store.SetSetting("reanalyse", string(rj))
	log.WithField("endpoint", "Backups").Infof("Reanalysis: %d artworks, %d claims, %d new badges", rep.Oeuvres, rep.Claims, len(rep.Badges))
	return rep, nil
}

// analyseFresh bypasses the analysis cache (the index just changed).
func (api *API) analyseFresh(m Mask) (*ZoneAnalysis, error) {
	idx := api.index()
	if idx == nil {
		return nil, nil
	}
	pos, err := m.Positions(idx.W, idx.H)
	if err != nil {
		return nil, err
	}
	return AnalyzeZone(idx, pos, api.store.MigrationTime(), api.store.now()), nil
}

// announceBadge tells the player (live toast + alert) and the activity feed.
func (api *API) announceBadge(uid uint32, badge string, o *Oeuvre) {
	b := BadgeByID(badge)
	if b == nil {
		return
	}
	api.hub.SendToUser(uid, mustJSON(badgeMsg{"badge", b.ID, b.Name, b.Sprite}))
	if api.community != nil {
		api.community.alert(uid, "badge_oeuvre", map[string]any{"badge": b.ID, "nom": b.Name, "sprite": b.Sprite, "oeuvre": o.ID, "titre": o.Titre})
		if u, _ := api.store.UserByID(uid); u != nil {
			api.community.Emit("badge", u, 0, 0, 1, "badge:"+b.ID, b.Name)
		}
	}
}

// LastReanalysis returns the report of the last run, if any.
func (api *API) LastReanalysis() *ReanalysisReport {
	v := api.store.Setting("reanalyse", "")
	if v == "" {
		return nil
	}
	var r ReanalysisReport
	if json.Unmarshal([]byte(v), &r) != nil {
		return nil
	}
	return &r
}

// watchBackups re-analyses after each index update that brought new captures.
func (api *API) watchBackups(b *Backups) {
	b.OnUpdate(func(frames int) {
		go func() {
			time.Sleep(time.Second)
			if _, err := api.Reanalyse(); err != nil {
				log.WithField("endpoint", "Backups").Warning("Reanalysis: ", err)
			}
		}()
	})
}
