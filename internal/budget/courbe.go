// SPDX-License-Identifier: GPL-3.0-or-later

package budget

import (
	"errors"
	"fmt"
	"html/template"
	"strconv"
	"strings"
)

// Courbe des dépenses du mois, dessinée en SVG par le serveur (pas de
// bibliothèque de graphiques côté navigateur) : dépenses cumulées jour après
// jour, ce mois-ci (trait plein) et le mois précédent (pointillés).
// Les couleurs viennent de la feuille de style (classes) : le mode sombre
// marche tout seul.

const (
	largeur = 600
	hauteur = 220
	margeG  = 8
	margeD  = 8
	margeH  = 16
	margeB  = 26
)

// Courbe : cumuls par jour (index 0 = le 1er du mois), en centimes.
type Courbe struct {
	Actuel    []int64 // jusqu'à aujourd'hui seulement
	Precedent []int64 // mois entier
	Jours     int     // nombre de jours du mois affiché
	Devise    string
}

// SVG dessine la courbe.
func (c Courbe) SVG() template.HTML {
	maxi := int64(1)
	for _, l := range [][]int64{c.Actuel, c.Precedent} {
		for _, v := range l {
			maxi = max(maxi, v)
		}
	}
	maxi = arrondiHaut(maxi)
	x := func(i, n int) float64 {
		if n <= 1 {
			return margeG
		}
		return margeG + float64(i)*float64(largeur-margeG-margeD)/float64(n-1)
	}
	y := func(v int64) float64 {
		return margeH + float64(hauteur-margeH-margeB)*(1-float64(v)/float64(maxi))
	}
	points := func(l []int64, n int) string {
		var b strings.Builder
		for i, v := range l {
			fmt.Fprintf(&b, "%.1f,%.1f ", x(i, n), y(v))
		}
		return strings.TrimSpace(b.String())
	}
	var s strings.Builder
	fmt.Fprintf(&s, `<svg class="courbe" viewBox="0 0 %d %d" role="img" aria-label="Dépenses cumulées du mois">`, largeur, hauteur)
	// Lignes de repère : 0, moitié, maximum.
	for _, f := range []int64{0, maxi / 2, maxi} {
		fmt.Fprintf(&s, `<line class="repere" x1="%d" y1="%.1f" x2="%d" y2="%.1f"/>`, margeG, y(f), largeur-margeD, y(f))
		if f > 0 {
			fmt.Fprintf(&s, `<text class="etiquette-axe" x="%d" y="%.1f">%s</text>`, margeG+2, y(f)-4, template.HTMLEscapeString(court(f, c.Devise)))
		}
	}
	// Jours repères : 1, 15, dernier.
	for _, j := range []int{1, 15, c.Jours} {
		fmt.Fprintf(&s, `<text class="etiquette-axe" x="%.1f" y="%d" text-anchor="middle">%d</text>`, max(min(x(j-1, c.Jours), largeur-14), 14), hauteur-6, j)
	}
	if len(c.Precedent) > 1 {
		fmt.Fprintf(&s, `<polyline class="precedent" points="%s"/>`, points(c.Precedent, len(c.Precedent)))
	}
	if len(c.Actuel) > 0 {
		pts := points(c.Actuel, c.Jours)
		// Surface sous la courbe.
		fmt.Fprintf(&s, `<polygon class="surface" points="%.1f,%.1f %s %.1f,%.1f"/>`, x(0, c.Jours), y(0), pts, x(len(c.Actuel)-1, c.Jours), y(0))
		fmt.Fprintf(&s, `<polyline class="actuel" points="%s"/>`, pts)
		dernier := len(c.Actuel) - 1
		fmt.Fprintf(&s, `<circle class="point" cx="%.1f" cy="%.1f" r="6"/>`, x(dernier, c.Jours), y(c.Actuel[dernier]))
	}
	s.WriteString(`</svg>`)
	return template.HTML(s.String()) // #nosec G203 -- construit ici, textes échappés
}

// arrondiHaut : 1 234,00 € → 1 500 € (pour une échelle lisible).
func arrondiHaut(v int64) int64 {
	pas := int64(1000) // 10 €
	for pas*10 < v {
		pas *= 10
	}
	for _, m := range []int64{1, 2, 5, 10} {
		if pas*m >= v {
			return pas * m
		}
	}
	return v
}

// court : « 1,5 k€ », « 300 € ».
func court(centimes int64, devise string) string {
	sym := map[string]string{"EUR": "€", "USD": "$", "GBP": "£"}[devise]
	if sym == "" {
		sym = devise
	}
	e := centimes / 100
	if e >= 1000 {
		k := strconv.FormatFloat(float64(e)/1000, 'f', 1, 64)
		return strings.Replace(strings.TrimSuffix(k, ".0"), ".", ",", 1) + " k" + sym
	}
	return strconv.FormatInt(e, 10) + " " + sym
}

// LireMontant lit un montant saisi : « 12 », « 12,5 », « 1 234,56 € »,
// « 12.50 » → centimes. Strictement positif, au plus 10 millions.
func LireMontant(s string) (int64, error) {
	s = strings.NewReplacer(" ", "", " ", "", " ", "", "€", "", "$", "", "£", "").Replace(strings.TrimSpace(s))
	s = strings.Replace(s, ",", ".", 1)
	if s == "" || strings.Count(s, ".") > 1 || strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") {
		return 0, errMontant
	}
	entier, dec, _ := strings.Cut(s, ".")
	if len(dec) > 2 || entier == "" && dec == "" {
		return 0, errMontant
	}
	// Chiffres seulement (« 12.-5 » refusé).
	for _, c := range entier + dec {
		if c < '0' || c > '9' {
			return 0, errMontant
		}
	}
	for len(dec) < 2 {
		dec += "0"
	}
	if entier == "" {
		entier = "0"
	}
	e, err1 := strconv.ParseInt(entier, 10, 64)
	d, err2 := strconv.ParseInt(dec, 10, 64)
	if err1 != nil || err2 != nil || e > 10_000_000 {
		return 0, errMontant
	}
	c := e*100 + d
	if c <= 0 {
		return 0, errMontant
	}
	return c, nil
}

var errMontant = errors.New("Montant invalide (ex. : 12,50).")
