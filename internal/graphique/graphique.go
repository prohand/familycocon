// SPDX-License-Identifier: GPL-3.0-or-later

// Package graphique : petits graphiques dessinés par le serveur, en SVG
// (courbes et barres). Aucune bibliothèque, aucun script : le SVG est
// écrit dans la page et suit le thème (couleurs par classes CSS, voir
// app.css « .graphique »). Pensés pour la largeur d'un téléphone.
package graphique

import (
	"fmt"
	"html/template"
	"math"
	"strings"
	"time"
)

// Dimensions du dessin (le SVG s'étire à la largeur de la carte).
const (
	largeur = 340
	hauteur = 170
	gauche  = 46 // place des valeurs de l'axe vertical
	droite  = 8
	haut    = 10
	bas     = 22 // place des dates
)

// Point : une valeur à une date (courbe).
type Point struct {
	Date   time.Time
	Valeur float64
}

// Barre : une colonne, faite d'une ou plusieurs valeurs empilées.
type Barre struct {
	Etiquette string    // sous la colonne (« janv. »)
	Detail    string    // bulle au survol (« janvier 2025 : 120 € »)
	Valeurs   []float64 // de bas en haut
	// Prevus (BarresCote seulement) : posé sur la valeur de même rang,
	// plus clair (classe g-p0, g-p1…) : ce qui est attendu, pas encore là.
	Prevus []float64
}

// esc : texte sûr dans le SVG.
func esc(s string) string { return template.HTMLEscapeString(s) }

// bornes : un intervalle « rond » qui contient [mini, maxi], et un pas
// pour 3 ou 4 graduations.
func bornes(mini, maxi float64, depuisZero bool) (bas, haut, pas float64) {
	if depuisZero {
		mini = math.Min(0, mini)
	}
	if maxi <= mini {
		maxi = mini + 1
	}
	brut := (maxi - mini) / 3
	ordre := math.Pow(10, math.Floor(math.Log10(brut)))
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if pas = m * ordre; pas >= brut {
			break
		}
	}
	bas = math.Floor(mini/pas) * pas
	haut = math.Ceil(maxi/pas) * pas
	if haut == bas {
		haut = bas + pas
	}
	return bas, haut, pas
}

// cadre : le début du SVG, le titre (lu par les lecteurs d'écran) et les
// lignes de graduation avec leurs valeurs.
func cadre(b *strings.Builder, titre string, ymin, ymax, pas float64, format func(float64) string) func(float64) float64 {
	fmt.Fprintf(b, `<svg class="graphique" viewBox="0 0 %d %d" role="img" aria-label="%s"><title>%s</title>`, largeur, hauteur, esc(titre), esc(titre))
	zone := float64(hauteur - haut - bas)
	y := func(v float64) float64 { return float64(haut) + zone*(1-(v-ymin)/(ymax-ymin)) }
	for v := ymin; v <= ymax+pas/2; v += pas {
		fmt.Fprintf(b, `<line class="g-grille" x1="%d" x2="%d" y1="%.1f" y2="%.1f"/>`, gauche, largeur-droite, y(v), y(v))
		fmt.Fprintf(b, `<text class="g-valeur" x="%d" y="%.1f">%s</text>`, gauche-4, y(v)+3, esc(format(v)))
	}
	return y
}

// Courbe : les valeurs dans le temps, et leur moyenne en pointillé.
// Rien (« ») s'il n'y a aucun point.
func Courbe(titre string, points []Point, format func(float64) string, dateCourte func(time.Time) string) template.HTML {
	if len(points) == 0 {
		return ""
	}
	mini, maxi, somme := points[0].Valeur, points[0].Valeur, 0.0
	for _, p := range points {
		mini, maxi, somme = math.Min(mini, p.Valeur), math.Max(maxi, p.Valeur), somme+p.Valeur
	}
	marge := (maxi - mini) * 0.1
	if marge == 0 {
		marge = math.Max(math.Abs(maxi)*0.1, 0.1)
	}
	ymin, ymax, pas := bornes(math.Max(0, mini-marge), maxi+marge, false)
	var b strings.Builder
	y := cadre(&b, titre, ymin, ymax, pas, format)
	debut, fin := points[0].Date, points[len(points)-1].Date
	duree := fin.Sub(debut).Seconds()
	zone := float64(largeur - gauche - droite)
	x := func(t time.Time) float64 {
		if duree <= 0 {
			return float64(gauche) + zone/2
		}
		return float64(gauche) + zone*t.Sub(debut).Seconds()/duree
	}
	moyenne := somme / float64(len(points))
	fmt.Fprintf(&b, `<line class="g-moyenne" x1="%d" x2="%d" y1="%.1f" y2="%.1f"><title>Moyenne : %s</title></line>`,
		gauche, largeur-droite, y(moyenne), y(moyenne), esc(format(moyenne)))
	var trace []string
	for _, p := range points {
		trace = append(trace, fmt.Sprintf("%.1f,%.1f", x(p.Date), y(p.Valeur)))
	}
	fmt.Fprintf(&b, `<polyline class="g-ligne" pathLength="1" points="%s"/>`, strings.Join(trace, " "))
	// Chaque point porte sa date et sa valeur (data-info) : app.js les
	// montre au toucher. Au-delà de 60 points, ils sont cachés (la courbe
	// suffit) mais restent là pour le toucher.
	classe := "g-point"
	if len(points) > 60 {
		classe += " cache"
	}
	for _, p := range points {
		info := p.Date.Format("02/01/2006") + " : " + format(p.Valeur)
		fmt.Fprintf(&b, `<circle class="%s" cx="%.1f" cy="%.1f" r="2.5" data-info="%s"><title>%s</title></circle>`,
			classe, x(p.Date), y(p.Valeur), esc(info), esc(info))
	}
	// Dates : la première et la dernière.
	fmt.Fprintf(&b, `<text class="g-date" x="%d" y="%d">%s</text>`, gauche, hauteur-6, esc(dateCourte(debut)))
	if duree > 0 {
		fmt.Fprintf(&b, `<text class="g-date fin" x="%d" y="%d">%s</text>`, largeur-droite, hauteur-6, esc(dateCourte(fin)))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String()) // #nosec G203 -- dessin produit ici, textes échappés
}

// Barres : des colonnes (valeurs empilées : classes g-s0, g-s1…). Rien
// (« ») si tout est à zéro.
func Barres(titre string, barres []Barre, format func(float64) string) template.HTML {
	maxi := 0.0
	for _, br := range barres {
		t := 0.0
		for _, v := range br.Valeurs {
			t += math.Max(0, v)
		}
		maxi = math.Max(maxi, t)
	}
	if len(barres) == 0 || maxi == 0 {
		return ""
	}
	ymin, ymax, pas := bornes(0, maxi, true)
	var b strings.Builder
	y := cadre(&b, titre, ymin, ymax, pas, format)
	case_ := float64(largeur-gauche-droite) / float64(len(barres))
	l := math.Max(1, case_*0.7)
	// Une étiquette sur combien : qu'elles ne se chevauchent pas (~28 px).
	tous := int(math.Ceil(28 / case_))
	for i, br := range barres {
		x := float64(gauche) + case_*float64(i) + (case_-l)/2
		base := ymin
		fmt.Fprintf(&b, `<g class="g-barre" data-x="%.1f" data-info="%s"><title>%s</title>`, x+l/2, esc(br.Detail), esc(br.Detail))
		for s, v := range br.Valeurs {
			if v <= 0 {
				continue
			}
			fmt.Fprintf(&b, `<rect class="g-s%d" x="%.1f" y="%.1f" width="%.1f" height="%.1f"/>`, s, x, y(base+v), l, y(base)-y(base+v))
			base += v
		}
		b.WriteString(`</g>`)
		if i%tous == 0 && br.Etiquette != "" {
			fmt.Fprintf(&b, `<text class="g-date milieu" x="%.1f" y="%d">%s</text>`, x+l/2, hauteur-6, esc(br.Etiquette))
		}
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String()) // #nosec G203 -- dessin produit ici, textes échappés
}

// BarresCote : comme Barres, mais les valeurs de chaque colonne côte à
// côte (revenus et dépenses d'un mois), et non empilées.
func BarresCote(titre string, barres []Barre, format func(float64) string) template.HTML {
	// haut : la valeur s de la colonne, avec son prévu.
	haut := func(br Barre, s int) float64 {
		v := math.Max(0, br.Valeurs[s])
		if s < len(br.Prevus) {
			v += math.Max(0, br.Prevus[s])
		}
		return v
	}
	maxi, n := 0.0, 1
	for _, br := range barres {
		n = max(n, len(br.Valeurs))
		for s := range br.Valeurs {
			maxi = math.Max(maxi, haut(br, s))
		}
	}
	if len(barres) == 0 || maxi == 0 {
		return ""
	}
	ymin, ymax, pas := bornes(0, maxi, true)
	var b strings.Builder
	y := cadre(&b, titre, ymin, ymax, pas, format)
	case_ := float64(largeur-gauche-droite) / float64(len(barres))
	l := math.Max(1, case_*0.8) // largeur du groupe de la colonne
	une := l / float64(n)       // largeur d'une valeur
	tous := int(math.Ceil(28 / case_))
	for i, br := range barres {
		x := float64(gauche) + case_*float64(i) + (case_-l)/2
		fmt.Fprintf(&b, `<g class="g-barre" data-x="%.1f" data-info="%s"><title>%s</title>`, x+l/2, esc(br.Detail), esc(br.Detail))
		// Fond invisible : toute la colonne se touche, même un mois vide.
		fmt.Fprintf(&b, `<rect class="g-zone" x="%.1f" y="%.1f" width="%.1f" height="%.1f"/>`, x, y(ymax), l, y(ymin)-y(ymax))
		for s, v := range br.Valeurs {
			xs, ls := x+une*float64(s), math.Max(1, une-1)
			v = math.Max(0, v)
			if v > 0 {
				fmt.Fprintf(&b, `<rect class="g-s%d" x="%.1f" y="%.1f" width="%.1f" height="%.1f"/>`, s, xs, y(v), ls, y(ymin)-y(v))
			}
			if t := haut(br, s); t > v {
				fmt.Fprintf(&b, `<rect class="g-p%d" x="%.1f" y="%.1f" width="%.1f" height="%.1f"/>`, s, xs, y(t), ls, y(v)-y(t))
			}
		}
		b.WriteString(`</g>`)
		if i%tous == 0 && br.Etiquette != "" {
			fmt.Fprintf(&b, `<text class="g-date milieu" x="%.1f" y="%d">%s</text>`, x+l/2, hauteur-6, esc(br.Etiquette))
		}
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String()) // #nosec G203 -- dessin produit ici, textes échappés
}
