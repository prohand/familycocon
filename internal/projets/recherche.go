// SPDX-License-Identifier: GPL-3.0-or-later

package projets

import (
	"context"
	"strconv"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
)

// chercher : recherche globale (nom et description des projets).
func (m *Module) chercher(ctx context.Context, c *appli.Contexte, q string) ([]appli.Resultat, error) {
	l, err := m.a.Q.ChercherProjets(ctx, donnees.ChercherProjetsParams{FamilleID: c.Famille.ID, Motif: q})
	if err != nil {
		return nil, err
	}
	var out []appli.Resultat
	for _, p := range l {
		r := appli.Resultat{Emoji: p.Emoji, Titre: p.Nom, URL: "/projets/" + strconv.FormatInt(p.ID, 10)}
		if p.TermineLe != 0 {
			r.Detail = "Terminé"
		}
		out = append(out, r)
	}
	return out, nil
}
