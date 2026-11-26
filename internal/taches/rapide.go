// SPDX-License-Identifier: GPL-3.0-or-later

package taches

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
)

// Liste des tâches : filtres repliés et ajout rapide.
//
// Les trois rangées de filtres (membres, catégories, « Voir ») sont rangées
// dans un bloc « 🔎 Filtrer » replié, avec un petit résumé des filtres
// actifs. Il s'ouvre quand un filtre vient d'être choisi (il est dans
// l'adresse) ; le filtre « pour qui » gardé par l'appareil (cookie) est
// seulement rappelé dans le résumé : le bloc reste fermé.
//
// Ajout rapide « Ajouter une tâche… » (comme celui des courses) : une tâche
// « une fois », pour le membre filtré (sinon tout le monde), dans la
// catégorie filtrée. Le formulaire complet reste à « ＋ Tâche ».

// nomsVoir : les choix du filtre « Voir », pour le résumé.
var nomsVoir = map[string]string{
	voirPartagees: "👪 Partagées",
	voirPrivees:   "🔒 Privées",
	voirEnfants:   "🧒 Vues des enfants",
}

// resumer : le résumé des filtres actifs (« Léa · 🧹 Ménage »), et le nom
// du membre filtré.
func (v *vueTaches) resumer() {
	var t []string
	for _, mb := range v.Membres {
		if mb.ID == v.Pour {
			v.NomPour = mb.Nom
			t = append(t, mb.Nom)
		}
	}
	switch {
	case v.Categorie < 0:
		t = append(t, "Sans catégorie")
	case v.Categorie > 0:
		for _, k := range v.Categories {
			if k.ID == v.Categorie {
				t = append(t, k.Etiquette())
			}
		}
	}
	if n, ok := nomsVoir[v.Voir]; ok {
		t = append(t, n)
	}
	v.Resume = strings.Join(t, " · ")
}

// filtreDansAdresse : un filtre autre que celui par défaut est dans
// l'adresse (il vient d'être choisi : le bloc des filtres est ouvert).
func filtreDansAdresse(q url.Values) bool {
	return (q.Get("pour") != "" && q.Get("pour") != "0") || q.Get("cat") != "" || q.Get("voir") != ""
}

// ajoutRapide : « Ajouter une tâche… » en haut de la liste.
func (m *Module) ajoutRapide(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	titre := strings.TrimSpace(r.PostFormValue("titre"))
	if n := utf8.RuneCountInString(titre); n < 1 || n > 80 {
		m.a.Echec(w, r, http.StatusBadRequest, "Le titre doit faire entre 1 et 80 caractères.")
		return
	}
	if n, err := m.a.Q.CompterTachesActives(ctx, donnees.CompterTachesActivesParams{FamilleID: c.Famille.ID, Depuis: debutDuJour(c)}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	} else if n >= tachesMax {
		m.a.Echec(w, r, http.StatusBadRequest, fmt.Sprintf("%d tâches au plus : demandez à un parent d'en supprimer.", tachesMax))
		return
	}
	pour, cat, voir := lirePourForm(r), lireFiltreCategorie(r.PostFormValue("cat")), lireVoir(r.PostFormValue("voir"), c)
	// Pour le membre filtré (de la famille), sinon pour tout le monde.
	var assigne sql.NullInt64
	var notifier []int64
	if pour > 0 {
		if _, err := m.a.Q.MembreParID(ctx, donnees.MembreParIDParams{ID: pour, FamilleID: c.Famille.ID}); err == nil {
			assigne = sql.NullInt64{Int64: pour, Valid: true}
			notifier = []int64{pour}
		}
	}
	// Dans la catégorie filtrée, si ce membre peut s'en servir.
	var categorie sql.NullInt64
	if cat > 0 {
		if n, err := m.a.Q.CategorieTacheExiste(ctx, donnees.CategorieTacheExisteParams{ID: cat, FamilleID: c.Famille.ID, Parent: estParent(c)}); err == nil && n > 0 {
			categorie = sql.NullInt64{Int64: cat, Valid: true}
		}
	}
	t, err := m.a.Q.CreerTache(ctx, donnees.CreerTacheParams{
		FamilleID: c.Famille.ID, Titre: titre, Emoji: DevinerEmoji(titre), AssigneA: assigne, CategorieID: categorie,
		CreePar: sql.NullInt64{Int64: c.Membre.ID, Valid: true}, CreeLe: time.Now().Unix(),
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.annoncer(ctx, c, t, fmt.Sprintf("%s a ajouté la tâche « %s » %s", c.Membre.Nom, t.Titre, t.Emoji), notifier)
	if !appli.EstHtmx(r) {
		m.a.Flash(w, "Tâche ajoutée ✓")
		v := &vueTaches{}
		http.Redirect(w, r, v.Adresse(pour, paramCategorie(cat), voir), http.StatusSeeOther)
		return
	}
	appli.Notifier(w, "Tâche ajoutée ✓", false)
	m.renvoyer(w, r, c, pour, cat, voir)
}
