package place

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Page routes of the redesign. The historical ones (/home, /ace, /timelapse, /login)
// stay in web/root/_filters.txt; these need path parameters or several segments,
// which httpfilter cannot express.
type pageRoute struct {
	path   string // exact path, or prefix when it ends with "/"
	file   string
	prefix bool
}

var pageRoutes = []pageRoute{}

// AddPage registers a static page for a path ("/musee") or a prefix ("/u/").
func AddPage(path, file string) {
	pageRoutes = append(pageRoutes, pageRoute{path, file, strings.HasSuffix(path, "/")})
}

func init() {
	AddPage("/u/", "profil.html")
	AddPage("/moi/profil", "profil-edition.html")
}

// PagesHandler serves the page registered for a request path.
type PagesHandler struct {
	Root string
}

func (p PagesHandler) match(path string) string {
	best := ""
	file := ""
	for _, r := range pageRoutes {
		if r.prefix && strings.HasPrefix(path, r.path) && len(r.path) > len(best) {
			best, file = r.path, r.file
		} else if !r.prefix && (path == r.path || path == r.path+"/") {
			return r.file
		}
	}
	return file
}

// Paths returns the mux patterns to register.
func (p PagesHandler) Paths() []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range pageRoutes {
		if !seen[r.path] {
			seen[r.path] = true
			out = append(out, r.path)
		}
	}
	return out
}

func (p PagesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	file := p.match(r.URL.Path)
	if file == "" {
		http.NotFound(w, r)
		return
	}
	b, err := os.ReadFile(filepath.Join(p.Root, file))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(b)
}
