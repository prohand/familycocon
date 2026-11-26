// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"errors"
	"familycocon/internal/riche"
	"fmt"
	"html/template"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// fonctionsDeBase : petites fonctions utilisables dans tous les gabarits.
func fonctionsDeBase(a *App) template.FuncMap {
	return template.FuncMap{
		// {{statique "app.css"}} → /statique/app.css?v=…
		"statique": a.Statique.URL,
		// {{initiale "marie"}} → M
		"initiale": Initiale,
		// {{visage .}} : l'avatar du membre, sinon son initiale.
		"visage":  Visage,
		"avatars": func() []string { return Avatars },
		// {{pluriel 3 "produit" "produits"}} → 3 produits
		// accepteJoints : les extensions des pièces jointes (champ fichier).
		"accepteJoints": func() string { return AccepteJoints },
		"pluriel": func(n any, un, plusieurs string) string {
			switch x := n.(type) {
			case int:
				return Pluriel(x, un, plusieurs)
			case int64:
				return Pluriel(x, un, plusieurs)
			}
			return fmt.Sprint(n, " ", plusieurs)
		},
		// {{montant 123456 "EUR"}} → 1 234,56 €
		"montant": Montant,
		// {{segment .Nom}} : un nom de fichier dans un chemin d'adresse
		// (« #1 facture.pdf » → « %231%20facture.pdf »).
		"segment": url.PathEscape,
		// {{emoji .Emoji}} : l'emoji, ou son dessin (carte de fidélité).
		"emoji": Icone,
		// {{template "avatar" dict "Nom" .Nom "Couleur" .Couleur}}
		"dict": func(kv ...any) (map[string]any, error) {
			if len(kv)%2 != 0 {
				return nil, errors.New("dict : nombre d'arguments impair")
			}
			m := make(map[string]any, len(kv)/2)
			for i := 0; i < len(kv); i += 2 {
				k, ok := kv[i].(string)
				if !ok {
					return nil, errors.New("dict : clé non texte")
				}
				m[k] = kv[i+1]
			}
			return m, nil
		},
		"contient": func(liste []string, v string) bool {
			for _, x := range liste {
				if x == v {
					return true
				}
			}
			return false
		},
		"ajouter": func(a, b int) int { return a + b },
		// Échéances : fréquences proposées (en mois) et leur nom.
		"moisRepetition": func() []int64 { return []int64{0, 1, 3, 6, 12, 24} },
		"texteMois":      TexteMois,
		// {{litres 4250}} → 42,5 (centilitres)
		// {{litres 851}} → 8,51 ; 4110 → 41,1 ; 4000 → 40 (au centilitre : le
		// formulaire « Corriger » ne doit rien arrondir).
		"litres": func(cl int64) string {
			s := strconv.FormatInt(cl/100, 10)
			if d := cl % 100; d != 0 {
				s += strings.TrimRight(fmt.Sprintf(",%02d", d), "0")
			}
			return s
		},
		// {{montantSaisie 1250}} → 12,50 (pour un champ de formulaire)
		"montantSaisie": func(c int64) string { return fmt.Sprintf("%d,%02d", c/100, c%100) },
		"milliers":      Milliers,
		// {{pourcent 3 10}} → 30 (arrondi à 5 : classes CSS p0 à p100, la CSP
		// interdit les largeurs écrites dans la page).
		"pourcent": func(n, total int) int {
			if total <= 0 {
				return 0
			}
			return (n*100/total + 2) / 5 * 5
		},
		// Textes longs mis en forme (notes des fiches, descriptions : voir
		// internal/riche) : {{riche .Notes}} pour les lire,
		// {{richeEdition .Notes}} pour l'éditeur, {{simple .Idees}} sans les signes.
		"riche":        func(s string) template.HTML { return riche.HTML(s, riche.Options{}) },
		"richeEdition": func(s string) template.HTML { return riche.HTML(s, riche.Options{Mode: riche.Edition}) },
		"simple":       func(s string) string { return strings.Join(strings.Fields(riche.Simple(s)), " ") },
		"couleurs":     func() []int { return []int{1, 2, 3, 4, 5, 6, 7, 8} },
		// {{etoiles 3}} → ⭐⭐⭐
		"etoiles": func(n int) string { return strings.Repeat("⭐", n) },
		// Initiales des jours, index 1 (lundi) à 7 (dimanche).
		"joursSemaine": func() []string { return []string{"", "Lun", "Mar", "Mer", "Jeu", "Ven", "Sam", "Dim"} },
	}
}

// Pluriel : « 1 produit », « 3 produits » (0 et 1 au singulier, en français).
func Pluriel[T ~int | ~int64](n T, un, plusieurs string) string {
	if n >= -1 && n <= 1 {
		return fmt.Sprintf("%d %s", n, un)
	}
	return fmt.Sprintf("%d %s", n, plusieurs)
}

// Initiale : première lettre, en majuscule.
func Initiale(nom string) string {
	r, _ := utf8.DecodeRuneInString(strings.TrimSpace(nom))
	if r == utf8.RuneError {
		return "?"
	}
	return string(unicode.ToUpper(r))
}

// symboles des devises proposées dans les réglages.
var symboles = map[string]string{"EUR": "€", "CHF": "CHF", "USD": "$", "GBP": "£", "CAD": "$ CA", "XOF": "FCFA", "MAD": "DH"}

// Devises : codes proposés dans les réglages de la famille.
var Devises = []string{"EUR", "CHF", "CAD", "USD", "GBP", "MAD", "XOF"}

// Montant écrit des centimes à la française : 1 234,56 €.
func Montant(centimes int64, devise string) string {
	signe := ""
	if centimes < 0 {
		signe, centimes = "−", -centimes
	}
	entier := fmt.Sprint(centimes / 100)
	// Espace fine insécable entre les milliers.
	var b strings.Builder
	for i, c := range entier {
		if i > 0 && (len(entier)-i)%3 == 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(c)
	}
	s, ok := symboles[devise]
	if !ok {
		s = devise
	}
	return fmt.Sprintf("%s%s,%02d %s", signe, b.String(), centimes%100, s)
}

// Milliers : 123456 → « 123 456 » (espace fine insécable, à la française).
func Milliers(n int64) string {
	s := strconv.FormatInt(n, 10)
	var b strings.Builder
	if n < 0 {
		return "-" + Milliers(-n)
	}
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteRune('\u202f')
		}
		b.WriteRune(c)
	}
	return b.String()
}

// TexteMois : « Une fois », « Chaque mois », « Tous les 3 mois », « Chaque
// année », « Tous les 2 ans ».
func TexteMois(n int64) string {
	switch {
	case n == 0:
		return "Une fois"
	case n == 1:
		return "Chaque mois"
	case n == 12:
		return "Chaque année"
	case n%12 == 0:
		return "Tous les " + strconv.FormatInt(n/12, 10) + " ans"
	}
	return "Tous les " + strconv.FormatInt(n, 10) + " mois"
}

// Symbole : « € » pour EUR (le code lui-même si la devise est inconnue).
func Symbole(devise string) string {
	if s, ok := symboles[devise]; ok {
		return s
	}
	return devise
}
