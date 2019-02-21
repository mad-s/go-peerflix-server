package main

import (
	"flag"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"encoding/base64"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
)

type indexData struct {
	FlashMessage string
	FlashType    string
	Torrents     []*torrent.Torrent
}

var templates = template.Must(template.New("").Funcs(template.FuncMap{
	"progress": func(t *torrent.Torrent) string {
		if t.Info() != nil {
			perc := 100 * float64(t.BytesCompleted()) / float64(t.Length())
			return strconv.FormatFloat(perc, 'f', 3, 64)
		}
		return "0"
	},
}).ParseGlob("site/*.html"))

func renderTemplate(w http.ResponseWriter, tmpl string, data interface{}) {
	err := templates.ExecuteTemplate(w, tmpl+".html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func getFlash(w http.ResponseWriter, r *http.Request) (flashType, flashMessage string, err error) {
	c, err := r.Cookie("flash")
	if err != nil {
		switch err {
		case http.ErrNoCookie:
			return "", "", nil
		default:
			return "", "", err
		}
	}
	http.SetCookie(w, &http.Cookie{
		Name:    "flash",
		Expires: time.Unix(1, 0),
		MaxAge:  -1,
	})
	parts := strings.Split(c.Value, ":")
	if len(parts) != 2 {
		return "", "", nil
	}
	if msg, err := base64.URLEncoding.DecodeString(parts[1]); err == nil {
		return parts[0], string(msg), nil
	}
	return "", "", nil
}

func setFlash(w http.ResponseWriter, flashType, flash string) {
	http.SetCookie(w, &http.Cookie{
		Name:   "flash",
		Value:  flashType + ":" + base64.URLEncoding.EncodeToString([]byte(flash)),
		MaxAge: 600,
	})
}

var client *torrent.Client

func addTorrent(t *torrent.Torrent) {
	<-t.GotInfo()

	// save torrent to file
	file, err := os.Create("torrent/" + t.InfoHash().HexString() + ".torrent")
	if err == nil {
		defer file.Close()
		t.Metainfo().Write(file)
	}

	t.DownloadAll()
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	if r.Method == "POST" {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if magnet := r.FormValue("magnet"); magnet != "" {
			if strings.HasPrefix(magnet, "magnet:") {
				t, err := client.AddMagnet(magnet)
				if err != nil {
					log.Println(err.Error())
					setFlash(w, "error", err.Error())
				} else {
					go addTorrent(t)
				}
			} else {
				setFlash(w, "error", "Not a magnet link!")
			}
		}
		http.Redirect(w, r, "", 303)
		return
	}

	flashType, flashMessage, err := getFlash(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	torrents := client.Torrents()
	sort.Slice(torrents, func(i, j int) bool {
		return strings.Compare(torrents[i].Name(), torrents[j].Name()) < 0
	})
	data := indexData{
		FlashType:    flashType,
		FlashMessage: flashMessage,
		Torrents:     torrents,
	}
	renderTemplate(w, "index", data)
}

func torrentHandler(w http.ResponseWriter, r *http.Request) {
	p := strings.Split(r.URL.Path[len("/torrent/"):], "/")
	log.Println(p)
	if len(p) == 0 {
		http.NotFound(w, r)
		return
	}
	path := strings.Join(p[1:], "/")

	var hash metainfo.Hash
	err := hash.FromHexString(p[0])
	if err != nil {
		http.NotFound(w, r)
		return
	}

	t, ok := client.Torrent(hash)
	if !ok {
		http.NotFound(w, r)
		return
	}

	var file *torrent.File
	for _, f := range t.Files() {
		if f.Path() == path {
			file = f
			break
		}
	}
	if file == nil {
		http.NotFound(w, r)
		return
	}

	reader := file.NewReader()
	reader.SetResponsive()

	defer func() {
		reader.Close()
	}()

	w.Header().Set("Content-Disposition", "attachment; filename=\""+file.Path()+"\"")
	http.ServeContent(w, r, file.DisplayPath(), time.Now(), reader)
}

func main() {
	listenAddr := flag.String("listen-address", "0.0.0.0:8080", "Address to listen on for HTTP requests")
	upload := flag.Bool("upload", false, "Whether or not to upload data")
	rootDir := flag.String("root-dir", ".", "Root directory of the application")
	storageDir := flag.String("storage-dir", "torrent", "Where to store existing torrents and downloaded data")

	flag.Parse()

	err := os.Chdir(*rootDir)
	if err != nil {
		log.Fatal(err)
	}

	stat, err := os.Stat(*storageDir)
	if err != nil {
		err := os.Mkdir(*storageDir, 0755)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		if !stat.IsDir() {
			log.Fatal(*storageDir + " needs to be a directory!")
		}
	}

	log.Println("Creating client")
	clientConfig := torrent.NewDefaultClientConfig()
	clientConfig.DefaultStorage = storage.NewFileByInfoHash(*storageDir)
	clientConfig.Seed = *upload
	clientConfig.NoUpload = !*upload
	c, err := torrent.NewClient(clientConfig)
	if err != nil {
		log.Fatal(err)
	}
	client = c
	oldTorrents, err := filepath.Glob(*storageDir + "/*.torrent")
	if err != nil {
		log.Fatal(err)
	}
	for _, f := range oldTorrents {
		t, err := c.AddTorrentFromFile(f)
		if err != nil {
			log.Println("Error loading torrent " + f)
		} else {
			go addTorrent(t)
		}
	}

	log.Println("Handling requests on " + *listenAddr)

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))))
	http.HandleFunc("/torrent/", torrentHandler)
	http.HandleFunc("/", indexHandler)
	log.Fatal(http.ListenAndServe(*listenAddr, nil))
}
