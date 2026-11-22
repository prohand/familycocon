// SPDX-License-Identifier: GPL-3.0-or-later

// Package appli : le cœur du serveur web (configuration, base, gabarits,
// fichiers statiques, intergiciels, sessions) partagé par tous les modules
// (courses, agenda…). Chaque module a son propre paquet, qui ajoute ses
// routes avec App.Route.
package appli

import (
	"context"
	"database/sql"
	"encoding/json"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"familycocon/internal/base"
	"familycocon/internal/config"
	"familycocon/internal/donnees"
	"familycocon/internal/version"
	"familycocon/web"
)

// App : tout ce dont un gestionnaire de requête a besoin.
type App struct {
	Config   *config.Config
	DB       *sql.DB
	Q        *donnees.Queries
	Gabarits *Gabarits
	Statique *Statique
	Journal  *slog.Logger
	Mux      *http.ServeMux
	Demarre  time.Time

	// Limite des tentatives de connexion.
	Limiteur *Limiteur

	// Fonctions de gabarit ajoutées par les modules avant Charger.
	fonctions template.FuncMap
	resumes   map[string]ResumeFunc
	blocs     []BlocAccueil
	ajouts    []Ajout
}

// Nouvelle prépare l'application (sans encore charger les gabarits : les
// modules peuvent d'abord ajouter leurs fonctions avec AjouterFonctions).
func Nouvelle(cfg *config.Config, db *sql.DB, journal *slog.Logger) (*App, error) {
	a := &App{
		Config:   cfg,
		DB:       db,
		Q:        donnees.New(db),
		Journal:  journal,
		Mux:      http.NewServeMux(),
		Demarre:  time.Now(),
		Limiteur: NouveauLimiteur(),
		resumes:  map[string]ResumeFunc{},
	}
	sous, err := fs.Sub(web.Fichiers, "statique")
	if err != nil {
		return nil, err
	}
	if a.Statique, err = ChargerStatique(sous); err != nil {
		return nil, err
	}
	a.fonctions = fonctionsDeBase(a)
	a.routesDeBase()
	a.routesComptes()
	a.routesAccueil()
	a.routesFamille()
	a.routesMoi()
	return a, nil
}

// AjouterFonctions : fonctions supplémentaires pour les gabarits.
func (a *App) AjouterFonctions(f template.FuncMap) {
	for k, v := range f {
		a.fonctions[k] = v
	}
}

// ChargerGabarits analyse les gabarits (à appeler une fois les modules prêts).
func (a *App) ChargerGabarits() error {
	sous, err := fs.Sub(web.Fichiers, "gabarits")
	if err != nil {
		return err
	}
	a.Gabarits, err = ChargerGabarits(sous, a.fonctions)
	return err
}

// Handler : le serveur complet, intergiciels compris.
func (a *App) Handler() http.Handler { return a.Envelopper(a.Mux) }

func (a *App) routesDeBase() {
	a.Mux.HandleFunc("GET /health", a.sante)
	a.Mux.HandleFunc("GET /statique/{fichier...}", func(w http.ResponseWriter, r *http.Request) {
		a.Statique.Servir(w, r, r.PathValue("fichier"), true)
	})
	// Le service worker et le manifeste gardent une adresse fixe, à la
	// racine (le service worker ne contrôle que les pages sous son dossier).
	a.Mux.HandleFunc("GET /sw.js", func(w http.ResponseWriter, r *http.Request) {
		a.Statique.Servir(w, r, "sw.js", false)
	})
	a.Mux.HandleFunc("GET /manifest.webmanifest", func(w http.ResponseWriter, r *http.Request) {
		a.Statique.Servir(w, r, "manifest.webmanifest", false)
	})
	a.Mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		a.Statique.Servir(w, r, "img/logo.svg", false)
	})
}

// Sante : réponse de /health (supervision : Uptime Kuma, Docker, Zabbix…).
type Sante struct {
	Etat     string `json:"etat"` // « ok » ou « erreur »
	Version  string `json:"version"`
	Schema   int    `json:"schema"`
	Depuis_s int64  `json:"depuis_s"`
	Erreur   string `json:"erreur,omitempty"`
}

func (a *App) sante(w http.ResponseWriter, r *http.Request) {
	ctx, annuler := context.WithTimeout(r.Context(), 3*time.Second)
	defer annuler()
	s := Sante{Etat: "ok", Version: version.Version, Depuis_s: int64(time.Since(a.Demarre).Seconds())}
	v, err := base.Version(ctx, a.DB)
	s.Schema = v
	statut := http.StatusOK
	if err != nil {
		s.Etat, s.Erreur, statut = "erreur", "base inaccessible", http.StatusServiceUnavailable
		a.Journal.Error("santé : base inaccessible", "erreur", err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(statut)
	_ = json.NewEncoder(w).Encode(s)
}
