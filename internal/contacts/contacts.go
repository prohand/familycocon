// SPDX-License-Identifier: GPL-3.0-or-later

// Package contacts : deux modules sur les mêmes fiches.
//   - Adresses : le carnet de la famille (téléphone, e-mail, adresse) ;
//   - Anniversaires : les dates des proches et des membres, l'âge qu'ils
//     auront, des idées de cadeaux, et des rappels (accueil, notifications).
//
// Droits : tout le monde consulte, ajoute et corrige ; seuls les parents
// suppriment.
package contacts

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
)

// Module : Adresses et Anniversaires.
type Module struct{ a *appli.App }

const (
	modAdresses = "adresses"
	modAnniv    = "anniversaires"
)

// Relations proposées.
var Relations = []struct{ Cle, Nom, Emoji string }{
	{"famille", "Famille", "👪"}, {"ami", "Amis", "🤝"}, {"voisin", "Voisins", "🏘️"},
	{"ecole", "École", "🏫"}, {"travail", "Travail", "💼"}, {"autre", "Autres", "📇"},
}

func relationValide(r string) bool {
	for _, x := range Relations {
		if x.Cle == r {
			return true
		}
	}
	return false
}

// Brancher ajoute les routes des deux modules.
func Brancher(a *appli.App) {
	m := &Module{a: a}
	a.Route("GET /adresses", modAdresses, appli.Membre, m.pageAdresses)
	a.Route("GET /anniversaires", modAnniv, appli.Membre, m.pageAnniversaires)
	// Fiches : communes aux deux modules (l'un des deux doit être activé).
	a.Route("GET /contacts/nouveau", "", appli.Membre, m.formulaire)
	a.Route("POST /contacts", "", appli.Membre, m.creer)
	a.Route("GET /contacts/{id}", "", appli.Membre, m.fiche)
	a.Route("GET /contacts/{id}/modifier", "", appli.Membre, m.formulaire)
	a.Route("POST /contacts/{id}", "", appli.Membre, m.modifier)
	a.Route("POST /contacts/{id}/supprimer", "", appli.Parent, m.supprimer)

	a.AjouterResume(modAdresses, m.resumeAdresses)
	a.AjouterResume(modAnniv, m.resumeAnniv)
	a.AjouterEcheances(modAnniv, m.echeances)
	a.AjouterAction(appli.Ajout{Module: modAdresses, Emoji: "📇", Texte: "Un contact", URL: "/contacts/nouveau"})
	a.AjouterAction(appli.Ajout{Module: modAnniv, Emoji: "🎂", Texte: "Un anniversaire", URL: "/contacts/nouveau?anniv=1"})
}

// actif : l'un des deux modules est-il activé ?
func actif(c *appli.Contexte) bool { return c.Actif(modAdresses) || c.Actif(modAnniv) }

// --- Dates de naissance ---------------------------------------------------------

// Naissance : une date de naissance (année 0 : inconnue).
type Naissance struct {
	Annee, Mois, Jour int
}

// LireNaissance : « 2018-03-05 » ou « 0000-03-05 » (année inconnue).
func LireNaissance(s string) (Naissance, bool) {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return Naissance{}, false
	}
	a, e1 := strconv.Atoi(s[:4])
	m, e2 := strconv.Atoi(s[5:7])
	j, e3 := strconv.Atoi(s[8:])
	if e1 != nil || e2 != nil || e3 != nil || m < 1 || m > 12 || j < 1 || j > 31 {
		return Naissance{}, false
	}
	// Le jour doit exister (29 février accepté quand l'année est inconnue).
	ref := a
	if ref == 0 {
		ref = 2000
	}
	if time.Date(ref, time.Month(m), j, 0, 0, 0, 0, time.UTC).Day() != j {
		return Naissance{}, false
	}
	return Naissance{Annee: a, Mois: m, Jour: j}, true
}

// Prochain : le prochain anniversaire à partir du jour auj (inclus). Né un
// 29 février : fêté le 28 les années sans 29.
func (n Naissance) Prochain(auj time.Time) time.Time {
	date := func(annee int) time.Time {
		j := n.Jour
		if n.Mois == 2 && j == 29 && time.Date(annee, 2, 29, 0, 0, 0, 0, auj.Location()).Day() != 29 {
			j = 28
		}
		return time.Date(annee, time.Month(n.Mois), j, 0, 0, 0, 0, auj.Location())
	}
	d := date(auj.Year())
	if d.Before(auj) {
		d = date(auj.Year() + 1)
	}
	return d
}

// Age : l'âge atteint à la date d (0 si l'année est inconnue).
func (n Naissance) Age(d time.Time) int {
	if n.Annee == 0 {
		return 0
	}
	return d.Year() - n.Annee
}

// Anniversaire : un anniversaire à venir (contact ou membre).
type Anniversaire struct {
	Nom    string
	Date   time.Time
	Age    int
	Quand  string
	URL    string
	Membre *donnees.Membre // nil pour un contact
	Idees  string
	Proche bool
}

func (m *Module) anniversaires(ctx context.Context, f donnees.Famille, auj time.Time) ([]Anniversaire, error) {
	var out []Anniversaire
	contacts, err := m.a.Q.ContactsAvecNaissance(ctx, f.ID)
	if err != nil {
		return nil, err
	}
	for _, c := range contacts {
		n, ok := LireNaissance(c.Naissance)
		if !ok {
			continue
		}
		d := n.Prochain(auj)
		out = append(out, Anniversaire{Nom: c.Nom, Date: d, Age: n.Age(d), URL: fmt.Sprintf("/contacts/%d", c.ID), Idees: c.Idees})
	}
	membres, err := m.a.Q.MembresDeFamille(ctx, f.ID)
	if err != nil {
		return nil, err
	}
	for i := range membres {
		n, ok := LireNaissance(membres[i].Naissance)
		if !ok {
			continue
		}
		d := n.Prochain(auj)
		out = append(out, Anniversaire{Nom: membres[i].Nom, Date: d, Age: n.Age(d), URL: "/anniversaires", Membre: &membres[i]})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	for i := range out {
		out[i].Quand = appli.Quand(out[i].Date, auj)
		out[i].Proche = out[i].Date.Sub(auj) <= 7*24*time.Hour
	}
	return out, nil
}

func minuit(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func titreAnniv(a Anniversaire) string {
	if a.Age > 0 {
		return fmt.Sprintf("Anniversaire de %s (%d ans)", a.Nom, a.Age)
	}
	return "Anniversaire de " + a.Nom
}

// echeances : les anniversaires de la période (rappels).
func (m *Module) echeances(ctx context.Context, f donnees.Famille, debut, fin time.Time) ([]appli.Echeance, error) {
	l, err := m.anniversaires(ctx, f, debut)
	if err != nil {
		return nil, err
	}
	var out []appli.Echeance
	for _, a := range l {
		if a.Date.Before(fin) {
			out = append(out, appli.Echeance{Jour: a.Date, Titre: titreAnniv(a), Emoji: "🎂", URL: a.URL, Module: modAnniv})
		}
	}
	return out, nil
}

type moisAnniv struct {
	Mois  string
	Liste []Anniversaire
}

func (m *Module) pageAnniversaires(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	auj := minuit(c.Maintenant())
	l, err := m.anniversaires(r.Context(), c.Famille, auj)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	var parMois []moisAnniv
	for _, a := range l {
		nom := appli.MoisFR(a.Date.Month())
		nom = strings.ToUpper(nom[:1]) + nom[1:]
		if a.Date.Year() != auj.Year() {
			nom += " " + strconv.Itoa(a.Date.Year())
		}
		if len(parMois) == 0 || parMois[len(parMois)-1].Mois != nom {
			parMois = append(parMois, moisAnniv{Mois: nom})
		}
		parMois[len(parMois)-1].Liste = append(parMois[len(parMois)-1].Liste, a)
	}
	m.a.Afficher(w, r, c, "contacts/anniversaires", "Anniversaires", modAnniv, parMois)
}

func (m *Module) resumeAnniv(ctx context.Context, c *appli.Contexte) appli.Resume {
	auj := minuit(c.Maintenant())
	l, err := m.anniversaires(ctx, c.Famille, auj)
	if err != nil || len(l) == 0 {
		return appli.Resume{Info: "Aucune date"}
	}
	r := appli.Resume{Info: l[0].Nom + " : " + strings.ToLower(l[0].Quand[:1]) + l[0].Quand[1:]}
	if l[0].Proche {
		r.Badge = 1
	}
	return r
}

// --- Adresses -------------------------------------------------------------------

type groupe struct {
	Nom, Emoji string
	Contacts   []donnees.Contact
}

func (m *Module) pageAdresses(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	tous, err := m.a.Q.ContactsFamille(r.Context(), c.Famille.ID)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	var groupes []groupe
	for _, rel := range Relations {
		g := groupe{Nom: rel.Nom, Emoji: rel.Emoji}
		for _, ct := range tous {
			if ct.Relation != rel.Cle {
				continue
			}
			if q != "" && !strings.Contains(strings.ToLower(ct.Nom+" "+ct.Telephone+" "+ct.Email+" "+ct.Adresse), q) {
				continue
			}
			g.Contacts = append(g.Contacts, ct)
		}
		if len(g.Contacts) > 0 {
			groupes = append(groupes, g)
		}
	}
	m.a.Afficher(w, r, c, "contacts/adresses", "Adresses", modAdresses, map[string]any{"Groupes": groupes, "Recherche": q})
}

func (m *Module) resumeAdresses(ctx context.Context, c *appli.Contexte) appli.Resume {
	n, err := m.a.Q.CompterContacts(ctx, c.Famille.ID)
	if err != nil || n == 0 {
		return appli.Resume{Info: "Carnet vide"}
	}
	return appli.Resume{Info: appli.Pluriel(n, "contact", "contacts")}
}

// --- Fiches ---------------------------------------------------------------------

func (m *Module) contact(r *http.Request, c *appli.Contexte) (donnees.Contact, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return donnees.Contact{}, false
	}
	ct, err := m.a.Q.ContactParID(r.Context(), donnees.ContactParIDParams{ID: id, FamilleID: c.Famille.ID})
	return ct, err == nil
}

// lienTel : « 06 12 34 56 78 » → « tel:0612345678 » (chiffres et + seulement).
func lienTel(t string) string {
	var b strings.Builder
	for _, r := range t {
		if (r >= '0' && r <= '9') || (r == '+' && b.Len() == 0) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

type vueFiche struct {
	Contact   donnees.Contact
	Tel       string
	Carte     string // lien OpenStreetMap
	Relation  string
	Naissance string // « 5 mars 2018 (8 ans) »
	Adresses  bool   // module Adresses activé
	Anniv     bool
}

func (m *Module) fiche(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	if !actif(c) {
		m.a.PageErreur(w, r, c, http.StatusNotFound, "Module désactivé", "Ce module n'est pas activé pour votre famille.")
		return
	}
	ct, ok := m.contact(r, c)
	if !ok {
		m.a.PageErreur(w, r, c, http.StatusNotFound, "Contact introuvable", "Ce contact n'existe plus.")
		return
	}
	v := vueFiche{Contact: ct, Tel: lienTel(ct.Telephone), Adresses: c.Actif(modAdresses), Anniv: c.Actif(modAnniv)}
	if ct.Adresse != "" {
		v.Carte = "https://www.openstreetmap.org/search?query=" + strings.ReplaceAll(url.QueryEscape(strings.ReplaceAll(ct.Adresse, "\n", ", ")), "+", "%20")
	}
	for _, rel := range Relations {
		if rel.Cle == ct.Relation {
			v.Relation = rel.Emoji + " " + rel.Nom
		}
	}
	if n, ok := LireNaissance(ct.Naissance); ok {
		v.Naissance = fmt.Sprintf("%d %s", n.Jour, appli.MoisFR(time.Month(n.Mois)))
		if n.Annee > 0 {
			auj := minuit(c.Maintenant())
			age := auj.Year() - n.Annee
			if n.Prochain(auj).Year() == auj.Year() && !n.Prochain(auj).Equal(auj) {
				age-- // pas encore fêté cette année
			}
			v.Naissance += fmt.Sprintf(" %d (%d ans)", n.Annee, age)
		}
	}
	m.a.Afficher(w, r, c, "contacts/fiche", ct.Nom, modAdresses, v)
}

type formContact struct {
	Nouveau   bool
	Contact   donnees.Contact
	Relations any
	Date      string // AAAA-MM-JJ pour le champ date
	SansAnnee bool
	Anniv     bool // venu d'Anniversaires : la date d'abord
}

func (m *Module) formulaire(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	if !actif(c) {
		m.a.PageErreur(w, r, c, http.StatusNotFound, "Module désactivé", "Ce module n'est pas activé pour votre famille.")
		return
	}
	f := formContact{Nouveau: r.PathValue("id") == "", Relations: Relations, Anniv: r.URL.Query().Get("anniv") == "1" || !c.Actif(modAdresses)}
	if f.Nouveau {
		f.Contact = donnees.Contact{Relation: "famille"}
	} else {
		ct, ok := m.contact(r, c)
		if !ok {
			m.a.PageErreur(w, r, c, http.StatusNotFound, "Contact introuvable", "Ce contact n'existe plus.")
			return
		}
		f.Contact = ct
		if n, ok := LireNaissance(ct.Naissance); ok {
			f.SansAnnee = n.Annee == 0
			annee := n.Annee
			if annee == 0 {
				annee = 2000
			}
			f.Date = fmt.Sprintf("%04d-%02d-%02d", annee, n.Mois, n.Jour)
		}
	}
	titre := "Nouveau contact"
	if !f.Nouveau {
		titre = f.Contact.Nom
	}
	m.a.Afficher(w, r, c, "contacts/formulaire", titre, modAdresses, f)
}

type saisie struct {
	nom, relation, naissance, tel, email, adresse, notes, idees string
}

func lire(r *http.Request) (saisie, string) {
	s := saisie{
		nom: strings.TrimSpace(r.PostFormValue("nom")), relation: r.PostFormValue("relation"),
		tel: strings.TrimSpace(r.PostFormValue("telephone")), email: strings.TrimSpace(r.PostFormValue("email")),
		adresse: strings.TrimSpace(r.PostFormValue("adresse")), notes: strings.TrimSpace(r.PostFormValue("notes")),
		idees: strings.TrimSpace(r.PostFormValue("idees")),
	}
	if n := utf8.RuneCountInString(s.nom); n < 1 || n > 80 {
		return s, "Le nom doit faire entre 1 et 80 caractères."
	}
	if !relationValide(s.relation) {
		s.relation = "autre"
	}
	if utf8.RuneCountInString(s.tel) > 30 || utf8.RuneCountInString(s.email) > 120 || utf8.RuneCountInString(s.adresse) > 300 ||
		utf8.RuneCountInString(s.notes) > 2000 || utf8.RuneCountInString(s.idees) > 1000 {
		return s, "Un des champs est trop long."
	}
	if s.email != "" && (!strings.Contains(s.email, "@") || strings.ContainsAny(s.email, " <>\"")) {
		return s, "Adresse e-mail invalide."
	}
	if d := r.PostFormValue("naissance"); d != "" {
		t, err := time.Parse("2006-01-02", d)
		if err != nil {
			return s, "Date de naissance invalide."
		}
		if r.PostFormValue("sans_annee") == "1" {
			s.naissance = "0000" + d[4:]
		} else {
			if t.After(time.Now()) || t.Year() < 1900 {
				return s, "Date de naissance invalide."
			}
			s.naissance = d
		}
	}
	return s, ""
}

func (m *Module) creer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	if !actif(c) {
		m.a.Echec(w, r, http.StatusNotFound, "Module désactivé.")
		return
	}
	s, msg := lire(r)
	if msg != "" {
		m.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	t := time.Now().Unix()
	ct, err := m.a.Q.CreerContact(r.Context(), donnees.CreerContactParams{
		FamilleID: c.Famille.ID, Nom: s.nom, Relation: s.relation, Naissance: s.naissance, Telephone: s.tel,
		Email: s.email, Adresse: s.adresse, Notes: s.notes, Idees: s.idees, CreeLe: t, ModifieLe: t,
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Flash(w, "Contact enregistré ✓")
	if s.naissance != "" && r.PostFormValue("anniv") == "1" {
		m.a.Rediriger(w, r, "/anniversaires")
		return
	}
	m.a.Rediriger(w, r, fmt.Sprintf("/contacts/%d", ct.ID))
}

func (m *Module) modifier(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ct, ok := m.contact(r, c)
	if !ok || !actif(c) {
		m.a.Echec(w, r, http.StatusNotFound, "Contact introuvable.")
		return
	}
	s, msg := lire(r)
	if msg != "" {
		m.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	if err := m.a.Q.ModifierContact(r.Context(), donnees.ModifierContactParams{
		Nom: s.nom, Relation: s.relation, Naissance: s.naissance, Telephone: s.tel, Email: s.email, Adresse: s.adresse,
		Notes: s.notes, Idees: s.idees, ModifieLe: time.Now().Unix(), ID: ct.ID, FamilleID: c.Famille.ID,
	}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Flash(w, "Contact enregistré ✓")
	m.a.Rediriger(w, r, fmt.Sprintf("/contacts/%d", ct.ID))
}

func (m *Module) supprimer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ct, ok := m.contact(r, c)
	if !ok || !actif(c) {
		m.a.Echec(w, r, http.StatusNotFound, "Contact introuvable.")
		return
	}
	if err := m.a.Q.SupprimerContact(r.Context(), donnees.SupprimerContactParams{ID: ct.ID, FamilleID: c.Famille.ID}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Flash(w, ct.Nom+" supprimé(e)")
	if c.Actif(modAdresses) {
		m.a.Rediriger(w, r, "/adresses")
		return
	}
	m.a.Rediriger(w, r, "/anniversaires")
}
