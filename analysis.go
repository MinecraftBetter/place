package place

// Zone analysis from the backups (port of analyse_sauvegardes.py "zone" and "apercu"):
// when an artwork appeared, when it was finished, drawing sessions, pixels placed,
// retouches and restorations, stability over a year, proposed badges, mini-timelapse.

import (
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"sort"
	"time"
)

const (
	pauseSession    = 45 * 60 // a longer pause starts a new drawing session (seconds)
	thAppear        = 0.10    // 10 % of the reference state present: the artwork appears
	thFinished      = 0.90    // 90 %: finished
	thRetouched     = 0.90    // back under 90 % after being finished: retouched by someone
	thRestored      = 0.95    // back to 95 %: restored
	maxMaskPixels   = 400_000
	maxPolygonPoint = 4000
)

// Mask is a selected zone: a rectangle, a polygon (lasso) or a bitmap (magic wand).
type Mask struct {
	Type   string   `json:"type"`
	Rect   [4]int   `json:"rect,omitempty"`   // x, y, w, h
	Points [][2]int `json:"points,omitempty"` // polygon, canvas pixels
	X      int      `json:"x,omitempty"`      // bitmap bounding box
	Y      int      `json:"y,omitempty"`
	W      int      `json:"w,omitempty"`
	H      int      `json:"h,omitempty"`
	Bits   string   `json:"bits,omitempty"` // base64, 1 bit per pixel of the box, row-major, MSB first
}

var ErrBadMask = errors.New("zone invalide")

// Bounds returns the bounding box of the mask, clipped to the canvas.
func (m Mask) Bounds(cw, ch int) image.Rectangle {
	var r image.Rectangle
	switch m.Type {
	case "rect":
		r = image.Rect(m.Rect[0], m.Rect[1], m.Rect[0]+m.Rect[2], m.Rect[1]+m.Rect[3])
	case "poly":
		if len(m.Points) == 0 {
			return image.Rectangle{}
		}
		r = image.Rect(m.Points[0][0], m.Points[0][1], m.Points[0][0]+1, m.Points[0][1]+1)
		for _, p := range m.Points {
			r = r.Union(image.Rect(p[0], p[1], p[0]+1, p[1]+1))
		}
	case "bitmap":
		r = image.Rect(m.X, m.Y, m.X+m.W, m.Y+m.H)
	}
	return r.Intersect(image.Rect(0, 0, cw, ch))
}

// Positions returns the pixels of the mask (y*cw+x), sorted, or an error if invalid.
func (m Mask) Positions(cw, ch int) ([]int32, error) {
	b := m.Bounds(cw, ch)
	if b.Empty() || b.Dx()*b.Dy() > maxMaskPixels*4 {
		return nil, ErrBadMask
	}
	var out []int32
	switch m.Type {
	case "rect":
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				out = append(out, int32(y*cw+x))
			}
		}
	case "poly":
		if len(m.Points) < 3 || len(m.Points) > maxPolygonPoint {
			return nil, ErrBadMask
		}
		for y := b.Min.Y; y < b.Max.Y; y++ {
			cy := float64(y) + 0.5
			// crossings of the scanline with the polygon edges (even-odd rule)
			var xs []float64
			n := len(m.Points)
			for i := 0; i < n; i++ {
				a, c := m.Points[i], m.Points[(i+1)%n]
				ay, cyy := float64(a[1])+0.5, float64(c[1])+0.5
				if (ay <= cy) != (cyy <= cy) {
					ax, cx := float64(a[0])+0.5, float64(c[0])+0.5
					xs = append(xs, ax+(cy-ay)*(cx-ax)/(cyy-ay))
				}
			}
			sort.Float64s(xs)
			for i := 0; i+1 < len(xs); i += 2 {
				for x := b.Min.X; x < b.Max.X; x++ {
					if cx := float64(x) + 0.5; cx >= xs[i] && cx < xs[i+1] {
						out = append(out, int32(y*cw+x))
					}
				}
			}
		}
	case "bitmap":
		if m.W <= 0 || m.H <= 0 || m.W*m.H > maxMaskPixels*4 {
			return nil, ErrBadMask
		}
		bits, err := base64.StdEncoding.DecodeString(m.Bits)
		if err != nil || len(bits) < (m.W*m.H+7)/8 {
			return nil, ErrBadMask
		}
		for y := 0; y < m.H; y++ {
			for x := 0; x < m.W; x++ {
				i := y*m.W + x
				if bits[i/8]&(0x80>>(i%8)) != 0 {
					px, py := m.X+x, m.Y+y
					if px >= 0 && py >= 0 && px < cw && py < ch {
						out = append(out, int32(py*cw+px))
					}
				}
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	default:
		return nil, ErrBadMask
	}
	if len(out) == 0 || len(out) > maxMaskPixels {
		return nil, ErrBadMask
	}
	return out, nil
}

// MaskFromPositions encodes a set of pixels as a bitmap mask.
func MaskFromPositions(pos []int32, cw int) Mask {
	if len(pos) == 0 {
		return Mask{Type: "bitmap"}
	}
	minX, minY, maxX, maxY := cw, 1<<30, -1, -1
	for _, p := range pos {
		x, y := int(p)%cw, int(p)/cw
		minX, maxX = min(minX, x), max(maxX, x)
		minY, maxY = min(minY, y), max(maxY, y)
	}
	w, h := maxX-minX+1, maxY-minY+1
	bits := make([]byte, (w*h+7)/8)
	for _, p := range pos {
		i := (int(p)/cw-minY)*w + int(p)%cw - minX
		bits[i/8] |= 0x80 >> (i % 8)
	}
	return Mask{Type: "bitmap", X: minX, Y: minY, W: w, H: h, Bits: base64.StdEncoding.EncodeToString(bits)}
}

// Overlap counts the pixels two sorted position lists share.
func Overlap(a, b []int32) int {
	n, i, j := 0, 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			n++
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	return n
}

// ------------------------------------------------------------------

type Session struct {
	Start  int64 `json:"debut"` // ms
	End    int64 `json:"fin"`
	Minute int   `json:"duree_min"`
	Pixels int   `json:"pixels"`
}

type Retouch struct {
	At         int64   `json:"le"`
	MinPct     float64 `json:"min_pct"`
	RestoredAt int64   `json:"restauree_le,omitempty"`
	Hours      float64 `json:"duree_h,omitempty"`
}

type ZoneAnalysis struct {
	ZonePixels     int       `json:"pixels_zone"`
	ArtPixels      int       `json:"pixels_oeuvre"`
	Changes        int       `json:"changements"`
	AlreadyThere   bool      `json:"deja_la"` // present in the very first capture
	Appeared       int64     `json:"apparition,omitempty"`
	Finished       int64     `json:"terminee,omitempty"`
	LastChange     int64     `json:"dernier_changement,omitempty"`
	NowPct         float64   `json:"etat_actuel_pct"`
	Placed         int       `json:"pixels_poses_estimes"`
	Sessions       []Session `json:"sessions"`
	Days           []string  `json:"jours"` // Paris dates of drawing, "2006-01-02"
	PerHour        [24]int   `json:"pixels_par_heure"`
	Night          int       `json:"pixels_nuit"`
	Retouches      []Retouch `json:"retouches"`
	StabilityPct   float64   `json:"stabilite_12_mois_pct"`
	Badges         []string  `json:"badges"`
	Frames         int       `json:"sauvegardes"` // captures in the index
	FirstCapture   int64     `json:"premiere_sauvegarde"`
	LastCapture    int64     `json:"derniere_sauvegarde"`
	MinuteCaptures bool      `json:"minute_par_minute"`
	keyFrames      []int
}

// zoneReplay is the change history restricted to a zone.
type zoneReplay struct {
	pos     []int32  // zone pixels
	target  []uint32 // reference colours (latest capture)
	useful  []bool   // pixels of the artwork (not white holes)
	nUseful int
	evFrame []int32 // changes inside the zone
	evRank  []int32 // index in pos
	evCol   []uint32
	rate    []float64 // share of the artwork present after each change
	toward  []bool    // change that brings a pixel to its final colour
	rate0   float64
}

func replayZone(idx *BackupIndex, pos []int32) *zoneReplay {
	z := &zoneReplay{pos: pos, target: make([]uint32, len(pos)), useful: make([]bool, len(pos))}
	rank := make(map[int32]int32, len(pos))
	for i, p := range pos {
		rank[p] = int32(i)
		z.target[i] = idx.Last[p]
		if z.target[i] != white {
			z.useful[i] = true
			z.nUseful++
		}
	}
	n := max(z.nUseful, 1)
	state := make([]uint32, len(pos))
	ok := make([]bool, len(pos))
	nOK := 0
	for i, p := range pos {
		state[i] = idx.First[p]
		ok[i] = z.useful[i] && state[i] == z.target[i]
		if ok[i] {
			nOK++
		}
	}
	z.rate0 = float64(nOK) / float64(n)
	for e, p := range idx.EvPos {
		r, in := rank[p]
		if !in {
			continue
		}
		c := idx.EvCol[e]
		before := ok[r]
		after := z.useful[r] && c == z.target[r]
		if before != after {
			if after {
				nOK++
			} else {
				nOK--
			}
			ok[r] = after
		}
		state[r] = c
		z.evFrame = append(z.evFrame, idx.EvFrame[e])
		z.evRank = append(z.evRank, r)
		z.evCol = append(z.evCol, c)
		z.rate = append(z.rate, float64(nOK)/float64(n))
		z.toward = append(z.toward, after && !before)
	}
	return z
}

func round1(f float64) float64 { return float64(int(f*1000+0.5)) / 10 }

// AnalyzeZone analyses a zone of the latest capture. migration is when accounts opened
// (zero: unknown, every artwork counts as pioneer).
func AnalyzeZone(idx *BackupIndex, pos []int32, migration time.Time, now time.Time) *ZoneAnalysis {
	z := replayZone(idx, pos)
	ms := func(k int32) int64 { return idx.Frames[k].T * 1000 }
	res := &ZoneAnalysis{
		ZonePixels: len(pos), ArtPixels: z.nUseful, Changes: len(z.evFrame),
		Frames: len(idx.Frames), FirstCapture: idx.Frames[0].T * 1000, LastCapture: idx.Frames[len(idx.Frames)-1].T * 1000,
		Sessions: []Session{}, Days: []string{}, Retouches: []Retouch{}, Badges: []string{},
	}
	// Minute by minute when captures are close together (bak/), daily otherwise.
	if len(idx.Frames) > 1 {
		gaps := 0
		for i := 1; i < len(idx.Frames); i++ {
			if idx.Frames[i].T-idx.Frames[i-1].T <= 120 {
				gaps++
			}
		}
		res.MinuteCaptures = gaps*2 > len(idx.Frames)
	}
	n := len(z.evFrame)
	if n == 0 {
		res.NowPct = round1(z.rate0)
		if z.rate0 >= thAppear {
			res.AlreadyThere = true
			res.Appeared = res.FirstCapture
		}
		res.StabilityPct = res.NowPct
		return res
	}
	res.LastChange = ms(z.evFrame[n-1])
	firstPass := func(th float64, from int) int {
		for i := from; i < n; i++ {
			if z.rate[i] >= th {
				return i
			}
		}
		return -1
	}
	iApp := -1
	if z.rate0 >= thAppear {
		iApp = 0
		res.AlreadyThere = true
		res.Appeared = res.FirstCapture
	} else if iApp = firstPass(thAppear, 0); iApp >= 0 {
		res.Appeared = ms(z.evFrame[iApp])
	}
	iFin := -1
	if iApp >= 0 {
		if iFin = firstPass(thFinished, iApp); iFin >= 0 {
			res.Finished = ms(z.evFrame[iFin])
		}
	}
	res.NowPct = round1(z.rate[n-1])

	// Construction: changes toward the final state, until the end of the session in
	// which the artwork reached 90 % (finishing touches of that session count too).
	limit := z.evFrame[n-1]
	if iFin >= 0 {
		j := iFin
		for j+1 < n && idx.Frames[z.evFrame[j+1]].T-idx.Frames[z.evFrame[j]].T <= pauseSession {
			j++
		}
		limit = z.evFrame[j]
	}
	days := map[string]bool{}
	var constr []int64
	for i := 0; i < n; i++ {
		if z.toward[i] && z.evFrame[i] <= limit {
			constr = append(constr, idx.Frames[z.evFrame[i]].T)
		}
	}
	res.Placed = len(constr)
	for _, t := range constr {
		pt := time.Unix(t, 0).In(Paris)
		res.PerHour[pt.Hour()]++
		days[pt.Format("2006-01-02")] = true
		ms := t * 1000
		if l := len(res.Sessions); l > 0 && t*1000-res.Sessions[l-1].End <= pauseSession*1000 {
			res.Sessions[l-1].End = ms
			res.Sessions[l-1].Pixels++
		} else {
			res.Sessions = append(res.Sessions, Session{Start: ms, End: ms, Pixels: 1})
		}
	}
	for i := range res.Sessions {
		res.Sessions[i].Minute = int((res.Sessions[i].End - res.Sessions[i].Start) / 60000)
	}
	for d := range days {
		res.Days = append(res.Days, d)
	}
	sort.Strings(res.Days)
	res.Night = res.PerHour[2] + res.PerHour[3] + res.PerHour[4]

	// Retouches by others (jokes, additions…) and restorations after the artwork was finished.
	key := []int{}
	if iApp >= 0 {
		key = append(key, int(z.evFrame[iApp]))
	}
	if iFin >= 0 {
		inRetouch, start, worst, worstI := false, 0, 1.0, 0
		for i := iFin; i < n; i++ {
			switch {
			case !inRetouch && z.rate[i] < thRetouched:
				inRetouch, start, worst, worstI = true, i, z.rate[i], i
			case inRetouch:
				if z.rate[i] < worst {
					worst, worstI = z.rate[i], i
				}
				if z.rate[i] >= thRestored {
					res.Retouches = append(res.Retouches, Retouch{
						At: ms(z.evFrame[start]), MinPct: round1(worst), RestoredAt: ms(z.evFrame[i]),
						Hours: float64(int(float64(idx.Frames[z.evFrame[i]].T-idx.Frames[z.evFrame[start]].T)/360+0.5)) / 10,
					})
					key = append(key, int(z.evFrame[worstI]), int(z.evFrame[i]))
					inRetouch = false
				}
			}
		}
		if inRetouch {
			res.Retouches = append(res.Retouches, Retouch{At: ms(z.evFrame[start]), MinPct: round1(worst)})
			key = append(key, int(z.evFrame[worstI]))
		}
		key = append(key, int(z.evFrame[iFin]))
	}
	if half := firstPass(0.5, max(iApp, 0)); half >= 0 {
		key = append(key, int(z.evFrame[half]))
	}
	if iApp > 0 {
		key = append(key, int(z.evFrame[iApp])-1) // just before
	}
	key = append(key, len(idx.Frames)-1)
	res.keyFrames = key

	// Stability over the last year (minimum share of the artwork present).
	yearAgo := now.AddDate(-1, 0, 0).Unix()
	minYear := z.rate[n-1]
	for i := 0; i < n; i++ {
		if idx.Frames[z.evFrame[i]].T >= yearAgo && z.rate[i] < minYear {
			minYear = z.rate[i]
		}
	}
	res.StabilityPct = round1(minYear)

	// Proposed badges (HANDOFF §7.3).
	app := time.UnixMilli(res.Appeared)
	if res.Appeared != 0 && (migration.IsZero() || app.Before(migration)) {
		res.Badges = append(res.Badges, "pionnier")
	}
	if res.Appeared != 0 && app.In(Paris).Year() == 2022 {
		res.Badges = append(res.Badges, "veteran")
	}
	if res.Placed >= 1000 {
		res.Badges = append(res.Badges, "batisseur")
	}
	if res.Finished != 0 && time.UnixMilli(res.Finished).Unix() <= yearAgo && minYear >= 0.90 {
		res.Badges = append(res.Badges, "indemodable")
	}
	return res
}

// ZoneStateAt returns the colours of a zone at frame k (for before/after and timelapses).
func ZoneStateAt(idx *BackupIndex, pos []int32, k int) []uint32 {
	rank := make(map[int32]int, len(pos))
	state := make([]uint32, len(pos))
	for i, p := range pos {
		rank[p] = i
		state[i] = idx.First[p]
	}
	for e, p := range idx.EvPos {
		if int(idx.EvFrame[e]) > k {
			break
		}
		if r, ok := rank[p]; ok {
			state[r] = idx.EvCol[e]
		}
	}
	return state
}

// cropBox is the zone's bounding box with a margin, clipped to the canvas.
func cropBox(pos []int32, cw, ch int) image.Rectangle {
	minX, minY, maxX, maxY := cw, ch, 0, 0
	for _, p := range pos {
		x, y := int(p)%cw, int(p)/cw
		minX, maxX = min(minX, x), max(maxX, x)
		minY, maxY = min(minY, y), max(maxY, y)
	}
	m := max(2, min(8, max(maxX-minX, maxY-minY)/6))
	return image.Rect(minX-m, minY-m, maxX+m+1, maxY+m+1).Intersect(image.Rect(0, 0, cw, ch))
}

// ZoneStrip renders a mini-timelapse: the zone (with a margin) at several captures,
// side by side, pixels scaled up. Returns the PNG and the time of each frame (ms).
func ZoneStrip(idx *BackupIndex, pos []int32, frames []int, height int) ([]byte, []int64) {
	box := cropBox(pos, idx.W, idx.H)
	bw, bh := box.Dx(), box.Dy()
	scale := max(1, height/bh)
	if bw*scale > 320 {
		scale = max(1, 320/bw)
	}
	fw, fh := bw*scale, bh*scale
	const gap = 8
	frames = uniqueSorted(frames, len(idx.Frames))
	if len(frames) > 6 {
		// keep the first, the last and evenly spread ones in between
		keep := []int{frames[0]}
		for i := 1; i < 5; i++ {
			keep = append(keep, frames[i*(len(frames)-1)/5])
		}
		frames = uniqueSorted(append(keep, frames[len(frames)-1]), len(idx.Frames))
	}
	img := image.NewNRGBA(image.Rect(0, 0, len(frames)*(fw+gap)-gap, fh))
	times := make([]int64, len(frames))
	inZone := make(map[int32]bool, len(pos))
	for _, p := range pos {
		inZone[p] = true
	}
	// Replay the box once, capturing each requested frame.
	boxPos := make([]int32, 0, bw*bh)
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			boxPos = append(boxPos, int32(y*idx.W+x))
		}
	}
	rank := make(map[int32]int, len(boxPos))
	state := make([]uint32, len(boxPos))
	for i, p := range boxPos {
		rank[p] = i
		state[i] = idx.First[p]
	}
	e := 0
	for fi, k := range frames {
		for e < len(idx.EvPos) && int(idx.EvFrame[e]) <= k {
			if r, ok := rank[idx.EvPos[e]]; ok {
				state[r] = idx.EvCol[e]
			}
			e++
		}
		times[fi] = idx.Frames[k].T * 1000
		ox := fi * (fw + gap)
		for i, p := range boxPos {
			c := rgbToNRGBA(state[i])
			if !inZone[p] {
				c = color.NRGBA{uint8(uint16(c.R) * 45 / 100), uint8(uint16(c.G) * 45 / 100), uint8(uint16(c.B) * 45 / 100), 255}
			}
			x0, y0 := ox+(i%bw)*scale, (i/bw)*scale
			for dy := 0; dy < scale; dy++ {
				row := img.Pix[(y0+dy)*img.Stride:]
				for dx := 0; dx < scale; dx++ {
					o := (x0 + dx) * 4
					row[o], row[o+1], row[o+2], row[o+3] = c.R, c.G, c.B, c.A
				}
			}
		}
	}
	return encodePNG(img), times
}

// ZoneImage renders the zone (with its margin) at frame k, or from the live canvas when k < 0.
func ZoneImage(idx *BackupIndex, live *Canvas, pos []int32, k int, scale int) []byte {
	var cw, ch int
	if idx != nil && k >= 0 {
		cw, ch = idx.W, idx.H
	} else {
		cw, ch = live.Size()
	}
	box := cropBox(pos, cw, ch)
	img := image.NewNRGBA(image.Rect(0, 0, box.Dx()*scale, box.Dy()*scale))
	var state func(x, y int) color.NRGBA
	if idx != nil && k >= 0 {
		boxPos := make([]int32, 0, box.Dx()*box.Dy())
		for y := box.Min.Y; y < box.Max.Y; y++ {
			for x := box.Min.X; x < box.Max.X; x++ {
				boxPos = append(boxPos, int32(y*cw+x))
			}
		}
		st := ZoneStateAt(idx, boxPos, k)
		state = func(x, y int) color.NRGBA { return rgbToNRGBA(st[(y-box.Min.Y)*box.Dx()+x-box.Min.X]) }
	} else {
		state = func(x, y int) color.NRGBA { c, _, _ := live.At(x, y); return c }
	}
	for y := box.Min.Y; y < box.Max.Y; y++ {
		for x := box.Min.X; x < box.Max.X; x++ {
			c := state(x, y)
			for dy := 0; dy < scale; dy++ {
				row := img.Pix[((y-box.Min.Y)*scale+dy)*img.Stride:]
				for dx := 0; dx < scale; dx++ {
					o := ((x-box.Min.X)*scale + dx) * 4
					row[o], row[o+1], row[o+2], row[o+3] = c.R, c.G, c.B, 255
				}
			}
		}
	}
	return encodePNG(img)
}

func uniqueSorted(v []int, n int) []int {
	sort.Ints(v)
	out := v[:0]
	for i, x := range v {
		if x < 0 || x >= n || (i > 0 && x == v[i-1]) {
			continue
		}
		out = append(out, x)
	}
	return out
}

// KeyFrames returns the captures worth showing in a mini-timelapse.
func (a *ZoneAnalysis) KeyFrames() []int { return a.keyFrames }
