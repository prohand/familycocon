// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"errors"
	"fmt"
	"html/template"
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
		// {{pluriel 3 "produit" "produits"}} → 3 produits
		"pluriel": func(n any, un, plusieurs string) string {
			var v int64
			switch x := n.(type) {
			case int:
				v = int64(x)
			case int64:
				v = x
			}
			if v == 0 || v == 1 || v == -1 {
				return fmt.Sprintf("%d %s", v, un)
			}
			return fmt.Sprintf("%d %s", v, plusieurs)
		},
		// {{montant 123456 "EUR"}} → 1 234,56 €
		"montant": Montant,
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
	}
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
