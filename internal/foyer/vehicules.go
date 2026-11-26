// SPDX-License-Identifier: GPL-3.0-or-later

package foyer

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
	"familycocon/internal/suivi"
)

const modVehicules = "vehicules"

// Genres de véhicules.
var Genres = []struct{ Cle, Nom, Emoji string }{
	{"voiture", "Voiture", "🚗"}, {"moto", "Moto, scooter", "🏍️"}, {"velo", "Vélo", "🚲"},
	{"campingcar", "Camping-car", "🚐"}, {"autre", "Autre", "🚙"},
}

func emojiGenre(g string) string {
	for _, x := range Genres {
		if x.Cle == g {
			return x.Emoji
		}
	}
	return "🚙"
}

func (m *Module) brancherVehicules() {
	a := m.a
	a.Route("GET /vehicules", modVehicules, appli.Membre, m.listeVehicules)
	a.Route("GET /vehicules/nouveau", modVehicules, appli.Membre, m.formVehicule)
	a.Route("POST /vehicules", modVehicules, appli.Membre, m.enregistrerVehicule)
	a.Route("GET /vehicules/{id}", modVehicules, appli.Membre, m.ficheVehicule)
	a.Route("GET /vehicules/plein", modVehicules, appli.Membre, m.raccourciPlein)
	a.AjouterRecherche(modVehicules, m.chercherVehicules)
	a.Route("GET /vehicules/{id}/modifier", modVehicules, appli.Membre, m.formVehicule)
	a.Route("POST /vehicules/{id}", modVehicules, appli.Membre, m.enregistrerVehicule)
	a.Route("POST /vehicules/{id}/km", modVehicules, appli.Membre, m.kilometrage)
	a.Route("POST /vehicules/{id}/fuelio", modVehicules, appli.Parent, m.importerFuelio)
	a.Route("POST /vehicules/fuelio", modVehicules, appli.Parent, m.importerFuelioTous)
	a.Route("POST /vehicules/{id}/supprimer", modVehicules, appli.Parent, m.supprimerVehicule)

	m.s.Enregistrer(suivi.Objet{
		Module: modVehicules, Emoji: "🚗", AvecKm: true, Categorie: "transport",
		Modeles: []suivi.Modele{{Libelle: "Contrôle technique", Mois: 24}, {Libelle: "Assurance", Mois: 12}, {Libelle: "Vidange", Mois: 12}, {Libelle: "Révision", Mois: 12},
			{Libelle: "Pneus", Mois: 0}, {Libelle: "Courroie de distribution", Mois: 0}, {Libelle: "Carte grise", Mois: 0}, {Libelle: "Vignette Crit'Air", Mois: 0}},
		Nom: func(ctx context.Context, famille, id int64) (string, bool) {
			v, err := a.Q.VehiculeParID(ctx, donnees.VehiculeParIDParams{ID: id, FamilleID: famille})
			return v.Nom, err == nil
		},
		URL: func(id int64) string { return fmt.Sprintf("/vehicules/%d", id) },
		Noms: func(ctx context.Context, famille int64) (map[int64]string, error) {
			l, err := a.Q.NomsVehicules(ctx, famille)
			noms := make(map[int64]string, len(l))
			for _, x := range l {
				noms[x.ID] = x.Nom
			}
			return noms, err
		},
		Budget: func(ctx context.Context, famille, id int64) (string, int64) {
			v, err := a.Q.VehiculeParID(ctx, donnees.VehiculeParIDParams{ID: id, FamilleID: famille})
			if err != nil {
				return suivi.BudgetNon, 0
			}
			return v.Budget, v.BudgetPersoDe
		},
		NoterKm: func(ctx context.Context, famille, id, km int64) error {
			return a.Q.NoterKilometrage(ctx, donnees.NoterKilometrageParams{Kilometrage: km, ID: id, FamilleID: famille, Kilometrage_2: km})
		},
	})
	a.AjouterResume(modVehicules, func(ctx context.Context, c *appli.Contexte) appli.Resume {
		tous, err := a.Q.VehiculesFamille(ctx, c.Famille.ID)
		var l []donnees.Vehicule
		for _, x := range tous {
			if x.VenduLe == "" {
				l = append(l, x)
			}
		}
		if err != nil || len(l) == 0 {
			return appli.Resume{Info: "Aucun véhicule"}
		}
		alertes := m.alertes(ctx, c, modVehicules)
		for _, v := range l {
			if al := alertes[v.ID]; al != "" {
				return appli.Resume{Info: al, Badge: 1}
			}
		}
		return appli.Resume{Info: appli.Pluriel(len(l), "véhicule", "véhicules")}
	})
	a.AjouterAction(appli.Ajout{Module: modVehicules, Emoji: "⛽", Texte: "Un plein", URL: "/vehicules/plein"})
	a.AjouterAction(appli.Ajout{Module: modVehicules, Emoji: "🚗", Texte: "Un véhicule", URL: "/vehicules/nouveau"})
}

// raccourciPlein : « Ajouter un plein » (page Ajouter, appui long sur
// l'icône de l'appli) : la fiche du véhicule du dernier plein (ou le seul),
// fenêtre d'ajout ouverte.
func (m *Module) raccourciPlein(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	tous, err := m.a.Q.VehiculesFamille(ctx, c.Famille.ID)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	var l []donnees.Vehicule // ceux qui sont encore à la famille
	for _, x := range tous {
		if x.VenduLe == "" {
			l = append(l, x)
		}
	}
	if len(l) == 0 {
		m.a.Flash(w, "Ajoutez d'abord un véhicule")
		m.a.Rediriger(w, r, "/vehicules/nouveau")
		return
	}
	id := l[0].ID
	if d, err := m.a.Q.DernierObjetPlein(ctx, donnees.DernierObjetPleinParams{FamilleID: c.Famille.ID, Module: modVehicules}); err == nil {
		for _, x := range l {
			if x.ID == d {
				id = d
			}
		}
	}
	m.a.Rediriger(w, r, fmt.Sprintf("/vehicules/%d?ajout=plein", id))
}

func (m *Module) vehicule(r *http.Request, c *appli.Contexte) (donnees.Vehicule, bool) {
	v, err := m.a.Q.VehiculeParID(r.Context(), donnees.VehiculeParIDParams{ID: idDe(r), FamilleID: c.Famille.ID})
	return v, err == nil
}

func (m *Module) listeVehicules(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	l, err := m.a.Q.VehiculesFamille(r.Context(), c.Famille.ID)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	v := vueListe{Titre: "Véhicules", Emoji: "🚗", Module: modVehicules, Nouveau: "/vehicules/nouveau", TexteNouveau: "＋ Véhicule",
		Vide: "Ajoutez la voiture, le scooter… pour ne plus oublier le contrôle technique ou l'assurance."}
	alertes := m.alertes(r.Context(), c, modVehicules)
	for _, x := range l {
		detail := x.Modele
		if x.Immatriculation != "" {
			detail = strings.TrimSpace(x.Immatriculation + " · " + detail)
		}
		e := Element{URL: fmt.Sprintf("/vehicules/%d", x.ID), Emoji: emojiGenre(x.Genre), Nom: x.Nom, Detail: strings.Trim(detail, " ·")}
		// Ancien véhicule : à part, en bas, sans alerte.
		if x.VenduLe != "" {
			e.Detail = strings.Trim("plus à nous depuis le "+dateFR(x.VenduLe)+" · "+e.Detail, " ·")
			v.Anciens = append(v.Anciens, e)
			continue
		}
		e.Alerte = alertes[x.ID]
		v.Elements = append(v.Elements, e)
	}
	if c.EstParent() {
		v.Fuelio = "/vehicules/fuelio"
		if peutBudget(c) {
			v.FuelioBudget = m.optionsBudget(r.Context(), c, nil)
		}
	}
	m.a.Afficher(w, r, c, "foyer/liste", "Véhicules", modVehicules, v)
}

func (m *Module) ficheVehicule(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	x, ok := m.vehicule(r, c)
	if !ok {
		m.a.PageErreur(w, r, c, http.StatusNotFound, "Véhicule introuvable", "Ce véhicule n'existe plus.")
		return
	}
	v := vueFiche{Retour: "/vehicules", TitreRetour: "Véhicules", Emoji: emojiGenre(x.Genre), Nom: x.Nom, Sous: x.Modele,
		TitreSuivi: "Échéances", TitreCarnet: "Carnet d'entretien",
		Modifier: fmt.Sprintf("/vehicules/%d/modifier", x.ID), Supprimer: fmt.Sprintf("/vehicules/%d/supprimer", x.ID)}
	if x.VenduLe != "" {
		v.Infos = append(v.Infos, "🏁 Ancien véhicule : plus à nous depuis le "+dateFR(x.VenduLe)+" (plus de rappels)")
	}
	if x.Immatriculation != "" {
		v.Infos = append(v.Infos, "🔖 "+x.Immatriculation)
	}
	v.Kilometrage = x.Kilometrage // dans le résumé, en haut de la fiche
	if x.Notes != "" {
		v.Notes = x.Notes
	}
	if ct, ok := m.citer(r.Context(), c, "🔧 Garage", x.GarageID); ok {
		v.Contacts = append(v.Contacts, ct)
	}
	if ct, ok := m.citer(r.Context(), c, "🛡️ Assurance", x.AssureurID); ok {
		v.Contacts = append(v.Contacts, ct)
	}
	v.KmURL = fmt.Sprintf("/vehicules/%d/km", x.ID)
	// Les autres véhicules : on peut changer de véhicule dans la fenêtre « Ajouter un plein ».
	if l, err := m.a.Q.VehiculesFamille(r.Context(), c.Famille.ID); err == nil && len(l) > 1 {
		for _, y := range l {
			if y.ID != x.ID && y.VenduLe == "" {
				v.Autres = append(v.Autres, Lien{URL: template.URL(fmt.Sprintf("/vehicules/%d?ajout=plein", y.ID)), Emoji: emojiGenre(y.Genre), Texte: y.Nom})
			}
		}
	}
	if c.EstParent() {
		v.FuelioURL = fmt.Sprintf("/vehicules/%d/fuelio", x.ID)
	}
	if peutBudget(c) {
		v.FuelioBudget = m.optionsBudget(r.Context(), c, &x)
		if t := m.texteBudget(r.Context(), c, x); t != "" {
			v.Infos = append(v.Infos, "💶 Dépenses au budget : "+t)
		}
	}
	m.afficherFiche(w, r, c, modVehicules, x.ID, v)
}

type formVehicule struct {
	Nouveau  bool
	V        donnees.Vehicule
	Genres   any
	Contacts []donnees.ContactsChoixRow
	Budget   []OptionBudget // « 💶 Dépenses au budget » (parents, budget activé)
}

func (m *Module) formVehicule(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	f := formVehicule{Nouveau: r.PathValue("id") == "", Genres: Genres, V: donnees.Vehicule{Genre: "voiture"}}
	if !f.Nouveau {
		x, ok := m.vehicule(r, c)
		if !ok {
			m.a.PageErreur(w, r, c, http.StatusNotFound, "Véhicule introuvable", "Ce véhicule n'existe plus.")
			return
		}
		f.V = x
	}
	f.Contacts = m.contactsChoix(r.Context(), c)
	if peutBudget(c) {
		f.Budget = m.optionsBudget(r.Context(), c, &f.V)
	}
	m.a.Afficher(w, r, c, "foyer/vehicule", "Véhicule", modVehicules, f)
}

func (m *Module) enregistrerVehicule(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	nom, err1 := texte(r, "nom", 60)
	immat, err2 := texte(r, "immatriculation", 20)
	modele, err3 := texte(r, "modele", 80)
	notes, err4 := texte(r, "notes", 1000)
	if err := firstErr(err1, err2, err3, err4); err != nil || nom == "" {
		m.a.Echec(w, r, http.StatusBadRequest, "Nom (60 caractères), immatriculation, modèle ou notes invalides.")
		return
	}
	genre := r.PostFormValue("genre")
	if emojiGenre(genre) == "🚙" && genre != "autre" {
		genre = "autre"
	}
	// Un kilométrage illisible est refusé (avant : remis à 0 sans rien dire).
	km, ok := lireKm(r.PostFormValue("kilometrage"))
	if !ok {
		m.a.Echec(w, r, http.StatusBadRequest, "Kilométrage invalide (un nombre, par exemple 45000).")
		return
	}
	immat = strings.ToUpper(immat)
	id := idDe(r)
	if r.PathValue("id") == "" {
		if n, err := m.a.Q.CompterVehicules(ctx, c.Famille.ID); err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		} else if n >= ObjetsMax {
			m.a.Echec(w, r, http.StatusBadRequest, fmt.Sprintf("Trop de véhicules (%d au plus).", ObjetsMax))
			return
		}
		v, err := m.a.Q.CreerVehicule(ctx, donnees.CreerVehiculeParams{FamilleID: c.Famille.ID, Nom: nom, Genre: genre,
			Immatriculation: immat, Modele: modele, Kilometrage: km, Notes: notes, GarageID: m.lireContact(r, c, "garage_id"),
			AssureurID: m.lireContact(r, c, "assureur_id"), CreeLe: time.Now().Unix()})
		if err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
		id = v.ID
	} else {
		if _, ok := m.vehicule(r, c); !ok {
			m.a.Echec(w, r, http.StatusNotFound, "Véhicule introuvable.")
			return
		}
		if err := m.a.Q.ModifierVehicule(ctx, donnees.ModifierVehiculeParams{Nom: nom, Genre: genre, Immatriculation: immat,
			Modele: modele, Kilometrage: km, Notes: notes, GarageID: m.lireContact(r, c, "garage_id"),
			AssureurID: m.lireContact(r, c, "assureur_id"), ID: id, FamilleID: c.Famille.ID}); err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
	}
	// Ancien véhicule : « Je ne l'ai plus », depuis le (aujourd'hui si la
	// date manque ou est dans le futur).
	vendu := ""
	if r.PostFormValue("ancien") == "1" {
		vendu = c.Maintenant().Format("2006-01-02")
		if d, err := time.Parse("2006-01-02", r.PostFormValue("vendu_le")); err == nil && d.Year() >= 1950 && d.Format("2006-01-02") <= vendu {
			vendu = d.Format("2006-01-02")
		}
	}
	if err := m.a.Q.ChangerVenduVehicule(ctx, donnees.ChangerVenduVehiculeParams{VenduLe: vendu, ID: id, FamilleID: c.Famille.ID}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	// Dépenses au budget : le réglage vaut aussi pour le carnet déjà noté.
	msg := "Véhicule enregistré ✓"
	if x, err := m.a.Q.VehiculeParID(ctx, donnees.VehiculeParIDParams{ID: id, FamilleID: c.Famille.ID}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	} else if ret, err := m.reglerBudget(ctx, c, x, r.PostFormValue("budget"), nil); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	} else if t := ret.Message(); t != "" {
		msg += " · " + t
	}
	m.a.Publier(c, modVehicules, "")
	m.a.Flash(w, msg)
	m.a.Rediriger(w, r, fmt.Sprintf("/vehicules/%d", id))
}

// dateFR : « 2025-03-12 » → « 12/03/2025 ».
func dateFR(j string) string {
	if t, err := time.Parse("2006-01-02", j); err == nil {
		return t.Format("02/01/2006")
	}
	return j
}

// kilometrage : mise à jour rapide du compteur (il ne peut que monter).
func (m *Module) kilometrage(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	x, ok := m.vehicule(r, c)
	km, okKm := lireKm(r.PostFormValue("km"))
	if !ok || !okKm || strings.TrimSpace(r.PostFormValue("km")) == "" {
		m.a.Echec(w, r, http.StatusBadRequest, "Kilométrage invalide.")
		return
	}
	// Le compteur ne recule jamais : on le dit au lieu d'afficher « enregistré ».
	if km < x.Kilometrage {
		m.a.Echec(w, r, http.StatusBadRequest, "Le compteur indique déjà "+appli.Milliers(x.Kilometrage)+" km.")
		return
	}
	if err := m.a.Q.NoterKilometrage(r.Context(), donnees.NoterKilometrageParams{Kilometrage: km, ID: x.ID, FamilleID: c.Famille.ID, Kilometrage_2: km}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, modVehicules, "")
	m.a.Flash(w, "Kilométrage enregistré ✓")
	m.a.Rediriger(w, r, fmt.Sprintf("/vehicules/%d", x.ID))
}

func (m *Module) supprimerVehicule(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	x, ok := m.vehicule(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Véhicule introuvable.")
		return
	}
	m.supprimerObjet(w, r, c, modVehicules, x.ID, x.Nom, "/vehicules", func(ctx context.Context, q *donnees.Queries) error {
		return q.SupprimerVehicule(ctx, donnees.SupprimerVehiculeParams{ID: x.ID, FamilleID: c.Famille.ID})
	})
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}
