// SPDX-License-Identifier: GPL-3.0-or-later

// Package agenda : agenda partagé de la famille, vue semaine et vue mois.
// Chaque événement concerne un ou plusieurs membres (pastilles à leur
// couleur) ou toute la famille. Répétition possible chaque semaine (sport,
// musique) ou chaque année (anniversaires).
//
// Droits : tout le monde crée ; on modifie ses propres événements (un
// parent modifie tout) ; seuls les parents suppriment.
package agenda

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"familycocon/internal/appli"
	"familycocon/internal/contacts"
	"familycocon/internal/donnees"
	"familycocon/internal/repetition"
)

// Module : le module Agenda.
type Module struct {
	a *appli.App
	// aujourdhui : occurrences du jour, gardées le temps d'une requête
	// (l'accueil les demande deux fois : tuile et bloc).
	aujourdhui appli.MemoRequete[[]Occurrence]
}

// evenementsMax : événements par famille.
const evenementsMax = 20000

// contactsMax : contacts cités par un événement.
const contactsMax = 10

// idsMax : au-delà, on ne passe plus la liste des événements à SQLite
// (nombre de paramètres) : on lit ceux de toute la famille.
const idsMax = 500

const mod = "agenda"

// Brancher ajoute les routes du module.
func Brancher(a *appli.App) {
	m := &Module{a: a}
	a.Route("GET /agenda", mod, appli.Membre, m.page)
	a.Route("GET /agenda/contenu", mod, appli.Membre, m.contenu)
	a.Route("GET /agenda/nouveau", mod, appli.Membre, m.formulaire)
	a.Route("GET /agenda/calendrier", mod, appli.Membre, m.pageCalendrier)
	a.Route("POST /agenda/calendrier", mod, appli.Membre, m.creerLien)
	a.Route("POST /agenda/calendrier/couper", mod, appli.Membre, m.couperLien)
	// Flux iCalendar : sans session, le lien secret suffit.
	a.Mux.HandleFunc("GET /ical/{jeton}", m.flux)
	a.Route("POST /agenda", mod, appli.Membre, m.creer)
	a.Route("GET /agenda/{id}", mod, appli.Membre, m.formulaire)
	a.Route("POST /agenda/{id}", mod, appli.Membre, m.modifier)
	a.Route("POST /agenda/{id}/supprimer", mod, appli.Parent, m.supprimer)
	a.Route("POST /agenda/{id}/sauter", mod, appli.Membre, m.sauter)
	a.Route("POST /agenda/{id}/retablir", mod, appli.Membre, m.retablir)

	a.AjouterResume(mod, m.resume)
	a.AjouterEcheances(mod, m.echeances)
	a.AjouterRecherche(mod, m.chercher)
	a.AjouterBloc(appli.BlocAccueil{Module: mod, Ordre: 1, Rendre: m.blocAccueil})
	a.AjouterAction(appli.Ajout{Module: mod, Emoji: "🗓️", Texte: "Un événement", URL: "/agenda/nouveau"})
	a.AjouterPartage(appli.Partage{Module: mod, Emoji: "🗓️", Texte: "À l'agenda", URL: "/agenda/nouveau"})
}

// Occurrence : un événement à une date (un événement répété en a plusieurs).
type Occurrence struct {
	Evt      donnees.Evenement
	Debut    time.Time
	Fin      time.Time
	Membres  []*donnees.Membre // vide : toute la famille
	Contacts []ContactEvt      // contacts du carnet cités (dentiste, Léo…)
}

// ContactEvt : un contact cité par un événement.
type ContactEvt struct {
	ID       int64
	Nom, Tel string
}

// Lien : la fiche de l'événement ; pour un événement qui revient, avec la
// date de cette fois-ci (retour à la bonne semaine, « annuler cette fois »).
func (o Occurrence) Lien() string {
	u := "/agenda/" + strconv.FormatInt(o.Evt.ID, 10)
	if o.Evt.Repetition != "" {
		u += "?jour=" + repetition.CleJour(o.Debut)
	}
	return u
}

// Heure : « 10:00 », « Journée ».
func (o Occurrence) Heure() string {
	if o.Evt.Journee == 1 {
		return "Journée"
	}
	return o.Debut.Format("15:04")
}

// HeureFin : « 11:30 » (vide pour un événement d'une journée).
func (o Occurrence) HeureFin() string {
	if o.Evt.Journee == 1 || o.Fin.Equal(o.Debut) {
		return ""
	}
	return o.Fin.Format("15:04")
}

// chevauche : l'occurrence touche-t-elle la période [debut, fin[ ?
func chevauche(d, f, debut, fin time.Time) bool {
	return d.Before(fin) && (f.After(debut) || !d.Before(debut))
}

// carnetActif : un module montre-t-il le carnet (contacts cités) ?
func carnetActif(c *appli.Contexte) bool { return c.Actif("contacts") || c.Actif("adresses") }

// contactsParEvenement : les contacts cités par ces événements (ids déjà
// vérifiés : ils viennent d'une requête filtrée par famille).
func (m *Module) contactsParEvenement(ctx context.Context, c *appli.Contexte, ids []int64) (map[int64][]ContactEvt, error) {
	out := map[int64][]ContactEvt{}
	if len(ids) == 0 {
		return out, nil
	}
	if len(ids) > idsMax {
		l, err := m.a.Q.ContactsEvenementsFamille(ctx, c.Famille.ID)
		if err != nil {
			return nil, err
		}
		for _, x := range l {
			out[x.EvenementID] = append(out[x.EvenementID], ContactEvt{ID: x.ID, Nom: x.Nom, Tel: contacts.LienTel(x.Telephone)})
		}
		return out, nil
	}
	l, err := m.a.Q.ContactsDesEvenements(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, x := range l {
		out[x.EvenementID] = append(out[x.EvenementID], ContactEvt{ID: x.ID, Nom: x.Nom, Tel: contacts.LienTel(x.Telephone)})
	}
	return out, nil
}

// participants : les membres concernés par ces événements (même règle).
func (m *Module) participants(ctx context.Context, c *appli.Contexte, ids []int64) ([]donnees.EvenementsMembre, error) {
	switch {
	case len(ids) == 0:
		return nil, nil
	case len(ids) > idsMax:
		return m.a.Q.ParticipantsDeFamille(ctx, c.Famille.ID)
	}
	return m.a.Q.ParticipantsDesEvenements(ctx, ids)
}

// Occurrences : les occurrences de la période [debut, fin[, triées.
func (m *Module) Occurrences(ctx context.Context, c *appli.Contexte, debut, fin time.Time) ([]Occurrence, error) {
	// Un événement répété dont la fin de répétition est passée n'est plus lu.
	// Marge d'un an : une occurrence de plusieurs jours commencée avant la
	// fin peut encore toucher la période (deplier fait le tri exact).
	evts, err := m.a.Q.EvenementsPeriode(ctx, donnees.EvenementsPeriodeParams{FamilleID: c.Famille.ID, FinPeriode: fin.Unix(),
		DebutPeriode: debut.Unix(), JourMin: repetition.CleJour(debut.AddDate(-1, 0, -1))})
	if err != nil {
		return nil, err
	}
	// Participants et contacts : seulement ceux des événements lus.
	ids := make([]int64, len(evts))
	for i := range evts {
		ids[i] = evts[i].ID
	}
	parts, err := m.participants(ctx, c, ids)
	if err != nil {
		return nil, err
	}
	membres, err := m.a.Membres(ctx, c)
	if err != nil {
		return nil, err
	}
	parID := appli.MembresParID(membres)
	parEvt := map[int64][]*donnees.Membre{}
	for _, p := range parts {
		if mb := parID[p.MembreID]; mb != nil {
			parEvt[p.EvenementID] = append(parEvt[p.EvenementID], mb)
		}
	}
	// Contacts cités : seulement si un module montre le carnet.
	cites := map[int64][]ContactEvt{}
	if carnetActif(c) {
		if cites, err = m.contactsParEvenement(ctx, c, ids); err != nil {
			return nil, err
		}
	}
	annulees, err := m.exceptions(ctx, ids)
	if err != nil {
		return nil, err
	}
	var out []Occurrence
	for _, e := range evts {
		for _, dates := range deplierSans(e, c.Lieu, debut, fin, annulees[e.ID]) {
			out = append(out, Occurrence{Evt: e, Debut: dates[0], Fin: dates[1], Membres: parEvt[e.ID], Contacts: cites[e.ID]})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Evt.Journee != out[j].Evt.Journee {
			return out[i].Evt.Journee == 1 // les journées entières d'abord
		}
		return out[i].Debut.Before(out[j].Debut)
	})
	return out, nil
}

// minuit : le jour t à 00:00.
func minuit(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// duJour : occurrences qui touchent le jour j.
func duJour(occ []Occurrence, j time.Time) []Occurrence {
	var out []Occurrence
	fin := j.AddDate(0, 0, 1)
	for _, o := range occ {
		if chevauche(o.Debut, o.Fin, j, fin) {
			out = append(out, o)
		}
	}
	return out
}

// Jour : une case de la vue (semaine ou mois).
type Jour struct {
	Date        time.Time
	Cle         string // 2026-11-22
	Titre       string // « Lundi 16 »
	Aujourdhui  bool
	DansMois    bool
	Occurrences []Occurrence
	Plus        int                 // vue mois : événements non montrés
	Elements    []appli.ElementJour // tâches du jour (en petit)
}

type vueAgenda struct {
	Vue        string // semaine | mois
	Titre      string // « Semaine du 16 au 22 novembre », « Novembre 2026 »
	Precedent  string // date (2006-01-02) de la période précédente
	Suivant    string
	Jours      []Jour // semaine : 7 jours ; mois : 5 ou 6 semaines de 7 jours
	Choisi     *Jour  // vue mois : le jour touché
	NomsJours  []string
	Aujourdhui string
	Ref        string // date de référence de la période affichée
	Taches     string // tous | moi | non : tâches montrées sous les événements
	AvecTaches bool   // le module Tâches est activé (le filtre est proposé)
}

// filtreTaches : réglage « tâches dans l'agenda », gardé par le navigateur.
func (m *Module) filtreTaches(w http.ResponseWriter, r *http.Request) string {
	if v := r.URL.Query().Get("taches"); v == "tous" || v == "moi" || v == "non" {
		if w != nil {
			m.a.PoserPreference(w, "agenda_taches", v)
		}
		return v
	}
	if v := m.a.LirePreference(r, "agenda_taches"); v == "moi" || v == "non" {
		return v
	}
	return "tous"
}

func (m *Module) vue(ctx context.Context, c *appli.Contexte, r *http.Request, taches string) (*vueAgenda, error) {
	q := r.URL.Query()
	maintenant := c.Maintenant()
	aujourdhui := minuit(maintenant)
	ref := aujourdhui
	if d, err := time.ParseInLocation("2006-01-02", q.Get("date"), c.Lieu); err == nil {
		ref = d
	}
	v := &vueAgenda{Vue: "semaine", NomsJours: repetition.NomsJours[1:], Aujourdhui: repetition.CleJour(aujourdhui),
		Taches: taches, AvecTaches: c.Actif("taches")}
	if q.Get("vue") == "mois" {
		v.Vue = "mois"
	}
	v.Ref = repetition.CleJour(ref)
	var debut, fin time.Time
	if v.Vue == "semaine" {
		debut = repetition.Lundi(ref)
		fin = debut.AddDate(0, 0, 7)
		dimanche := fin.AddDate(0, 0, -1)
		if debut.Month() == dimanche.Month() {
			v.Titre = fmt.Sprintf("Du %d au %d %s", debut.Day(), dimanche.Day(), appli.MoisFR(dimanche.Month()))
		} else {
			v.Titre = fmt.Sprintf("Du %d %s au %d %s", debut.Day(), appli.MoisFR(debut.Month()), dimanche.Day(), appli.MoisFR(dimanche.Month()))
		}
		v.Precedent = repetition.CleJour(debut.AddDate(0, 0, -7))
		v.Suivant = repetition.CleJour(fin)
	} else {
		premier := time.Date(ref.Year(), ref.Month(), 1, 0, 0, 0, 0, c.Lieu)
		debut = repetition.Lundi(premier)
		dernier := premier.AddDate(0, 1, -1)
		fin = repetition.Lundi(dernier).AddDate(0, 0, 7)
		mois := appli.MoisFR(premier.Month())
		v.Titre = strings.ToUpper(mois[:1]) + mois[1:] + " " + strconv.Itoa(premier.Year()) // les mois commencent par une lettre sans accent
		v.Precedent = repetition.CleJour(premier.AddDate(0, -1, 0))
		v.Suivant = repetition.CleJour(premier.AddDate(0, 1, 0))
	}
	occ, err := m.Occurrences(ctx, c, debut, fin)
	if err != nil {
		return nil, err
	}
	// Tâches (et autres éléments discrets) par jour, selon le filtre.
	elements := map[string][]appli.ElementJour{}
	if taches != "non" {
		for _, e := range m.a.ElementsCalendrier(ctx, c, debut, fin) {
			if taches == "moi" && (e.Membre != nil && e.Membre.ID != c.Membre.ID || e.PourParents && !c.EstParent()) {
				continue
			}
			elements[e.Jour] = append(elements[e.Jour], e)
		}
	}
	choisi := q.Get("jour")
	if choisi == "" {
		choisi = v.Aujourdhui
	}
	for j := debut; j.Before(fin); j = j.AddDate(0, 0, 1) {
		cle := repetition.CleJour(j)
		jour := Jour{Date: j, Cle: cle, Titre: appli.TitreJour(j), Aujourdhui: j.Equal(aujourdhui),
			DansMois: v.Vue == "semaine" || j.Month() == ref.Month(), Occurrences: duJour(occ, j), Elements: elements[cle]}
		v.Jours = append(v.Jours, jour)
	}
	if v.Vue == "mois" {
		for i := range v.Jours {
			if v.Jours[i].Cle == choisi && v.Jours[i].DansMois {
				copie := v.Jours[i]
				v.Choisi = &copie
			}
			if n := len(v.Jours[i].Occurrences); n > 3 {
				v.Jours[i].Plus = n - 3
				v.Jours[i].Occurrences = v.Jours[i].Occurrences[:3]
			}
		}
	}
	return v, nil
}

func (m *Module) page(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	v, err := m.vue(r.Context(), c, r, m.filtreTaches(w, r))
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Afficher(w, r, c, "agenda/agenda", "Agenda", mod, v)
}

func (m *Module) contenu(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	v, err := m.vue(r.Context(), c, r, m.filtreTaches(nil, r))
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Fragment(w, r, c, "agenda/agenda", "agenda", v)
}

// formEvt : données du formulaire.
type formEvt struct {
	Nouveau    bool
	Modifiable bool
	Evt        donnees.Evenement
	Date       string
	DateFin    string
	HeureDebut string
	HeureFin   string
	Membres    []donnees.Membre
	Choisis    map[int64]bool
	Choix      []ChoixRepetition
	Regle      regle  // répétition actuelle (cases des jours)
	TexteRep   string // « Chaque 2e mardi du mois »
	Semaine    []caseJour
	Contacts   []donnees.ContactsChoixAgendaRow // carnet (liste de choix : nom et métier)
	Cites      map[int64]bool                   // contacts cités par l'événement
	ContactsEv []ContactEvt
	Jour       string // événement qui revient, ouvert depuis une de ses dates (2026-12-01)
	JourTitre  string // « Mardi 1er décembre »
	Retour     string // date de la semaine où revenir
	Annulees   []jourAnnule
}

// jourAnnule : une fois annulée d'un événement qui revient.
type jourAnnule struct {
	Cle, Titre string
}

// caseJour : une case « lun … dim » de la répétition « certains jours ».
type caseJour struct {
	N     int
	Nom   string
	Coche bool
}

func (m *Module) evenement(r *http.Request, c *appli.Contexte) (donnees.Evenement, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return donnees.Evenement{}, sql.ErrNoRows
	}
	return m.a.Q.EvenementParID(r.Context(), donnees.EvenementParIDParams{ID: id, FamilleID: c.Famille.ID})
}

func modifiable(c *appli.Contexte, e donnees.Evenement) bool {
	return appli.PeutModifier(c, e.CreePar.Int64, e.CreePar.Valid)
}

func (m *Module) formulaire(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	f := &formEvt{Nouveau: r.PathValue("id") == "", Modifiable: true, Choisis: map[int64]bool{}, Choix: Choix, Cites: map[int64]bool{}}
	var err error
	if carnetActif(c) {
		if f.Contacts, err = m.a.Q.ContactsChoixAgenda(ctx, c.Famille.ID); err != nil {
			m.a.Journal.Warn("agenda : carnet illisible", "erreur", err) // le formulaire marche sans
		}
	}
	if f.Membres, err = m.a.Membres(ctx, c); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	if f.Nouveau {
		jour := minuit(c.Maintenant())
		if d, err := time.ParseInLocation("2006-01-02", r.URL.Query().Get("date"), c.Lieu); err == nil {
			jour = d
		}
		f.Date, f.DateFin, f.HeureDebut, f.HeureFin = repetition.CleJour(jour), repetition.CleJour(jour), "", ""
		// Venu de « Partager » : titre et note préremplis.
		f.Evt.Titre, f.Evt.Note = appli.LirePrerempli(r, 80, 500)
	} else {
		e, err := m.evenement(r, c)
		if err != nil {
			m.a.PageErreur(w, r, c, http.StatusNotFound, "Événement introuvable", "Cet événement n'existe plus.")
			return
		}
		f.Evt, f.Modifiable = e, modifiable(c, e)
		f.Regle, f.TexteRep = lireRegle(e.Repetition), TexteRepetition(e, c.Lieu)
		if f.Contacts != nil {
			cites, err := m.contactsParEvenement(ctx, c, []int64{e.ID})
			if err != nil {
				m.a.ErreurInterne(w, r, err)
				return
			}
			f.ContactsEv = cites[e.ID]
			for _, x := range f.ContactsEv {
				f.Cites[x.ID] = true
			}
		}
		d, fin := c.Heure(e.Debut), c.Heure(e.Fin)
		f.Date, f.DateFin = repetition.CleJour(d), repetition.CleJour(fin)
		if e.Journee == 0 {
			f.HeureDebut, f.HeureFin = d.Format("15:04"), fin.Format("15:04")
		}
		parts, err := m.participants(ctx, c, []int64{e.ID})
		if err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
		for _, p := range parts {
			f.Choisis[p.MembreID] = true
		}
		if e.Repetition != "" {
			jours, err := m.a.Q.ExceptionsEvenement(ctx, e.ID)
			if err != nil {
				m.a.ErreurInterne(w, r, err)
				return
			}
			for _, j := range jours {
				if d, err := time.ParseInLocation("2006-01-02", j, c.Lieu); err == nil && !d.Before(minuit(c.Maintenant())) {
					f.Annulees = append(f.Annulees, jourAnnule{Cle: j, Titre: appli.DateLongue(d)})
				}
			}
			if j, err := time.ParseInLocation("2006-01-02", r.URL.Query().Get("jour"), c.Lieu); err == nil {
				f.Jour, f.JourTitre = repetition.CleJour(j), appli.DateLongue(j)
			}
		}
	}
	f.Retour = f.Date
	if f.Jour != "" {
		f.Retour = f.Jour
	}
	for i := 1; i <= 7; i++ {
		f.Semaine = append(f.Semaine, caseJour{N: i, Nom: repetition.NomsJours[i], Coche: f.Regle.Jours[i]})
	}
	titre := "Nouvel événement"
	if !f.Nouveau {
		titre = f.Evt.Titre
	}
	m.a.Afficher(w, r, c, "agenda/evenement", titre, mod, f)
}

// saisie : un événement lu dans le formulaire.
type saisie struct {
	titre, lieu, note, repetition string
	repetitionFin                 string
	debut, fin                    time.Time
	journee                       bool
	rappel                        bool
	membres                       []int64
	contacts                      []int64
	// carnet : le formulaire proposait-il les contacts ? Sinon (carnet
	// désactivé) les contacts déjà cités ne sont pas touchés.
	carnet bool
}

func (m *Module) lire(r *http.Request, c *appli.Contexte) (saisie, string) {
	var s saisie
	if err := r.ParseForm(); err != nil {
		return s, "Formulaire invalide."
	}
	f := r.PostForm
	s.titre = strings.TrimSpace(f.Get("titre"))
	s.lieu = strings.TrimSpace(f.Get("lieu"))
	s.note = strings.TrimSpace(f.Get("note"))
	s.rappel = f.Get("rappel") == "1"
	if n := utf8.RuneCountInString(s.titre); n < 1 || n > 80 {
		return s, "Le titre doit faire entre 1 et 80 caractères."
	}
	if utf8.RuneCountInString(s.lieu) > 80 || utf8.RuneCountInString(s.note) > 500 {
		return s, "Lieu (80 caractères) ou note (500) trop long."
	}
	rg := lireRegle(f.Get("repetition"))
	if rg.Type == "jours" {
		for i := 1; i <= 7; i++ {
			rg.Jours[i] = f.Get("rjour"+strconv.Itoa(i)) == "1"
		}
		if rg.String() == "jours:" {
			return s, "Cochez au moins un jour de la semaine."
		}
	}
	s.repetition = rg.String()
	jour, err := time.ParseInLocation("2006-01-02", f.Get("date"), c.Lieu)
	if err != nil || jour.Year() < 1900 || jour.Year() > 2200 {
		return s, "Date invalide."
	}
	jourFin := jour
	if f.Get("date_fin") != "" {
		if jourFin, err = time.ParseInLocation("2006-01-02", f.Get("date_fin"), c.Lieu); err != nil || jourFin.Before(jour) {
			return s, "La date de fin doit être après le début."
		}
	}
	if s.repetition != "" && f.Get("repetition_fin") != "" {
		rf, err := time.ParseInLocation("2006-01-02", f.Get("repetition_fin"), c.Lieu)
		if err != nil || rf.Before(jour) {
			return s, "La fin de la répétition doit être après la date de l'événement."
		}
		s.repetitionFin = repetition.CleJour(rf)
	}
	s.journee = f.Get("journee") == "1" || f.Get("heure_debut") == ""
	if s.journee {
		s.debut = jour
		s.fin = jourFin.AddDate(0, 0, 1).Add(-time.Second)
	} else {
		hd, err1 := time.Parse("15:04", f.Get("heure_debut"))
		s.debut = time.Date(jour.Year(), jour.Month(), jour.Day(), hd.Hour(), hd.Minute(), 0, 0, c.Lieu)
		// Sans heure de fin : une heure, le dernier jour choisi.
		s.fin = time.Date(jourFin.Year(), jourFin.Month(), jourFin.Day(), hd.Hour(), hd.Minute(), 0, 0, c.Lieu).Add(time.Hour)
		var err2 error
		if f.Get("heure_fin") != "" {
			var hf time.Time
			hf, err2 = time.Parse("15:04", f.Get("heure_fin"))
			s.fin = time.Date(jourFin.Year(), jourFin.Month(), jourFin.Day(), hf.Hour(), hf.Minute(), 0, 0, c.Lieu)
		}
		if err1 != nil || err2 != nil {
			return s, "Heure invalide."
		}
		if s.fin.Before(s.debut) {
			return s, "La fin doit être après le début."
		}
	}
	if jourFin.Year() > 2200 {
		return s, "Date de fin invalide."
	}
	// Un événement répété ne dure pas plus longtemps que l'écart entre deux
	// répétitions (sinon il se chevauche lui-même à l'infini).
	if max := dureeMax(rg.Type); s.repetition != "" && s.fin.Sub(s.debut) > max {
		return s, "Un événement qui revient ne peut pas durer plus longtemps que l'écart entre deux fois."
	}
	membres, err := m.a.Membres(r.Context(), c)
	if err != nil {
		return s, "Erreur interne."
	}
	for _, mb := range membres {
		if f.Get("membre_"+strconv.FormatInt(mb.ID, 10)) == "1" {
			s.membres = append(s.membres, mb.ID)
		}
	}
	// Contacts cités : seulement ceux de la famille, 10 au plus. La liste
	// reçue est bornée AVANT la boucle (une requête par valeur).
	s.carnet = carnetActif(c)
	if len(f["contacts"]) > contactsMax {
		return s, fmt.Sprintf("%d contacts au plus par événement.", contactsMax)
	}
	if s.carnet {
		vus := map[int64]bool{}
		for _, v := range f["contacts"] {
			id, err := strconv.ParseInt(v, 10, 64)
			if err != nil || id <= 0 || vus[id] {
				continue
			}
			vus[id] = true
			if _, err := m.a.Q.ContactParID(r.Context(), donnees.ContactParIDParams{ID: id, FamilleID: c.Famille.ID}); err == nil {
				s.contacts = append(s.contacts, id)
			}
		}
	}
	return s, ""
}

// errAgendaPlein : plafond d'événements atteint.
var errAgendaPlein = errors.New("agenda plein")

func bool01(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func enregistrerContacts(ctx context.Context, q *donnees.Queries, evt int64, contacts []int64) error {
	if err := q.ViderContactsEvenement(ctx, evt); err != nil {
		return err
	}
	for _, id := range contacts {
		if err := q.AjouterContactEvenement(ctx, donnees.AjouterContactEvenementParams{EvenementID: evt, ContactID: id}); err != nil {
			return err
		}
	}
	return nil
}

func (m *Module) enregistrerMembres(ctx context.Context, q *donnees.Queries, evt int64, membres []int64) error {
	if err := q.ViderParticipants(ctx, evt); err != nil {
		return err
	}
	for _, id := range membres {
		if err := q.AjouterParticipant(ctx, donnees.AjouterParticipantParams{EvenementID: evt, MembreID: id}); err != nil {
			return err
		}
	}
	return nil
}

func (m *Module) creer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	s, msg := m.lire(r, c)
	if msg != "" {
		m.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	var e donnees.Evenement
	err := m.a.Transaction(ctx, func(q *donnees.Queries) error {
		n, err := q.CompterEvenements(ctx, c.Famille.ID)
		if err != nil {
			return err
		}
		if n >= evenementsMax {
			return errAgendaPlein
		}
		e, err = q.CreerEvenement(ctx, donnees.CreerEvenementParams{
			FamilleID: c.Famille.ID, Titre: s.titre, Lieu: s.lieu, Note: s.note, Debut: s.debut.Unix(), Fin: s.fin.Unix(),
			Journee: bool01(s.journee), Repetition: s.repetition, RepetitionFin: s.repetitionFin, Rappel: bool01(s.rappel), CreePar: sql.NullInt64{Int64: c.Membre.ID, Valid: true}, CreeLe: time.Now().Unix(),
		})
		if err != nil {
			return err
		}
		if err := m.enregistrerMembres(ctx, q, e.ID, s.membres); err != nil {
			return err
		}
		return enregistrerContacts(ctx, q, e.ID, s.contacts)
	})
	if errors.Is(err, errAgendaPlein) {
		m.a.Echec(w, r, http.StatusBadRequest, fmt.Sprintf("L'agenda est plein (%d événements) : demandez à un parent d'en supprimer.", evenementsMax))
		return
	}
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	quand := strings.ToLower(appli.TitreJour(s.debut))
	// Pour certains membres seulement : eux seuls sont notifiés.
	m.a.PublierPour(c, mod, fmt.Sprintf("%s a ajouté « %s » (%s) 🗓️", c.Membre.Nom, e.Titre, quand), s.membres)
	m.a.Flash(w, "Événement ajouté ✓")
	m.a.Rediriger(w, r, "/agenda?date="+repetition.CleJour(s.debut))
}

func (m *Module) modifier(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	e, err := m.evenement(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Événement introuvable.")
		return
	}
	if !modifiable(c, e) {
		m.a.Echec(w, r, http.StatusForbidden, "Seul un parent (ou qui l'a créé) peut le modifier.")
		return
	}
	s, msg := m.lire(r, c)
	if msg != "" {
		m.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	err = m.a.Transaction(ctx, func(q *donnees.Queries) error {
		if err := q.ModifierEvenement(ctx, donnees.ModifierEvenementParams{
			Titre: s.titre, Lieu: s.lieu, Note: s.note, Debut: s.debut.Unix(), Fin: s.fin.Unix(),
			Journee: bool01(s.journee), Repetition: s.repetition, RepetitionFin: s.repetitionFin, Rappel: bool01(s.rappel), ID: e.ID, FamilleID: c.Famille.ID,
		}); err != nil {
			return err
		}
		if err := m.enregistrerMembres(ctx, q, e.ID, s.membres); err != nil {
			return err
		}
		if !s.carnet { // carnet désactivé : les contacts cités restent tels quels
			return nil
		}
		return enregistrerContacts(ctx, q, e.ID, s.contacts)
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Événement enregistré ✓")
	retour := repetition.CleJour(s.debut)
	if j, err := time.ParseInLocation("2006-01-02", r.PostFormValue("retour"), c.Lieu); err == nil && s.repetition != "" {
		retour = repetition.CleJour(j) // ouvert depuis une de ses dates : on y revient
	}
	m.a.Rediriger(w, r, "/agenda?date="+retour)
}

func (m *Module) supprimer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	e, err := m.evenement(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Événement introuvable.")
		return
	}
	if err := m.a.Q.SupprimerEvenement(r.Context(), donnees.SupprimerEvenementParams{ID: e.ID, FamilleID: c.Famille.ID}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Événement « "+e.Titre+" » supprimé")
	m.a.Rediriger(w, r, "/agenda")
}

// duJourMeme : les occurrences d'aujourd'hui (calculées une fois par requête).
func (m *Module) duJourMeme(ctx context.Context, c *appli.Contexte) ([]Occurrence, error) {
	return m.aujourdhui.Lire(c, func() ([]Occurrence, error) {
		j := minuit(c.Maintenant())
		return m.Occurrences(ctx, c, j, j.AddDate(0, 0, 1))
	})
}

func (m *Module) resume(ctx context.Context, c *appli.Contexte) appli.Resume {
	occ, err := m.duJourMeme(ctx, c)
	if err != nil || len(occ) == 0 {
		return appli.Resume{Info: "Rien aujourd'hui"}
	}
	return appli.Resume{Info: appli.Pluriel(len(occ), "événement aujourd'hui", "événements aujourd'hui"), Badge: len(occ)}
}

func (m *Module) blocAccueil(ctx context.Context, c *appli.Contexte) (template.HTML, error) {
	occ, err := m.duJourMeme(ctx, c)
	if err != nil || len(occ) == 0 {
		return "", err
	}
	return m.a.RendreBloc("agenda/agenda", "accueil-agenda", c, occ)
}

// dureeMax : la durée la plus longue d'un événement selon sa répétition.
func dureeMax(t string) time.Duration {
	jour := 24 * time.Hour
	switch t {
	case "jour", "jours":
		return jour
	case "semaine":
		return 7 * jour
	case "2semaines":
		return 14 * jour
	case "mois", "mois-rang":
		return 28 * jour
	case "2mois":
		return 59 * jour
	case "trimestre":
		return 89 * jour
	case "semestre":
		return 181 * jour
	}
	return 365 * jour
}

// echeances : les événements « à rappeler » (case cochée), pour le bloc
// « À venir » de l'accueil et la notification du matin. Seul le premier
// jour d'un événement de plusieurs jours est rappelé.
func (m *Module) echeances(ctx context.Context, f donnees.Famille, debut, fin time.Time) ([]appli.Echeance, error) {
	evts, err := m.a.Q.EvenementsPeriode(ctx, donnees.EvenementsPeriodeParams{FamilleID: f.ID, FinPeriode: fin.Unix(),
		DebutPeriode: debut.Unix(), JourMin: repetition.CleJour(debut.AddDate(-1, 0, -1))})
	if err != nil {
		return nil, err
	}
	var ids []int64
	for _, e := range evts {
		if e.Rappel == 1 {
			ids = append(ids, e.ID)
		}
	}
	annulees, err := m.exceptions(ctx, ids)
	if err != nil {
		return nil, err
	}
	var out []appli.Echeance
	for _, e := range evts {
		if e.Rappel == 0 {
			continue
		}
		for _, dates := range deplierSans(e, debut.Location(), debut, fin, annulees[e.ID]) {
			if dates[0].Before(debut) {
				continue
			}
			o := Occurrence{Evt: e, Debut: dates[0], Fin: dates[1]}
			titre := e.Titre
			if e.Journee == 0 {
				titre += " à " + strings.Replace(o.Heure(), ":", " h ", 1)
			}
			out = append(out, appli.Echeance{Jour: minuit(dates[0]), Titre: titre, Emoji: "🗓️", URL: o.Lien(), Module: mod})
		}
	}
	return out, nil
}
