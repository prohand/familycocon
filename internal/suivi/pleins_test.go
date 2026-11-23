// SPDX-License-Identifier: GPL-3.0-or-later

package suivi

import (
	"testing"
	"time"

	"familycocon/internal/donnees"
)

// Périodes des chiffres : 3, 6, 12 mois, une année, tout, des dates.
func TestPeriode(t *testing.T) {
	auj := time.Date(2026, 11, 23, 15, 0, 0, 0, time.UTC)
	f := func(d time.Time) string { return d.Format("2006-01-02") }
	for _, c := range []struct {
		cle, du, au, premier string
		debut, fin, cleOK    string
	}{
		{"", "", "", "", "2025-12-01", "2026-11-23", ""},
		{"3m", "", "", "", "2026-09-01", "2026-11-23", "3m"},
		{"6m", "", "", "", "2026-06-01", "2026-11-23", "6m"},
		{"2025", "", "", "", "2025-01-01", "2025-12-31", "2025"},
		{"2026", "", "", "", "2026-01-01", "2026-11-23", "2026"},
		{"2030", "", "", "", "2025-12-01", "2026-11-23", ""}, // année future : 12 mois
		{"tout", "", "", "2019-05-10", "2019-05-01", "2026-11-23", "tout"},
		{"dates", "2025-06-30", "2025-03-01", "", "2025-03-01", "2025-06-30", "dates"}, // dans le désordre
		{"dates", "2026-10-01", "2027-01-01", "", "2026-10-01", "2026-11-23", "dates"}, // pas après aujourd'hui
		{"dates", "n'importe", "quoi", "", "2025-12-01", "2026-11-23", ""},
		{"<script>", "", "", "", "2025-12-01", "2026-11-23", ""},
	} {
		d, fi, cle, titre := periode(c.cle, c.du, c.au, auj, c.premier)
		if f(d) != c.debut || f(fi) != c.fin || cle != c.cleOK || titre == "" {
			t.Errorf("%q %q %q : %s → %s (%q, %q)", c.cle, c.du, c.au, f(d), f(fi), cle, titre)
		}
	}
	// La période d'avant.
	d, fi, _, _ := periode("2026", "", "", auj, "")
	if a, b, nom, ok := avant("2026", d, fi); !ok || f(a) != "2025-01-01" || f(b) != "2025-11-23" || nom != "2025 (même période)" {
		t.Errorf("avant 2026 : %s %s %q", f(a), f(b), nom)
	}
	d, fi, _, _ = periode("3m", "", "", auj, "")
	if a, b, _, ok := avant("3m", d, fi); !ok || f(a) != "2026-06-01" || f(b) != "2026-08-31" {
		t.Errorf("avant 3 mois : %s %s", f(a), f(b))
	}
	if _, _, _, ok := avant("tout", d, fi); ok {
		t.Error("rien avant « tout »")
	}
	if ecart(96, 100) != "−4 %" || ecart(112, 100) != "+12 %" || ecart(1, 0) != "" {
		t.Error("écart en pourcentage")
	}
}

// Consommation de chaque plein complet ; un partiel compte au suivant.
func TestConsommations(t *testing.T) {
	l := consommations([]donnees.PleinsPeriodeObjetRow{
		{ID: 1, Jour: "2026-01-01", Libelle: "Plein", Km: 1000, Litres: 4000},
		{ID: 2, Jour: "2026-01-10", Libelle: "Plein partiel", Km: 1300, Litres: 1000},
		{ID: 3, Jour: "2026-01-20", Libelle: "Plein", Km: 1800, Litres: 3000},
		{ID: 4, Jour: "2026-01-30", Libelle: "Plein", Km: 2300, Litres: 2000},
	})
	if len(l) != 2 || l[0].id != 3 || virgule(l[0].l100, 1) != "5,0" || l[1].id != 4 || virgule(l[1].l100, 1) != "4,0" {
		t.Fatalf("%+v", l)
	}
	if s := stationsFrequentes([]string{"Total", "Leclerc", "Total", "une très longue remarque sur ce plein-là, pas une station", "Auchan"}, 2); len(s) != 2 || s[0] != "Total" || s[1] != "Leclerc" {
		t.Errorf("stations : %v", s)
	}
}
