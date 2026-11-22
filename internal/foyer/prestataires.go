// SPDX-License-Identifier: GPL-3.0-or-later

package foyer

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
	"familycocon/internal/suivi"
)

// Prestataires : les contacts « pro » du carnet (paquet contacts), avec ici
// leurs rendez-vous, entretiens et interventions. La fiche d'identité
// (téléphone, adresse…) se modifie dans le carnet.

const modPrestataires = "prestataires"

func (m *Module) brancherPrestataires() {
	a := m.a
	a.Route("GET /prestataires", modPrestataires, appli.Membre, m.listePrestataires)
	a.Route("GET /prestataires/{id}", modPrestataires, appli.Membre, m.fichePrestataire)

	m.s.Enregistrer(suivi.Objet{
		Module: modPrestataires, Emoji: "🧰",
		Modeles: []suivi.Modele{{Libelle: "Entretien de la chaudière", Mois: 12}, {Libelle: "Ramonage", Mois: 12},
			{Libelle: "Visite", Mois: 0}, {Libelle: "Rendez-vous", Mois: 0}, {Libelle: "Intervention", Mois: 0}, {Libelle: "Devis", Mois: 0}},
		Nom: func(ctx context.Context, famille, id int64) (string, bool) {
			x, err := a.Q.ContactParID(ctx, donnees.ContactParIDParams{ID: id, FamilleID: famille})
			return x.Nom, err == nil
		},
		URL: func(id int64) string { return fmt.Sprintf("/prestataires/%d", id) },
	})
	a.AjouterResume(modPrestataires, func(ctx context.Context, c *appli.Contexte) appli.Resume {
		n, err := a.Q.CompterPrestataires(ctx, c.Famille.ID)
		if err != nil || n == 0 {
			return appli.Resume{Info: "Aucun prestataire"}
		}
		return appli.Resume{Info: appli.Pluriel(n, "prestataire", "prestataires")}
	})
	a.AjouterAction(appli.Ajout{Module: modPrestataires, Emoji: "🧰", Texte: "Un prestataire", URL: "/contacts/nouveau?pro=1"})
}

func (m *Module) listePrestataires(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	l, err := m.a.Q.PrestatairesFamille(r.Context(), c.Famille.ID)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	v := vueListe{Titre: "Prestataires", Emoji: "🧰", Module: modPrestataires, Nouveau: "/contacts/nouveau?pro=1", TexteNouveau: "＋ Prestataire",
		Vide: "Le plombier, la nounou, le ramoneur… avec leurs coordonnées et l'historique de leurs interventions. Ce sont des contacts marqués « professionnel »."}
	for _, x := range l {
		detail := strings.Trim(x.Metier+" · "+x.Societe, " ·")
		if x.Telephone != "" {
			detail = strings.Trim(detail+" · 📞 "+x.Telephone, " ·")
		}
		v.Elements = append(v.Elements, Element{URL: fmt.Sprintf("/prestataires/%d", x.ID), Emoji: "🧰", Nom: x.Nom, Detail: detail,
			Alerte: m.alerte(r.Context(), c, modPrestataires, x.ID)})
	}
	m.a.Afficher(w, r, c, "foyer/liste", "Prestataires", modPrestataires, v)
}

// liensContact : appeler, SMS, e-mail, site, plan.
func liensContact(x donnees.Contact) ([]Lien, []string) {
	var liens []Lien
	var infos []string
	tel := x.Telephone
	if tel == "" {
		tel = x.TelFixe
	}
	if tel == "" {
		tel = x.TelTravail
	}
	if t := lienTel(tel); t != "" {
		liens = append(liens, Lien{URL: template.URL("tel:" + t), Emoji: "📞", Texte: "Appeler"}, Lien{URL: template.URL("sms:" + t), Emoji: "💬", Texte: "SMS"})
	}
	for _, t := range []string{x.Telephone, x.TelFixe, x.TelTravail} {
		if t != "" {
			infos = append(infos, "📞 "+t)
		}
	}
	if x.Email != "" {
		liens = append(liens, Lien{URL: template.URL("mailto:" + url.PathEscape(x.Email)), Emoji: "✉️", Texte: "E-mail"})
		infos = append(infos, "✉️ "+x.Email)
	}
	if s := siteSur(x.Site); s != "" {
		liens = append(liens, Lien{URL: s, Emoji: "🌐", Texte: "Site", Externe: true})
	}
	if x.Adresse != "" {
		liens = append(liens, Lien{URL: template.URL("https://www.openstreetmap.org/search?query=" + strings.ReplaceAll(url.QueryEscape(strings.ReplaceAll(x.Adresse, "\n", ", ")), "+", "%20")),
			Emoji: "🗺️", Texte: "Plan", Externe: true})
		infos = append(infos, "🏠 "+x.Adresse)
	}
	return liens, infos
}

func (m *Module) fichePrestataire(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	x, err := m.a.Q.ContactParID(r.Context(), donnees.ContactParIDParams{ID: idDe(r), FamilleID: c.Famille.ID})
	if err != nil {
		m.a.PageErreur(w, r, c, http.StatusNotFound, "Prestataire introuvable", "Ce prestataire n'existe plus.")
		return
	}
	v := vueFiche{Retour: "/prestataires", TitreRetour: "Prestataires", Emoji: "🧰", Nom: x.Nom, Sous: strings.Trim(x.Metier+" · "+x.Societe, " ·"),
		TitreSuivi: "Rendez-vous et entretiens", TitreCarnet: "Interventions",
		Modifier: fmt.Sprintf("/contacts/%d/modifier", x.ID), Supprimer: fmt.Sprintf("/contacts/%d/supprimer", x.ID)}
	v.Liens, v.Infos = liensContact(x)
	if x.Notes != "" {
		v.Infos = append(v.Infos, "📝 "+x.Notes)
	}
	m.afficherFiche(w, r, c, modPrestataires, x.ID, v)
}

// siteSur : le site seulement s'il commence par http:// ou https://.
func siteSur(s string) template.URL {
	u, err := url.Parse(s)
	if s == "" || err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return ""
	}
	return template.URL(u.String()) // #nosec G203 -- schéma vérifié
}
