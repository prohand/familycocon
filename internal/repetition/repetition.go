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
)

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
	case s == Jour || s == Semaine:
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
	}
	return CleUnique, true
}

// DepuisFormulaire lit les champs « repetition » (unique, jour, jours,
// semaine) et « jour1 » … « jour7 » d'un formulaire.
func DepuisFormulaire(v url.Values) (Regle, error) {
	switch t := v.Get("repetition"); t {
	case "", "unique":
		return Regle{}, nil
	case Jour, Semaine:
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
