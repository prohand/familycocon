// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"context"
	"net/http"
	"time"

	"familycocon/internal/donnees"
)

// Calendrier : ce que les autres modules montrent dans l'agenda, en petit,
// sous les événements (les tâches ménagères du jour, par exemple). L'agenda
// ne connaît pas ces modules : chacun fournit ses éléments.

// ElementJour : une ligne discrète dans un jour de l'agenda.
type ElementJour struct {
	Jour   string // 2026-11-22
	Emoji  string
	Titre  string
	URL    string
	Membre *donnees.Membre // à qui c'est (nil : tout le monde)
	Faite  bool
	Module string
}

// CalendrierFunc : éléments d'un module, du jour debut (inclus) au jour fin (exclu).
type CalendrierFunc func(ctx context.Context, c *Contexte, debut, fin time.Time) ([]ElementJour, error)

// AjouterCalendrier : le module fournit des éléments à l'agenda.
func (a *App) AjouterCalendrier(module string, f CalendrierFunc) { a.calendrier[module] = f }

// ElementsCalendrier : les éléments des modules activés et visibles par ce membre.
func (a *App) ElementsCalendrier(ctx context.Context, c *Contexte, debut, fin time.Time) []ElementJour {
	var out []ElementJour
	for module, f := range a.calendrier {
		def, ok := ModuleParCle(module)
		if !ok || !c.Actif(module) || (def.Parents && !c.EstParent()) {
			continue
		}
		l, err := f(ctx, c, debut, fin)
		if err != nil {
			a.Journal.Error("calendrier", "module", module, "erreur", err)
			continue
		}
		out = append(out, l...)
	}
	// Les échéances des autres modules (contrôle technique, anniversaires,
	// étapes de projet, renouvellements…) y sont aussi, en petit. Celles de
	// l'agenda lui-même sont déjà ses événements.
	for _, e := range a.Echeances(ctx, c.Famille, debut, fin) {
		if e.Module == "agenda" || !e.PourMembre(c.Membre.ID, c.EstParent()) {
			continue
		}
		out = append(out, ElementJour{Jour: e.Jour.Format("2006-01-02"), Emoji: e.Emoji, Titre: e.Titre, URL: e.URL, Module: e.Module})
	}
	return out
}

// PoserPreference : un petit réglage d'affichage gardé par le navigateur
// (un an). Le nom est préfixé (fc_pref_).
func (a *App) PoserPreference(w http.ResponseWriter, nom, valeur string) {
	a.poserCookie(w, "fc_pref_"+nom, valeur, 365*24*time.Hour)
}

// LirePreference : le réglage, ou « » s'il n'existe pas.
func (a *App) LirePreference(r *http.Request, nom string) string {
	ck, err := r.Cookie(a.nomCookie("fc_pref_" + nom))
	if err != nil {
		return ""
	}
	return ck.Value
}
