// SPDX-License-Identifier: GPL-3.0-or-later

package foyer

import (
	"context"
	"strconv"

	"familycocon/internal/appli"
	txt "familycocon/internal/texte"
)

// chercherVehicules : recherche globale (nom, modèle, immatriculation).
func (m *Module) chercherVehicules(ctx context.Context, c *appli.Contexte, q string) ([]appli.Resultat, error) {
	l, err := m.a.Q.VehiculesFamille(ctx, c.Famille.ID)
	if err != nil {
		return nil, err
	}
	var out []appli.Resultat
	for _, x := range l {
		if txt.Contient(x.Nom+" "+x.Modele+" "+x.Immatriculation, q) {
			out = append(out, appli.Resultat{Emoji: "🚗", Titre: x.Nom, Detail: x.Modele, URL: "/vehicules/" + strconv.FormatInt(x.ID, 10)})
		}
	}
	return out, nil
}

// chercherAnimaux : recherche globale (nom, espèce, race).
func (m *Module) chercherAnimaux(ctx context.Context, c *appli.Contexte, q string) ([]appli.Resultat, error) {
	l, err := m.a.Q.AnimauxFamille(ctx, c.Famille.ID)
	if err != nil {
		return nil, err
	}
	var out []appli.Resultat
	for _, x := range l {
		if txt.Contient(x.Nom+" "+x.Espece+" "+x.Race, q) {
			out = append(out, appli.Resultat{Emoji: "🐾", Titre: x.Nom, Detail: x.Race, URL: "/animaux/" + strconv.FormatInt(x.ID, 10)})
		}
	}
	return out, nil
}
