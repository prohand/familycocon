// SPDX-License-Identifier: GPL-3.0-or-later

package courses

import (
	"context"
	"strconv"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
)

// chercher : recherche globale (produits à prendre, sur toutes les listes).
func (m *Module) chercher(ctx context.Context, c *appli.Contexte, q string) ([]appli.Resultat, error) {
	l, err := m.a.Q.ChercherProduits(ctx, donnees.ChercherProduitsParams{FamilleID: c.Famille.ID, Motif: q})
	if err != nil {
		return nil, err
	}
	var out []appli.Resultat
	for _, p := range l {
		detail := "Liste « " + p.Liste + " »"
		if p.Quantite != "" {
			detail = p.Quantite + " · " + detail
		}
		out = append(out, appli.Resultat{Emoji: p.Emoji, Titre: p.Nom, Detail: detail, URL: "/courses/liste/" + strconv.FormatInt(p.ListeID, 10)})
	}
	return out, nil
}
