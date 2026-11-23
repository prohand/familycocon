// SPDX-License-Identifier: GPL-3.0-or-later

package agenda

import (
	"context"
	"strconv"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
)

// chercher : recherche globale (titre, lieu, note des événements).
func (m *Module) chercher(ctx context.Context, c *appli.Contexte, q string) ([]appli.Resultat, error) {
	l, err := m.a.Q.ChercherEvenements(ctx, donnees.ChercherEvenementsParams{FamilleID: c.Famille.ID, Motif: q})
	if err != nil {
		return nil, err
	}
	var out []appli.Resultat
	for _, e := range l {
		d := c.Heure(e.Debut)
		detail := appli.DateLongue(d) + " " + strconv.Itoa(d.Year())
		if e.Repetition != "" {
			detail += " · 🔁"
		}
		if e.Lieu != "" {
			detail += " · 📍 " + e.Lieu
		}
		out = append(out, appli.Resultat{Emoji: "🗓️", Titre: e.Titre, Detail: detail, URL: "/agenda/" + strconv.FormatInt(e.ID, 10)})
	}
	return out, nil
}
