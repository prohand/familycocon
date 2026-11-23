// SPDX-License-Identifier: GPL-3.0-or-later

package menus

import (
	"context"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
)

// chercher : recherche globale (repas prévus, par nom ou ingrédient).
func (m *Module) chercher(ctx context.Context, c *appli.Contexte, q string) ([]appli.Resultat, error) {
	l, err := m.a.Q.ChercherRepas(ctx, donnees.ChercherRepasParams{FamilleID: c.Famille.ID, Motif: q})
	if err != nil {
		return nil, err
	}
	var out []appli.Resultat
	for _, r := range l {
		detail := r.Jour
		if j, err := time.Parse("2006-01-02", r.Jour); err == nil {
			detail = appli.DateLongue(j) + " " + j.Format("2006")
		}
		if r.Moment == "midi" {
			detail += ", midi"
		} else {
			detail += ", soir"
		}
		out = append(out, appli.Resultat{Emoji: "🍽️", Titre: r.Nom, Detail: detail, URL: "/menus?date=" + r.Jour})
	}
	return out, nil
}
