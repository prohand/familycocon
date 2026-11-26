// SPDX-License-Identifier: GPL-3.0-or-later

package agenda

import (
	"strconv"
	"strings"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
	"familycocon/internal/repetition"
)

// Répétition d'un événement (colonne evenements.repetition) :
//
//	""            une fois
//	"jour"        chaque jour
//	"jours:2,4"   certains jours de la semaine (1 = lundi … 7 = dimanche)
//	"semaine"     chaque semaine
//	"2semaines"   toutes les 2 semaines
//	"mois"        chaque mois, à la même date (le 31 → dernier jour du mois)
//	"2mois"       tous les 2 mois (même date)
//	"trimestre"   tous les 3 mois (même date)
//	"semestre"    tous les 6 mois (même date)
//	"mois-rang"   chaque mois, le même jour de la semaine (ex. le 2e mardi)
//	"an"          chaque année
//
// repetition_fin (facultatif) : plus d'occurrence après ce jour.

// ChoixRepetition : une option du formulaire.
type ChoixRepetition struct{ Cle, Nom string }

// Choix proposés, dans l'ordre du formulaire.
var Choix = []ChoixRepetition{
	{"", "Non"}, {"jour", "Chaque jour"}, {"jours", "Certains jours de la semaine"},
	{"semaine", "Chaque semaine"}, {"2semaines", "Toutes les 2 semaines"},
	{"mois", "Chaque mois, à la même date"}, {"mois-rang", "Chaque mois, le même jour (ex. le 2e mardi)"},
	{"2mois", "Tous les 2 mois"}, {"trimestre", "Tous les 3 mois (trimestriel)"},
	{"semestre", "Tous les 6 mois (semestriel)"}, {"an", "Chaque année"},
}

// regle : une répétition décodée.
type regle struct {
	Type  string  // une des clés de Choix
	Jours [8]bool // « jours » : index 1 (lundi) à 7 (dimanche)
}

// lireRegle : « certains jours » se lit et s'écrit comme pour les tâches
// (paquet repetition) ; les autres types sont ceux de Choix.
func lireRegle(s string) regle {
	if strings.HasPrefix(s, repetition.Jours+":") {
		r := repetition.Lire(s) // règle abîmée (aucun jour) : « une fois »
		return regle{Type: r.Type, Jours: r.Jours}
	}
	for _, c := range Choix {
		if c.Cle == s {
			return regle{Type: s}
		}
	}
	return regle{}
}

func (r regle) String() string {
	if r.Type != repetition.Jours {
		return r.Type
	}
	return repetition.Regle{Type: repetition.Jours, Jours: r.Jours}.String()
}

// rang : 1 pour le 1er mardi du mois… 5 pour « le dernier ».
func rang(t time.Time) int { return (t.Day()-1)/7 + 1 }

var rangs = []string{"", "1er", "2e", "3e", "4e", "dernier"}

// TexteRepetition : « Chaque 2e mardi du mois, jusqu'au 30 juin 2027 ».
func TexteRepetition(e donnees.Evenement, lieu *time.Location) string {
	r := lireRegle(e.Repetition)
	d0 := time.Unix(e.Debut, 0).In(lieu)
	var t string
	switch r.Type {
	case "":
		return ""
	case "jours":
		var l []string
		for i := 1; i <= 7; i++ {
			if r.Jours[i] {
				l = append(l, strings.ToLower(repetition.NomsJours[i]))
			}
		}
		t = "Chaque " + strings.Join(l, ", ")
	case "mois":
		t = "Chaque mois, le " + strconv.Itoa(d0.Day())
	case "2mois", "trimestre", "semestre":
		t = map[string]string{"2mois": "Tous les 2 mois", "trimestre": "Tous les 3 mois", "semestre": "Tous les 6 mois"}[r.Type] +
			", le " + strconv.Itoa(d0.Day())
	case "mois-rang":
		t = "Chaque " + rangs[rang(d0)] + " " + appli.JourFR(d0) + " du mois"
	case "an":
		t = "Chaque année, le " + strconv.Itoa(d0.Day()) + " " + appli.MoisFR(d0.Month())
	default:
		for _, c := range Choix {
			if c.Cle == r.Type {
				t = c.Nom
			}
		}
	}
	if f, err := time.Parse("2006-01-02", e.RepetitionFin); err == nil {
		t += ", jusqu'au " + strconv.Itoa(f.Day()) + " " + appli.MoisFR(f.Month()) + " " + strconv.Itoa(f.Year())
	}
	return t
}

// numeroJour : jours depuis une origine fixe (pour compter les semaines
// sans être gêné par les changements d'heure).
func numeroJour(t time.Time) int64 {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC).Unix() / 86400
}

// moisDecale : le jour d du mois (annee, mois+k), ou le dernier jour du mois s'il n'existe pas.
func moisDecale(d0 time.Time, k int, lieu *time.Location) time.Time {
	premier := time.Date(d0.Year(), d0.Month()+time.Month(k), 1, d0.Hour(), d0.Minute(), 0, 0, lieu)
	dernier := premier.AddDate(0, 1, -1).Day()
	return time.Date(premier.Year(), premier.Month(), min(d0.Day(), dernier), d0.Hour(), d0.Minute(), 0, 0, lieu)
}

// rangDecale : le même jour de la semaine, au même rang, le mois mois+k
// (rang 5 : le dernier de ce jour dans le mois).
func rangDecale(d0 time.Time, k int, lieu *time.Location) time.Time {
	premier := time.Date(d0.Year(), d0.Month()+time.Month(k), 1, d0.Hour(), d0.Minute(), 0, 0, lieu)
	ecart := (int(d0.Weekday()) - int(premier.Weekday()) + 7) % 7
	d := premier.AddDate(0, 0, ecart+7*(rang(d0)-1))
	for d.Month() != premier.Month() { // 5e semaine absente : le dernier
		d = d.AddDate(0, 0, -7)
	}
	return d
}

// occurrencesMax : au plus par événement et par période affichée (garde-fou).
// Assez pour un événement de chaque jour sur tout le lien du calendrier du
// téléphone (3 mois avant, 12 après : ~457 jours).
const occurrencesMax = 500

// deplier : les occurrences d'un événement dans la période [debut, fin[.
func deplier(e donnees.Evenement, lieu *time.Location, debut, fin time.Time) [][2]time.Time {
	d0 := time.Unix(e.Debut, 0).In(lieu)
	duree := time.Duration(e.Fin-e.Debut) * time.Second
	r := lireRegle(e.Repetition)
	finRep := e.RepetitionFin // '' : sans fin
	// Journée entière : la fin se compte en jours de calendrier (un jour de
	// changement d'heure dure 23 ou 25 h).
	f0 := time.Unix(e.Fin, 0).In(lieu)
	nbJours := int(numeroJour(f0) - numeroJour(d0))
	finDe := func(d time.Time) time.Time {
		if e.Journee == 1 {
			return time.Date(d.Year(), d.Month(), d.Day()+nbJours+1, 0, 0, 0, 0, lieu).Add(-time.Second)
		}
		return d.Add(duree)
	}
	var out [][2]time.Time
	ajouter := func(d time.Time) {
		if f := finDe(d); chevauche(d, f, debut, fin) && len(out) < occurrencesMax {
			out = append(out, [2]time.Time{d, f})
		}
	}
	apresFin := func(d time.Time) bool { return finRep != "" && repetition.CleJour(d) > finRep }

	switch r.Type {
	case "":
		ajouter(d0)

	case "jour", "jours", "semaine", "2semaines":
		// Jour par jour sur la période (et un peu avant, pour un événement
		// de plusieurs jours commencé avant).
		j := minuit(debut).AddDate(0, 0, -int(duree/(24*time.Hour))-1)
		if j.Before(minuit(d0)) {
			j = minuit(d0)
		}
		n0 := numeroJour(d0)
		for ; j.Before(fin); j = j.AddDate(0, 0, 1) {
			d := time.Date(j.Year(), j.Month(), j.Day(), d0.Hour(), d0.Minute(), 0, 0, lieu)
			if apresFin(d) {
				break
			}
			ecart := numeroJour(j) - n0
			garde := r.Type == "jour" ||
				(r.Type == "jours" && r.Jours[repetition.NumeroJour(j)]) ||
				(r.Type == "semaine" && ecart%7 == 0) ||
				(r.Type == "2semaines" && ecart%14 == 0)
			if garde {
				ajouter(d)
			}
		}

	case "mois", "2mois", "trimestre", "semestre", "mois-rang", "an":
		pas := map[string]int{"2mois": 2, "trimestre": 3, "semestre": 6, "an": 12}[r.Type]
		pas = max(pas, 1)
		// On commence un peu avant la période (événements de plusieurs jours).
		k := ((debut.Year()-d0.Year())*12 + int(debut.Month()) - int(d0.Month()) - 2) / pas
		k = max(k, 0)
		for n := 0; n < 1000; n, k = n+1, k+1 {
			var d time.Time
			if r.Type == "mois-rang" {
				d = rangDecale(d0, k, lieu)
			} else {
				d = moisDecale(d0, k*pas, lieu)
			}
			if !d.Before(fin) || apresFin(d) {
				break
			}
			ajouter(d)
		}
	}
	return out
}
