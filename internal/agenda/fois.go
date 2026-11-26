// SPDX-License-Identifier: GPL-3.0-or-later

package agenda

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
	"familycocon/internal/repetition"
)

// « Modifier cette fois seulement » : une fois d'un événement qui revient
// change de date, d'heures, de lieu ou de titre, sans toucher aux autres.
// C'est une ligne de evenements_exceptions (modifiee = 1), repérée par la
// date d'origine de la fois (jour). La fois n'est plus montrée à sa date
// d'origine (comme une fois annulée) ; elle l'est à sa nouvelle date, dans
// l'agenda, l'accueil, le lien du téléphone et les rappels.

// fois : une fois d'un événement, à sa vraie date (d'origine ou modifiée).
type fois struct {
	evt        donnees.Evenement // titre, lieu, journée : ceux de cette fois
	debut, fin time.Time
	jour       string // événement qui revient : date d'origine de la fois
	modifiee   bool
}

// lesFois : les fois des événements evts dans [debut, fin[, sans les fois
// annulées ou modifiées, plus les fois modifiées de la famille qui tombent
// dans la période (à leur nouvelle date, même hors des dates de leur
// événement). garder : seulement les fois modifiées de ces événements-là
// (nil : toutes).
func (m *Module) lesFois(ctx context.Context, famille int64, lieu *time.Location, debut, fin time.Time,
	evts []donnees.Evenement, garder func(donnees.Evenement) bool) ([]fois, error) {
	ids := make([]int64, len(evts))
	for i := range evts {
		ids[i] = evts[i].ID
	}
	exc, err := m.exceptions(ctx, ids)
	if err != nil {
		return nil, err
	}
	var out []fois
	for _, e := range evts {
		for _, d := range deplierSans(e, lieu, debut, fin, exc[e.ID]) {
			f := fois{evt: e, debut: d[0], fin: d[1]}
			if e.Repetition != "" {
				f.jour = repetition.CleJour(d[0])
			}
			out = append(out, f)
		}
	}
	mods, err := m.a.Q.FoisModifieesPeriode(ctx, donnees.FoisModifieesPeriodeParams{FamilleID: famille,
		FinPeriode: fin.Unix(), DebutPeriode: debut.Unix()})
	if err != nil {
		return nil, err
	}
	for _, x := range mods {
		if garder != nil && !garder(x.Evenement) {
			continue
		}
		f, ok := foisModifiee(x.Evenement, lieu, ligneModifiee{Jour: x.Jour, NouveauDebut: x.NouveauDebut, NouveauFin: x.NouveauFin,
			NouvelleJournee: x.NouvelleJournee, NouveauTitre: x.NouveauTitre, NouveauLieu: x.NouveauLieu})
		if ok && chevauche(f.debut, f.fin, debut, fin) {
			out = append(out, f)
		}
	}
	return out, nil
}

// ligneModifiee : une fois modifiée telle qu'elle est en base.
type ligneModifiee struct {
	Jour            string
	NouveauDebut    int64
	NouveauFin      int64
	NouvelleJournee int64
	NouveauTitre    string
	NouveauLieu     sql.NullString
}

// foisModifiee : la fois modifiée, à sa nouvelle date. Faux si la date
// d'origine n'est plus une fois de l'événement (sa répétition a changé
// depuis) : la modification ne compte plus.
func foisModifiee(e donnees.Evenement, lieu *time.Location, x ligneModifiee) (fois, bool) {
	if _, ok := foisDu(e, lieu, x.Jour); !ok {
		return fois{}, false
	}
	if x.NouveauTitre != "" {
		e.Titre = x.NouveauTitre
	}
	if x.NouveauLieu.Valid {
		e.Lieu = x.NouveauLieu.String
	}
	e.Journee = x.NouvelleJournee
	return fois{evt: e, debut: time.Unix(x.NouveauDebut, 0).In(lieu), fin: time.Unix(x.NouveauFin, 0).In(lieu),
		jour: x.Jour, modifiee: true}, true
}

// foisDu : le début et la fin de la fois qui commence le jour cle
// (2026-12-01), si l'événement qui revient a bien lieu ce jour-là.
func foisDu(e donnees.Evenement, lieu *time.Location, cle string) ([2]time.Time, bool) {
	j, err := time.ParseInLocation("2006-01-02", cle, lieu)
	if err != nil || e.Repetition == "" {
		return [2]time.Time{}, false
	}
	for _, d := range deplier(e, lieu, j, j.AddDate(0, 0, 1)) {
		if repetition.CleJour(d[0]) == cle {
			return d, true
		}
	}
	return [2]time.Time{}, false
}

// dureeFois : la durée habituelle d'une fois (une heure pour un événement
// sur toute la journée qu'on met à une heure précise).
func dureeFois(e donnees.Evenement) time.Duration {
	if e.Journee == 1 || e.Fin <= e.Debut {
		return time.Hour
	}
	return time.Duration(e.Fin-e.Debut) * time.Second
}

// formFois : le formulaire « Modifier cette fois seulement », prérempli
// avec cette fois telle qu'elle est (modifiée ou non).
type formFois struct {
	Titre, Lieu string
	Date        string
	HeureDebut  string
	HeureFin    string
	Modifiee    bool
	Texte       string // fois modifiée : « mercredi 25 novembre à 18 h 00 »
}

// preparerFois : le formulaire de la fois du jour cle (nil si l'événement
// n'a pas lieu ce jour-là).
func preparerFois(e donnees.Evenement, lieu *time.Location, cle string, lignes []donnees.ExceptionsEvenementRow) *formFois {
	d, ok := foisDu(e, lieu, cle)
	if !ok {
		return nil
	}
	f := fois{evt: e, debut: d[0], fin: d[1]}
	for _, x := range lignes {
		if x.Jour == cle && x.Modifiee == 1 {
			f, _ = foisModifiee(e, lieu, ligneModifiee{Jour: x.Jour, NouveauDebut: x.NouveauDebut, NouveauFin: x.NouveauFin,
				NouvelleJournee: x.NouvelleJournee, NouveauTitre: x.NouveauTitre, NouveauLieu: x.NouveauLieu})
		}
	}
	ff := &formFois{Titre: f.evt.Titre, Lieu: f.evt.Lieu, Date: repetition.CleJour(f.debut), Modifiee: f.modifiee}
	if f.evt.Journee == 0 {
		ff.HeureDebut, ff.HeureFin = f.debut.Format("15:04"), f.fin.Format("15:04")
	}
	if f.modifiee {
		ff.Texte = texteFois(f)
	}
	return ff
}

// texteFois : « mercredi 25 novembre à 18 h 00 » (sans l'heure pour une
// journée entière).
func texteFois(f fois) string {
	t := strings.ToLower(appli.DateLongue(f.debut))
	if f.evt.Journee == 0 {
		t += " à " + heureLongue(f.debut)
	}
	return t
}

// modifierFois : « Modifier cette fois seulement » (date, heures, lieu,
// titre). Le formulaire de l'événement, lui, modifie toutes les fois.
func (m *Module) modifierFois(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	e, err := m.evenement(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Événement introuvable.")
		return
	}
	if !modifiable(c, e) {
		m.a.Echec(w, r, http.StatusForbidden, "Seul un parent (ou qui l'a créé) peut le modifier.")
		return
	}
	if err := r.ParseForm(); err != nil {
		m.a.Echec(w, r, http.StatusBadRequest, "Formulaire invalide.")
		return
	}
	f := r.PostForm
	cle := f.Get("jour")
	origine, ok := foisDu(e, c.Lieu, cle)
	if !ok {
		m.a.Echec(w, r, http.StatusBadRequest, "L'événement n'a pas lieu ce jour-là.")
		return
	}
	titre, lieu := strings.TrimSpace(f.Get("titre")), strings.TrimSpace(f.Get("lieu"))
	if n := utf8.RuneCountInString(titre); n < 1 || n > 80 {
		m.a.Echec(w, r, http.StatusBadRequest, "Le titre doit faire entre 1 et 80 caractères.")
		return
	}
	if utf8.RuneCountInString(lieu) > 80 {
		m.a.Echec(w, r, http.StatusBadRequest, "Lieu trop long (80 caractères).")
		return
	}
	jour, err := time.ParseInLocation("2006-01-02", f.Get("date"), c.Lieu)
	if err != nil || jour.Year() < 1900 || jour.Year() > 2200 {
		m.a.Echec(w, r, http.StatusBadRequest, "Date invalide.")
		return
	}
	// Une fois tient sur un seul jour (sans heure : toute la journée).
	var debut, fin time.Time
	journee := f.Get("heure_debut") == ""
	if journee {
		debut, fin = jour, jour.AddDate(0, 0, 1).Add(-time.Second)
	} else {
		hd, err1 := time.Parse("15:04", f.Get("heure_debut"))
		debut = time.Date(jour.Year(), jour.Month(), jour.Day(), hd.Hour(), hd.Minute(), 0, 0, c.Lieu)
		fin = debut.Add(dureeFois(e)) // sans heure de fin : la durée habituelle
		var err2 error
		if f.Get("heure_fin") != "" {
			var hf time.Time
			hf, err2 = time.Parse("15:04", f.Get("heure_fin"))
			fin = time.Date(jour.Year(), jour.Month(), jour.Day(), hf.Hour(), hf.Minute(), 0, 0, c.Lieu)
		}
		if err1 != nil || err2 != nil {
			m.a.Echec(w, r, http.StatusBadRequest, "Heure invalide.")
			return
		}
		if fin.Before(debut) {
			m.a.Echec(w, r, http.StatusBadRequest, "La fin doit être après le début.")
			return
		}
	}
	// Rien de différent des autres fois : la fois redevient normale.
	if titre == e.Titre && lieu == e.Lieu && journee == (e.Journee == 1) && debut.Equal(origine[0]) && fin.Equal(origine[1]) {
		if err := m.a.Q.RetirerException(ctx, donnees.RetirerExceptionParams{EvenementID: e.ID, Jour: cle}); err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
		m.a.Publier(c, mod, "")
		m.a.Flash(w, "« "+e.Titre+" » : "+strings.ToLower(appli.DateLongue(origine[0]))+" comme les autres fois")
		m.a.Rediriger(w, r, "/agenda?date="+cle)
		return
	}
	if n, err := m.a.Q.CompterExceptions(ctx, e.ID); err != nil || n >= exceptionsMax {
		m.a.Echec(w, r, http.StatusBadRequest, "Trop de fois annulées ou modifiées pour cet événement.")
		return
	}
	p := donnees.ModifierFoisParams{EvenementID: e.ID, Jour: cle, NouveauDebut: debut.Unix(), NouveauFin: fin.Unix(),
		NouvelleJournee: bool01(journee), NouveauTitre: titre}
	if titre == e.Titre {
		p.NouveauTitre = "" // suit le titre de l'événement s'il change
	}
	if lieu != e.Lieu {
		p.NouveauLieu = sql.NullString{String: lieu, Valid: true}
	}
	if err := m.a.Q.ModifierFois(ctx, p); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	// Les participants sont prévenus ; toucher la notification ouvre le
	// nouveau jour.
	parts, err := m.participants(ctx, c, []int64{e.ID})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	var pour []int64
	for _, x := range parts {
		pour = append(pour, x.MembreID)
	}
	e.Titre, e.Journee = titre, bool01(journee)
	quand := texteFois(fois{evt: e, debut: debut})
	m.a.PublierLien(c, mod, fmt.Sprintf("%s a modifié « %s » du %s : %s 🗓️", c.Membre.Nom, titre,
		strings.ToLower(appli.DateLongue(origine[0])), quand), "/agenda?date="+repetition.CleJour(debut), pour)
	m.a.Flash(w, "« "+titre+" » : cette fois seulement, "+quand+" ✓")
	m.a.Rediriger(w, r, "/agenda?date="+repetition.CleJour(debut))
}
