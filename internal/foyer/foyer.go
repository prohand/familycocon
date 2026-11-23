// SPDX-License-Identifier: GPL-3.0-or-later

// Package foyer : trois modules bâtis sur le même modèle (liste, fiche,
// échéances et carnet via le paquet suivi) :
//   - Véhicules : contrôle technique, assurance, vidange, kilométrage ;
//   - Animaux : vaccins, vermifuge, vétérinaire ;
//   - Prestataires : plombier, nounou, ramonage… et leurs interventions.
//
// Droits : tout le monde consulte, ajoute et corrige ; seuls les parents
// suppriment.
package foyer

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
	"familycocon/internal/suivi"
)

// Module : les trois modules du foyer.
type Module struct {
	a *appli.App
	s *suivi.Suivi
}

// Brancher ajoute les routes des trois modules.
func Brancher(a *appli.App, s *suivi.Suivi) {
	m := &Module{a: a, s: s}
	m.brancherVehicules()
	m.brancherAnimaux()
	m.brancherPrestataires()
}

// Element : une ligne de la liste d'un module.
type Element struct {
	URL, Emoji, Nom, Detail string
	Alerte                  string // échéance en retard ou proche
}

type vueListe struct {
	Titre, Emoji, Module, Nouveau, TexteNouveau, Vide string
	Elements                                          []Element
}

// Lien : un bouton de la fiche (appeler, écrire, site…).
type Lien struct {
	URL          template.URL // tel:, sms:, mailto: ou http(s), construit ici
	Emoji, Texte string
	Externe      bool
}

type vueFiche struct {
	Retour, TitreRetour string
	Emoji, Nom, Sous    string
	Infos               []string
	Liens               []Lien
	Suivi               *suivi.Bloc
	TitreSuivi          string
	TitreCarnet         string
	Modifier, Supprimer string
	KmURL               string // formulaire « kilométrage » (véhicules)
	Contacts            []ContactCite
}

// ContactCite : un contact du carnet cité par la fiche (vétérinaire, garage…).
type ContactCite struct {
	Role, Nom, URL string
	Tel            string // pour « appeler »
}

// contactsChoix : les contacts du carnet, pour les listes de choix.
func (m *Module) contactsChoix(ctx context.Context, c *appli.Contexte) []donnees.Contact {
	l, _ := m.a.Q.ContactsFamille(ctx, c.Famille.ID)
	return l
}

// lireContact : le contact choisi dans le champ (seulement de la famille).
func (m *Module) lireContact(r *http.Request, c *appli.Contexte, champ string) sql.NullInt64 {
	id, _ := strconv.ParseInt(r.PostFormValue(champ), 10, 64)
	if id <= 0 {
		return sql.NullInt64{}
	}
	if _, err := m.a.Q.ContactParID(r.Context(), donnees.ContactParIDParams{ID: id, FamilleID: c.Famille.ID}); err != nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: id, Valid: true}
}

// citer : le contact id (s'il existe encore), pour la fiche.
func (m *Module) citer(ctx context.Context, c *appli.Contexte, role string, id sql.NullInt64) (ContactCite, bool) {
	// Rien si aucun module ne montre le carnet.
	if !id.Valid || !(c.Actif("contacts") || c.Actif("adresses") || c.Actif("prestataires")) {
		return ContactCite{}, false
	}
	x, err := m.a.Q.ContactParID(ctx, donnees.ContactParIDParams{ID: id.Int64, FamilleID: c.Famille.ID})
	if err != nil {
		return ContactCite{}, false
	}
	tel := x.Telephone
	if tel == "" {
		tel = x.TelFixe
	}
	if tel == "" {
		tel = x.TelTravail
	}
	return ContactCite{Role: role, Nom: x.Nom, URL: fmt.Sprintf("/contacts/%d", x.ID), Tel: lienTel(tel)}, true
}

// alerte : la prochaine échéance d'un objet, si elle est en retard ou proche.
func (m *Module) alerte(ctx context.Context, c *appli.Contexte, module string, objet int64) string {
	b, err := m.s.Bloc(ctx, c, module, objet)
	if err != nil || len(b.Echeances) == 0 {
		return ""
	}
	e := b.Echeances[0]
	if e.Retard || e.Proche {
		return "⏰ " + e.Libelle + " : " + strings.ToLower(e.Quand[:1]) + e.Quand[1:]
	}
	return ""
}

func idDe(r *http.Request) int64 {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id
}

func (m *Module) afficherFiche(w http.ResponseWriter, r *http.Request, c *appli.Contexte, module string, objet int64, v vueFiche) {
	b, err := m.s.Bloc(r.Context(), c, module, objet)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	v.Suivi = b
	m.a.Afficher(w, r, c, "foyer/fiche", v.Nom, module, v)
}

// supprimerObjet : l'objet, ses échéances et son carnet, ensemble.
func (m *Module) supprimerObjet(w http.ResponseWriter, r *http.Request, c *appli.Contexte, module string, id int64, nom, retour string,
	suppr func(ctx context.Context, q *donnees.Queries) error) {
	ctx := r.Context()
	err := m.a.Transaction(ctx, func(q *donnees.Queries) error {
		if err := suppr(ctx, q); err != nil {
			return err
		}
		return m.s.SupprimerObjet(ctx, q, c.Famille.ID, module, id)
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Flash(w, nom+" supprimé(e)")
	m.a.Rediriger(w, r, retour)
}

// age : « 3 ans », « 8 mois » depuis la date de naissance (« » si inconnue).
func age(naissance string, maintenant time.Time) string {
	n, err := time.Parse("2006-01-02", naissance)
	if err != nil || n.After(maintenant) {
		return ""
	}
	mois := (maintenant.Year()-n.Year())*12 + int(maintenant.Month()) - int(n.Month())
	if maintenant.Day() < n.Day() {
		mois--
	}
	if mois < 24 {
		return appli.Pluriel(mois, "mois", "mois")
	}
	return appli.Pluriel(mois/12, "an", "ans")
}

// lienTel : chiffres et « + » seulement.
func lienTel(t string) string {
	var b strings.Builder
	for _, r := range t {
		if (r >= '0' && r <= '9') || (r == '+' && b.Len() == 0) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// texte : champ de formulaire nettoyé, au plus max caractères.
func texte(r *http.Request, champ string, max int) (string, error) {
	v := strings.TrimSpace(r.PostFormValue(champ))
	if len([]rune(v)) > max {
		return "", fmt.Errorf("%s : %d caractères au plus", champ, max)
	}
	return v, nil
}
