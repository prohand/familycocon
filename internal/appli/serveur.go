// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"context"
	"encoding/json"
	"time"

	"familycocon/internal/config"
	"familycocon/internal/donnees"
	"familycocon/internal/version"
)

// État du serveur pour les parents (page Ma famille) et la supervision
// (/health) : la dernière sauvegarde automatique, la taille de la base.
// Une sauvegarde ratée n'était écrite qu'au journal : personne ne la voyait.

// cleSauvegarde : ligne de la table serveur qui garde la dernière sauvegarde.
const cleSauvegarde = "sauvegarde"

// EtatSauvegarde : la dernière sauvegarde automatique.
type EtatSauvegarde struct {
	Le     int64  `json:"le"`               // date (Unix)
	OK     bool   `json:"ok"`               // réussie ?
	Taille int64  `json:"taille,omitempty"` // octets
	Erreur string `json:"erreur,omitempty"` // message court, sans chemin
}

// noterSauvegarde : retient le résultat de la sauvegarde automatique.
func (a *App) noterSauvegarde(ctx context.Context, e EtatSauvegarde) {
	b, _ := json.Marshal(e)
	if err := a.Q.NoterServeur(ctx, donnees.NoterServeurParams{Nom: cleSauvegarde, Valeur: b}); err != nil {
		a.Journal.Warn("sauvegarde : résultat non noté", "erreur", err)
	}
}

// DerniereSauvegarde : la dernière sauvegarde automatique (ok = faux s'il
// n'y en a jamais eu).
func (a *App) DerniereSauvegarde(ctx context.Context) (EtatSauvegarde, bool) {
	var e EtatSauvegarde
	b, err := a.Q.LireServeur(ctx, cleSauvegarde)
	if err != nil || json.Unmarshal(b, &e) != nil {
		return e, false
	}
	return e, true
}

// tailleBase : la taille de la base, en octets.
func (a *App) tailleBase(ctx context.Context) int64 {
	var pages, taille int64
	if err := a.DB.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pages); err != nil {
		return 0
	}
	if err := a.DB.QueryRowContext(ctx, "PRAGMA page_size").Scan(&taille); err != nil {
		return 0
	}
	return pages * taille
}

// carteServeur : ce que la carte « Serveur » montre aux parents.
type carteServeur struct {
	Version      string
	Base         string // « 2,4 Mo »
	Connexions   int
	Sauvegarde   string // « 23 novembre à 03:30 », vide : jamais
	SauvegardeOK bool
	Erreur       string
	SansAuto     bool // FC_SAUVEGARDE_HEURE=non
}

// etatServeur : la carte n'existe que sur un serveur d'une seule famille
// (FC_INSCRIPTIONS=premiere) : sur un serveur partagé, ces informations
// regardent l'administrateur, pas chaque famille.
func (a *App) etatServeur(ctx context.Context, c *Contexte) *carteServeur {
	if a.Config.Inscriptions != config.InscriptionPremiere {
		return nil
	}
	cs := &carteServeur{Version: version.Version, Base: Octets(a.tailleBase(ctx)), Connexions: a.Direct.Nombre(), SansAuto: a.Config.SauvegardeA == ""}
	if e, ok := a.DerniereSauvegarde(ctx); ok {
		t := time.Unix(e.Le, 0).In(c.Lieu)
		cs.Sauvegarde = DateLongue(t) + " à " + t.Format("15:04")
		cs.SauvegardeOK, cs.Erreur = e.OK, e.Erreur
	}
	return cs
}

// Octets : « 850 o », « 12 Ko », « 2,4 Mo ».
func Octets(n int64) string {
	switch {
	case n < 1024:
		return Milliers(n) + " o"
	case n < 1024*1024:
		return Milliers(n/1024) + " Ko"
	}
	dixiemes := (n*10 + 512*1024) / (1024 * 1024) // arrondi au dixième
	s := Milliers(dixiemes/10) + "," + string(rune('0'+dixiemes%10)) + " Mo"
	return s
}
