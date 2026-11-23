// SPDX-License-Identifier: GPL-3.0-or-later

package notes

import (
	"strconv"
	"strings"

	"familycocon/internal/donnees"
	"familycocon/internal/riche"
)

// Photos dans le texte d'une note : une ligne « [photo 12] » (12 : le
// numéro de la photo de cette note) montre la photo à cet endroit.
//   - l'éditeur de la page (app.js) montre la photo pendant la saisie et
//     écrit cette ligne dans le texte enregistré ;
//   - une photo retirée du texte retourne en bas de la note (rien n'est
//     effacé sans le ✕ de la photo) ;
//   - la note reste du texte : recherche et export marchent comme avant.

// marque : voir riche.Marque.
var marque = riche.Marque

// placees : les photos placées dans le texte.
func placees(texte string) map[int64]bool {
	p := map[int64]bool{}
	for _, l := range strings.Split(texte, "\n") {
		if id := marque(l); id > 0 {
			p[id] = true
		}
	}
	return p
}

// garderMarques : le texte sans les repères d'une photo qui n'est pas à
// cette note (ou déjà placée plus haut).
func garderMarques(texte string, photos []donnees.PhotoNote) string {
	connues := map[int64]bool{}
	for _, p := range photos {
		connues[p.ID] = true
	}
	vues := map[int64]bool{}
	lignes := strings.Split(texte, "\n")
	out := lignes[:0]
	for _, l := range lignes {
		if id := marque(l); id > 0 {
			if !connues[id] || vues[id] {
				continue
			}
			vues[id] = true
			l = "[photo " + strconv.FormatInt(id, 10) + "]"
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// enBas : les photos qui ne sont pas placées dans le texte (montrées en bas).
func enBas(texte string, photos []donnees.PhotoNote) []donnees.PhotoNote {
	p := placees(texte)
	var l []donnees.PhotoNote
	for _, x := range photos {
		if !p[x.ID] {
			l = append(l, x)
		}
	}
	return l
}
