// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"context"
	"net/http"
	"strings"
	"unicode/utf8"
)

// Recherche globale (/chercher) : un seul champ qui cherche dans tous les
// modules activés et visibles par ce membre. Chaque module fournit sa
// recherche (quelques résultats au plus).

// Resultat : une ligne de résultat.
type Resultat struct {
	Emoji  string
	Titre  string
	Detail string
	URL    string
}

// RechercheFunc : les résultats d'un module pour le texte q (déjà nettoyé).
type RechercheFunc func(ctx context.Context, c *Contexte, q string) ([]Resultat, error)

// AjouterRecherche : le module se rend cherchable.
func (a *App) AjouterRecherche(module string, f RechercheFunc) { a.recherches[module] = f }

// resultatsMax : résultats montrés par module.
const resultatsMax = 8

type groupeResultats struct {
	Module    ModuleDef
	Resultats []Resultat
}

type donneesChercher struct {
	Q       string
	Groupes []groupeResultats
}

func (a *App) pageChercher(w http.ResponseWriter, r *http.Request, c *Contexte) {
	d := donneesChercher{Q: strings.TrimSpace(r.URL.Query().Get("q"))}
	if utf8.RuneCountInString(d.Q) > 60 {
		d.Q = string([]rune(d.Q)[:60])
	}
	// Une seule lettre trouverait tout : deux au moins.
	if utf8.RuneCountInString(d.Q) >= 2 {
		for _, m := range ModulesVisibles(c) { // dans l'ordre des tuiles
			f, ok := a.recherches[m.Cle]
			if !ok {
				continue
			}
			l, err := f(r.Context(), c, d.Q)
			if err != nil {
				a.Journal.Error("recherche", "module", m.Cle, "erreur", err)
				continue
			}
			if len(l) > resultatsMax {
				l = l[:resultatsMax]
			}
			if len(l) > 0 {
				d.Groupes = append(d.Groupes, groupeResultats{Module: m, Resultats: l})
			}
		}
	}
	a.Afficher(w, r, c, "chercher", "Chercher", "", d)
}
