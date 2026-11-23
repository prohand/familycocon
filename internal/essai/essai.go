// SPDX-License-Identifier: GPL-3.0-or-later

// Package essai : outils pour les tests des modules (serveur complet sur une
// base temporaire, « navigateur » avec ses cookies). N'est importé que par
// des fichiers _test.go : rien de tout cela n'entre dans le binaire.
package essai

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"familycocon/internal/appli"
	"familycocon/internal/base"
	"familycocon/internal/config"
)

// NouvelleApp : application complète (modules branchés par brancher) sur
// une base temporaire, et son serveur HTTP de test.
func NouvelleApp(t *testing.T, brancher func(a *appli.App)) (*appli.App, *httptest.Server) {
	t.Helper()
	cfg, err := config.LireDepuis(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	cfg.Donnees = t.TempDir()
	db, err := base.Ouvrir(filepath.Join(cfg.Donnees, base.NomFichier))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := base.Migrer(context.Background(), db, cfg.Donnees); err != nil {
		t.Fatal(err)
	}
	a, err := appli.Nouvelle(cfg, db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if brancher != nil {
		brancher(a)
	}
	if err := a.ChargerGabarits(); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(func() {
		a.Arreter()
		srv.Close()
	})
	return a, srv
}

// Navigateur : un client HTTP avec ses cookies, comme un vrai navigateur.
type Navigateur struct {
	t     *testing.T
	srv   *httptest.Server
	C     *http.Client
	Jeton string // jeton CSRF lu dans la dernière page
	Htmx  bool   // envoie les en-têtes de htmx (HX-Request, X-Jeton)
	// EnTetes : en-têtes de la dernière réponse (HX-Trigger…).
	EnTetes http.Header
	mu      sync.Mutex // un test peut envoyer plusieurs requêtes à la fois
}

// NouveauNavigateur : navigateur sans cookie.
func NouveauNavigateur(t *testing.T, srv *httptest.Server) *Navigateur {
	jar, _ := cookiejar.New(nil)
	return &Navigateur{t: t, srv: srv, C: &http.Client{Jar: jar}}
}

var reJeton = regexp.MustCompile(`name="_jeton" value="([^"]+)"|"X-Jeton": "([^"]+)"`)

func (n *Navigateur) lire(rep *http.Response, err error) (int, string) {
	n.t.Helper()
	if err != nil {
		n.t.Fatal(err)
	}
	defer rep.Body.Close()
	b, _ := io.ReadAll(rep.Body)
	n.mu.Lock()
	defer n.mu.Unlock()
	n.EnTetes = rep.Header
	if m := reJeton.FindSubmatch(b); m != nil {
		n.Jeton = string(m[1]) + string(m[2])
	}
	return rep.StatusCode, string(b)
}

// Get : page complète (ou fragment si Htmx).
func (n *Navigateur) Get(chemin string) (int, string) {
	n.t.Helper()
	req, _ := http.NewRequest("GET", n.srv.URL+chemin, nil)
	if n.Htmx {
		req.Header.Set("HX-Request", "true")
	}
	return n.lire(n.C.Do(req))
}

// Post envoie un formulaire, avec le jeton CSRF de la dernière page.
func (n *Navigateur) Post(chemin string, champs map[string]string) (int, string) {
	n.t.Helper()
	v := url.Values{}
	for k, x := range champs {
		v.Set(k, x)
	}
	return n.PostValeurs(chemin, v)
}

// PostValeurs : comme Post, avec plusieurs valeurs possibles par champ
// (cases à cocher du même nom).
func (n *Navigateur) PostValeurs(chemin string, v url.Values) (int, string) {
	n.t.Helper()
	req, _ := http.NewRequest("POST", n.srv.URL+chemin, strings.NewReader(v.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if n.Jeton != "" {
		req.Header.Set("X-Jeton", n.Jeton)
	}
	if n.Htmx {
		req.Header.Set("HX-Request", "true")
	}
	return n.lire(n.C.Do(req))
}

// Famille crée la famille « Martin » (Marie, parent) et renvoie son
// navigateur connecté.
func Famille(t *testing.T, srv *httptest.Server) *Navigateur {
	t.Helper()
	n := NouveauNavigateur(t, srv)
	code, page := n.Post("/inscription", map[string]string{
		"famille": "Famille Martin", "prenom": "Marie", "identifiant": "marie",
		"mdp": "un grand nid douillet", "mdp2": "un grand nid douillet",
	})
	if code != 200 || !strings.Contains(page, "Ma famille") {
		t.Fatalf("inscription : %d", code)
	}
	return n
}

// Enfant ajoute un enfant (code PIN 1234) et renvoie un navigateur connecté
// en son nom (sur le même appareil de confiance que le parent).
func Enfant(t *testing.T, srv *httptest.Server, parent *Navigateur, prenom string) *Navigateur {
	t.Helper()
	parent.Get("/famille")
	if code, _ := parent.Post("/famille/enfants", map[string]string{"prenom": prenom, "pin": "1234"}); code != 200 {
		t.Fatalf("ajout de l'enfant : %d", code)
	}
	// Même appareil : on recopie le cookie d'appareil de confiance.
	e := NouveauNavigateur(t, srv)
	u, _ := url.Parse(srv.URL)
	for _, ck := range parent.C.Jar.Cookies(u) {
		if strings.HasSuffix(ck.Name, "fc_appareil") {
			e.C.Jar.SetCookies(u, []*http.Cookie{ck})
		}
	}
	_, page := e.Get("/connexion")
	m := regexp.MustCompile(`/connexion/enfant/(\d+)">.{0,120}` + regexp.QuoteMeta(prenom)).FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("enfant %s absent de la page de connexion", prenom)
	}
	if code, page := e.Post("/connexion/enfant/"+m[1], map[string]string{"pin": "1234"}); code != 200 || !strings.Contains(page, "Bonjour "+prenom) {
		t.Fatalf("connexion de l'enfant : %d", code)
	}
	return e
}

// Trouver : premier groupe d'une expression régulière dans la page.
func Trouver(t *testing.T, page, expr string) string {
	t.Helper()
	m := regexp.MustCompile(expr).FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("%q introuvable dans la page", expr)
	}
	return m[1]
}
