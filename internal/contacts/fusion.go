// SPDX-License-Identifier: GPL-3.0-or-later

package contacts

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
	"familycocon/internal/texte"
)

// Citation : un endroit de l'application qui cite ce contact.
type Citation struct {
	Emoji, Texte, URL string
}

// citations : « 🐾 Rex · vétérinaire », « 🚗 La Clio · garage »,
// « 🗓️ 2 événements », pour la fiche (et la confirmation de suppression).
func (m *Module) citations(ctx context.Context, c *appli.Contexte, id int64) ([]Citation, error) {
	var out []Citation
	nid := sql.NullInt64{Int64: id, Valid: true}
	if c.Voit("animaux") {
		l, err := m.a.Q.CitesAnimaux(ctx, donnees.CitesAnimauxParams{FamilleID: c.Famille.ID, VeterinaireID: nid})
		if err != nil {
			return nil, err
		}
		for _, x := range l {
			out = append(out, Citation{"🐾", x.Nom + " · vétérinaire", "/animaux/" + strconv.FormatInt(x.ID, 10)})
		}
	}
	if c.Voit("vehicules") {
		l, err := m.a.Q.CitesVehicules(ctx, donnees.CitesVehiculesParams{Contact: nid, FamilleID: c.Famille.ID})
		if err != nil {
			return nil, err
		}
		for _, x := range l {
			role := "assureur"
			if x.Garage == 1 {
				role = "garage"
			}
			out = append(out, Citation{"🚗", x.Nom + " · " + role, "/vehicules/" + strconv.FormatInt(x.ID, 10)})
		}
	}
	if c.Voit("garanties") {
		l, err := m.a.Q.CitesGaranties(ctx, donnees.CitesGarantiesParams{FamilleID: c.Famille.ID, MagasinID: nid})
		if err != nil {
			return nil, err
		}
		for _, x := range l {
			out = append(out, Citation{"🧾", x.Nom + " · magasin", "/garanties/" + strconv.FormatInt(x.ID, 10)})
		}
	}
	if c.Voit("agenda") {
		n, err := m.a.Q.CitesEvenements(ctx, donnees.CitesEvenementsParams{FamilleID: c.Famille.ID, ContactID: id})
		if err != nil {
			return nil, err
		}
		if n > 0 {
			out = append(out, Citation{"🗓️", appli.Pluriel(int(n), "événement de l'agenda", "événements de l'agenda"), ""})
		}
	}
	return out, nil
}

// doublons : les autres fiches qui ont le même nom (sans majuscules ni
// accents) ou le même numéro de téléphone.
func (m *Module) doublons(ctx context.Context, c *appli.Contexte, ct donnees.Contact) ([]donnees.Contact, error) {
	tous, err := m.a.Q.ContactsFamille(ctx, c.Famille.ID)
	if err != nil {
		return nil, err
	}
	tels := map[string]bool{}
	for _, t := range []string{ct.Telephone, ct.TelFixe, ct.TelTravail} {
		if n := LienTel(t); len(n) >= 6 {
			tels[n] = true
		}
	}
	nom := texte.Plier(strings.TrimSpace(ct.Nom))
	var out []donnees.Contact
	for _, x := range tous {
		if x.ID == ct.ID {
			continue
		}
		pareil := texte.Plier(strings.TrimSpace(x.Nom)) == nom
		for _, t := range []string{x.Telephone, x.TelFixe, x.TelTravail} {
			if n := LienTel(t); n != "" && tels[n] {
				pareil = true
			}
		}
		if pareil && peutVoir(c, x) {
			out = append(out, x)
		}
	}
	return out, nil
}

// premierRempli : a s'il est rempli, sinon b.
func premierRempli(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// fusionner : la fiche {id} garde tout ; l'autre (« ancien ») lui donne ce
// qui lui manque, ses liens (vétérinaire, garage, magasin, agenda,
// interventions) passent sur elle, puis elle est supprimée.
func (m *Module) fusionner(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	garde, ok := m.contact(r, c)
	if !ok || !actif(c) {
		m.a.Echec(w, r, http.StatusNotFound, "Contact introuvable.")
		return
	}
	idAncien, _ := strconv.ParseInt(r.PostFormValue("autre"), 10, 64)
	ancien, err := m.a.Q.ContactParID(ctx, donnees.ContactParIDParams{ID: idAncien, FamilleID: c.Famille.ID})
	if err != nil || ancien.ID == garde.ID {
		m.a.Echec(w, r, http.StatusNotFound, "Contact à fusionner introuvable.")
		return
	}
	g := garde
	g.Relation = premierRempli(g.Relation, ancien.Relation)
	g.Naissance = premierRempli(g.Naissance, ancien.Naissance)
	g.Telephone = premierRempli(g.Telephone, ancien.Telephone)
	g.TelFixe = premierRempli(g.TelFixe, ancien.TelFixe)
	g.TelTravail = premierRempli(g.TelTravail, ancien.TelTravail)
	g.Email = premierRempli(g.Email, ancien.Email)
	g.Adresse = premierRempli(g.Adresse, ancien.Adresse)
	g.Societe = premierRempli(g.Societe, ancien.Societe)
	g.Metier = premierRempli(g.Metier, ancien.Metier)
	g.Site = premierRempli(g.Site, ancien.Site)
	g.Favori = max(g.Favori, ancien.Favori)
	g.Pro = max(g.Pro, ancien.Pro)
	g.Groupes = strings.Join(Groupes(g.Groupes+","+ancien.Groupes), ", ")
	if ancien.Notes != "" && !strings.Contains(g.Notes, ancien.Notes) {
		g.Notes = strings.TrimSpace(g.Notes + "\n" + ancien.Notes)
	}
	if ancien.Idees != "" && !strings.Contains(g.Idees, ancien.Idees) {
		g.Idees = strings.TrimSpace(g.Idees + "\n" + ancien.Idees)
	}
	gid, aid := sql.NullInt64{Int64: g.ID, Valid: true}, sql.NullInt64{Int64: ancien.ID, Valid: true}
	err = m.a.Transaction(ctx, func(q *donnees.Queries) error {
		if err := q.ModifierContact(ctx, donnees.ModifierContactParams{Nom: g.Nom, Relation: g.Relation, Naissance: g.Naissance,
			Telephone: g.Telephone, TelFixe: g.TelFixe, TelTravail: g.TelTravail, Email: g.Email, Adresse: g.Adresse,
			Societe: g.Societe, Metier: g.Metier, Site: g.Site, Favori: g.Favori, Pro: g.Pro, Groupes: g.Groupes,
			Notes: g.Notes, Idees: g.Idees, ModifieLe: time.Now().Unix(), ID: g.ID, FamilleID: c.Famille.ID}); err != nil {
			return err
		}
		f := c.Famille.ID
		for _, e := range []error{
			q.FusionAnimaux(ctx, donnees.FusionAnimauxParams{Garde: gid, FamilleID: f, Ancien: aid}),
			q.FusionGarages(ctx, donnees.FusionGaragesParams{Garde: gid, FamilleID: f, Ancien: aid}),
			q.FusionAssureurs(ctx, donnees.FusionAssureursParams{Garde: gid, FamilleID: f, Ancien: aid}),
			q.FusionMagasins(ctx, donnees.FusionMagasinsParams{Garde: gid, FamilleID: f, Ancien: aid}),
			q.FusionEvenements(ctx, donnees.FusionEvenementsParams{Garde: g.ID, Ancien: ancien.ID}),
			q.FusionEcheances(ctx, donnees.FusionEcheancesParams{Garde: g.ID, FamilleID: f, Ancien: ancien.ID}),
			q.FusionCarnet(ctx, donnees.FusionCarnetParams{Garde: g.ID, FamilleID: f, Ancien: ancien.ID}),
		} {
			if e != nil {
				return e
			}
		}
		return q.SupprimerContact(ctx, donnees.SupprimerContactParams{ID: ancien.ID, FamilleID: f})
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, modContacts, "")
	m.a.Flash(w, fmt.Sprintf("Fiches fusionnées : %s", g.Nom))
	m.a.Rediriger(w, r, fmt.Sprintf("/contacts/%d", g.ID))
}
