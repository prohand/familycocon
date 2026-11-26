// SPDX-License-Identifier: GPL-3.0-or-later

// Package menus : les menus de la semaine (midi et soir). Chaque plat a sa
// liste d'ingrédients, qu'un clic ajoute à la liste de courses. Les plats
// sont retenus comme recettes : la prochaine fois, leurs ingrédients
// reviennent tout seuls.
//
// Droits : tout le monde propose un plat ; seuls les parents vident un créneau.
package menus

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"familycocon/internal/appli"
	"familycocon/internal/courses"
	"familycocon/internal/donnees"
	"familycocon/internal/repetition"
)

// Module : le module Menus.
type Module struct{ a *appli.App }

const mod = "menus"

// Brancher ajoute les routes du module.
func Brancher(a *appli.App) {
	m := &Module{a: a}
	a.Route("GET /menus", mod, appli.Membre, m.page)
	a.AjouterRecherche(mod, m.chercher)
	a.Route("GET /menus/contenu", mod, appli.Membre, m.contenu)
	m.brancherRecettes()
	a.Route("POST /menus/semaine/courses", mod, appli.Membre, m.semaineAuxCourses)
	a.Route("GET /menus/{jour}/{moment}", mod, appli.Membre, m.formulaire)
	a.Route("POST /menus/{jour}/{moment}", mod, appli.Membre, m.enregistrer)
	a.Route("POST /menus/{jour}/{moment}/courses", mod, appli.Membre, m.auxCourses)
	a.Route("POST /menus/{jour}/{moment}/vider", mod, appli.Parent, m.vider)

	a.AjouterResume(mod, m.resume)
	a.AjouterAction(appli.Ajout{Module: mod, Emoji: "🍽️", Texte: "Un repas", URL: "/menus"})
}

// Creneau : un repas (midi ou soir) d'un jour.
type Creneau struct {
	Jour   string // 2026-11-22
	Moment string // midi | soir
	Repas  *donnees.Repas
	NbIngr int
}

// Libelle : « ☀️ Midi », « 🌙 Soir ».
func (c Creneau) Libelle() string {
	if c.Moment == "midi" {
		return "☀️ Midi"
	}
	return "🌙 Soir"
}

type jourMenu struct {
	Cle        string
	Titre      string
	Aujourdhui bool
	Passe      bool // jour déjà passé de la semaine en cours (replié sur téléphone)
	Midi, Soir Creneau
}

type vueMenus struct {
	Titre         string
	Ref           string
	Precedent     string
	Suivant       string
	Jours         []jourMenu
	AjoutPossible int             // plats dont les ingrédients ne sont pas encore dans les courses
	Courses       bool            // module Courses activé
	Listes        []donnees.Liste // listes de courses (s'il y en a plusieurs)
	Liste         int64           // la dernière choisie sur cet appareil
	Passes        string          // « Lundi, mardi : passés » : jours repliés (semaine en cours)
}

func lignes(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(l), "-•*")); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func (m *Module) vue(ctx context.Context, c *appli.Contexte, ref time.Time) (*vueMenus, error) {
	lundi := repetition.Lundi(ref)
	dimanche := lundi.AddDate(0, 0, 6)
	v := &vueMenus{
		Ref:       repetition.CleJour(lundi),
		Precedent: repetition.CleJour(lundi.AddDate(0, 0, -7)),
		Suivant:   repetition.CleJour(lundi.AddDate(0, 0, 7)),
		Courses:   c.Actif("courses"),
		Titre:     fmt.Sprintf("Semaine du %d %s", lundi.Day(), appli.MoisFR(lundi.Month())),
	}
	repas, err := m.a.Q.RepasPeriode(ctx, donnees.RepasPeriodeParams{FamilleID: c.Famille.ID, Du: repetition.CleJour(lundi), Au: repetition.CleJour(dimanche)})
	if err != nil {
		return nil, err
	}
	trouver := func(jour, moment string) Creneau {
		cr := Creneau{Jour: jour, Moment: moment}
		for i := range repas {
			if repas[i].Jour == jour && repas[i].Moment == moment {
				cr.Repas = &repas[i]
				cr.NbIngr = len(lignes(repas[i].Ingredients))
				if cr.NbIngr > 0 && repas[i].DansCourses == 0 {
					v.AjoutPossible++
				}
			}
		}
		return cr
	}
	aujourdhui := repetition.CleJour(c.Maintenant())
	// Semaine en cours : les jours déjà passés se replient (« Lundi, mardi :
	// passés ▸ ») pour que le jour courant se voie sans défiler. Une semaine
	// passée, ouverte exprès, reste entière.
	enCours := aujourdhui >= v.Ref && aujourdhui <= repetition.CleJour(dimanche)
	var passes []string
	for j := lundi; !j.After(dimanche); j = j.AddDate(0, 0, 1) {
		cle := repetition.CleJour(j)
		jm := jourMenu{
			Cle: cle, Titre: appli.TitreJour(j), Aujourdhui: cle == aujourdhui, Passe: enCours && cle < aujourdhui,
			Midi: trouver(cle, "midi"), Soir: trouver(cle, "soir"),
		}
		if jm.Passe {
			passes = append(passes, appli.JourFR(j))
		}
		v.Jours = append(v.Jours, jm)
	}
	if len(passes) > 0 {
		v.Passes = strings.ToUpper(passes[0][:1]) + strings.Join(passes, ", ")[1:] + " : passé"
		if len(passes) > 1 {
			v.Passes += "s"
		}
	}
	return v, nil
}

func reference(r *http.Request, c *appli.Contexte) time.Time {
	q := r.URL.Query().Get("date")
	if q == "" {
		q = r.PostFormValue("date")
	}
	if d, err := time.ParseInLocation("2006-01-02", q, c.Lieu); err == nil && anneeValide(d) {
		return d
	}
	return c.Maintenant()
}

// anneeValide : mêmes bornes que l'agenda (1900 à 2200).
func anneeValide(t time.Time) bool { return t.Year() >= 1900 && t.Year() <= 2200 }

// recettesMax : recettes retenues par famille (suggestions).
const recettesMax = 2000

func (m *Module) page(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	v, err := m.vue(r.Context(), c, reference(r, c))
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	v.Listes, v.Liste = m.listes(r, c)
	m.a.Afficher(w, r, c, "menus/semaine", "Menus", mod, v)
}

func (m *Module) renvoyer(w http.ResponseWriter, r *http.Request, c *appli.Contexte, ref time.Time) {
	if !appli.EstHtmx(r) {
		http.Redirect(w, r, "/menus?date="+repetition.CleJour(ref), http.StatusSeeOther)
		return
	}
	v, err := m.vue(r.Context(), c, ref)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	v.Listes, v.Liste = m.listes(r, c)
	m.a.Fragment(w, r, c, "menus/semaine", "menus", v)
}

func (m *Module) contenu(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	m.renvoyer(w, r, c, reference(r, c))
}

// creneau : jour et moment de l'adresse, vérifiés.
func creneau(r *http.Request, c *appli.Contexte) (time.Time, string, bool) {
	j, err := time.ParseInLocation("2006-01-02", r.PathValue("jour"), c.Lieu)
	moment := r.PathValue("moment")
	return j, moment, err == nil && anneeValide(j) && (moment == "midi" || moment == "soir")
}

type formRepas struct {
	Creneau     Creneau
	Titre       string
	Recettes    []string
	Courses     bool
	Lignes      []string // ingrédients, à cocher avant l'envoi aux courses
	Ingredients string
	Listes      []donnees.Liste // listes de courses (choix s'il y en a plusieurs)
	Liste       int64           // la dernière choisie sur cet appareil
	Preparation string          // préparation de la recette du plat (texte mis en forme)
}

func (m *Module) formulaire(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	j, moment, ok := creneau(r, c)
	if !ok {
		m.a.PageErreur(w, r, c, http.StatusNotFound, "Créneau inconnu", "Ce repas n'existe pas.")
		return
	}
	f := formRepas{Creneau: Creneau{Jour: repetition.CleJour(j), Moment: moment}, Courses: c.Actif("courses")}
	if rp, err := m.a.Q.RepasDuCreneau(ctx, donnees.RepasDuCreneauParams{FamilleID: c.Famille.ID, Jour: f.Creneau.Jour, Moment: moment}); err == nil {
		f.Creneau.Repas = &rp
		f.Lignes, f.Ingredients = lignes(rp.Ingredients), rp.Ingredients
		f.Listes, f.Liste = m.listes(r, c)
		// La préparation de la recette, sous la main pour cuisiner.
		if rec, err := m.a.Q.RecetteParCle(ctx, donnees.RecetteParCleParams{FamilleID: c.Famille.ID, Cle: courses.Cle(rp.Nom)}); err == nil {
			f.Preparation = rec.Preparation
		}
	}
	f.Titre = appli.TitreJour(j) + ", " + moment
	var err error
	if f.Recettes, err = m.a.Q.RecettesConnues(ctx, c.Famille.ID); err != nil {
		m.a.Journal.Warn("menus : recettes illisibles", "erreur", err) // le formulaire marche sans
	}
	m.a.Afficher(w, r, c, "menus/repas", f.Titre, mod, f)
}

func (m *Module) enregistrer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	j, moment, ok := creneau(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Créneau inconnu.")
		return
	}
	nom := strings.TrimSpace(r.PostFormValue("nom"))
	ingr := strings.Join(lignes(r.PostFormValue("ingredients")), "\n")
	if n := utf8.RuneCountInString(nom); n < 1 || n > 80 {
		m.a.Echec(w, r, http.StatusBadRequest, "Le nom du plat doit faire entre 1 et 80 caractères.")
		return
	}
	if utf8.RuneCountInString(ingr) > 2000 || len(lignes(ingr)) > 40 {
		m.a.Echec(w, r, http.StatusBadRequest, "40 ingrédients au plus.")
		return
	}
	// Chaque ligne doit pouvoir devenir un produit de la liste de courses.
	for _, l := range lignes(ingr) {
		if !courses.SaisieValide(l) {
			m.a.Echec(w, r, http.StatusBadRequest, "Un ingrédient par ligne, de 80 caractères au plus.")
			return
		}
	}
	cle := courses.Cle(nom)
	// Recette connue et aucun ingrédient saisi : on reprend les siens.
	rec, err := m.a.Q.RecetteParCle(ctx, donnees.RecetteParCleParams{FamilleID: c.Famille.ID, Cle: cle})
	connue := err == nil
	if ingr == "" && connue {
		ingr = rec.Ingredients
	}
	if _, err := m.a.Q.EnregistrerRepas(ctx, donnees.EnregistrerRepasParams{
		FamilleID: c.Famille.ID, Jour: repetition.CleJour(j), Moment: moment, Nom: nom, Ingredients: ingr,
		CreePar: sql.NullInt64{Int64: c.Membre.ID, Valid: true}, CreeLe: time.Now().Unix(),
	}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	// La recette est retenue (une nouvelle : seulement s'il reste de la place).
	retenir := connue
	if !retenir {
		n, err := m.a.Q.CompterRecettes(ctx, c.Famille.ID)
		retenir = err == nil && n < recettesMax
	}
	if retenir {
		if err := m.a.Q.RetenirRecette(ctx, donnees.RetenirRecetteParams{FamilleID: c.Famille.ID, Cle: cle, Nom: nom, Ingredients: ingr, VuLe: time.Now().Unix()}); err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
	}
	m.a.Publier(c, mod, fmt.Sprintf("%s a prévu « %s » 🍽️", c.Membre.Nom, nom))
	m.a.Flash(w, "Repas enregistré ✓")
	m.a.Rediriger(w, r, "/menus?date="+repetition.CleJour(j))
}

func (m *Module) vider(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	j, moment, ok := creneau(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Créneau inconnu.")
		return
	}
	if err := m.a.Q.ViderRepas(r.Context(), donnees.ViderRepasParams{FamilleID: c.Famille.ID, Jour: repetition.CleJour(j), Moment: moment}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Créneau vidé")
	m.a.Rediriger(w, r, "/menus?date="+repetition.CleJour(j))
}

// ajouterAuxCourses : les ingrédients d'un repas vont dans la liste de
// courses, une seule fois. Le repas est d'abord marqué « aux courses » : si
// aucune ligne ne change (double clic, deux membres en même temps), rien
// n'est ajouté. q : les requêtes d'une transaction (tout ou rien).
// choisis : les ingrédients à ajouter (nil : tous) ; liste : la liste de
// courses choisie (0 : la première).
func ajouterAuxCourses(ctx context.Context, q *donnees.Queries, c *appli.Contexte, rp donnees.Repas, choisis []string, liste int64) (int, error) {
	marques, err := q.MarquerDansCourses(ctx, donnees.MarquerDansCoursesParams{ID: rp.ID, FamilleID: c.Famille.ID})
	if err != nil || marques == 0 {
		return 0, err
	}
	return courses.AjouterProduitsDans(ctx, q, c, liste, lignesChoisies(rp.Ingredients, choisis))
}

func (m *Module) auxCourses(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	if !c.Actif("courses") {
		m.a.Echec(w, r, http.StatusBadRequest, "Le module Courses est désactivé.")
		return
	}
	j, moment, ok := creneau(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Créneau inconnu.")
		return
	}
	rp, err := m.a.Q.RepasDuCreneau(ctx, donnees.RepasDuCreneauParams{FamilleID: c.Famille.ID, Jour: repetition.CleJour(j), Moment: moment})
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Pas de repas à ce créneau.")
		return
	}
	// Depuis la page du repas : seulement les ingrédients cochés (ceux qui
	// ne sont pas déjà à la maison).
	var choisis []string
	if r.PostFormValue("choix") == "1" {
		choisis = append([]string{}, r.PostForm["ingr"]...)
	}
	var n int
	err = m.a.Transaction(ctx, func(q *donnees.Queries) (err error) {
		n, err = ajouterAuxCourses(ctx, q, c, rp, choisis, m.listeChoisie(w, r))
		return err
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	if n > 0 {
		m.a.Publier(c, "courses", fmt.Sprintf("%s a ajouté les ingrédients de « %s » aux courses 🛒", c.Membre.Nom, rp.Nom))
	}
	m.a.Publier(c, mod, "")
	appli.Notifier(w, appli.Pluriel(n, "ingrédient ajouté", "ingrédients ajoutés")+" aux courses 🛒", false)
	m.renvoyer(w, r, c, j)
}

func (m *Module) semaineAuxCourses(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	if !c.Actif("courses") {
		m.a.Echec(w, r, http.StatusBadRequest, "Le module Courses est désactivé.")
		return
	}
	ref := reference(r, c)
	lundi := repetition.Lundi(ref)
	liste := m.listeChoisie(w, r)
	// Lecture et ajouts dans une seule transaction : toute la semaine, ou rien.
	total := 0
	err := m.a.Transaction(ctx, func(q *donnees.Queries) error {
		total = 0
		repas, err := q.RepasPeriode(ctx, donnees.RepasPeriodeParams{FamilleID: c.Famille.ID, Du: repetition.CleJour(lundi), Au: repetition.CleJour(lundi.AddDate(0, 0, 6))})
		if err != nil {
			return err
		}
		for _, rp := range repas {
			if rp.DansCourses == 1 || rp.Ingredients == "" {
				continue
			}
			n, err := ajouterAuxCourses(ctx, q, c, rp, nil, liste)
			if err != nil {
				return err
			}
			total += n
		}
		return nil
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	if total > 0 {
		m.a.Publier(c, "courses", fmt.Sprintf("%s a ajouté les ingrédients de la semaine aux courses 🛒", c.Membre.Nom))
		m.a.Publier(c, mod, "")
	}
	appli.Notifier(w, appli.Pluriel(total, "ingrédient ajouté", "ingrédients ajoutés")+" aux courses 🛒", false)
	m.renvoyer(w, r, c, ref)
}

func (m *Module) resume(ctx context.Context, c *appli.Contexte) appli.Resume {
	maintenant := c.Maintenant()
	moment, libelle := "soir", "Ce soir"
	if maintenant.Hour() < 14 {
		moment, libelle = "midi", "Ce midi"
	}
	rp, err := m.a.Q.RepasDuCreneau(ctx, donnees.RepasDuCreneauParams{FamilleID: c.Famille.ID, Jour: repetition.CleJour(maintenant), Moment: moment})
	if err != nil {
		return appli.Resume{Info: libelle + " : à prévoir"}
	}
	return appli.Resume{Info: libelle + " : " + rp.Nom}
}
