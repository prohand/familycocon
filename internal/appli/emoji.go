// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Emoji choisi dans un formulaire (grille « champ-emoji », ou « Autre » :
// écrit au clavier). Un emoji peut compter plusieurs caractères (☁️, 👨‍👩‍👧) :
// 8 au plus, sans lettre, chiffre, espace ni signe ASCII.

// EmojiMax : longueur d'un emoji, en caractères.
const EmojiMax = 8

// EmojiValide : vrai pour un emoji seul (jamais pour un texte vide).
func EmojiValide(e string) bool {
	if e == "" || utf8.RuneCountInString(e) > EmojiMax {
		return false
	}
	for _, r := range e {
		if r < 0x80 || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
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
