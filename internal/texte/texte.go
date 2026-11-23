// SPDX-License-Identifier: GPL-3.0-or-later

// Package texte : petites aides sur le texte, sans dépendance.
package texte

import (
	"strings"
	"unicode"
)

// sansAccents : lettres accentuées (minuscules) et leur lettre de base.
// Les majuscules sont d'abord mises en minuscules par Plier.
var sansAccents = map[rune]string{
	'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'ä': "a", 'å': "a", 'æ': "ae",
	'ç': "c", 'è': "e", 'é': "e", 'ê': "e", 'ë': "e",
	'ì': "i", 'í': "i", 'î': "i", 'ï': "i", 'ñ': "n",
	'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o", 'ö': "o", 'ø': "o", 'œ': "oe",
	'ù': "u", 'ú': "u", 'û': "u", 'ü': "u", 'ý': "y", 'ÿ': "y", 'ß': "ss",
}

// Plier : le texte en minuscules et sans accents, pour chercher sans
// se soucier ni des majuscules ni des accents (« ÉCOLE » = « ecole »,
// « Noël » = « noel »).
func Plier(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		r = unicode.ToLower(r)
		if rempl, ok := sansAccents[r]; ok {
			b.WriteString(rempl)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Contient : vrai si cherche est dans s, sans tenir compte des majuscules
// ni des accents.
func Contient(s, cherche string) bool {
	return strings.Contains(Plier(s), Plier(cherche))
}
