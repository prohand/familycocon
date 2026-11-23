// SPDX-License-Identifier: GPL-3.0-or-later

package graphique

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"
)

// valide : le SVG est du XML bien formé.
func valide(t *testing.T, svg string) {
	t.Helper()
	d := xml.NewDecoder(strings.NewReader(svg))
	for {
		if _, err := d.Token(); err != nil {
			if err.Error() == "EOF" {
				return
			}
			t.Fatalf("SVG invalide : %v\n%s", err, svg)
		}
	}
}

func TestCourbe(t *testing.T) {
	if Courbe("vide", nil, nil, nil) != "" {
		t.Fatal("courbe sans point")
	}
	j := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	pts := []Point{{j, 6.2}, {j.AddDate(0, 1, 0), 5.8}, {j.AddDate(0, 2, 0), 7.1}}
	date := func(t time.Time) string { return t.Format("01/06") }
	svg := string(Courbe(`Conso <test> & "x"`, pts, func(v float64) string { return "v" }, date))
	valide(t, svg)
	if !strings.Contains(svg, "<polyline") || strings.Count(svg, "<circle") != 3 || !strings.Contains(svg, "&lt;test&gt;") || !strings.Contains(svg, "g-moyenne") {
		t.Fatalf("courbe :\n%s", svg)
	}
	// Un seul point : dessiné au milieu, sans erreur.
	valide(t, string(Courbe("un", pts[:1], func(v float64) string { return "v" }, date)))
}

func TestBarres(t *testing.T) {
	if Barres("zéro", []Barre{{Valeurs: []float64{0, 0}}}, nil) != "" {
		t.Fatal("barres toutes à zéro")
	}
	var l []Barre
	for i := 0; i < 24; i++ {
		l = append(l, Barre{Etiquette: "m", Detail: "d", Valeurs: []float64{float64(i * 10), 5}})
	}
	svg := string(Barres("Dépenses", l, func(v float64) string { return "€" }))
	valide(t, svg)
	// 24 colonnes, 2 rectangles chacune sauf la première (0 €).
	if n := strings.Count(svg, "<rect"); n != 47 {
		t.Fatalf("%d rectangles", n)
	}
	// Étiquettes espacées : pas une par colonne sur 24 colonnes.
	if n := strings.Count(svg, `class="g-date milieu"`); n >= 24 || n < 6 {
		t.Fatalf("%d étiquettes", n)
	}
}

func TestBornes(t *testing.T) {
	for _, c := range []struct{ mini, maxi float64 }{{0, 1}, {5.3, 7.9}, {0, 1234}, {1.62, 1.97}, {3, 3}} {
		b, h, p := bornes(c.mini, c.maxi, false)
		if b > c.mini || h < c.maxi || p <= 0 || (h-b)/p > 8 {
			t.Errorf("bornes(%v, %v) = %v, %v, %v", c.mini, c.maxi, b, h, p)
		}
	}
}
