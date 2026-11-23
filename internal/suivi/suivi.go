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
	"errors"
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
	// SansEcheances : seulement le carnet (projets : les étapes tiennent lieu d'échéances).
	SansEcheances bool
	// Nom : le nom de l'objet {id} de la famille ; ok = faux s'il n'existe pas.
	Nom func(ctx context.Context, famille, id int64) (nom string, ok bool)
	// URL : la fiche de l'objet.
	URL func(id int64) string
	// Noms (facultatif) : les noms de tous les objets de la famille, en une
	// requête (rappels : pas une requête par échéance).
	Noms func(ctx context.Context, famille int64) (map[int64]string, error)
	// NoterKm (facultatif) : un kilométrage saisi au carnet met la fiche à jour.
	NoterKm func(ctx context.Context, famille, id, km int64) error
}

// Plafonds par objet (garde-fous : la base ne grossit pas sans fin).
const (
	EcheancesMax = 100
	CarnetMax    = 5000
)

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
	if o.SansEcheances {
		return
	}
	s.a.AjouterEcheances(o.Module, func(ctx context.Context, f donnees.Famille, debut, fin time.Time) ([]appli.Echeance, error) {
		l, err := s.a.Q.EcheancesPeriode(ctx, donnees.EcheancesPeriodeParams{FamilleID: f.ID, Module: o.Module,
			Du: debut.Format("2006-01-02"), Au: fin.Format("2006-01-02")})
		if err != nil {
			return nil, err
		}
		if len(l) == 0 {
			return nil, nil
		}
		// Les noms : tous d'un coup si le module sait le faire.
		nomDe := func(id int64) (string, bool) { return o.Nom(ctx, f.ID, id) }
		if o.Noms != nil {
			noms, err := o.Noms(ctx, f.ID)
			if err != nil {
				return nil, err
			}
			nomDe = func(id int64) (string, bool) { n, ok := noms[id]; return n, ok }
		}
		var out []appli.Echeance
		for _, e := range l {
			nom, ok := nomDe(e.ObjetID)
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
	Module        string
	ObjetID       int64
	Echeances     []LigneEcheance
	Carnet        []donnees.Carnet
	TotalAnnee    int64
	Total         int64 // depuis le début (tout le carnet, pas seulement les lignes affichées)
	SansEcheances bool
	Modeles       []Modele
	AvecKm        bool
	Devise        string
	Aujourdhui    string
}

// Bloc : les échéances et le carnet de l'objet.
func (s *Suivi) Bloc(ctx context.Context, c *appli.Contexte, module string, objet int64) (*Bloc, error) {
	o := s.objets[module]
	auj := c.Maintenant()
	b := &Bloc{Module: module, ObjetID: objet, Modeles: o.Modeles, AvecKm: o.AvecKm, SansEcheances: o.SansEcheances, Devise: c.Famille.Devise, Aujourdhui: auj.Format("2006-01-02")}
	ech, err := s.a.Q.EcheancesObjet(ctx, donnees.EcheancesObjetParams{FamilleID: c.Famille.ID, Module: module, ObjetID: objet})
	if err != nil {
		return nil, err
	}
	for _, e := range ech {
		l := LigneEcheance{Echeance: e}
		l.Quand, l.Retard, l.Proche = quand(e.Jour, c)
		b.Echeances = append(b.Echeances, l)
	}
	// La liste montre les 100 dernières lignes ; les totaux comptent tout.
	if b.Carnet, err = s.a.Q.CarnetObjet(ctx, donnees.CarnetObjetParams{FamilleID: c.Famille.ID, Module: module, ObjetID: objet}); err != nil {
		return nil, err
	}
	t, err := s.a.Q.TotauxCarnetObjet(ctx, donnees.TotauxCarnetObjetParams{FamilleID: c.Famille.ID, Module: module, ObjetID: objet,
		DebutAnnee: fmt.Sprintf("%04d-01-01", auj.Year()), FinAnnee: fmt.Sprintf("%04d-01-01", auj.Year()+1)})
	if err != nil {
		return nil, err
	}
	b.Total, b.TotalAnnee = t.Total, t.Annee
	return b, nil
}

// quand : « Dans 12 jours », « En retard (…) » pour une échéance ; proche =
// dans les 30 jours.
func quand(jour string, c *appli.Contexte) (texte string, retard, proche bool) {
	j, err := time.ParseInLocation("2006-01-02", jour, c.Lieu)
	if err != nil {
		return "", false, false
	}
	auj := c.Maintenant()
	minuit := time.Date(auj.Year(), auj.Month(), auj.Day(), 0, 0, 0, 0, c.Lieu)
	if j.Before(minuit) {
		return "En retard (" + appli.DateLongue(j) + ")", true, false
	}
	return appli.Quand(j, minuit), false, j.Sub(minuit) <= 30*24*time.Hour
}

// minuscule : « Dans 3 jours » → « dans 3 jours » (au milieu d'une phrase).
func minuscule(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return strings.ToLower(string(r)) + s[n:]
}

// Alertes : pour chaque objet du module, sa première échéance si elle est en
// retard ou proche (« ⏰ Vaccin : dans 3 jours »). Une seule requête pour
// toute une liste.
func (s *Suivi) Alertes(ctx context.Context, c *appli.Contexte, module string) (map[int64]string, error) {
	l, err := s.a.Q.PremieresEcheancesModule(ctx, donnees.PremieresEcheancesModuleParams{FamilleID: c.Famille.ID, Module: module})
	if err != nil {
		return nil, err
	}
	out := map[int64]string{}
	for _, e := range l {
		if texte, retard, proche := quand(e.Jour, c); texte != "" && (retard || proche) {
			out[e.ObjetID] = "⏰ " + e.Libelle + " : " + minuscule(texte)
		}
	}
	return out, nil
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

// publier : les écrans ouverts sur ce module se rafraîchissent.
func (s *Suivi) publier(c *appli.Contexte, o Objet) { s.a.Publier(c, o.Module, "") }

func (s *Suivi) ajouterEcheance(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	objet, _ := strconv.ParseInt(r.PathValue("objet"), 10, 64)
	o, ok := s.cible(r, c, r.PathValue("module"), objet)
	if !ok || o.SansEcheances {
		s.a.Echec(w, r, http.StatusNotFound, "Introuvable.")
		return
	}
	libelle := strings.TrimSpace(r.PostFormValue("libelle"))
	jour, okJour := budget.LireJour(r.PostFormValue("jour"), c.Maintenant())
	mois, _ := strconv.ParseInt(r.PostFormValue("repetition_mois"), 10, 64)
	if n := utf8.RuneCountInString(libelle); n < 1 || n > 80 || !okJour || mois < 0 || mois > 120 {
		s.a.Echec(w, r, http.StatusBadRequest, "Indiquez quoi (80 caractères au plus) et une date.")
		return
	}
	if n, err := s.a.Q.CompterEcheancesObjet(r.Context(), donnees.CompterEcheancesObjetParams{FamilleID: c.Famille.ID, Module: o.Module, ObjetID: objet}); err != nil {
		s.a.ErreurInterne(w, r, err)
		return
	} else if n >= EcheancesMax {
		s.a.Echec(w, r, http.StatusBadRequest, fmt.Sprintf("Trop d'échéances ici (%d au plus) : supprimez-en d'abord.", EcheancesMax))
		return
	}
	if _, err := s.a.Q.CreerEcheance(r.Context(), donnees.CreerEcheanceParams{
		FamilleID: c.Famille.ID, Module: o.Module, ObjetID: objet, Libelle: libelle, Jour: jour, RepetitionMois: mois, CreeLe: time.Now().Unix(),
	}); err != nil {
		s.a.ErreurInterne(w, r, err)
		return
	}
	s.publier(c, o)
	s.a.Flash(w, "Échéance ajoutée ⏰")
	s.a.Rediriger(w, r, o.URL(objet))
}

// ligneCarnet : date, libellé, coût, kilométrage et note lus dans le formulaire.
func lireCarnet(r *http.Request, c *appli.Contexte) (jour, libelle, note string, montant, km int64, msg string) {
	jour, ok := budget.LireJour(r.PostFormValue("jour"), c.Maintenant())
	if !ok {
		return "", "", "", 0, 0, "Date invalide."
	}
	libelle = strings.TrimSpace(r.PostFormValue("libelle"))
	note = strings.TrimSpace(r.PostFormValue("note"))
	if utf8.RuneCountInString(libelle) > 80 || utf8.RuneCountInString(note) > 500 {
		return "", "", "", 0, 0, "Texte trop long."
	}
	if m := strings.TrimSpace(r.PostFormValue("montant")); m != "" {
		// « 0 » est accepté : une intervention gratuite.
		v, err := budget.LireMontantOuZero(m)
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

func (s *Suivi) ecrireCarnet(ctx context.Context, q *donnees.Queries, c *appli.Contexte, o Objet, objet int64, jour, libelle, note string, montant, km int64) error {
	_, err := q.AjouterCarnet(ctx, donnees.AjouterCarnetParams{
		FamilleID: c.Famille.ID, Module: o.Module, ObjetID: objet, Jour: jour, Libelle: libelle, Montant: montant, Km: km,
		Note: note, CreePar: memberID(c), CreeLe: time.Now().Unix(),
	})
	return err
}

// noterKm : un kilométrage saisi met la fiche à jour (hors transaction).
// La ligne du carnet est déjà écrite : une erreur ici va au journal.
func (s *Suivi) noterKm(ctx context.Context, c *appli.Contexte, o Objet, objet, km int64) {
	if km > 0 && o.NoterKm != nil {
		if err := o.NoterKm(ctx, c.Famille.ID, objet, km); err != nil {
			s.a.Journal.Error("suivi : kilométrage non noté", "module", o.Module, "objet", objet, "erreur", err)
		}
	}
}

// AjouterMois : n mois plus tard, le même jour, ou le dernier jour du mois
// s'il n'existe pas (31 janvier + 1 mois = 28 février, pas 3 mars).
func AjouterMois(t time.Time, n int) time.Time {
	premier := time.Date(t.Year(), t.Month()+time.Month(n), 1, 0, 0, 0, 0, t.Location())
	return premier.AddDate(0, 0, min(t.Day(), premier.AddDate(0, 1, -1).Day())-1)
}

// errDejaFaite : l'échéance a déjà été notée (double envoi).
var errDejaFaite = errors.New("échéance déjà notée")

func (s *Suivi) ajouterCarnet(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	objet, _ := strconv.ParseInt(r.PathValue("objet"), 10, 64)
	o, ok := s.cible(r, c, r.PathValue("module"), objet)
	if !ok {
		s.a.Echec(w, r, http.StatusNotFound, "Introuvable.")
		return
	}
	jour, libelle, note, montant, km, msg := lireCarnet(r, c)
	if msg == "" && libelle == "" {
		msg = "Indiquez ce qui a été fait."
	}
	if msg != "" {
		s.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	ctx := r.Context()
	if n, err := s.a.Q.CompterCarnetObjet(ctx, donnees.CompterCarnetObjetParams{FamilleID: c.Famille.ID, Module: o.Module, ObjetID: objet}); err != nil {
		s.a.ErreurInterne(w, r, err)
		return
	} else if n >= CarnetMax {
		s.a.Echec(w, r, http.StatusBadRequest, fmt.Sprintf("Ce carnet est plein (%d lignes au plus).", CarnetMax))
		return
	}
	// Double envoi (double clic, réseau lent) : la même ligne, écrite par la
	// même personne dans les 10 dernières secondes, n'est pas répétée.
	deja, err := s.a.Q.CarnetIdentiqueRecent(ctx, donnees.CarnetIdentiqueRecentParams{FamilleID: c.Famille.ID, Module: o.Module, ObjetID: objet,
		Jour: jour, Libelle: libelle, Montant: montant, Km: km, Note: note, CreePar: memberID(c), CreeLe: time.Now().Unix() - 10})
	if err == nil && deja == 0 {
		err = s.ecrireCarnet(ctx, s.a.Q, c, o, objet, jour, libelle, note, montant, km)
	}
	if err != nil {
		s.a.ErreurInterne(w, r, err)
		return
	}
	s.noterKm(ctx, c, o, objet, km)
	s.publier(c, o)
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
	jour, _, note, montant, km, msg := lireCarnet(r, c)
	if msg != "" {
		s.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	// Une seule fois, même si le bouton est envoyé deux fois : l'échéance
	// n'est reportée (ou retirée) que si elle est encore à sa date.
	var suivante time.Time
	err = s.a.Transaction(ctx, func(q *donnees.Queries) error {
		var n int64
		var err error
		if e.RepetitionMois > 0 {
			// Reportée à partir de la date où elle a été faite.
			fait, _ := time.Parse("2006-01-02", jour)
			suivante = AjouterMois(fait, int(e.RepetitionMois))
			n, err = q.ReporterEcheanceSiJour(ctx, donnees.ReporterEcheanceSiJourParams{Nouveau: suivante.Format("2006-01-02"),
				ID: e.ID, FamilleID: c.Famille.ID, Ancien: e.Jour})
		} else {
			n, err = q.SupprimerEcheanceSiJour(ctx, donnees.SupprimerEcheanceSiJourParams{ID: e.ID, FamilleID: c.Famille.ID, Jour: e.Jour})
		}
		if err != nil || n != 1 {
			return errors.Join(err, errDejaFaite)
		}
		return s.ecrireCarnet(ctx, q, c, o, e.ObjetID, jour, e.Libelle, note, montant, km)
	})
	switch {
	case errors.Is(err, errDejaFaite):
		s.a.Flash(w, "Déjà noté ✓")
	case err != nil:
		s.a.ErreurInterne(w, r, err)
		return
	case e.RepetitionMois > 0:
		s.noterKm(ctx, c, o, e.ObjetID, km)
		s.publier(c, o)
		s.a.Flash(w, fmt.Sprintf("Fait ✓ Prochaine fois : %s", appli.DateLongue(suivante)))
	default:
		s.noterKm(ctx, c, o, e.ObjetID, km)
		s.publier(c, o)
		s.a.Flash(w, "Fait ✓")
	}
	s.a.Rediriger(w, r, o.URL(e.ObjetID))
}

func (s *Suivi) supprimerEcheance(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	e, err := s.a.Q.EcheanceParID(r.Context(), donnees.EcheanceParIDParams{ID: id, FamilleID: c.Famille.ID})
	o, ok := s.objets[e.Module]
	if err != nil || !ok || !c.Actif(e.Module) {
		s.a.Echec(w, r, http.StatusNotFound, "Échéance introuvable.")
		return
	}
	if err := s.a.Q.SupprimerEcheance(r.Context(), donnees.SupprimerEcheanceParams{ID: e.ID, FamilleID: c.Famille.ID}); err != nil {
		s.a.ErreurInterne(w, r, err)
		return
	}
	s.publier(c, o)
	s.a.Flash(w, "Échéance supprimée")
	s.a.Rediriger(w, r, o.URL(e.ObjetID))
}

func (s *Suivi) supprimerCarnet(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	l, err := s.a.Q.LigneCarnetParID(r.Context(), donnees.LigneCarnetParIDParams{ID: id, FamilleID: c.Famille.ID})
	o, ok := s.objets[l.Module]
	if err != nil || !ok || !c.Actif(l.Module) {
		s.a.Echec(w, r, http.StatusNotFound, "Ligne introuvable.")
		return
	}
	if err := s.a.Q.SupprimerLigneCarnet(r.Context(), donnees.SupprimerLigneCarnetParams{ID: l.ID, FamilleID: c.Famille.ID}); err != nil {
		s.a.ErreurInterne(w, r, err)
		return
	}
	s.publier(c, o)
	s.a.Flash(w, "Ligne supprimée")
	s.a.Rediriger(w, r, o.URL(l.ObjetID))
}

func memberID(c *appli.Contexte) sql.NullInt64 { return sql.NullInt64{Int64: c.Membre.ID, Valid: true} }
