// SPDX-License-Identifier: GPL-3.0-or-later

package menus

import (
	"database/sql"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"familycocon/internal/appli"
	"familycocon/internal/courses"
	"familycocon/internal/donnees"
	"familycocon/internal/repetition"
)

// Recettes et petits outils des menus :
//   - choisir une recette connue remplit ses ingrédients (htmx) ;
//   - cases « déjà à la maison » avant d'envoyer aux courses ;
//   - « Reprendre la semaine dernière » ;
//   - page Recettes : corriger les ingrédients, oublier une recette.

func (m *Module) brancherRecettes() {
	a := m.a
	a.Route("GET /menus/ingredients", mod, appli.Membre, m.ingredientsRecette)
	a.Route("POST /menus/semaine/reprendre", mod, appli.Membre, m.reprendreSemaine)
	a.Route("GET /menus/recettes", mod, appli.Membre, m.pageRecettes)
	a.Route("POST /menus/recettes/modifier", mod, appli.Membre, m.modifierRecette)
	a.Route("POST /menus/recettes/oublier", mod, appli.Parent, m.oublierRecette)
}

// ingredientsRecette : le champ « ingrédients », rempli avec ceux de la
// recette choisie s'il est encore vide (rien n'est jamais écrasé).
func (m *Module) ingredientsRecette(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	q := r.URL.Query()
	ingr := q.Get("ingredients")
	if strings.TrimSpace(ingr) == "" {
		if rec, err := m.a.Q.RecetteParCle(r.Context(), donnees.RecetteParCleParams{FamilleID: c.Famille.ID, Cle: courses.Cle(q.Get("nom"))}); err == nil {
			ingr = rec.Ingredients
		}
	}
	m.a.Fragment(w, r, c, "menus/repas", "champ-ingredients", ingr)
}

// lignesChoisies : les ingrédients cochés (« pas déjà à la maison »),
// seulement parmi ceux du repas. coches nil : tous.
func lignesChoisies(ingredients string, coches []string) []string {
	tous := lignes(ingredients)
	if coches == nil {
		return tous
	}
	var out []string
	for _, l := range tous {
		if slices.Contains(coches, l) {
			out = append(out, l)
		}
	}
	return out
}

// reprendreSemaine : les repas de la semaine d'avant, dans les créneaux
// encore vides de cette semaine.
func (m *Module) reprendreSemaine(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	ref := reference(r, c)
	lundi := repetition.Lundi(ref)
	avant := lundi.AddDate(0, 0, -7)
	var n int64
	err := m.a.Transaction(ctx, func(q *donnees.Queries) error {
		repas, err := q.RepasPeriode(ctx, donnees.RepasPeriodeParams{FamilleID: c.Famille.ID, Du: repetition.CleJour(avant), Au: repetition.CleJour(avant.AddDate(0, 0, 6))})
		if err != nil {
			return err
		}
		for _, rp := range repas {
			j, err := time.ParseInLocation("2006-01-02", rp.Jour, c.Lieu)
			if err != nil {
				continue
			}
			k, err := q.CopierRepas(ctx, donnees.CopierRepasParams{FamilleID: c.Famille.ID, Jour: repetition.CleJour(j.AddDate(0, 0, 7)),
				Moment: rp.Moment, Nom: rp.Nom, Ingredients: rp.Ingredients, CreePar: sql.NullInt64{Int64: c.Membre.ID, Valid: true}, CreeLe: time.Now().Unix()})
			if err != nil {
				return err
			}
			n += k
		}
		return nil
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	if n > 0 {
		m.a.Publier(c, mod, "")
	}
	appli.Notifier(w, appli.Pluriel(int(n), "repas repris", "repas repris")+" de la semaine dernière", false)
	m.renvoyer(w, r, c, ref)
}

type ligneRecette struct {
	donnees.Recette
	NbIngr int
}

func (m *Module) pageRecettes(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	l, err := m.a.Q.ToutesRecettes(r.Context(), c.Famille.ID)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	var v []ligneRecette
	for _, x := range l {
		v = append(v, ligneRecette{Recette: x, NbIngr: len(lignes(x.Ingredients))})
	}
	m.a.Afficher(w, r, c, "menus/recettes", "Recettes", mod, v)
}

// modifierRecette : nom et ingrédients d'une recette retenue (la clé, tirée
// de l'ancien nom, ne change pas : les repas déjà prévus ne bougent pas).
func (m *Module) modifierRecette(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	cle := r.PostFormValue("cle")
	nom := strings.TrimSpace(r.PostFormValue("nom"))
	ingr := strings.Join(lignes(r.PostFormValue("ingredients")), "\n")
	if n := utf8.RuneCountInString(nom); n < 1 || n > 80 || utf8.RuneCountInString(ingr) > 2000 || len(lignes(ingr)) > 40 {
		m.a.Echec(w, r, http.StatusBadRequest, "Nom (80 caractères) ou ingrédients (40 au plus) invalides.")
		return
	}
	for _, l := range lignes(ingr) {
		if !courses.SaisieValide(l) {
			m.a.Echec(w, r, http.StatusBadRequest, "Un ingrédient par ligne, de 80 caractères au plus.")
			return
		}
	}
	if err := m.a.Q.ModifierRecette(r.Context(), donnees.ModifierRecetteParams{Nom: nom, Ingredients: ingr, FamilleID: c.Famille.ID, Cle: cle}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Flash(w, "Recette enregistrée ✓")
	m.a.Rediriger(w, r, "/menus/recettes")
}

// oublierRecette : elle n'est plus proposée (faute de frappe…).
func (m *Module) oublierRecette(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	if err := m.a.Q.OublierRecette(r.Context(), donnees.OublierRecetteParams{FamilleID: c.Famille.ID, Cle: r.PostFormValue("cle")}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Flash(w, "Recette oubliée")
	m.a.Rediriger(w, r, "/menus/recettes")
}

// listes : les listes de courses de la famille (vide s'il n'y en a qu'une :
// pas de choix à faire) et la dernière choisie sur cet appareil.
func (m *Module) listes(r *http.Request, c *appli.Contexte) ([]donnees.Liste, int64) {
	if !c.Actif("courses") {
		return nil, 0
	}
	l, err := m.a.Q.ListesDeFamille(r.Context(), c.Famille.ID)
	if err != nil || len(l) < 2 {
		return nil, 0
	}
	choisie, _ := strconv.ParseInt(m.a.LirePreference(r, "menus_liste"), 10, 64)
	return l, choisie
}

// listeChoisie : la liste de courses demandée (champ « liste »), retenue par
// l'appareil pour la prochaine fois ; sans champ, la dernière choisie ; 0 :
// la première liste.
func (m *Module) listeChoisie(w http.ResponseWriter, r *http.Request) int64 {
	id, err := strconv.ParseInt(r.PostFormValue("liste"), 10, 64)
	if err != nil || id <= 0 {
		n, _ := strconv.ParseInt(m.a.LirePreference(r, "menus_liste"), 10, 64)
		return n
	}
	m.a.PoserPreference(w, "menus_liste", strconv.FormatInt(id, 10))
	return id
}
