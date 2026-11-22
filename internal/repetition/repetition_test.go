// SPDX-License-Identifier: GPL-3.0-or-later

package repetition

import (
	"net/url"
	"testing"
	"time"
)

func TestLectureEcriture(t *testing.T) {
	for _, s := range []string{"", "jour", "semaine", "jours:1,3,5", "jours:6,7"} {
		if got := Lire(s).String(); got != s {
			t.Errorf("%q relu en %q", s, got)
		}
	}
	if Lire("jours:").Type != Unique || Lire("n'importe quoi").Type != Unique || Lire("jours:9").Type != Unique {
		t.Fatal("règle abîmée : « une fois » attendu")
	}
	if Lire("jours:1,2,3,4,5").Texte() != "En semaine" || Lire("jours:3,6").Texte() != "Mer, Sam" {
		t.Fatal("texte")
	}
}

func TestOccurrence(t *testing.T) {
	paris, _ := time.LoadLocation("Europe/Paris")
	dimanche := time.Date(2026, 11, 22, 21, 0, 0, 0, paris)
	mercredi := time.Date(2026, 11, 18, 8, 0, 0, 0, paris)
	if c, ok := Lire("jour").Occurrence(dimanche); !ok || c != "2026-11-22" {
		t.Fatal(c)
	}
	if _, ok := Lire("jours:3").Occurrence(dimanche); ok {
		t.Fatal("mercredi seulement : pas dimanche")
	}
	if _, ok := Lire("jours:3").Occurrence(mercredi); !ok {
		t.Fatal("mercredi attendu")
	}
	// Semaine : même clé du lundi au dimanche.
	c1, _ := Lire("semaine").Occurrence(dimanche)
	c2, _ := Lire("semaine").Occurrence(mercredi)
	if c1 != c2 || c1 != "s2026-11-16" {
		t.Fatalf("%s / %s", c1, c2)
	}
	if c, _ := Lire("").Occurrence(dimanche); c != CleUnique {
		t.Fatal(c)
	}
}

func TestFormulaire(t *testing.T) {
	r, err := DepuisFormulaire(url.Values{"repetition": {"jours"}, "jour2": {"1"}, "jour4": {"1"}})
	if err != nil || r.String() != "jours:2,4" {
		t.Fatal(r, err)
	}
	if _, err := DepuisFormulaire(url.Values{"repetition": {"jours"}}); err == nil {
		t.Fatal("aucun jour accepté")
	}
	if _, err := DepuisFormulaire(url.Values{"repetition": {"mois"}}); err == nil {
		t.Fatal("type inconnu accepté")
	}
}
