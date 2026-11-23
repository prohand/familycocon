// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"slices"

	"familycocon/internal/donnees"
)

// Avatars : chaque membre peut remplacer l'initiale de son prénom par un
// emoji (un animal, un visage…), dans son rond de couleur. Une liste
// fermée (rien d'autre n'est accepté), pas de photo : la base reste légère.
var Avatars = []string{
	"🐶", "🐱", "🦊", "🐻", "🐼", "🐨", "🐯", "🦁", "🐸", "🐵",
	"🐰", "🦄", "🐲", "🐙", "🦋", "🐢", "🐬", "🦉", "🐧", "🐝",
	"👩", "👨", "👧", "👦", "👵", "👴", "🧑", "👶",
	"⭐", "🌸", "🌈", "⚽", "🎸", "🚀", "🍕", "🎨",
}

// AvatarValide : un emoji de la liste, ou « » (l'initiale).
func AvatarValide(a string) bool { return a == "" || slices.Contains(Avatars, a) }

// Visage : ce qui s'affiche dans le rond d'un membre : son avatar, sinon
// l'initiale de son prénom. Accepte un membre ou un pointeur (nil : « ? »).
func Visage(m any) string {
	switch x := m.(type) {
	case donnees.Membre:
		return visage(x)
	case *donnees.Membre:
		if x != nil {
			return visage(*x)
		}
	}
	return "?"
}

func visage(m donnees.Membre) string {
	if m.Avatar != "" {
		return m.Avatar
	}
	return Initiale(m.Nom)
}
