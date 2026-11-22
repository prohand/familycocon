// SPDX-License-Identifier: GPL-3.0-or-later

// Package suivi : la mécanique commune aux modules Véhicules, Animaux et
// Prestataires.
//   - Échéances : « Contrôle technique le 15 mars », « Vaccin tous les
//     12 mois »… Elles apparaissent dans « À venir » et dans les rappels.
//   - Carnet : ce qui a été fait (date, coût, kilométrage, note).
//
// Marquer une échéance « faite » l'écrit au carnet et la reporte (si elle
// revient) ou l'enlève.
//
// Droits : tout le monde ajoute et coche ; seuls les parents suppriment.
package suivi

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
)

// Modele : une échéance proposée (« Contrôle technique », 24 mois).
type Modele struct {
	Libelle string
	Mois    int64
}

// Objet : ce que suit un module (un véhicule, un animal, un prestataire).
type Objet struct {
	Module  string
	Emoji   string
	Modeles []Modele
	AvecKm  bool
	// Nom : le nom de l'objet {id} de la famille ; ok = faux s'il n'existe pas.
	Nom func(ctx context.Context, famille, id int64) (nom string, ok bool)
	// URL : la fiche de l'objet.
	URL func(id int64) string
	// NoterKm (facultatif) : un kilométrage saisi au carnet met la fiche à jour.
	NoterKm func(ctx context.Context, famille, id, km int64)
}

// Suivi : les objets suivis, par module.
type Suivi struct {
	a      *appli.App
	objets map[string]Objet
}

// Brancher ajoute les routes communes.
func Brancher(a *appli.App) *Suivi {
	s := &Suivi{a: a, objets: map[string]Objet{}}
	a.Route("POST /suivi/{module}/{objet}/echeances", "", appli.Membre, s.ajouterEcheance)
	a.Route("POST /suivi/{module}/{objet}/carnet", "", appli.Membre, s.ajouterCarnet)
	a.Route("POST /suivi/echeances/{id}/faite", "", appli.Membre, s.faite)
	a.Route("POST /suivi/echeances/{id}/supprimer", "", appli.Parent, s.supprimerEcheance)
	a.Route("POST /suivi/carnet/{id}/supprimer", "", appli.Parent, s.supprimerCarnet)
	return s
}

// Enregistrer : un module confie ses objets au suivi (et ses échéances aux rappels).
func (s *Suivi) Enregistrer(o Objet) {
	s.objets[o.Module] = o
	s.a.AjouterEcheances(o.Module, func(ctx context.Context, f donnees.Famille, debut, fin time.Time) ([]appli.Echeance, error) {
		l, err := s.a.Q.EcheancesPeriode(ctx, donnees.EcheancesPeriodeParams{FamilleID: f.ID, Module: o.Module,
			Du: debut.Format("2006-01-02"), Au: fin.Format("2006-01-02")})
		if err != nil {
			return nil, err
		}
		var out []appli.Echeance
		for _, e := range l {
			nom, ok := o.Nom(ctx, f.ID, e.ObjetID)
			if !ok {
				continue
			}
			j, err := time.ParseInLocation("2006-01-02", e.Jour, debut.Location())
			if err != nil {
				continue
			}
			out = append(out, appli.Echeance{Jour: j, Titre: e.Libelle + " · " + nom, Emoji: o.Emoji, URL: o.URL(e.ObjetID), Module: o.Module})
		}
		return out, nil
	})
}

// LigneEcheance : une échéance et quand elle tombe.
type LigneEcheance struct {
	donnees.Echeance
	Quand  string
	Retard bool
	Proche bool
}

// Bloc : échéances et carnet d'un objet (affichés sur sa fiche).
type Bloc struct {
	Module     string
	ObjetID    int64
	Echeances  []LigneEcheance
	Carnet     []donnees.Carnet
	TotalAnnee int64
	Modeles    []Modele
	AvecKm     bool
	Devise     string
	Aujourdhui string
}

// Bloc : les échéances et le carnet de l'objet.
func (s *Suivi) Bloc(ctx context.Context, c *appli.Contexte, module string, objet int64) (*Bloc, error) {
	o := s.objets[module]
	auj := c.Maintenant()
	b := &Bloc{Module: module, ObjetID: objet, Modeles: o.Modeles, AvecKm: o.AvecKm, Devise: c.Famille.Devise, Aujourdhui: auj.Format("2006-01-02")}
	ech, err := s.a.Q.EcheancesObjet(ctx, donnees.EcheancesObjetParams{FamilleID: c.Famille.ID, Module: module, ObjetID: objet})
	if err != nil {
		return nil, err
	}
	minuit := time.Date(auj.Year(), auj.Month(), auj.Day(), 0, 0, 0, 0, c.Lieu)
	for _, e := range ech {
		l := LigneEcheance{Echeance: e}
		if j, err := time.ParseInLocation("2006-01-02", e.Jour, c.Lieu); err == nil {
			l.Quand = appli.Quand(j, minuit)
			l.Retard = j.Before(minuit)
			l.Proche = !l.Retard && j.Sub(minuit) <= 30*24*time.Hour
			if l.Retard {
				l.Quand = "En retard (" + appli.DateLongue(j) + ")"
			}
		}
		b.Echeances = append(b.Echeances, l)
	}
	if b.Carnet, err = s.a.Q.CarnetObjet(ctx, donnees.CarnetObjetParams{FamilleID: c.Famille.ID, Module: module, ObjetID: objet}); err != nil {
		return nil, err
	}
	annee := strconv.Itoa(auj.Year())
	for _, l := range b.Carnet {
		if strings.HasPrefix(l.Jour, annee) {
			b.TotalAnnee += l.Montant
		}
	}
	return b, nil
}

// SupprimerObjet : efface les échéances et le carnet d'un objet supprimé.
func (s *Suivi) SupprimerObjet(ctx context.Context, q *donnees.Queries, famille int64, module string, objet int64) error {
	if err := q.SupprimerSuiviObjet(ctx, donnees.SupprimerSuiviObjetParams{FamilleID: famille, Module: module, ObjetID: objet}); err != nil {
		return err
	}
	return q.SupprimerCarnetObjet(ctx, donnees.SupprimerCarnetObjetParams{FamilleID: famille, Module: module, ObjetID: objet})
}

// cible : l'objet de l'adresse (module activé, objet de la famille).
func (s *Suivi) cible(r *http.Request, c *appli.Contexte, module string, objet int64) (Objet, bool) {
	o, ok := s.objets[module]
	if !ok || !c.Actif(module) {
		return o, false
	}
	_, existe := o.Nom(r.Context(), c.Famille.ID, objet)
	return o, existe
}

func lireJour(v string) (string, bool) {
	_, err := time.Parse("2006-01-02", v)
	return v, err == nil
}

func (s *Suivi) ajouterEcheance(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	objet, _ := strconv.ParseInt(r.PathValue("objet"), 10, 64)
	o, ok := s.cible(r, c, r.PathValue("module"), objet)
	if !ok {
		s.a.Echec(w, r, http.StatusNotFound, "Introuvable.")
		return
	}
	libelle := strings.TrimSpace(r.PostFormValue("libelle"))
	jour, okJour := lireJour(r.PostFormValue("jour"))
	mois, _ := strconv.ParseInt(r.PostFormValue("repetition_mois"), 10, 64)
	if n := utf8.RuneCountInString(libelle); n < 1 || n > 80 || !okJour || mois < 0 || mois > 120 {
		s.a.Echec(w, r, http.StatusBadRequest, "Indiquez quoi (80 caractères au plus) et une date.")
		return
	}
	if _, err := s.a.Q.CreerEcheance(r.Context(), donnees.CreerEcheanceParams{
		FamilleID: c.Famille.ID, Module: o.Module, ObjetID: objet, Libelle: libelle, Jour: jour, RepetitionMois: mois, CreeLe: time.Now().Unix(),
	}); err != nil {
		s.a.ErreurInterne(w, r, err)
		return
	}
	s.a.Flash(w, "Échéance ajoutée ⏰")
	s.a.Rediriger(w, r, o.URL(objet))
}

// ligneCarnet : date, libellé, coût, kilométrage et note lus dans le formulaire.
func lireCarnet(r *http.Request) (jour, libelle, note string, montant, km int64, msg string) {
	jour, ok := lireJour(r.PostFormValue("jour"))
	if !ok {
		return "", "", "", 0, 0, "Date invalide."
	}
	libelle = strings.TrimSpace(r.PostFormValue("libelle"))
	note = strings.TrimSpace(r.PostFormValue("note"))
	if utf8.RuneCountInString(libelle) > 80 || utf8.RuneCountInString(note) > 500 {
		return "", "", "", 0, 0, "Texte trop long."
	}
	if m := strings.TrimSpace(r.PostFormValue("montant")); m != "" {
		v, err := budget.LireMontant(m)
		if err != nil {
			return "", "", "", 0, 0, err.Error()
		}
		montant = v
	}
	if k := strings.TrimSpace(strings.ReplaceAll(r.PostFormValue("km"), " ", "")); k != "" {
		v, err := strconv.ParseInt(k, 10, 64)
		if err != nil || v < 0 || v > 5_000_000 {
			return "", "", "", 0, 0, "Kilométrage invalide."
		}
		km = v
	}
	return jour, libelle, note, montant, km, ""
}

func (s *Suivi) ecrireCarnet(ctx context.Context, c *appli.Contexte, o Objet, objet int64, jour, libelle, note string, montant, km int64) error {
	if _, err := s.a.Q.AjouterCarnet(ctx, donnees.AjouterCarnetParams{
		FamilleID: c.Famille.ID, Module: o.Module, ObjetID: objet, Jour: jour, Libelle: libelle, Montant: montant, Km: km,
		Note: note, CreePar: memberID(c), CreeLe: time.Now().Unix(),
	}); err != nil {
		return err
	}
	if km > 0 && o.NoterKm != nil {
		o.NoterKm(ctx, c.Famille.ID, objet, km)
	}
	return nil
}

func (s *Suivi) ajouterCarnet(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	objet, _ := strconv.ParseInt(r.PathValue("objet"), 10, 64)
	o, ok := s.cible(r, c, r.PathValue("module"), objet)
	if !ok {
		s.a.Echec(w, r, http.StatusNotFound, "Introuvable.")
		return
	}
	jour, libelle, note, montant, km, msg := lireCarnet(r)
	if msg == "" && libelle == "" {
		msg = "Indiquez ce qui a été fait."
	}
	if msg != "" {
		s.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	if err := s.ecrireCarnet(r.Context(), c, o, objet, jour, libelle, note, montant, km); err != nil {
		s.a.ErreurInterne(w, r, err)
		return
	}
	s.a.Flash(w, "Ajouté au carnet ✓")
	s.a.Rediriger(w, r, o.URL(objet))
}

// faite : l'échéance est faite → carnet, puis reportée ou enlevée.
func (s *Suivi) faite(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	e, err := s.a.Q.EcheanceParID(ctx, donnees.EcheanceParIDParams{ID: id, FamilleID: c.Famille.ID})
	if err != nil {
		s.a.Echec(w, r, http.StatusNotFound, "Échéance introuvable.")
		return
	}
	o, ok := s.cible(r, c, e.Module, e.ObjetID)
	if !ok {
		s.a.Echec(w, r, http.StatusNotFound, "Introuvable.")
		return
	}
	jour, _, note, montant, km, msg := lireCarnet(r)
	if msg != "" {
		s.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	if err := s.ecrireCarnet(ctx, c, o, e.ObjetID, jour, e.Libelle, note, montant, km); err != nil {
		s.a.ErreurInterne(w, r, err)
		return
	}
	if e.RepetitionMois > 0 {
		// Reportée à partir de la date où elle a été faite.
		fait, _ := time.Parse("2006-01-02", jour)
		suivante := fait.AddDate(0, int(e.RepetitionMois), 0).Format("2006-01-02")
		err = s.a.Q.ReporterEcheance(ctx, donnees.ReporterEcheanceParams{Jour: suivante, ID: e.ID, FamilleID: c.Famille.ID})
		s.a.Flash(w, fmt.Sprintf("Fait ✓ Prochaine fois : %s", appli.DateLongue(fait.AddDate(0, int(e.RepetitionMois), 0))))
	} else {
		err = s.a.Q.SupprimerEcheance(ctx, donnees.SupprimerEcheanceParams{ID: e.ID, FamilleID: c.Famille.ID})
		s.a.Flash(w, "Fait ✓")
	}
	if err != nil {
		s.a.ErreurInterne(w, r, err)
		return
	}
	s.a.Rediriger(w, r, o.URL(e.ObjetID))
}

func (s *Suivi) supprimerEcheance(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	e, err := s.a.Q.EcheanceParID(r.Context(), donnees.EcheanceParIDParams{ID: id, FamilleID: c.Famille.ID})
	o, ok := s.objets[e.Module]
	if err != nil || !ok {
		s.a.Echec(w, r, http.StatusNotFound, "Échéance introuvable.")
		return
	}
	if err := s.a.Q.SupprimerEcheance(r.Context(), donnees.SupprimerEcheanceParams{ID: e.ID, FamilleID: c.Famille.ID}); err != nil {
		s.a.ErreurInterne(w, r, err)
		return
	}
	s.a.Flash(w, "Échéance supprimée")
	s.a.Rediriger(w, r, o.URL(e.ObjetID))
}

func (s *Suivi) supprimerCarnet(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	l, err := s.a.Q.LigneCarnetParID(r.Context(), donnees.LigneCarnetParIDParams{ID: id, FamilleID: c.Famille.ID})
	o, ok := s.objets[l.Module]
	if err != nil || !ok {
		s.a.Echec(w, r, http.StatusNotFound, "Ligne introuvable.")
		return
	}
	if err := s.a.Q.SupprimerLigneCarnet(r.Context(), donnees.SupprimerLigneCarnetParams{ID: l.ID, FamilleID: c.Famille.ID}); err != nil {
		s.a.ErreurInterne(w, r, err)
		return
	}
	s.a.Flash(w, "Ligne supprimée")
	s.a.Rediriger(w, r, o.URL(l.ObjetID))
}

func memberID(c *appli.Contexte) sql.NullInt64 { return sql.NullInt64{Int64: c.Membre.ID, Valid: true} }
