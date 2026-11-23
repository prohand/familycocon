// SPDX-License-Identifier: GPL-3.0-or-later

package foyer

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/budget"
	"familycocon/internal/donnees"
	"familycocon/internal/suivi"
	txt "familycocon/internal/texte"
)

// Garanties : les appareils achetés (lave-linge, téléphone, aspirateur…),
// jusqu'à quand ils sont garantis et où est rangée la facture. La fin de
// garantie est une échéance du suivi : elle arrive dans « À venir », dans
// l'agenda et dans la notification du matin, sans rien de plus. Le carnet
// garde les réparations. Aucun fichier n'est envoyé (la base resterait
// légère) : on note où est la facture.

const modGaranties = "garanties"

// libelleFinGarantie : l'échéance tenue à jour par le module.
const libelleFinGarantie = "Fin de garantie"

func (m *Module) brancherGaranties() {
	a := m.a
	a.Route("GET /garanties", modGaranties, appli.Membre, m.listeGaranties)
	a.Route("GET /garanties/nouvelle", modGaranties, appli.Membre, m.formGarantie)
	a.Route("POST /garanties", modGaranties, appli.Membre, m.enregistrerGarantie)
	a.Route("GET /garanties/{id}", modGaranties, appli.Membre, m.ficheGarantie)
	a.Route("GET /garanties/{id}/modifier", modGaranties, appli.Membre, m.formGarantie)
	a.Route("POST /garanties/{id}", modGaranties, appli.Membre, m.enregistrerGarantie)
	a.Route("POST /garanties/{id}/supprimer", modGaranties, appli.Parent, m.supprimerGarantie)
	a.AjouterRecherche(modGaranties, m.chercherGaranties)

	m.s.Enregistrer(suivi.Objet{
		Module: modGaranties, Emoji: "🧾", Categorie: "logement",
		Modeles: []suivi.Modele{{Libelle: "Entretien", Mois: 12}, {Libelle: "Réparation", Mois: 0}, {Libelle: "Détartrage", Mois: 6}},
		Nom: func(ctx context.Context, famille, id int64) (string, bool) {
			x, err := a.Q.GarantieParID(ctx, donnees.GarantieParIDParams{ID: id, FamilleID: famille})
			return x.Nom, err == nil
		},
		Noms: func(ctx context.Context, famille int64) (map[int64]string, error) {
			l, err := a.Q.NomsGaranties(ctx, famille)
			noms := make(map[int64]string, len(l))
			for _, x := range l {
				noms[x.ID] = x.Nom
			}
			return noms, err
		},
		URL: func(id int64) string { return fmt.Sprintf("/garanties/%d", id) },
	})
	a.AjouterResume(modGaranties, func(ctx context.Context, c *appli.Contexte) appli.Resume {
		l, err := a.Q.GarantiesFamille(ctx, c.Famille.ID)
		if err != nil || len(l) == 0 {
			return appli.Resume{Info: "Aucun appareil"}
		}
		alertes := m.alertes(ctx, c, modGaranties)
		for _, x := range l {
			if al := alertes[x.ID]; al != "" {
				return appli.Resume{Info: al, Badge: 1}
			}
		}
		n := 0
		for _, x := range l {
			if fin, ok := finGarantie(x); ok && !fin.Before(c.Maintenant()) {
				n++
			}
		}
		return appli.Resume{Info: appli.Pluriel(n, "sous garantie", "sous garantie")}
	})
	a.AjouterAction(appli.Ajout{Module: modGaranties, Emoji: "🧾", Texte: "Un achat garanti", URL: "/garanties/nouvelle"})
}

// finGarantie : le dernier jour de la garantie (ok = faux sans date d'achat
// ou sans garantie).
func finGarantie(x donnees.Garantie) (time.Time, bool) {
	achat, err := time.Parse("2006-01-02", x.Achat)
	if err != nil || x.GarantieMois <= 0 {
		return time.Time{}, false
	}
	return suivi.AjouterMois(achat, int(x.GarantieMois)), true
}

func (m *Module) garantie(r *http.Request, c *appli.Contexte) (donnees.Garantie, bool) {
	x, err := m.a.Q.GarantieParID(r.Context(), donnees.GarantieParIDParams{ID: idDe(r), FamilleID: c.Famille.ID})
	return x, err == nil
}

// etatGarantie : « Garanti jusqu'au 3 mars 2028 » ou « Garantie finie ».
func etatGarantie(x donnees.Garantie, maintenant time.Time) string {
	fin, ok := finGarantie(x)
	switch {
	case !ok:
		return ""
	case fin.Before(maintenant):
		return "Garantie finie (" + appli.DateLongue(fin) + " " + strconv.Itoa(fin.Year()) + ")"
	}
	return "Garanti jusqu'au " + strings.ToLower(appli.DateLongue(fin)[:1]) + appli.DateLongue(fin)[1:] + " " + strconv.Itoa(fin.Year())
}

func (m *Module) listeGaranties(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	l, err := m.a.Q.GarantiesFamille(r.Context(), c.Famille.ID)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	v := vueListe{Titre: "Garanties", Emoji: "🧾", Module: modGaranties, Nouveau: "/garanties/nouvelle", TexteNouveau: "＋ Achat",
		Vide: "Notez vos achats (lave-linge, téléphone…) : la fin de garantie sera rappelée, et vous saurez où est la facture."}
	alertes := m.alertes(r.Context(), c, modGaranties)
	for _, x := range l {
		detail := etatGarantie(x, c.Maintenant())
		if x.Marque != "" {
			detail = strings.TrimSuffix(x.Marque+" · "+detail, " · ")
		}
		v.Elements = append(v.Elements, Element{URL: fmt.Sprintf("/garanties/%d", x.ID), Emoji: "🧾", Nom: x.Nom, Detail: detail, Alerte: alertes[x.ID]})
	}
	m.a.Afficher(w, r, c, "foyer/liste", "Garanties", modGaranties, v)
}

func (m *Module) ficheGarantie(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	x, ok := m.garantie(r, c)
	if !ok {
		m.a.PageErreur(w, r, c, http.StatusNotFound, "Achat introuvable", "Cet achat n'existe plus.")
		return
	}
	v := vueFiche{Retour: "/garanties", TitreRetour: "Garanties", Emoji: "🧾", Nom: x.Nom, Sous: x.Marque,
		TitreSuivi: "À venir", TitreCarnet: "Réparations et entretien",
		Modifier: fmt.Sprintf("/garanties/%d/modifier", x.ID), Supprimer: fmt.Sprintf("/garanties/%d/supprimer", x.ID)}
	if e := etatGarantie(x, c.Maintenant()); e != "" {
		v.Infos = append(v.Infos, "🛡️ "+e)
	}
	if a, err := time.Parse("2006-01-02", x.Achat); err == nil {
		info := "🛒 Acheté le " + appli.DateLongue(a) + " " + strconv.Itoa(a.Year())
		if x.Prix > 0 && c.EstParent() {
			info += " · " + appli.Montant(x.Prix, c.Famille.Devise)
		}
		v.Infos = append(v.Infos, info)
	}
	if ct, ok := m.citer(r.Context(), c, "🏬 Magasin", x.MagasinID); ok {
		v.Contacts = append(v.Contacts, ct)
	}
	if x.Magasin != "" {
		v.Infos = append(v.Infos, "🏬 "+x.Magasin)
	}
	if x.Facture != "" {
		v.Infos = append(v.Infos, "📁 Facture : "+x.Facture)
	}
	if x.Notes != "" {
		v.Infos = append(v.Infos, "📝 "+x.Notes)
	}
	m.afficherFiche(w, r, c, modGaranties, x.ID, v)
}

type formGarantie struct {
	Nouveau  bool
	G        donnees.Garantie
	Prix     string
	Contacts []donnees.ContactsChoixRow
}

func (m *Module) formGarantie(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	f := formGarantie{Nouveau: r.PathValue("id") == "", G: donnees.Garantie{GarantieMois: 24, Achat: c.Maintenant().Format("2006-01-02")}}
	if !f.Nouveau {
		x, ok := m.garantie(r, c)
		if !ok {
			m.a.PageErreur(w, r, c, http.StatusNotFound, "Achat introuvable", "Cet achat n'existe plus.")
			return
		}
		f.G = x
		if x.Prix > 0 {
			f.Prix = budget.MontantSaisie(x.Prix)
		}
	}
	f.Contacts = m.contactsChoix(r.Context(), c)
	m.a.Afficher(w, r, c, "foyer/garantie", "Achat", modGaranties, f)
}

func (m *Module) enregistrerGarantie(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	nom, err1 := texte(r, "nom", 60)
	marque, err2 := texte(r, "marque", 60)
	magasin, err3 := texte(r, "magasin", 200)
	facture, err4 := texte(r, "facture", 200)
	notes, err5 := texte(r, "notes", 1000)
	if err := firstErr(err1, err2, err3, err4, err5); err != nil || nom == "" {
		m.a.Echec(w, r, http.StatusBadRequest, "Nom (60 caractères) ou un autre champ invalide.")
		return
	}
	achat := r.PostFormValue("achat")
	if t, err := time.Parse("2006-01-02", achat); achat != "" && (err != nil || t.After(time.Now().AddDate(0, 0, 1)) || t.Year() < 1950) {
		m.a.Echec(w, r, http.StatusBadRequest, "Date d'achat invalide.")
		return
	}
	mois, err := strconv.ParseInt(r.PostFormValue("garantie_mois"), 10, 64)
	if err != nil || mois < 0 || mois > 240 {
		m.a.Echec(w, r, http.StatusBadRequest, "Durée de garantie invalide (0 à 240 mois).")
		return
	}
	var prix int64
	if p := strings.TrimSpace(r.PostFormValue("prix")); p != "" {
		if prix, err = budget.LireMontantOuZero(p); err != nil {
			m.a.Echec(w, r, http.StatusBadRequest, err.Error())
			return
		}
	}
	if !c.EstParent() {
		// Prix réservé aux parents : celui déjà noté est gardé.
		prix = 0
		if x, ok := m.garantie(r, c); ok && r.PathValue("id") != "" {
			prix = x.Prix
		}
	}
	magasinID := m.lireContact(r, c, "magasin_id")
	id := idDe(r)
	if r.PathValue("id") == "" {
		if n, err := m.a.Q.CompterGaranties(ctx, c.Famille.ID); err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		} else if n >= 500 {
			m.a.Echec(w, r, http.StatusBadRequest, "Trop d'achats (500 au plus).")
			return
		}
	} else if _, ok := m.garantie(r, c); !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Achat introuvable.")
		return
	}
	// L'achat et son échéance « Fin de garantie » changent ensemble.
	err = m.a.Transaction(ctx, func(q *donnees.Queries) error {
		var x donnees.Garantie
		if r.PathValue("id") == "" {
			var err error
			x, err = q.CreerGarantie(ctx, donnees.CreerGarantieParams{FamilleID: c.Famille.ID, Nom: nom, Marque: marque, Achat: achat,
				Prix: prix, GarantieMois: mois, Magasin: magasin, MagasinID: magasinID, Facture: facture, Notes: notes, CreeLe: time.Now().Unix()})
			if err != nil {
				return err
			}
			id = x.ID
		} else {
			if err := q.ModifierGarantie(ctx, donnees.ModifierGarantieParams{Nom: nom, Marque: marque, Achat: achat, Prix: prix,
				GarantieMois: mois, Magasin: magasin, MagasinID: magasinID, Facture: facture, Notes: notes, ID: id, FamilleID: c.Famille.ID}); err != nil {
				return err
			}
			x = donnees.Garantie{Achat: achat, GarantieMois: mois}
		}
		if err := q.SupprimerEcheanceLibelle(ctx, donnees.SupprimerEcheanceLibelleParams{FamilleID: c.Famille.ID, Module: modGaranties,
			ObjetID: id, Libelle: libelleFinGarantie}); err != nil {
			return err
		}
		x.Achat, x.GarantieMois = achat, mois
		if fin, ok := finGarantie(x); ok && !fin.Before(c.Maintenant().AddDate(0, 0, -1)) {
			_, err := q.CreerEcheance(ctx, donnees.CreerEcheanceParams{FamilleID: c.Famille.ID, Module: modGaranties, ObjetID: id,
				Libelle: libelleFinGarantie, Jour: fin.Format("2006-01-02"), CreeLe: time.Now().Unix()})
			return err
		}
		return nil
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, modGaranties, "")
	m.a.Flash(w, "Achat enregistré 🧾")
	m.a.Rediriger(w, r, fmt.Sprintf("/garanties/%d", id))
}

func (m *Module) supprimerGarantie(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	x, ok := m.garantie(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Achat introuvable.")
		return
	}
	m.supprimerObjet(w, r, c, modGaranties, x.ID, x.Nom, "/garanties", func(ctx context.Context, q *donnees.Queries) error {
		return q.SupprimerGarantie(ctx, donnees.SupprimerGarantieParams{ID: x.ID, FamilleID: c.Famille.ID})
	})
}

// chercherGaranties : recherche globale (nom, marque, magasin).
func (m *Module) chercherGaranties(ctx context.Context, c *appli.Contexte, q string) ([]appli.Resultat, error) {
	l, err := m.a.Q.GarantiesFamille(ctx, c.Famille.ID)
	if err != nil {
		return nil, err
	}
	var out []appli.Resultat
	for _, x := range l {
		if txt.Contient(x.Nom+" "+x.Marque+" "+x.Magasin, q) {
			out = append(out, appli.Resultat{Emoji: "🧾", Titre: x.Nom, Detail: etatGarantie(x, c.Maintenant()), URL: fmt.Sprintf("/garanties/%d", x.ID)})
		}
	}
	return out, nil
}
