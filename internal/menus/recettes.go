// SPDX-License-Identifier: GPL-3.0-or-later

package menus

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
//   - page Recettes : créer une recette (sans prévoir de repas), la prévoir
//     à un jour et un repas, corriger nom, ingrédients et préparation
//     (texte mis en forme), oublier une recette.

func (m *Module) brancherRecettes() {
	a := m.a
	a.Route("GET /menus/ingredients", mod, appli.Membre, m.ingredientsRecette)
	a.Route("POST /menus/semaine/reprendre", mod, appli.Membre, m.reprendreSemaine)
	a.Route("GET /menus/recettes", mod, appli.Membre, m.pageRecettes)
	a.Route("POST /menus/recettes", mod, appli.Membre, m.creerRecette)
	a.Route("GET /menus/recettes/nouvelle", mod, appli.Membre, m.formulaireRecette)
	a.Route("GET /menus/recettes/modifier", mod, appli.Membre, m.formulaireRecette)
	a.Route("POST /menus/recettes/modifier", mod, appli.Membre, m.modifierRecette)
	a.Route("POST /menus/recettes/prevoir", mod, appli.Membre, m.prevoirRecette)
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
	Lignes []string // ingrédients, un par ligne
}

type vueRecettes struct {
	Recettes []ligneRecette
	// « Prévoir ce plat » : le premier repas encore libre, proposé par défaut.
	Jour, Moment string
	Min          string // pas de repas prévu dans le passé (champ date)
}

func (m *Module) pageRecettes(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	l, err := m.a.Q.ToutesRecettes(r.Context(), c.Famille.ID)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	v := vueRecettes{Min: repetition.CleJour(c.Maintenant())}
	for _, x := range l {
		lg := lignes(x.Ingredients)
		v.Recettes = append(v.Recettes, ligneRecette{Recette: x, NbIngr: len(lg), Lignes: lg})
	}
	v.Jour, v.Moment = m.premierLibre(r, c)
	m.a.Afficher(w, r, c, "menus/recettes", "Recettes", mod, v)
}

// premierLibre : le premier repas sans plat à partir de maintenant (le midi
// d'aujourd'hui avant 14 h), sur deux semaines ; sinon ce soir.
func (m *Module) premierLibre(r *http.Request, c *appli.Contexte) (string, string) {
	maintenant := c.Maintenant()
	aujourdhui := repetition.CleJour(maintenant)
	pris := map[string]bool{}
	if repas, err := m.a.Q.RepasPeriode(r.Context(), donnees.RepasPeriodeParams{FamilleID: c.Famille.ID,
		Du: aujourdhui, Au: repetition.CleJour(maintenant.AddDate(0, 0, 13))}); err == nil {
		for _, rp := range repas {
			pris[rp.Jour+" "+rp.Moment] = true
		}
	}
	for i := range 14 {
		jour := repetition.CleJour(maintenant.AddDate(0, 0, i))
		for _, moment := range []string{"midi", "soir"} {
			if i == 0 && moment == "midi" && maintenant.Hour() >= 14 {
				continue
			}
			if !pris[jour+" "+moment] {
				return jour, moment
			}
		}
	}
	return aujourdhui, "soir"
}

// Bornes d'une recette.
const (
	ingredientsMax = 40
	preparationMax = 5000 // caractères (texte mis en forme)
)

// saisieRecette : nom, ingrédients (un par ligne) et préparation du
// formulaire, vérifiés ; message : la raison d'un refus (vide : tout va bien).
func saisieRecette(r *http.Request) (nom, ingr, prep, message string) {
	nom = strings.TrimSpace(r.PostFormValue("nom"))
	ingr = strings.Join(lignes(r.PostFormValue("ingredients")), "\n")
	prep = strings.TrimSpace(strings.ReplaceAll(r.PostFormValue("preparation"), "\r\n", "\n"))
	if n := utf8.RuneCountInString(nom); n < 1 || n > 80 || utf8.RuneCountInString(ingr) > 2000 || len(lignes(ingr)) > ingredientsMax {
		return "", "", "", "Nom (80 caractères) ou ingrédients (40 au plus) invalides."
	}
	if utf8.RuneCountInString(prep) > preparationMax {
		return "", "", "", "Préparation trop longue (5000 caractères au plus)."
	}
	for _, l := range lignes(ingr) {
		if !courses.SaisieValide(l) {
			return "", "", "", "Un ingrédient par ligne, de 80 caractères au plus."
		}
	}
	return nom, ingr, prep, ""
}

type formRecette struct {
	Recette  donnees.Recette
	Nouvelle bool
}

// formulaireRecette : « ＋ Recette » (nouvelle) ou la modification d'une
// recette (?cle=…), sur sa propre page : un seul éditeur de texte à la fois.
func (m *Module) formulaireRecette(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	f := formRecette{Nouvelle: r.URL.Path == "/menus/recettes/nouvelle"}
	titre := "Nouvelle recette"
	if !f.Nouvelle {
		rec, err := m.a.Q.RecetteParCle(r.Context(), donnees.RecetteParCleParams{FamilleID: c.Famille.ID, Cle: r.URL.Query().Get("cle")})
		if err != nil {
			m.a.PageErreur(w, r, c, http.StatusNotFound, "Recette introuvable", "Cette recette n'existe plus.")
			return
		}
		f.Recette, titre = rec, rec.Nom
	}
	m.a.Afficher(w, r, c, "menus/recette", titre, mod, f)
}

var (
	errRecetteExiste = errors.New("recette déjà connue")
	errRecettesPlein = errors.New("trop de recettes")
)

// creerRecette : une recette retenue sans prévoir de repas (compte 0).
func (m *Module) creerRecette(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	nom, ingr, prep, message := saisieRecette(r)
	if message != "" {
		m.a.Echec(w, r, http.StatusBadRequest, message)
		return
	}
	ctx := r.Context()
	err := m.a.Transaction(ctx, func(q *donnees.Queries) error {
		n, err := q.CompterRecettes(ctx, c.Famille.ID)
		if err != nil {
			return err
		}
		if n >= recettesMax {
			return errRecettesPlein
		}
		k, err := q.CreerRecette(ctx, donnees.CreerRecetteParams{FamilleID: c.Famille.ID, Cle: courses.Cle(nom), Nom: nom,
			Ingredients: ingr, Preparation: prep, VuLe: time.Now().Unix()})
		if err == nil && k == 0 {
			return errRecetteExiste
		}
		return err
	})
	switch {
	case errors.Is(err, errRecetteExiste):
		m.a.Echec(w, r, http.StatusConflict, "« "+nom+" » existe déjà : modifiez-la depuis la page Recettes.")
		return
	case errors.Is(err, errRecettesPlein):
		m.a.Echec(w, r, http.StatusBadRequest, "Trop de recettes : demandez à un parent d'en oublier.")
		return
	case err != nil:
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Flash(w, "Recette « "+nom+" » ajoutée ✓")
	m.a.Rediriger(w, r, "/menus/recettes")
}

// prevoirRecette : « Prévoir ce plat » depuis une recette, au jour et au
// repas choisis. Un repas déjà prévu n'est jamais écrasé.
func (m *Module) prevoirRecette(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	rec, err := m.a.Q.RecetteParCle(ctx, donnees.RecetteParCleParams{FamilleID: c.Famille.ID, Cle: r.PostFormValue("cle")})
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Recette introuvable.")
		return
	}
	j, err := time.ParseInLocation("2006-01-02", r.PostFormValue("jour"), c.Lieu)
	moment := r.PostFormValue("moment")
	if err != nil || !anneeValide(j) || (moment != "midi" && moment != "soir") {
		m.a.Echec(w, r, http.StatusBadRequest, "Choisissez un jour et un repas.")
		return
	}
	jour := repetition.CleJour(j)
	quand := appli.TitreJour(j) + ", " + moment
	occupe := func(nom string) {
		m.a.Echec(w, r, http.StatusConflict, quand+" : « "+nom+" » est déjà prévu. Choisissez un autre repas.")
	}
	if rp, err := m.a.Q.RepasDuCreneau(ctx, donnees.RepasDuCreneauParams{FamilleID: c.Famille.ID, Jour: jour, Moment: moment}); err == nil {
		occupe(rp.Nom)
		return
	}
	var k int64
	err = m.a.Transaction(ctx, func(q *donnees.Queries) error {
		// CopierRepas n'écrit que dans un créneau vide (k = 0 sinon : prévu
		// par quelqu'un d'autre entre-temps).
		k, err = q.CopierRepas(ctx, donnees.CopierRepasParams{FamilleID: c.Famille.ID, Jour: jour, Moment: moment, Nom: rec.Nom,
			Ingredients: rec.Ingredients, CreePar: sql.NullInt64{Int64: c.Membre.ID, Valid: true}, CreeLe: time.Now().Unix()})
		if err != nil || k == 0 {
			return err
		}
		return q.RetenirRecette(ctx, donnees.RetenirRecetteParams{FamilleID: c.Famille.ID, Cle: rec.Cle, Nom: rec.Nom,
			Ingredients: rec.Ingredients, VuLe: time.Now().Unix()})
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	if k == 0 {
		occupe("un autre plat")
		return
	}
	m.a.PublierLien(c, mod, fmt.Sprintf("%s a prévu « %s » 🍽️", c.Membre.Nom, rec.Nom), "/menus?date="+jour, nil)
	m.a.Flash(w, "« "+rec.Nom+" » prévu : "+quand+" ✓")
	m.a.Rediriger(w, r, "/menus?date="+jour)
}

// modifierRecette : nom, ingrédients et préparation d'une recette retenue.
// Renommée, sa clé suit le nouveau nom (le repas tapé sous ce nom retrouve
// ses ingrédients) ; si ce nom est déjà une autre recette, les deux
// fusionnent. Les repas déjà prévus gardent leur texte.
func (m *Module) modifierRecette(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	cle := r.PostFormValue("cle")
	nom, ingr, prep, message := saisieRecette(r)
	if message != "" {
		m.a.Echec(w, r, http.StatusBadRequest, message)
		return
	}
	ctx := r.Context()
	nouvelle := courses.Cle(nom)
	err := m.a.Transaction(ctx, func(q *donnees.Queries) error {
		if err := renommerRecette(ctx, q, c, cle, nouvelle, nom, ingr); err != nil {
			return err
		}
		// La préparation suit la recette (sous sa nouvelle clé).
		return q.ModifierPreparation(ctx, donnees.ModifierPreparationParams{Preparation: prep, FamilleID: c.Famille.ID, Cle: nouvelle})
	})
	if errors.Is(err, sql.ErrNoRows) {
		m.a.Echec(w, r, http.StatusNotFound, "Recette introuvable.")
		return
	}
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Flash(w, "Recette enregistrée ✓")
	m.a.Rediriger(w, r, "/menus/recettes")
}

// renommerRecette : nom et ingrédients de la recette cle ; nouvelle : la clé
// du nouveau nom (fusion avec la recette qui l'a déjà, s'il y en a une).
func renommerRecette(ctx context.Context, q *donnees.Queries, c *appli.Contexte, cle, nouvelle, nom, ingr string) error {
	if nouvelle == cle {
		return q.ModifierRecette(ctx, donnees.ModifierRecetteParams{Nom: nom, Ingredients: ingr, FamilleID: c.Famille.ID, Cle: cle})
	}
	if _, err := q.RecetteParCle(ctx, donnees.RecetteParCleParams{FamilleID: c.Famille.ID, Cle: nouvelle}); err == nil {
		ancienne, err := q.RecetteParCle(ctx, donnees.RecetteParCleParams{FamilleID: c.Famille.ID, Cle: cle})
		if err != nil {
			return err
		}
		if err := q.FusionnerRecette(ctx, donnees.FusionnerRecetteParams{Nom: nom, Ingredients: ingr, Compte: ancienne.Compte,
			FamilleID: c.Famille.ID, Cle: nouvelle}); err != nil {
			return err
		}
		return q.OublierRecette(ctx, donnees.OublierRecetteParams{FamilleID: c.Famille.ID, Cle: cle})
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return q.RenommerRecette(ctx, donnees.RenommerRecetteParams{Nouvelle: nouvelle, Nom: nom, Ingredients: ingr,
		FamilleID: c.Famille.ID, Cle: cle})
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
