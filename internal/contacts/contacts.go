// SPDX-License-Identifier: GPL-3.0-or-later

// Package contacts : le carnet de la famille et ses vues.
//   - Contacts : le carnet central (téléphones, e-mail, adresse, société,
//     favoris, groupes) ; import et export vCard ;
//   - Adresses : les contacts qui ont une adresse postale (avec un plan) ;
//   - Anniversaires : les dates des proches et des membres, l'âge qu'ils
//     auront, des idées de cadeaux, et des rappels (accueil, notifications).
//
// Les prestataires sont des contacts « pro » (module Prestataires, paquet
// foyer) ; Animaux, Véhicules et Agenda peuvent citer un contact.
//
// Droits : tout le monde consulte, ajoute et corrige ; seuls les parents
// suppriment, importent et exportent.
package contacts

import (
	"familycocon/internal/appli"
)

// Module : Contacts, Adresses et Anniversaires.
type Module struct{ a *appli.App }

const (
	modContacts     = "contacts"
	modAdresses     = "adresses"
	modAnniv        = "anniversaires"
	modPrestataires = "prestataires"
)

// Relations proposées.
var Relations = []struct{ Cle, Nom, Emoji string }{
	{"famille", "Famille", "👪"}, {"ami", "Amis", "🤝"}, {"voisin", "Voisins", "🏘️"},
	{"ecole", "École", "🏫"}, {"travail", "Travail", "💼"}, {"pro", "Professionnels", "🧰"}, {"autre", "Autres", "📇"},
}

func relationValide(r string) bool {
	for _, x := range Relations {
		if x.Cle == r {
			return true
		}
	}
	return false
}

// Metiers proposés pour les contacts pros (saisie libre possible).
var Metiers = []string{"Plombier", "Électricien", "Chauffagiste", "Ramoneur", "Jardinier", "Femme / homme de ménage",
	"Nounou", "Baby-sitter", "Médecin", "Dentiste", "Pédiatre", "Vétérinaire", "Garagiste", "Assurance", "Serrurier",
	"Peintre", "Menuisier", "Informaticien", "Coiffeur", "Banque", "Notaire", "Syndic", "Kinésithérapeute", "Orthophoniste"}

// Brancher ajoute les routes des trois modules.
func Brancher(a *appli.App) {
	m := &Module{a: a}
	a.Route("GET /contacts", modContacts, appli.Membre, m.pageContacts)
	a.Route("GET /contacts/export.vcf", modContacts, appli.Parent, m.exporter)
	a.Route("POST /contacts/import", modContacts, appli.Parent, m.importer)
	a.Route("GET /adresses", modAdresses, appli.Membre, m.pageAdresses)
	a.Route("GET /anniversaires", modAnniv, appli.Membre, m.pageAnniversaires)
	// Fiches : communes aux modules (l'un d'eux doit être activé).
	a.Route("GET /contacts/nouveau", "", appli.Membre, m.formulaire)
	a.Route("POST /contacts", "", appli.Membre, m.creer)
	a.Route("GET /contacts/{id}", "", appli.Membre, m.fiche)
	a.Route("GET /contacts/{id}/modifier", "", appli.Membre, m.formulaire)
	a.Route("POST /contacts/{id}", "", appli.Membre, m.modifier)
	a.Route("POST /contacts/{id}/favori", "", appli.Membre, m.favori)
	a.Route("POST /contacts/{id}/supprimer", "", appli.Parent, m.supprimer)

	a.AjouterResume(modContacts, m.resumeContacts)
	a.AjouterResume(modAdresses, m.resumeAdresses)
	a.AjouterResume(modAnniv, m.resumeAnniv)
	a.AjouterEcheances(modAnniv, m.echeances)
	a.AjouterAction(appli.Ajout{Module: modContacts, Emoji: "👥", Texte: "Un contact", URL: "/contacts/nouveau"})
	a.AjouterAction(appli.Ajout{Module: modAnniv, Emoji: "🎂", Texte: "Un anniversaire", URL: "/contacts/nouveau?anniv=1"})
}

// actif : l'un des modules qui montrent des contacts est-il activé ?
func actif(c *appli.Contexte) bool {
	return c.Actif(modContacts) || c.Actif(modAdresses) || c.Actif(modAnniv) || c.Actif(modPrestataires)
}

// LienTel : « 06 12 34 56 78 » → « 0612345678 » (chiffres et + au début).
func LienTel(t string) string {
	var b []rune
	for _, r := range t {
		if (r >= '0' && r <= '9') || (r == '+' && len(b) == 0) {
			b = append(b, r)
		}
	}
	return string(b)
}
