// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

// fichierStatique : un fichier de web/statique, gardé en mémoire avec sa
// version compressée (gzip) et son empreinte (cache des navigateurs).
type fichierStatique struct {
	contenu []byte
	gz      []byte // nil si la compression ne fait pas gagner de place
	type_   string
	hash    string // 10 caractères : sert d'ETag et de numéro de version dans l'URL
}

// Statique : tous les fichiers statiques (quelques centaines de Ko).
type Statique struct {
	fichiers map[string]*fichierStatique
}

// compressibles : types texte, qui gagnent à être compressés.
var compressibles = map[string]bool{".css": true, ".js": true, ".svg": true, ".webmanifest": true, ".json": true, ".txt": true}

// versionSW : texte de sw.js remplacé par la version de l'ensemble des
// fichiers ; policeCSS : adresse de la police dans app.css, à versionner.
const (
	versionSW = "familycocon-1"
	policeCSS = "fonts/nunito.woff2"
)

func empreinteCourte(b []byte) string {
	somme := sha256.Sum256(b)
	return hex.EncodeToString(somme[:5])
}

// ChargerStatique lit et compresse les fichiers une seule fois, au démarrage.
//   - la police citée par app.css reçoit sa version (« ?v=… ») : elle est
//     gardée un an par le navigateur au lieu d'être revérifiée à chaque page ;
//   - le nom du cache du service worker suit la version de l'ensemble des
//     fichiers : à chaque mise à jour, sw.js change, le nouveau service
//     worker s'installe et supprime l'ancien cache (qui sinon grossit).
func ChargerStatique(racine fs.FS) (*Statique, error) {
	s := &Statique{fichiers: map[string]*fichierStatique{}}
	bruts := map[string][]byte{}
	var noms []string
	err := fs.WalkDir(racine, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		contenu, err := fs.ReadFile(racine, p)
		if err != nil {
			return err
		}
		bruts[p] = contenu
		noms = append(noms, p) // WalkDir : ordre alphabétique, toujours le même
		return nil
	})
	if err != nil {
		return nil, err
	}
	if police, ok := bruts[policeCSS]; ok {
		bruts["app.css"] = bytes.ReplaceAll(bruts["app.css"], []byte(`"`+policeCSS+`"`), []byte(`"`+policeCSS+"?v="+empreinteCourte(police)+`"`))
	}
	// Version de l'ensemble : empreinte de toutes les empreintes (sauf sw.js).
	ensemble := sha256.New()
	for _, p := range noms {
		if p != "sw.js" {
			fmt.Fprintf(ensemble, "%s %s\n", p, empreinteCourte(bruts[p]))
		}
	}
	if sw, ok := bruts["sw.js"]; ok {
		bruts["sw.js"] = bytes.ReplaceAll(sw, []byte(versionSW), []byte("familycocon-"+hex.EncodeToString(ensemble.Sum(nil)[:5])))
	}
	for _, p := range noms {
		contenu := bruts[p]
		ext := path.Ext(p)
		t := mime.TypeByExtension(ext)
		switch ext {
		case ".webmanifest":
			t = "application/manifest+json"
		case ".woff2":
			t = "font/woff2"
		case ".js":
			t = "text/javascript; charset=utf-8"
		}
		if t == "" {
			t = "application/octet-stream"
		}
		f := &fichierStatique{contenu: contenu, type_: t, hash: empreinteCourte(contenu)}
		if compressibles[ext] {
			var b bytes.Buffer
			zw, _ := gzip.NewWriterLevel(&b, gzip.BestCompression)
			_, _ = zw.Write(contenu)
			_ = zw.Close()
			if b.Len() < len(contenu) {
				f.gz = b.Bytes()
			}
		}
		s.fichiers[p] = f
	}
	return s, nil
}

// URL : adresse d'un fichier, avec sa version (« ?v=… ») : le navigateur le
// garde en cache un an, et le recharge dès que le fichier change.
func (s *Statique) URL(nom string) string {
	f, ok := s.fichiers[nom]
	if !ok {
		panic(fmt.Sprintf("fichier statique inconnu : %s", nom))
	}
	return "/statique/" + nom + "?v=" + f.hash
}

// Servir : sert le fichier nom (chemin relatif à web/statique).
// longCache : vrai pour /statique/… versionné ; faux pour /sw.js et le
// manifeste, qui doivent garder la même adresse et être revérifiés.
func (s *Statique) Servir(w http.ResponseWriter, r *http.Request, nom string, longCache bool) {
	f, ok := s.fichiers[nom]
	if !ok {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	etag := `"` + f.hash + `"`
	h.Set("ETag", etag)
	if longCache && r.URL.Query().Get("v") == f.hash {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "no-cache")
	}
	h.Set("Content-Type", f.type_)
	h.Add("Vary", "Accept-Encoding")
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	corps := f.contenu
	if f.gz != nil && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		h.Set("Content-Encoding", "gzip")
		corps = f.gz
	}
	h.Set("Content-Length", fmt.Sprint(len(corps)))
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(corps)
}
