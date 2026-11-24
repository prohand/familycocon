// SPDX-License-Identifier: GPL-3.0-or-later

package notes

import (
	"context"
	"database/sql"
	"strconv"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
)

// chercher : recherche globale (notes visibles par ce membre).
func (m *Module) chercher(ctx context.Context, c *appli.Contexte, q string) ([]appli.Resultat, error) {
	l, err := m.a.Q.NotesVisibles(ctx, donnees.NotesVisiblesParams{FamilleID: c.Famille.ID,
		MembreID: sql.NullInt64{Int64: c.Membre.ID, Valid: true}, Parent: estParent(c), Filtre: toutes, Categorie: int64(0), Motif: q, Nombre: 8})
	if err != nil {
		return nil, err
	}
	var out []appli.Resultat
	for _, n := range l {
		emoji := "📝"
		if n.Privee == 1 {
			emoji = "🔒"
		}
		out = append(out, appli.Resultat{Emoji: emoji, Titre: apercu(n.Titre, n.Debut), URL: "/notes/" + strconv.FormatInt(n.ID, 10)})
	}
	return out, nil
}
