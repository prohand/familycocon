// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"familycocon/internal/base"
	"familycocon/internal/config"
)

// nouvelleAppTest : application complète sur une base temporaire.
func nouvelleAppTest(t *testing.T, env map[string]string) *App {
	t.Helper()
	cfg, err := config.LireDepuis(func(k string) string { return env[k] })
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
	a, err := Nouvelle(cfg, db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ChargerGabarits(); err != nil {
		t.Fatal(err)
	}
	return a
}

func TestSante(t *testing.T) {
	a := nouvelleAppTest(t, nil)
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/health", nil))
	if rec.Code != 200 {
		t.Fatalf("statut %d", rec.Code)
	}
	var s Sante
	if err := json.NewDecoder(rec.Body).Decode(&s); err != nil || s.Etat != "ok" || s.Schema != base.VersionAttendue() {
		t.Fatalf("réponse %+v (%v)", s, err)
	}
	if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "script-src 'self'") {
		t.Fatal("CSP absente")
	}
	// Base fermée : 503.
	a.DB.Close()
	rec = httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/health", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("statut %d, 503 attendu", rec.Code)
	}
}

func TestStatique(t *testing.T) {
	a := nouvelleAppTest(t, nil)
	url := a.Statique.URL("app.css")
	req := httptest.NewRequest("GET", url, nil)
	req.Header.Set("Accept-Encoding", "gzip, br")
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("statut %d, encodage %q", rec.Code, rec.Header().Get("Content-Encoding"))
	}
	if !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Fatal("cache long attendu pour une URL versionnée")
	}
	// Revalidation : 304.
	req = httptest.NewRequest("GET", url, nil)
	req.Header.Set("If-None-Match", rec.Header().Get("ETag"))
	rec = httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("statut %d, 304 attendu", rec.Code)
	}
	// Fichier inconnu, remontée de dossier : 404.
	for _, p := range []string{"/statique/inconnu.css", "/statique/../base.html"} {
		rec = httptest.NewRecorder()
		a.Handler().ServeHTTP(rec, httptest.NewRequest("GET", p, nil))
		if rec.Code != 404 && rec.Code != 301 && rec.Code != 307 {
			t.Fatalf("%s : statut %d", p, rec.Code)
		}
	}
}

func TestRequeteAutreSiteRefusee(t *testing.T) {
	a := nouvelleAppTest(t, nil)
	a.Mux.HandleFunc("POST /essai", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	req := httptest.NewRequest("POST", "/essai", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("statut %d, 403 attendu", rec.Code)
	}
	req = httptest.NewRequest("POST", "/essai", nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec = httptest.NewRecorder()
	a.Handler().ServeHTTP(rec, req)
	if rec.Code != 204 {
		t.Fatalf("statut %d, 204 attendu", rec.Code)
	}
}

func TestIPClient(t *testing.T) {
	a := nouvelleAppTest(t, map[string]string{"FC_PROXYS": "172.18.0.0/16"})
	cas := []struct{ distante, xff, attendue string }{
		{"203.0.113.5:1234", "1.2.3.4", "203.0.113.5"},                  // pas un proxy : XFF ignoré
		{"172.18.0.2:1234", "198.51.100.7", "198.51.100.7"},             // proxy de confiance
		{"172.18.0.2:1234", "6.6.6.6, 198.51.100.7", "198.51.100.7"},    // la gauche est ignorée
		{"172.18.0.2:1234", "198.51.100.7, 172.18.0.3", "198.51.100.7"}, // deux proxys
	}
	for _, c := range cas {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.distante
		r.Header.Set("X-Forwarded-For", c.xff)
		if ip := a.IPClient(r).String(); ip != c.attendue {
			t.Errorf("%s / %s : %s, attendu %s", c.distante, c.xff, ip, c.attendue)
		}
	}
}

func TestMontant(t *testing.T) {
	cas := map[int64]string{0: "0,00 €", 123456: "1 234,56 €", -5: "−0,05 €", 100000000: "1 000 000,00 €"}
	for c, attendu := range cas {
		if m := Montant(c, "EUR"); m != attendu {
			t.Errorf("%d : %q, attendu %q", c, m, attendu)
		}
	}
}

func TestNotifierASCII(t *testing.T) {
	rec := httptest.NewRecorder()
	Notifier(rec, "Bravo Léa ! 🎉", false)
	h := rec.Header().Get("HX-Trigger")
	for _, r := range h {
		if r > 127 {
			t.Fatalf("caractère non ASCII dans l'en-tête : %q", h)
		}
	}
	var v map[string]map[string]any
	if err := json.Unmarshal([]byte(h), &v); err != nil || v["fc-toast"]["texte"] != "Bravo Léa ! 🎉" {
		t.Fatalf("relu : %v (%v)", v, err)
	}
}
