// SPDX-License-Identifier: GPL-3.0-or-later

package taches

import (
	"context"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
)

// chercher : recherche globale (tâches en cours).
func (m *Module) chercher(ctx context.Context, c *appli.Contexte, q string) ([]appli.Resultat, error) {
	l, err := m.a.Q.ChercherTaches(ctx, donnees.ChercherTachesParams{FamilleID: c.Famille.ID, Motif: q})
	if err != nil {
		return nil, err
	}
	var out []appli.Resultat
	for _, t := range l {
		out = append(out, appli.Resultat{Emoji: t.Emoji, Titre: t.Titre, URL: "/taches"})
	}
	return out, nil
}
