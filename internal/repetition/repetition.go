// SPDX-License-Identifier: GPL-3.0-or-later

// Package repetition : règle de répétition d'une tâche (une fois, chaque
// jour, certains jours, chaque semaine), commune aux tâches ménagères et à
// l'espace enfants.
//
// Format en base (texte court) :
//
//	""              une fois
//	"jour"          chaque jour
//	"jours:1,3,5"   certains jours (1 = lundi … 7 = dimanche)
//	"semaine"       une fois par semaine (n'importe quel jour)
//	"2semaines"     une fois toutes les deux semaines
//	"mois"          une fois par mois
package repetition

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Types de règle.
const (
	Unique  = ""
	Jour    = "jour"
	Jours   = "jours"
	Semaine = "semaine"
	// DeuxSemaines : une fois dans chaque période de deux semaines (comptées
	// à partir du lundi 1er janvier 2024 : les mêmes pour toute la famille).
	DeuxSemaines = "2semaines"
	Mois         = "mois"
)

// origineQuinzaines : premier lundi des périodes de deux semaines.
var origineQuinzaines = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// CleUnique : clé de l'occurrence d'une tâche « une fois ».
const CleUnique = "unique"

// Regle : une règle de répétition.
type Regle struct {
	Type  string
	Jours [8]bool // index 1 (lundi) à 7 (dimanche)
}

// NomsJours : initiales des jours, de lundi à dimanche.
var NomsJours = []string{"", "Lun", "Mar", "Mer", "Jeu", "Ven", "Sam", "Dim"}

// Lire décode une règle (une règle abîmée vaut « une fois »).
func Lire(s string) Regle {
	switch {
	case s == Jour || s == Semaine || s == DeuxSemaines || s == Mois:
		return Regle{Type: s}
	case strings.HasPrefix(s, Jours+":"):
		r := Regle{Type: Jours}
		for _, j := range strings.Split(strings.TrimPrefix(s, Jours+":"), ",") {
			if n, err := strconv.Atoi(j); err == nil && n >= 1 && n <= 7 {
				r.Jours[n] = true
			}
		}
		if r.String() == Jours+":" {
			return Regle{}
		}
		return r
	}
	return Regle{}
}

// String : la règle au format de la base.
func (r Regle) String() string {
	if r.Type != Jours {
		return r.Type
	}
	var l []string
	for i := 1; i <= 7; i++ {
		if r.Jours[i] {
			l = append(l, strconv.Itoa(i))
		}
	}
	return Jours + ":" + strings.Join(l, ",")
}

// Texte : la règle en français (« Chaque jour », « Lun, Mer »).
func (r Regle) Texte() string {
	switch r.Type {
	case Jour:
		return "Chaque jour"
	case Semaine:
		return "Chaque semaine"
	case DeuxSemaines:
		return "Toutes les 2 semaines"
	case Mois:
		return "Chaque mois"
	case Jours:
		var l []string
		for i := 1; i <= 7; i++ {
			if r.Jours[i] {
				l = append(l, NomsJours[i])
			}
		}
		if len(l) == 5 && !r.Jours[6] && !r.Jours[7] {
			return "En semaine"
		}
		return strings.Join(l, ", ")
	}
	return "Une fois"
}

// Repete : la tâche revient-elle ?
func (r Regle) Repete() bool { return r.Type != Unique }

// NumeroJour : 1 (lundi) à 7 (dimanche).
func NumeroJour(t time.Time) int {
	if t.Weekday() == time.Sunday {
		return 7
	}
	return int(t.Weekday())
}

// Lundi : le lundi de la semaine du jour t (à minuit).
func Lundi(t time.Time) time.Time {
	j := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	return j.AddDate(0, 0, -(NumeroJour(j) - 1))
}

// CleJour : « 2026-11-22 ».
func CleJour(t time.Time) string { return t.Format("2006-01-02") }

// Occurrence : la tâche est-elle à faire le jour t, et sous quelle clé ?
// (la clé identifie l'occurrence : un même jour, une même semaine…).
func (r Regle) Occurrence(t time.Time) (cle string, aFaire bool) {
	switch r.Type {
	case Jour:
		return CleJour(t), true
	case Jours:
		return CleJour(t), r.Jours[NumeroJour(t)]
	case Semaine:
		return "s" + CleJour(Lundi(t)), true
	case DeuxSemaines:
		return "q" + CleJour(DebutQuinzaine(t)), true
	case Mois:
		return "m" + t.Format("2006-01"), true
	}
	return CleUnique, true
}

// DebutQuinzaine : le lundi qui ouvre la période de deux semaines du jour t.
func DebutQuinzaine(t time.Time) time.Time {
	lundi := Lundi(t)
	// Nombre de jours depuis l'origine, compté sur les dates (pas sur les
	// heures : un changement d'heure ne décale rien).
	jours := int(time.Date(lundi.Year(), lundi.Month(), lundi.Day(), 0, 0, 0, 0, time.UTC).Sub(origineQuinzaines).Hours() / 24)
	if (jours/7)%2 != 0 {
		lundi = lundi.AddDate(0, 0, -7)
	}
	return lundi
}

// DepuisFormulaire lit les champs « repetition » (unique, jour, jours,
// semaine, 2semaines, mois) et « jour1 » … « jour7 » d'un formulaire.
func DepuisFormulaire(v url.Values) (Regle, error) {
	switch t := v.Get("repetition"); t {
	case "", "unique":
		return Regle{}, nil
	case Jour, Semaine, DeuxSemaines, Mois:
		return Regle{Type: t}, nil
	case Jours:
		r := Regle{Type: Jours}
		n := 0
		for i := 1; i <= 7; i++ {
			if v.Get("jour"+strconv.Itoa(i)) == "1" {
				r.Jours[i] = true
				n++
			}
		}
		if n == 0 {
			return Regle{}, errors.New("Choisissez au moins un jour.")
		}
		return r, nil
	}
	return Regle{}, errors.New("Répétition inconnue.")
}
