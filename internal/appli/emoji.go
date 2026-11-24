// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Emoji choisi dans un formulaire (grille « champ-emoji », ou « Autre » :
// écrit au clavier). Un emoji peut compter plusieurs caractères (☁️, 👥‍👧) :
// 8 au plus, sans lettre, chiffre, espace ni signe ASCII (sauf les touches
// « 1️⃣ » et les symboles-lettres « ℹ️ »).

// EmojiMax : longueur d'un emoji, en caractères.
const EmojiMax = 8

// EmojiValide : vrai pour un emoji seul (jamais pour un texte vide).
func EmojiValide(e string) bool {
	if e == "" || utf8.RuneCountInString(e) > EmojiMax {
		return false
	}
	rs := []rune(e)
	for i, r := range rs {
		switch {
		case (r >= '0' && r <= '9' || r == '#' || r == '*') && i+1 < len(rs) && (rs[i+1] == 0xFE0F || rs[i+1] == 0x20E3):
			// Touche : « 1️⃣ », « #️⃣ ».
		case r >= 0x2100 && r <= 0x214F:
			// Symboles-lettres : « ℹ️ », « ™️ ».
		case r < 0x80 || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r):
			return false
		}
	}
	return true
}

// LireEmoji : l'emoji envoyé, ou defaut s'il est vide ou n'en est pas un.
func LireEmoji(texte, defaut string) string {
	if e := strings.TrimSpace(texte); EmojiValide(e) {
		return e
	}
	return defaut
}
