package place

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	log "github.com/sirupsen/logrus"
)

// Choices offered by the profile editor (d-profil-edition).
var (
	AccentColors = []string{"#FF63AA", "#5EB3FF", "#3EE06C", "#FFD623", "#FFA800", "#6A5CFF", "#00CCC0", "#FF2651"}
	Banners      = []string{"desert", "nuit", "uni", "custom"}
)

const (
	maxBio         = 160
	maxPinned      = 3
	maxAvatarBytes = 2 << 20
	avatarSize     = 144
)

func (s *Store) UserBySlug(slug string) (*User, error) {
	return scanUser(s.db.QueryRow(`SELECT `+userCols+` FROM users u WHERE slug = ?`, strings.ToLower(slug)))
}

// ProfileUpdate holds the editable fields; nil pointers are left unchanged.
type ProfileUpdate struct {
	Pseudo        *string   `json:"pseudo"`
	Bio           *string   `json:"bio"`
	Banner        *string   `json:"banner"`
	Accent        *string   `json:"accent"`
	FavColor      *string   `json:"couleur_pref"`
	PinnedBadges  *[]string `json:"badges_epingles"`
	MapPublic     *bool     `json:"carte_publique"`
	RetouchAlerts *bool     `json:"alertes_retouche"`
}

// FieldError is a validation error shown next to a field.
type FieldError struct {
	Field string `json:"field"`
	Msg   string `json:"error"`
}

func (e *FieldError) Error() string { return e.Field + ": " + e.Msg }

func inList(v string, list []string) bool {
	for _, x := range list {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}

var paletteHex = []string{"#000000", "#333434", "#D4D7D9", "#FFFFFF", "#6D302F", "#6D001A", "#9C451A", "#BE0027", "#FF2651", "#FF2D00",
	"#FFA800", "#FFD623", "#FFF8B8", "#7EED38", "#00CC4E", "#00A344", "#598D5A", "#004B6F", "#009EAA", "#00CCC0",
	"#33E9F4", "#5EB3FF", "#245AEA", "#313AC1", "#1832A4", "#511E9F", "#6A5CFF", "#B44AC0", "#FF63AA", "#E4ABFF"}

// UpdateProfile validates and saves a profile edit. Changing the pseudo changes the slug.
func (s *Store) UpdateProfile(u *User, p ProfileUpdate) (*User, error) {
	sets := []string{}
	args := []any{}
	add := func(col string, v any) {
		sets = append(sets, col+" = ?")
		args = append(args, v)
	}
	if p.Pseudo != nil {
		pseudo, ok := cleanPseudo(*p.Pseudo)
		if !ok {
			return nil, &FieldError{"pseudo", "Entre 2 et 24 caractères."}
		}
		if pseudo != u.Pseudo {
			slug, err := s.freeSlug(Slugify(pseudo), u.ID)
			if err != nil {
				return nil, err
			}
			add("pseudo", pseudo)
			add("slug", slug)
		}
	}
	if p.Bio != nil {
		bio := strings.TrimSpace(strings.ReplaceAll(*p.Bio, "\r", ""))
		if utf8.RuneCountInString(bio) > maxBio {
			return nil, &FieldError{"bio", fmt.Sprintf("%d caractères au maximum.", maxBio)}
		}
		add("bio", bio)
	}
	if p.Banner != nil {
		if !inList(*p.Banner, Banners) {
			return nil, &FieldError{"banner", "Bannière inconnue."}
		}
		if strings.EqualFold(*p.Banner, "custom") && u.BannerURL == "" {
			return nil, &FieldError{"banner", "Dessine d'abord ta bannière."}
		}
		add("banner", strings.ToLower(*p.Banner))
	}
	if p.Accent != nil {
		if !inList(*p.Accent, AccentColors) {
			return nil, &FieldError{"accent", "Couleur d'accent inconnue."}
		}
		add("accent", strings.ToLower(*p.Accent))
	}
	if p.FavColor != nil {
		if *p.FavColor != "" && !inList(*p.FavColor, paletteHex) {
			return nil, &FieldError{"couleur_pref", "Choisis une couleur de la palette."}
		}
		add("couleur_pref", strings.ToLower(*p.FavColor))
	}
	if p.PinnedBadges != nil {
		pins := *p.PinnedBadges
		if len(pins) > maxPinned {
			return nil, &FieldError{"badges_epingles", "3 badges au maximum."}
		}
		owned, err := s.UserBadges(u.ID)
		if err != nil {
			return nil, err
		}
		seen := map[string]bool{}
		for _, b := range pins {
			if _, ok := owned[b]; !ok || seen[b] {
				return nil, &FieldError{"badges_epingles", "Tu ne peux épingler que tes badges."}
			}
			seen[b] = true
		}
		add("badges_epingles", strings.Join(pins, ","))
	}
	if p.MapPublic != nil {
		add("carte_publique", *p.MapPublic)
	}
	if p.RetouchAlerts != nil {
		add("alertes_retouche", *p.RetouchAlerts)
	}
	if len(sets) > 0 {
		args = append(args, u.ID)
		if _, err := s.db.Exec(`UPDATE users SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
			return nil, err
		}
	}
	return s.UserByID(u.ID)
}

// freeSlug returns base, or base-2, base-3… if another player already has it.
func (s *Store) freeSlug(base string, self uint32) (string, error) {
	for i := 1; i < 1000; i++ {
		slug := base
		if i > 1 {
			slug = fmt.Sprintf("%s-%d", base, i)
		}
		var id uint32
		err := s.db.QueryRow(`SELECT id FROM users WHERE slug = ?`, slug).Scan(&id)
		if err != nil || id == self {
			return slug, nil
		}
	}
	return "", errors.New("no free slug")
}

func (s *Store) SetAvatar(uid uint32, url string) error {
	_, err := s.db.Exec(`UPDATE users SET avatar_url = ? WHERE id = ?`, url, uid)
	return err
}

// ------------------------------------------------------------------
// Avatars

var errAvatar = errors.New("image illisible : PNG, JPEG ou GIF de 2 Mo au plus")

// processAvatar decodes an uploaded image, crops it to a centred square and scales it
// to avatarSize×avatarSize. Small images (pixel art) are scaled with nearest neighbour
// so they stay crisp; big ones are averaged.
func processAvatar(r io.Reader) ([]byte, error) {
	src, _, err := image.Decode(io.LimitReader(r, maxAvatarBytes+1))
	if err != nil {
		return nil, errAvatar
	}
	b := src.Bounds()
	side := b.Dx()
	if b.Dy() < side {
		side = b.Dy()
	}
	if side < 8 || b.Dx() > 8000 || b.Dy() > 8000 {
		return nil, errAvatar
	}
	x0 := b.Min.X + (b.Dx()-side)/2
	y0 := b.Min.Y + (b.Dy()-side)/2
	dst := image.NewNRGBA(image.Rect(0, 0, avatarSize, avatarSize))
	for y := 0; y < avatarSize; y++ {
		for x := 0; x < avatarSize; x++ {
			sx0, sx1 := x*side/avatarSize, (x+1)*side/avatarSize
			sy0, sy1 := y*side/avatarSize, (y+1)*side/avatarSize
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			if sy1 <= sy0 {
				sy1 = sy0 + 1
			}
			if side <= avatarSize {
				sx1, sy1 = sx0+1, sy0+1
			}
			var r, g, bl, a, n uint32
			for sy := sy0; sy < sy1; sy++ {
				for sx := sx0; sx < sx1; sx++ {
					c := color.NRGBAModel.Convert(src.At(x0+sx, y0+sy)).(color.NRGBA)
					r, g, bl, a, n = r+uint32(c.R), g+uint32(c.G), bl+uint32(c.B), a+uint32(c.A), n+1
				}
			}
			dst.SetNRGBA(x, y, color.NRGBA{uint8(r / n), uint8(g / n), uint8(bl / n), uint8(a / n)})
		}
	}
	return encodePNG(dst), nil
}

// ------------------------------------------------------------------
// Contribution map: the canvas darkened, the player's visible pixels in their colours.

func (c *Canvas) ContributionsPNG(uid uint32, accent color.NRGBA, claimed []int32) []byte {
	c.mu.RLock()
	w, h := c.img.Rect.Dx(), c.img.Rect.Dy()
	out := image.NewNRGBA(c.img.Rect)
	mine := make([]bool, len(c.owners))
	for i, id := range c.owners {
		mine[i] = id == uid
	}
	for _, p := range claimed {
		// still the artwork's: nobody has painted over it since the accounts
		if int(p) < len(mine) && c.owners[p] == 0 {
			mine[p] = true
		}
	}
	for i := range c.owners {
		p := c.img.Pix[i*4 : i*4+4]
		o := out.Pix[i*4 : i*4+4]
		if mine[i] {
			copy(o, p)
		} else {
			o[0], o[1], o[2] = uint8(uint16(p[0])*28/100), uint8(uint16(p[1])*28/100), uint8(uint16(p[2])*28/100)
		}
		o[3] = 255
	}
	c.mu.RUnlock()
	// A soft halo around isolated pixels so single dots stay visible.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if !mine[y*w+x] {
				continue
			}
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					nx, ny := x+dx, y+dy
					if nx < 0 || ny < 0 || nx >= w || ny >= h || mine[ny*w+nx] {
						continue
					}
					o := out.Pix[(ny*w+nx)*4:]
					o[0] = uint8((uint16(o[0])*55 + uint16(accent.R)*45) / 100)
					o[1] = uint8((uint16(o[1])*55 + uint16(accent.G)*45) / 100)
					o[2] = uint8((uint16(o[2])*55 + uint16(accent.B)*45) / 100)
				}
			}
		}
	}
	return encodePNG(out)
}

func parseHexColor(s string) (color.NRGBA, bool) {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return color.NRGBA{}, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.NRGBA{}, false
	}
	return rgbToNRGBA(uint32(v)), true
}

// ------------------------------------------------------------------
// API

type profileBadge struct {
	Badge
	Earned bool   `json:"earned"`
	Detail string `json:"detail"`
	TS     int64  `json:"ts,omitempty"`
}

type profileJSON struct {
	*PublicUser
	Bio          string         `json:"bio"`
	Banner       string         `json:"banner"`
	BannerURL    string         `json:"banner_url,omitempty"`
	FavColor     string         `json:"couleur_pref"`
	CreatedAt    int64          `json:"cree_le"`
	Placed       int64          `json:"pixels_poses"`
	Visible      int64          `json:"pixels_visibles"`
	Pioneer      int64          `json:"pixels_pionniers"`
	RankWeek     int            `json:"rang_semaine"`
	RankAll      int            `json:"rang_total"`
	Badges       []profileBadge `json:"badges"`
	Pinned       []string       `json:"badges_epingles"`
	MapPublic    bool           `json:"carte_publique"`
	IsMe         bool           `json:"moi"`
	RetouchAlert *bool          `json:"alertes_retouche,omitempty"`
	Role         string         `json:"role"`
	Extra        map[string]any `json:"extra,omitempty"`
}

// ProfileExtras lets later phases add sections (artworks, claims, museum) to a profile.
var ProfileExtras []func(api *API, u *User, viewer *User) (string, any)

func (api *API) buildProfile(u *User, viewer *User) (*profileJSON, error) {
	owned, err := api.store.UserBadges(u.ID)
	if err != nil {
		return nil, err
	}
	week, err := api.store.UserRank(u.ID, WeekStart(api.store.now()).UnixMilli())
	if err != nil {
		return nil, err
	}
	all, err := api.store.UserRankAllTime(u.ID)
	if err != nil {
		return nil, err
	}
	p := &profileJSON{
		PublicUser: u.Public(), Bio: u.Bio, Banner: u.Banner, BannerURL: u.BannerURL, FavColor: u.FavColor, CreatedAt: u.CreatedAt,
		Placed: u.PixelsPlaced, Visible: u.PixelsVisible, Pioneer: u.PixelsPioneer, RankWeek: week, RankAll: all,
		Pinned: u.PinnedBadges, MapPublic: u.MapPublic, IsMe: viewer != nil && viewer.ID == u.ID, Role: u.Role,
	}
	if p.Pinned == nil {
		p.Pinned = []string{}
	}
	if p.IsMe {
		p.RetouchAlert = &u.RetouchAlerts
	}
	for _, b := range Badges {
		pb := profileBadge{Badge: b}
		if ub, ok := owned[b.ID]; ok {
			pb.Earned, pb.Detail, pb.TS = true, ub.Detail, ub.TS
		}
		p.Badges = append(p.Badges, pb)
	}
	for _, f := range ProfileExtras {
		if key, v := f(api, u, viewer); key != "" {
			if p.Extra == nil {
				p.Extra = map[string]any{}
			}
			p.Extra[key] = v
		}
	}
	return p, nil
}

// /api/users/:slug and /api/users/:slug/contributions.png
func (api *API) handleUserProfile(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/users/")
	slug, sub, _ := strings.Cut(rest, "/")
	u, err := api.store.UserBySlug(slug)
	if err != nil {
		log.WithField("endpoint", "API").Error("Profile: ", err)
		writeError(w, http.StatusInternalServerError, "Profil indisponible.")
		return
	}
	if u == nil || u.Status == "banni" && api.auth.User(r) == nil {
		writeError(w, http.StatusNotFound, "Ce joueur n'existe pas.")
		return
	}
	viewer := api.auth.User(r)
	switch sub {
	case "":
		p, err := api.buildProfile(u, viewer)
		if err != nil {
			log.WithField("endpoint", "API").Error("Profile: ", err)
			writeError(w, http.StatusInternalServerError, "Profil indisponible.")
			return
		}
		writeJSON(w, p)
	case "contributions.png":
		if !u.MapPublic && (viewer == nil || viewer.ID != u.ID) {
			writeError(w, http.StatusForbidden, "Ce joueur a masqué sa carte.")
			return
		}
		accent, ok := parseHexColor(u.Accent)
		if !ok {
			accent = color.NRGBA{0x3e, 0xe0, 0x6c, 255}
		}
		b := api.contributions(u.ID, accent)
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Content-Length", strconv.Itoa(len(b)))
		w.Write(b)
	default:
		writeError(w, http.StatusNotFound, "Introuvable.")
	}
}

// Small cache: contribution maps are recomputed only when the canvas or the artworks changed.
var contribCache = struct {
	sync.Mutex
	version [2]uint64
	m       map[uint32][]byte
}{m: map[uint32][]byte{}}

// oeuvresVersion grows each time the artworks change (RefreshOeuvres).
var oeuvresVersion atomic.Uint64

func (api *API) contributions(uid uint32, accent color.NRGBA) []byte {
	api.canvas.mu.RLock()
	ver := [2]uint64{api.canvas.version, oeuvresVersion.Load()}
	api.canvas.mu.RUnlock()
	contribCache.Lock()
	if contribCache.version != ver {
		contribCache.version, contribCache.m = ver, map[uint32][]byte{}
	}
	b, ok := contribCache.m[uid]
	contribCache.Unlock()
	if ok {
		return b
	}
	// the pixels of the artworks claimed before the accounts count as theirs too
	var claimed []int32
	if os, err := api.store.Oeuvres(uid, false); err == nil {
		cw, ch := api.canvas.Size()
		for _, o := range os {
			claimed = append(claimed, maskPositions(o.Mask, cw, ch)...)
		}
	}
	b = api.canvas.ContributionsPNG(uid, accent, claimed)
	contribCache.Lock()
	if contribCache.version == ver && len(contribCache.m) < 64 {
		contribCache.m[uid] = b
	}
	contribCache.Unlock()
	return b
}

// PATCH /api/me/profile
func (api *API) handleMeProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch && r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
		return
	}
	u := api.auth.User(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "Connecte-toi pour modifier ton profil.")
		return
	}
	var p ProfileUpdate
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "Requête illisible.")
		return
	}
	updated, err := api.store.UpdateProfile(u, p)
	var fe *FieldError
	if errors.As(err, &fe) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(fe)
		return
	}
	if err != nil {
		log.WithField("endpoint", "API").Error("Profile update: ", err)
		writeError(w, http.StatusInternalServerError, "Enregistrement impossible.")
		return
	}
	api.onProfileChanged(updated)
	pj, err := api.buildProfile(updated, updated)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Profil indisponible.")
		return
	}
	writeJSON(w, pj)
}

// ProfileHooks are called after a profile or avatar change (moderation queue…).
var ProfileHooks []func(api *API, u *User, what string)

func (api *API) onProfileChanged(u *User) {
	for _, f := range ProfileHooks {
		f(api, u, "profile")
	}
}

var avatarNameRE = regexp.MustCompile(`^[0-9]+-[0-9a-f]{12}\.png$`)

const (
	maxBannerBytes = 64 << 10
	maxBannerW     = 128
	maxBannerH     = 64
)

// processBanner checks a drawn banner: a small PNG (at most 128×64), between 2:1 and
// 4:1. It is re-encoded, which also drops any metadata.
func processBanner(r io.Reader) ([]byte, error) {
	errBanner := errors.New("bannière illisible : un PNG de 128 × 64 pixels au plus")
	img, err := png.Decode(io.LimitReader(r, maxBannerBytes+1))
	if err != nil {
		return nil, errBanner
	}
	b := img.Bounds()
	if b.Dx() < 8 || b.Dy() < 4 || b.Dx() > maxBannerW || b.Dy() > maxBannerH || b.Dx() < 2*b.Dy() || b.Dx() > 4*b.Dy() {
		return nil, errBanner
	}
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			c.A = 255
			out.SetNRGBA(x, y, c)
		}
	}
	return encodePNG(out), nil
}

// POST /api/me/banner (multipart field "banner"): saves a drawn banner and selects it.
func (api *API) handleMeBanner(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
		return
	}
	u := api.auth.User(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "Connecte-toi pour changer de bannière.")
		return
	}
	if api.mediaDir == "" {
		writeError(w, http.StatusServiceUnavailable, "Les imports sont désactivés sur ce serveur.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBannerBytes+16<<10)
	file, _, err := r.FormFile("banner")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Bannière manquante.")
		return
	}
	defer file.Close()
	data, err := processBanner(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	url, err := api.saveMedia("banners", u.ID, data)
	if err != nil {
		log.WithField("endpoint", "API").Error("Banner: ", err)
		writeError(w, http.StatusInternalServerError, "Enregistrement impossible.")
		return
	}
	if _, err := api.store.db.Exec(`UPDATE users SET banniere_url = ?, banner = 'custom' WHERE id = ?`, url, u.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "Enregistrement impossible.")
		return
	}
	u.BannerURL, u.Banner = url, "custom"
	for _, f := range ProfileHooks {
		f(api, u, "banner")
	}
	writeJSON(w, map[string]string{"banner_url": url})
}

// saveMedia stores an uploaded PNG under media/<kind>/<uid>-<hash>.png and returns its URL.
func (api *API) saveMedia(kind string, uid uint32, data []byte) (string, error) {
	return api.saveMediaAs(kind, uid, data, "png")
}

// saveMediaAs stores an upload under media/<kind>/<uid>-<hash>.<ext>.
func (api *API) saveMediaAs(kind string, uid uint32, data []byte, ext string) (string, error) {
	sum := sha1.Sum(data)
	name := fmt.Sprintf("%d-%s.%s", uid, hex.EncodeToString(sum[:6]), ext)
	dir := filepath.Join(api.mediaDir, kind)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	if err := writeFileAtomic(filepath.Join(dir, name), data); err != nil {
		return "", err
	}
	return "/media/" + kind + "/" + name, nil
}

// POST /api/me/avatar (multipart field "avatar")
func (api *API) handleMeAvatar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Méthode non autorisée.")
		return
	}
	u := api.auth.User(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "Connecte-toi pour changer d'avatar.")
		return
	}
	if api.mediaDir == "" {
		writeError(w, http.StatusServiceUnavailable, "Les imports sont désactivés sur ce serveur.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAvatarBytes+64<<10)
	file, _, err := r.FormFile("avatar")
	if err != nil {
		writeError(w, http.StatusBadRequest, errAvatar.Error())
		return
	}
	defer file.Close()
	data, err := processAvatar(file)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	url, err := api.saveMedia("avatars", u.ID, data)
	if err != nil {
		log.WithField("endpoint", "API").Error("Avatar: ", err)
		writeError(w, http.StatusInternalServerError, "Enregistrement impossible.")
		return
	}
	if err := api.store.SetAvatar(u.ID, url); err != nil {
		writeError(w, http.StatusInternalServerError, "Enregistrement impossible.")
		return
	}
	u.AvatarURL = url
	for _, f := range ProfileHooks {
		f(api, u, "avatar")
	}
	writeJSON(w, map[string]string{"avatar": url})
}

// MediaHandler serves uploaded files (avatars…) without directory listings.
func MediaHandler(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.StripPrefix("/media/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := filepath.Clean("/" + r.URL.Path)
		st, err := os.Stat(filepath.Join(dir, clean))
		if err != nil || st.IsDir() {
			http.NotFound(w, r)
			return
		}
		if (strings.HasPrefix(clean, "/avatars/") || strings.HasPrefix(clean, "/banners/")) && !avatarNameRE.MatchString(filepath.Base(clean)) ||
			strings.HasPrefix(clean, "/sons/") && !sonNameRE.MatchString(filepath.Base(clean)) ||
			strings.HasPrefix(clean, "/fonds/") && !fondNameRE.MatchString(filepath.Base(clean)) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		fs.ServeHTTP(w, r)
	}))
}

// used by tests
func decodePNGSize(b []byte) (int, int, error) {
	cfg, err := png.DecodeConfig(bytes.NewReader(b))
	return cfg.Width, cfg.Height, err
}
