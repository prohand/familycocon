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
	// Temps réel (Server-Sent Events).
	Direct *Direct
	// Corbeille : suppressions qu'on peut encore annuler (quelques secondes).
	Corbeille *Corbeille
	// Petites données de quelques minutes (connexion en attente, défis).
	Memoire *MemoireCourte
	// MemoireDefis : défis de passkey demandés sans être connecté, à part
	// (une avalanche de demandes ne bloque pas les autres connexions).
	MemoireDefis *MemoireCourte
	// Notifications Web Push (nil tant que DemarrerPush n'a pas été appelé).
	Push *Pousseur

	// Fonctions de gabarit ajoutées par les modules avant Charger.
	fonctions  template.FuncMap
	resumes    map[string]ResumeFunc
	echeances  map[string]EcheancesFunc
	calendrier map[string]CalendrierFunc
	blocs      []BlocAccueil
	ajouts     []Ajout
	partages   []Partage
	recherches map[string]RechercheFunc
	departs    []DepartFunc
	minutes    []MinuteFunc // tâches de chaque minute (AjouterMinute)
}

// DepartFunc : ce qu'un module efface quand un membre quitte la famille
// (ce qui était privé à ce membre), dans la transaction du départ.
type DepartFunc func(ctx context.Context, q *donnees.Queries, famille, membre int64) error

// AjouterDepart : le module efface ce qui était privé au membre qui part.
func (a *App) AjouterDepart(f DepartFunc) { a.departs = append(a.departs, f) }

// Nouvelle prépare l'application (sans encore charger les gabarits : les
// modules peuvent d'abord ajouter leurs fonctions avec AjouterFonctions).
func Nouvelle(cfg *config.Config, db *sql.DB, journal *slog.Logger) (*App, error) {
	a := &App{
		Config:       cfg,
		DB:           db,
		Q:            donnees.New(db),
		Journal:      journal,
		Mux:          http.NewServeMux(),
		Demarre:      time.Now(),
		Limiteur:     NouveauLimiteur(),
		Direct:       nouveauDirect(),
		Corbeille:    nouvelleCorbeille(),
		Memoire:      NouvelleMemoireCourte(2000),
		MemoireDefis: NouvelleMemoireCourte(2000),
		resumes:      map[string]ResumeFunc{},
		recherches:   map[string]RechercheFunc{},
		echeances:    map[string]EcheancesFunc{},
		calendrier:   map[string]CalendrierFunc{},
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
	a.Route("GET /direct", "", Membre, a.direct)
	a.routesPush()
	a.Route("POST /annuler/{jeton}", "", Membre, a.annuler)
	a.AjouterBloc(BlocAccueil{Module: "", Ordre: 0, Rendre: a.blocAVenir})
	a.routesMFA()
	a.Mux.HandleFunc("GET /hors-ligne", func(w http.ResponseWriter, r *http.Request) {
		a.Statique.Servir(w, r, "hors-ligne.html", false)
	})
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
	a.Mux.HandleFunc("GET /ocr/{fichier...}", a.servirOCR)
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
	// iPhone : quand l'icône citée par la page ne vient pas, Safari essaie
	// ces adresses à la racine (sinon : une lettre sur fond gris).
	for _, nom := range []string{"/apple-touch-icon.png", "/apple-touch-icon-precomposed.png",
		"/apple-touch-icon-180x180.png", "/apple-touch-icon-180x180-precomposed.png"} {
		a.Mux.HandleFunc("GET "+nom, func(w http.ResponseWriter, r *http.Request) {
			a.Statique.Servir(w, r, "img/icone-180.png", false)
		})
	}
}

// Sante : réponse de /health (supervision : Uptime Kuma, Docker, Zabbix…).
type Sante struct {
	Etat     string `json:"etat"` // « ok » ou « erreur »
	Version  string `json:"version,omitempty"`
	Schema   int    `json:"schema,omitempty"`
	Depuis_s int64  `json:"depuis_s,omitempty"`
	Erreur   string `json:"erreur,omitempty"`
	// Dernière sauvegarde automatique (réseau local seulement) : date
	// (RFC 3339) et réussite.
	Sauvegarde   string `json:"sauvegarde,omitempty"`
	SauvegardeOK *bool  `json:"sauvegarde_ok,omitempty"`
}

func (a *App) sante(w http.ResponseWriter, r *http.Request) {
	ctx, annuler := context.WithTimeout(r.Context(), 3*time.Second)
	defer annuler()
	s := Sante{Etat: "ok"}
	v, err := base.VersionLue(ctx, a.DB)
	statut := http.StatusOK
	if err != nil {
		s.Etat, s.Erreur, statut = "erreur", "base inaccessible", http.StatusServiceUnavailable
		a.Journal.Error("santé : base inaccessible", "erreur", err)
	}
	// Version et durée : seulement pour la machine elle-même et le réseau
	// local (Docker, supervision). Un visiteur d'Internet ne voit que l'état.
	if ip := a.IPClient(r); ip.IsLoopback() || ip.IsPrivate() {
		s.Version, s.Schema, s.Depuis_s = version.Version, v, int64(time.Since(a.Demarre).Seconds())
		if e, ok := a.DerniereSauvegarde(ctx); ok {
			s.Sauvegarde, s.SauvegardeOK = time.Unix(e.Le, 0).UTC().Format(time.RFC3339), &e.OK
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(statut)
	_ = json.NewEncoder(w).Encode(s)
}
