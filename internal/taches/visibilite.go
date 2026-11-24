// SPDX-License-Identifier: GPL-3.0-or-later

package taches

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
)

// Qui voit une tâche :
//   - privée : celui qui l'a créée, seul (elle lui revient) ;
//   - « parents seulement » (la tâche, ou sa catégorie) : les parents, celui
//     qui l'a créée et ceux qui doivent la faire (membre choisi, tour) ;
//   - sinon : toute la famille.
//
// La même règle est écrite en SQL (requetes/taches.sql : TachesActives…).

// cachee : la tâche est réservée aux parents (elle-même ou sa catégorie).
func (m *Module) cachee(ctx context.Context, t donnees.Tache) bool {
	return t.Parents == 1 || m.categorieParents(ctx, t.CategorieID)
}

// concernes : celui qui l'a créée et ceux qui doivent la faire.
func concernes(t donnees.Tache) []int64 {
	var out []int64
	if t.CreePar.Valid {
		out = append(out, t.CreePar.Int64)
	}
	if t.AssigneA.Valid {
		out = append(out, t.AssigneA.Int64)
	}
	for _, x := range strings.Split(t.Tour, ",") {
		if id, err := strconv.ParseInt(x, 10, 64); err == nil {
			out = append(out, id)
		}
	}
	return out
}

// visible : ce membre voit-il la tâche ?
func (m *Module) visible(ctx context.Context, c *appli.Contexte, t donnees.Tache) bool {
	if t.CreePar.Valid && t.CreePar.Int64 == c.Membre.ID {
		return true
	}
	if t.Privee == 1 {
		return false
	}
	return c.EstParent() || slices.Contains(concernes(t), c.Membre.ID) || !m.cachee(ctx, t)
}

// vueParEnfant : au moins un enfant voit la tâche (filtre « 🧒 Vues par
// les enfants »). cachee : réservée aux parents.
func vueParEnfant(t donnees.Tache, cachee bool, parID map[int64]*donnees.Membre) bool {
	if t.Privee == 1 {
		// Seul son créateur la voit : un enfant, si c'est lui.
		return t.CreePar.Valid && parID[t.CreePar.Int64] != nil && parID[t.CreePar.Int64].Role == appli.RoleEnfant
	}
	if !cachee {
		return true
	}
	for _, id := range concernes(t) {
		if mb := parID[id]; mb != nil && mb.Role == appli.RoleEnfant {
			return true
		}
	}
	return false
}

// annoncer : une nouvelle (« Léa a fait… ») à ceux qui voient la tâche :
// pour une tâche privée, rien (les pages se mettent seulement à jour) ;
// réservée aux parents, les parents et ceux qu'elle concerne ; sinon pour
// (vide : toute la famille).
func (m *Module) annoncer(ctx context.Context, c *appli.Contexte, t donnees.Tache, texte string, pour []int64) {
	switch {
	case t.Privee == 1:
		m.a.Publier(c, mod, "")
		return
	case len(pour) == 0 && m.cachee(ctx, t):
		pour = append(m.a.Parents(ctx, c), concernes(t)...)
	}
	m.a.PublierPour(c, mod, texte, pour)
}
