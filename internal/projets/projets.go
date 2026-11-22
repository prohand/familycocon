// SPDX-License-Identifier: GPL-3.0-or-later

// Package projets : les projets de la famille (vacances, travaux,
// déménagement, fête…). Un projet a des étapes à cocher (avec une date et
// quelqu'un qui s'en occupe), une date d'objectif et un budget ; les
// dépenses vont dans le carnet commun (paquet suivi).
//
// Droits : tout le monde consulte, ajoute et coche ; seuls les parents
// suppriment.
package projets

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"familycocon/internal/appli"
	"familycocon/internal/budget"
	"familycocon/internal/donnees"
	"familycocon/internal/suivi"
)

const mod = "projets"

// Emojis proposés pour un projet.
var Emojis = []string{"🎯", "🏖️", "✈️", "🏠", "🔨", "📦", "🎉", "🎄", "🌱", "🎓", "💍", "🚀"}

// Module : les projets.
type Module struct {
	a *appli.App
	s *suivi.Suivi
}

// Brancher ajoute les routes du module.
func Brancher(a *appli.App, s *suivi.Suivi) {
	m := &Module{a: a, s: s}
	a.Route("GET /projets", mod, appli.Membre, m.liste)
	a.Route("GET /projets/nouveau", mod, appli.Membre, m.formulaire)
	a.Route("POST /projets", mod, appli.Membre, m.enregistrer)
	a.Route("GET /projets/{id}", mod, appli.Membre, m.fiche)
	a.Route("GET /projets/{id}/modifier", mod, appli.Membre, m.formulaire)
	a.Route("POST /projets/{id}", mod, appli.Membre, m.enregistrer)
	a.Route("POST /projets/{id}/terminer", mod, appli.Membre, m.terminer)
	a.Route("POST /projets/{id}/supprimer", mod, appli.Parent, m.supprimer)
	a.Route("POST /projets/{id}/etapes", mod, appli.Membre, m.ajouterEtape)
	a.Route("POST /projets/etapes/{id}/cocher", mod, appli.Membre, m.cocher)
	a.Route("POST /projets/etapes/{id}/supprimer", mod, appli.Parent, m.supprimerEtape)

	// Dépenses : le carnet commun, sans échéances (les étapes en tiennent lieu).
	s.Enregistrer(suivi.Objet{
		Module: mod, Emoji: "🎯", SansEcheances: true,
		Nom: func(ctx context.Context, famille, id int64) (string, bool) {
			p, err := a.Q.ProjetParID(ctx, donnees.ProjetParIDParams{ID: id, FamilleID: famille})
			return p.Nom, err == nil
		},
		URL: func(id int64) string { return fmt.Sprintf("/projets/%d", id) },
	})
	// Rappels : les étapes datées et la date d'objectif.
	a.AjouterEcheances(mod, m.echeances)
	a.AjouterResume(mod, m.resume)
	a.AjouterAction(appli.Ajout{Module: mod, Emoji: "🎯", Texte: "Un projet", URL: "/projets/nouveau"})
}

func idDe(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id
}

func (m *Module) projet(r *http.Request, c *appli.Contexte, id int64) (donnees.Projet, bool) {
	p, err := m.a.Q.ProjetParID(r.Context(), donnees.ProjetParIDParams{ID: id, FamilleID: c.Famille.ID})
	return p, err == nil
}

// ---------- Liste ----------

// LigneProjet : un projet dans la liste.
type LigneProjet struct {
	donnees.ProjetsFamilleRow
	Objectif string // « Dans 3 semaines », « En retard »…
	Retard   bool
}

type vueListe struct {
	EnCours, Termines []LigneProjet
}

func (m *Module) liste(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	l, err := m.a.Q.ProjetsFamille(r.Context(), c.Famille.ID)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	var v vueListe
	for _, p := range l {
		lp := LigneProjet{ProjetsFamilleRow: p}
		if p.TermineLe > 0 {
			v.Termines = append(v.Termines, lp)
			continue
		}
		lp.Objectif, lp.Retard = quand(p.Objectif, c)
		v.EnCours = append(v.EnCours, lp)
	}
	m.a.Afficher(w, r, c, "projets/liste", "Projets", mod, v)
}

// quand : « Dans 12 jours » pour une date AAAA-MM-JJ ; retard si passée.
func quand(jour string, c *appli.Contexte) (string, bool) {
	j, err := time.ParseInLocation("2006-01-02", jour, c.Lieu)
	if err != nil {
		return "", false
	}
	auj := c.Maintenant()
	minuit := time.Date(auj.Year(), auj.Month(), auj.Day(), 0, 0, 0, 0, c.Lieu)
	if j.Before(minuit) {
		return "En retard (" + appli.DateLongue(j) + ")", true
	}
	return appli.Quand(j, minuit), false
}

// ---------- Fiche ----------

// LigneEtape : une étape et qui s'en occupe.
type LigneEtape struct {
	donnees.ProjetEtape
	Membre *donnees.Membre
	Quand  string
	Retard bool
}

type vueFiche struct {
	P                donnees.Projet
	Etapes           []LigneEtape
	NbFaites, Total  int
	Objectif         string
	Retard           bool
	Membres          []donnees.Membre
	Suivi            *suivi.Bloc
	Depense, Restant int64
	Depasse          bool
	Devise           string
}

func (m *Module) fiche(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	p, ok := m.projet(r, c, idDe(r))
	if !ok {
		m.a.PageErreur(w, r, c, http.StatusNotFound, "Projet introuvable", "Ce projet n'existe plus.")
		return
	}
	v := vueFiche{P: p, Devise: c.Famille.Devise}
	var err error
	if v.Membres, err = m.a.Membres(ctx, c); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	etapes, err := m.a.Q.EtapesProjet(ctx, donnees.EtapesProjetParams{ProjetID: p.ID, FamilleID: c.Famille.ID})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	for _, e := range etapes {
		l := LigneEtape{ProjetEtape: e}
		for i := range v.Membres {
			if e.MembreID.Valid && v.Membres[i].ID == e.MembreID.Int64 {
				l.Membre = &v.Membres[i]
			}
		}
		if e.FaiteLe > 0 {
			v.NbFaites++
		} else {
			l.Quand, l.Retard = quand(e.Jour, c)
		}
		v.Etapes = append(v.Etapes, l)
	}
	v.Total = len(etapes)
	if p.TermineLe == 0 {
		v.Objectif, v.Retard = quand(p.Objectif, c)
	}
	if v.Suivi, err = m.s.Bloc(ctx, c, mod, p.ID); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	v.Depense = v.Suivi.Total
	v.Restant = p.Budget - v.Depense
	v.Depasse = p.Budget > 0 && v.Restant < 0
	if v.Depasse {
		v.Restant = -v.Restant
	}
	m.a.Afficher(w, r, c, "projets/fiche", p.Nom, mod, v)
}

// ---------- Création, modification ----------

type formProjet struct {
	Nouveau bool
	P       donnees.Projet
	Emojis  []string
	Budget  string
}

func (m *Module) formulaire(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	f := formProjet{Nouveau: r.PathValue("id") == "", Emojis: Emojis, P: donnees.Projet{Emoji: "🎯", Couleur: 1}}
	if !f.Nouveau {
		p, ok := m.projet(r, c, idDe(r))
		if !ok {
			m.a.PageErreur(w, r, c, http.StatusNotFound, "Projet introuvable", "Ce projet n'existe plus.")
			return
		}
		f.P = p
		if p.Budget > 0 {
			f.Budget = strings.Replace(fmt.Sprintf("%d.%02d", p.Budget/100, p.Budget%100), ".", ",", 1)
		}
	}
	m.a.Afficher(w, r, c, "projets/formulaire", "Projet", mod, f)
}

func emojiValide(e string) string {
	for _, x := range Emojis {
		if x == e {
			return e
		}
	}
	return "🎯"
}

func (m *Module) enregistrer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	nom := strings.TrimSpace(r.PostFormValue("nom"))
	desc := strings.TrimSpace(r.PostFormValue("description"))
	if n := utf8.RuneCountInString(nom); n < 1 || n > 80 || utf8.RuneCountInString(desc) > 2000 {
		m.a.Echec(w, r, http.StatusBadRequest, "Nom (80 caractères) ou description (2000) invalide.")
		return
	}
	objectif := r.PostFormValue("objectif")
	if _, err := time.Parse("2006-01-02", objectif); objectif != "" && err != nil {
		m.a.Echec(w, r, http.StatusBadRequest, "Date d'objectif invalide.")
		return
	}
	var montant int64
	if b := strings.TrimSpace(r.PostFormValue("budget")); b != "" {
		v, err := budget.LireMontant(b)
		if err != nil || v < 0 {
			m.a.Echec(w, r, http.StatusBadRequest, "Budget invalide.")
			return
		}
		montant = v
	}
	couleur, _ := strconv.ParseInt(r.PostFormValue("couleur"), 10, 64)
	if couleur < 1 || couleur > 8 {
		couleur = 1
	}
	emoji := emojiValide(r.PostFormValue("emoji"))
	id := idDe(r)
	if r.PathValue("id") == "" {
		p, err := m.a.Q.CreerProjet(ctx, donnees.CreerProjetParams{FamilleID: c.Famille.ID, Nom: nom, Emoji: emoji, Couleur: couleur,
			Description: desc, Objectif: objectif, Budget: montant, CreeLe: time.Now().Unix()})
		if err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
		id = p.ID
		m.a.Publier(c, mod, "Nouveau projet : "+nom)
	} else {
		if _, ok := m.projet(r, c, id); !ok {
			m.a.Echec(w, r, http.StatusNotFound, "Projet introuvable.")
			return
		}
		if err := m.a.Q.ModifierProjet(ctx, donnees.ModifierProjetParams{Nom: nom, Emoji: emoji, Couleur: couleur, Description: desc,
			Objectif: objectif, Budget: montant, ID: id, FamilleID: c.Famille.ID}); err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
		m.a.Publier(c, mod, "")
	}
	m.a.Flash(w, "Projet enregistré "+emoji)
	m.a.Rediriger(w, r, fmt.Sprintf("/projets/%d", id))
}

// terminer : termine le projet, ou le rouvre.
func (m *Module) terminer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	p, ok := m.projet(r, c, idDe(r))
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Projet introuvable.")
		return
	}
	var le int64
	msg := "Projet rouvert"
	if p.TermineLe == 0 {
		le, msg = time.Now().Unix(), "Projet terminé, bravo ! 🎉"
	}
	if err := m.a.Q.TerminerProjet(r.Context(), donnees.TerminerProjetParams{TermineLe: le, ID: p.ID, FamilleID: c.Famille.ID}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, msg)
	m.a.Rediriger(w, r, fmt.Sprintf("/projets/%d", p.ID))
}

func (m *Module) supprimer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	p, ok := m.projet(r, c, idDe(r))
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Projet introuvable.")
		return
	}
	ctx := r.Context()
	// Les étapes partent avec le projet (ON DELETE CASCADE), le carnet à part.
	err := m.a.Transaction(ctx, func(q *donnees.Queries) error {
		if err := q.SupprimerProjet(ctx, donnees.SupprimerProjetParams{ID: p.ID, FamilleID: c.Famille.ID}); err != nil {
			return err
		}
		return m.s.SupprimerObjet(ctx, q, c.Famille.ID, mod, p.ID)
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Projet « "+p.Nom+" » supprimé")
	m.a.Rediriger(w, r, "/projets")
}

// ---------- Étapes ----------

func (m *Module) ajouterEtape(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	p, ok := m.projet(r, c, idDe(r))
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Projet introuvable.")
		return
	}
	texte := strings.TrimSpace(r.PostFormValue("texte"))
	if n := utf8.RuneCountInString(texte); n < 1 || n > 120 {
		m.a.Echec(w, r, http.StatusBadRequest, "L'étape doit faire entre 1 et 120 caractères.")
		return
	}
	jour := r.PostFormValue("jour")
	if _, err := time.Parse("2006-01-02", jour); jour != "" && err != nil {
		m.a.Echec(w, r, http.StatusBadRequest, "Date invalide.")
		return
	}
	var membre sql.NullInt64
	if id, _ := strconv.ParseInt(r.PostFormValue("membre"), 10, 64); id > 0 {
		// Seulement un membre de la famille.
		if _, err := m.a.Q.MembreParID(r.Context(), donnees.MembreParIDParams{ID: id, FamilleID: c.Famille.ID}); err == nil {
			membre = sql.NullInt64{Int64: id, Valid: true}
		}
	}
	if _, err := m.a.Q.AjouterEtape(r.Context(), donnees.AjouterEtapeParams{ProjetID: p.ID, FamilleID: c.Famille.ID, Texte: texte,
		Jour: jour, MembreID: membre, CreeLe: time.Now().Unix()}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Étape ajoutée ✓")
	m.a.Rediriger(w, r, fmt.Sprintf("/projets/%d", p.ID))
}

func (m *Module) etape(r *http.Request, c *appli.Contexte) (donnees.ProjetEtape, bool) {
	e, err := m.a.Q.EtapeParID(r.Context(), donnees.EtapeParIDParams{ID: idDe(r), FamilleID: c.Famille.ID})
	return e, err == nil
}

// cocher : faite ↔ à faire.
func (m *Module) cocher(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	e, ok := m.etape(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Étape introuvable.")
		return
	}
	arg := donnees.CocherEtapeParams{ID: e.ID, FamilleID: c.Famille.ID}
	msg := "À refaire"
	if e.FaiteLe == 0 {
		arg.FaiteLe, arg.FaitePar, msg = time.Now().Unix(), sql.NullInt64{Int64: c.Membre.ID, Valid: true}, "Étape faite ✓"
	}
	if err := m.a.Q.CocherEtape(r.Context(), arg); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, msg)
	m.a.Rediriger(w, r, fmt.Sprintf("/projets/%d", e.ProjetID))
}

func (m *Module) supprimerEtape(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	e, ok := m.etape(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Étape introuvable.")
		return
	}
	if err := m.a.Q.SupprimerEtape(r.Context(), donnees.SupprimerEtapeParams{ID: e.ID, FamilleID: c.Famille.ID}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Étape supprimée")
	m.a.Rediriger(w, r, fmt.Sprintf("/projets/%d", e.ProjetID))
}

// ---------- Accueil et rappels ----------

func (m *Module) resume(ctx context.Context, c *appli.Contexte) appli.Resume {
	l, err := m.a.Q.ProjetsFamille(ctx, c.Famille.ID)
	if err != nil {
		return appli.Resume{}
	}
	n := 0
	for _, p := range l {
		if p.TermineLe == 0 {
			n++
		}
	}
	if n == 0 {
		return appli.Resume{Info: "Aucun projet en cours"}
	}
	return appli.Resume{Info: appli.Pluriel(n, "projet en cours", "projets en cours")}
}

func (m *Module) echeances(ctx context.Context, f donnees.Famille, debut, fin time.Time) ([]appli.Echeance, error) {
	du, au := debut.Format("2006-01-02"), fin.Format("2006-01-02")
	etapes, err := m.a.Q.EtapesPeriode(ctx, donnees.EtapesPeriodeParams{FamilleID: f.ID, Du: du, Au: au})
	if err != nil {
		return nil, err
	}
	var out []appli.Echeance
	for _, e := range etapes {
		if j, err := time.ParseInLocation("2006-01-02", e.Jour, debut.Location()); err == nil {
			out = append(out, appli.Echeance{Jour: j, Titre: e.Texte + " · " + e.Projet, Emoji: e.Emoji,
				URL: fmt.Sprintf("/projets/%d", e.ProjetID), Module: mod})
		}
	}
	objectifs, err := m.a.Q.ObjectifsPeriode(ctx, donnees.ObjectifsPeriodeParams{FamilleID: f.ID, Du: du, Au: au})
	if err != nil {
		return nil, err
	}
	for _, p := range objectifs {
		if j, err := time.ParseInLocation("2006-01-02", p.Objectif, debut.Location()); err == nil {
			out = append(out, appli.Echeance{Jour: j, Titre: "Objectif : " + p.Nom, Emoji: p.Emoji,
				URL: fmt.Sprintf("/projets/%d", p.ID), Module: mod})
		}
	}
	return out, nil
}
