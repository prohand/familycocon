// SPDX-License-Identifier: GPL-3.0-or-later

package budget

import (
	"slices"
	"testing"
	"time"
)

// occurrenceLente : l'ancien calcul, mois par mois (référence).
func occurrenceLente(debut string, seulement []int, k int64) time.Time {
	d, _ := time.Parse("2006-01-02", debut)
	premier := time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, time.UTC)
	permis := func(t time.Time) bool { return slices.Contains(seulement, int(t.Month())) }
	for !permis(premier) {
		premier = premier.AddDate(0, 1, 0)
	}
	for n := int64(0); n < k; {
		premier = premier.AddDate(0, 1, 0)
		if permis(premier) {
			n++
		}
	}
	dernierJour := premier.AddDate(0, 1, -1).Day()
	return premier.AddDate(0, 0, min(d.Day(), dernierJour)-1)
}

// Le calcul direct des fréquences « certains mois » donne les mêmes dates
// que l'ancien, mois par mois, quel que soit le mois de départ.
func TestOccurrenceCertainsMois(t *testing.T) {
	for _, cle := range []string{"mois10", "scolaire"} {
		f, _ := frequence(cle)
		for mo := 1; mo <= 12; mo++ {
			debut := time.Date(2026, time.Month(mo), 31, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
			if mo == 2 {
				debut = "2026-02-28"
			}
			for k := int64(0); k < 40; k++ {
				j, err := Occurrence(debut, cle, k)
				if want := occurrenceLente(debut, f.Seulement, k); err != nil || !j.Equal(want) {
					t.Fatalf("%s depuis %s, fois %d : %v, attendu %v", cle, debut, k, j, want)
				}
			}
		}
	}
}
