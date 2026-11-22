// SPDX-License-Identifier: GPL-3.0-or-later

package foyer

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
	"familycocon/internal/suivi"
)

const modPrestataires = "prestataires"

// Metiers proposés (saisie libre possible).
var Metiers = []string{"Plombier", "Électricien", "Chauffagiste", "Ramoneur", "Jardinier", "Femme / homme de ménage",
	"Nounou", "Baby-sitter", "Médecin", "Dentiste", "Pédiatre", "Garagiste", "Serrurier", "Peintre", "Menuisier",
	"Informaticien", "Coiffeur", "Vétérinaire", "Assurance", "Banque", "Notaire", "Syndic"}

func (m *Module) brancherPrestataires() {
	a := m.a
	a.Route("GET /prestataires", modPrestataires, appli.Membre, m.listePrestataires)
	a.Route("GET /prestataires/nouveau", modPrestataires, appli.Membre, m.formPrestataire)
	a.Route("POST /prestataires", modPrestataires, appli.Membre, m.enregistrerPrestataire)
	a.Route("GET /prestataires/{id}", modPrestataires, appli.Membre, m.fichePrestataire)
	a.Route("GET /prestataires/{id}/modifier", modPrestataires, appli.Membre, m.formPrestataire)
	a.Route("POST /prestataires/{id}", modPrestataires, appli.Membre, m.enregistrerPrestataire)
	a.Route("POST /prestataires/{id}/supprimer", modPrestataires, appli.Parent, m.supprimerPrestataire)

	m.s.Enregistrer(suivi.Objet{
		Module: modPrestataires, Emoji: "🧰",
		Modeles: []suivi.Modele{{Libelle: "Entretien de la chaudière", Mois: 12}, {Libelle: "Ramonage", Mois: 12}, {Libelle: "Visite", Mois: 0}, {Libelle: "Rendez-vous", Mois: 0},
			{Libelle: "Intervention", Mois: 0}, {Libelle: "Devis", Mois: 0}},
		Nom: func(ctx context.Context, famille, id int64) (string, bool) {
			x, err := a.Q.PrestataireParID(ctx, donnees.PrestataireParIDParams{ID: id, FamilleID: famille})
			return x.Nom, err == nil
		},
		URL: func(id int64) string { return fmt.Sprintf("/prestataires/%d", id) },
	})
	a.AjouterResume(modPrestataires, func(ctx context.Context, c *appli.Contexte) appli.Resume {
		l, err := a.Q.PrestatairesFamille(ctx, c.Famille.ID)
		if err != nil || len(l) == 0 {
			return appli.Resume{Info: "Aucun prestataire"}
		}
		return appli.Resume{Info: appli.Pluriel(len(l), "prestataire", "prestataires")}
	})
	a.AjouterAction(appli.Ajout{Module: modPrestataires, Emoji: "🧰", Texte: "Un prestataire", URL: "/prestataires/nouveau"})
}

func (m *Module) prestataire(r *http.Request, c *appli.Contexte) (donnees.Prestataire, bool) {
	x, err := m.a.Q.PrestataireParID(r.Context(), donnees.PrestataireParIDParams{ID: idDe(r), FamilleID: c.Famille.ID})
	return x, err == nil
}

func (m *Module) listePrestataires(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	l, err := m.a.Q.PrestatairesFamille(r.Context(), c.Famille.ID)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	v := vueListe{Titre: "Prestataires", Emoji: "🧰", Module: modPrestataires, Nouveau: "/prestataires/nouveau", TexteNouveau: "＋ Prestataire",
		Vide: "Le plombier, la nounou, le ramoneur… avec leurs coordonnées et l'historique de leurs interventions."}
	for _, x := range l {
		detail := x.Metier
		if x.Telephone != "" {
			detail = strings.Trim(detail+" · 📞 "+x.Telephone, " ·")
		}
		v.Elements = append(v.Elements, Element{URL: fmt.Sprintf("/prestataires/%d", x.ID), Emoji: "🧰", Nom: x.Nom, Detail: detail,
			Alerte: m.alerte(r.Context(), c, modPrestataires, x.ID)})
	}
	m.a.Afficher(w, r, c, "foyer/liste", "Prestataires", modPrestataires, v)
}

func (m *Module) fichePrestataire(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	x, ok := m.prestataire(r, c)
	if !ok {
		m.a.PageErreur(w, r, c, http.StatusNotFound, "Prestataire introuvable", "Ce prestataire n'existe plus.")
		return
	}
	v := vueFiche{Retour: "/prestataires", TitreRetour: "Prestataires", Emoji: "🧰", Nom: x.Nom, Sous: x.Metier,
		TitreSuivi: "Rendez-vous et entretiens", TitreCarnet: "Interventions",
		Modifier: fmt.Sprintf("/prestataires/%d/modifier", x.ID), Supprimer: fmt.Sprintf("/prestataires/%d/supprimer", x.ID)}
	if t := lienTel(x.Telephone); t != "" {
		v.Liens = append(v.Liens, Lien{URL: template.URL("tel:" + t), Emoji: "📞", Texte: "Appeler"}, Lien{URL: template.URL("sms:" + t), Emoji: "💬", Texte: "SMS"})
		v.Infos = append(v.Infos, "📞 "+x.Telephone)
	}
	if x.Email != "" {
		v.Liens = append(v.Liens, Lien{URL: template.URL("mailto:" + url.PathEscape(x.Email)), Emoji: "✉️", Texte: "E-mail"})
		v.Infos = append(v.Infos, "✉️ "+x.Email)
	}
	if x.Site != "" {
		v.Liens = append(v.Liens, Lien{URL: siteSur(x.Site), Emoji: "🌐", Texte: "Site", Externe: true})
	}
	if x.Adresse != "" {
		v.Liens = append(v.Liens, Lien{URL: template.URL("https://www.openstreetmap.org/search?query=" + strings.ReplaceAll(url.QueryEscape(strings.ReplaceAll(x.Adresse, "\n", ", ")), "+", "%20")),
			Emoji: "🗺️", Texte: "Plan", Externe: true})
		v.Infos = append(v.Infos, "🏠 "+x.Adresse)
	}
	if x.Notes != "" {
		v.Infos = append(v.Infos, "📝 "+x.Notes)
	}
	m.afficherFiche(w, r, c, modPrestataires, x.ID, v)
}

type formPrestataire struct {
	Nouveau bool
	P       donnees.Prestataire
	Metiers []string
}

func (m *Module) formPrestataire(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	f := formPrestataire{Nouveau: r.PathValue("id") == "", Metiers: Metiers}
	if !f.Nouveau {
		x, ok := m.prestataire(r, c)
		if !ok {
			m.a.PageErreur(w, r, c, http.StatusNotFound, "Prestataire introuvable", "Ce prestataire n'existe plus.")
			return
		}
		f.P = x
	}
	m.a.Afficher(w, r, c, "foyer/prestataire", "Prestataire", modPrestataires, f)
}

func (m *Module) enregistrerPrestataire(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	nom, e1 := texte(r, "nom", 80)
	metier, e2 := texte(r, "metier", 60)
	tel, e3 := texte(r, "telephone", 30)
	email, e4 := texte(r, "email", 120)
	adresse, e5 := texte(r, "adresse", 300)
	site, e6 := texte(r, "site", 200)
	notes, e7 := texte(r, "notes", 1000)
	if err := firstErr(e1, e2, e3, e4, e5, e6, e7); err != nil || nom == "" {
		m.a.Echec(w, r, http.StatusBadRequest, "Nom (80 caractères) ou un autre champ invalide.")
		return
	}
	if site != "" {
		u, err := url.Parse(site)
		if err != nil || u.Host == "" {
			if u2, err2 := url.Parse("https://" + site); err2 == nil && u2.Host != "" && !strings.Contains(site, "://") {
				site = "https://" + site
			} else {
				m.a.Echec(w, r, http.StatusBadRequest, "Adresse du site invalide.")
				return
			}
		} else if u.Scheme != "https" && u.Scheme != "http" {
			m.a.Echec(w, r, http.StatusBadRequest, "Adresse du site : http ou https seulement.")
			return
		}
	}
	if email != "" && (!strings.Contains(email, "@") || strings.ContainsAny(email, " <>\"")) {
		m.a.Echec(w, r, http.StatusBadRequest, "Adresse e-mail invalide.")
		return
	}
	id := idDe(r)
	if r.PathValue("id") == "" {
		x, err := m.a.Q.CreerPrestataire(ctx, donnees.CreerPrestataireParams{FamilleID: c.Famille.ID, Nom: nom, Metier: metier,
			Telephone: tel, Email: email, Adresse: adresse, Site: site, Notes: notes, CreeLe: time.Now().Unix()})
		if err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
		id = x.ID
	} else {
		if _, ok := m.prestataire(r, c); !ok {
			m.a.Echec(w, r, http.StatusNotFound, "Prestataire introuvable.")
			return
		}
		if err := m.a.Q.ModifierPrestataire(ctx, donnees.ModifierPrestataireParams{Nom: nom, Metier: metier, Telephone: tel,
			Email: email, Adresse: adresse, Site: site, Notes: notes, ID: id, FamilleID: c.Famille.ID}); err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
	}
	m.a.Flash(w, "Prestataire enregistré ✓")
	m.a.Rediriger(w, r, fmt.Sprintf("/prestataires/%d", id))
}

func (m *Module) supprimerPrestataire(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	x, ok := m.prestataire(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Prestataire introuvable.")
		return
	}
	m.supprimerObjet(w, r, c, modPrestataires, x.ID, x.Nom, "/prestataires", func(ctx context.Context, q *donnees.Queries) error {
		return q.SupprimerPrestataire(ctx, donnees.SupprimerPrestataireParams{ID: x.ID, FamilleID: c.Famille.ID})
	})
}

// siteSur : le site seulement s'il commence par http:// ou https://
// (vérifié à l'enregistrement, revérifié ici par prudence).
func siteSur(s string) template.URL {
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return ""
	}
	return template.URL(u.String()) // #nosec G203 -- schéma vérifié
}
