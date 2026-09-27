package main

import (
	"crypto/tls"
	"errors"
	"flag"
	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/mattn/go-colorable"
	"github.com/sebest/xff"
	log "github.com/sirupsen/logrus"
	"image"
	"image/draw"
	"image/png"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/minecraftbetter/place"
	"github.com/rbxb/httpfilter"
)

var port string
var root string
var loadPath string
var savePath string
var ownersPath string
var dbPath string
var dataDir string
var width int
var height int
var count int
var saveInterval int
var cooldown time.Duration
var devAuth bool

func init() {
	flag.StringVar(&port, "port", ":8080", "The address and port the fileserver listens at.")
	flag.StringVar(&root, "root", "./web/root", "The directory serving files.")
	flag.StringVar(&loadPath, "load", "", "The png to load as the canvas.")
	flag.StringVar(&savePath, "save", "./place.png", "The path to save the canvas.")
	flag.StringVar(&ownersPath, "owners", "", "The path to save who placed each pixel. (default: owners.bin next to -save)")
	flag.StringVar(&dbPath, "db", "./betterplace.db", "The SQLite database (accounts, sessions, pixel journal).")
	flag.StringVar(&dataDir, "data", "./data", "The directory for uploaded files (avatars…), served at /media/.")
	flag.IntVar(&width, "width", 1024, "The width to create the canvas.")
	flag.IntVar(&height, "height", 1024, "The height to create the canvas.")
	flag.IntVar(&count, "count", 64, "The maximum number of connections.")
	flag.IntVar(&saveInterval, "saveInterval", 180, "Save interval in seconds.")
	flag.DurationVar(&cooldown, "cooldown", 5*time.Second, "Delay between two pixels of the same player.")
	flag.BoolVar(&devAuth, "devAuth", false, "Enable the \"dev\" login provider (fake accounts, no password). Never in production.")
}

func main() {
	flag.Parse()

	// Logging
	log.SetFormatter(&nested.Formatter{
		HideKeys: true,
	})
	log.SetLevel(log.DebugLevel)
	log.SetOutput(colorable.NewColorableStdout())

	if ownersPath == "" {
		ownersPath = filepath.Join(filepath.Dir(savePath), "owners.bin")
	}

	store, err := place.OpenStore(dbPath)
	if err != nil {
		log.Fatal("Opening the database: ", err)
	}
	if err := store.DeleteExpiredSessions(); err != nil {
		log.Warning("Cleaning sessions: ", err)
	}
	if n, err := store.BackfillBadges(); err != nil {
		log.Warning("Badges: ", err)
	} else if n > 0 {
		log.Info("Gave ", n, " badges earned before they existed")
	}

	// Load image
	var canvas *place.Canvas
	if loadPath != "" {
		if img := loadImage(loadPath); img != nil {
			canvas = place.NewCanvas(img)
		}
	}
	if canvas == nil {
		canvas = place.NewBlankCanvas(width, height)
	}
	replayed, err := place.RestoreOwners(canvas, store, ownersPath)
	if err != nil {
		log.Fatal("Restoring pixel owners: ", err)
	}
	if replayed > 0 {
		log.Info("Replayed ", replayed, " pixels placed after the last save")
	}

	var providers []place.Provider
	if devAuth {
		log.Warning("The \"dev\" login provider is enabled: anyone can log in as anyone.")
		providers = append(providers, place.DevProvider{})
	}
	auth := place.NewAuth(store, providers...)
	hub := place.NewHub(canvas, store, auth, count, cooldown)
	placeSv := place.NewServer(canvas, hub)

	// Save periodically and on shutdown
	save := func() {
		if err := canvas.SaveFiles(savePath, ownersPath); err != nil {
			log.Error("Saving the canvas: ", err)
		}
	}
	go func() {
		for {
			save()
			time.Sleep(time.Second * time.Duration(saveInterval))
		}
	}()
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		<-sig
		log.Info("Shutting down, saving the canvas")
		save()
		store.Close()
		os.Exit(0)
	}()

	fs := httpfilter.NewServer(root, "", map[string]httpfilter.OpFunc{
		"place": func(w http.ResponseWriter, req *http.Request, args ...string) {
			placeSv.ServeHTTP(w, req)
		},
	})
	api := place.NewAPI(canvas, store, auth, hub)
	mediaDir := filepath.Join(dataDir, "media")
	if err := os.MkdirAll(mediaDir, 0755); err != nil {
		log.Warning("Uploads disabled: ", err)
	} else {
		api.SetMediaDir(mediaDir)
	}

	// Weekly badges (Top 10): at startup, then every hour.
	go func() {
		for {
			if n, err := store.AwardWeeklyTop10(time.Now()); err != nil {
				log.Error("Top 10 badges: ", err)
			} else if n > 0 {
				log.Info("Top 10 badge given to ", n, " players")
			}
			time.Sleep(time.Hour)
		}
	}()

	mux := http.NewServeMux()
	mux.Handle("/api/", api)
	mux.Handle("/auth/", auth)
	mux.Handle("/media/", place.MediaHandler(mediaDir))
	pages := place.PagesHandler{Root: root}
	for _, p := range pages.Paths() {
		mux.Handle(p, pages)
	}
	mux.Handle("/", fs)

	xffmw, _ := xff.Default()
	server := http.Server{
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)), //disable HTTP/2
		Addr:         port,
		Handler:      xffmw.Handler(mux),
	}
	log.Info("Listening on ", port)
	log.Fatal(server.ListenAndServe())
}

// Loads an image
func loadImage(loadPath string) *image.NRGBA {
	f, err := os.Open(loadPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			log.Warning(err)
			return nil
		} else {
			panic(err)
		}
	}
	defer f.Close()

	pngImg, err := png.Decode(f)
	if err != nil {
		panic(err)
	}

	// We copy the PNG image into a Bitmap image, which allows us to remove the palette that causes colour problems
	b := pngImg.Bounds()
	m := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(m, m.Bounds(), pngImg, b.Min, draw.Src)
	return m
}
