// SPDX-License-Identifier: GPL-3.0-or-later

package fidelite

import (
	"context"
	"strconv"

	"familycocon/internal/appli"
	"familycocon/internal/texte"
)

// chercher : recherche globale (enseigne et titulaire des cartes).
func (m *Module) chercher(ctx context.Context, c *appli.Contexte, q string) ([]appli.Resultat, error) {
	l, err := m.a.Q.CartesFamille(ctx, c.Famille.ID)
	if err != nil {
		return nil, err
	}
	var out []appli.Resultat
	for _, x := range l {
		if texte.Contient(x.Enseigne+" "+x.Titulaire, q) {
			out = append(out, appli.Resultat{Emoji: appli.EmojiFidelite, Titre: x.Enseigne, Detail: x.Titulaire, URL: "/fidelite/" + strconv.FormatInt(x.ID, 10)})
		}
	}
	return out, nil
}
