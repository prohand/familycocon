// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import "html/template"

// Icônes dessinées : quand aucun emoji ne convient, un petit dessin SVG
// écrit dans la page (pas de fichier à charger, aucune dépendance) prend sa
// place partout où le module est montré (tuile, barre du bas, « + »…).
// L'emoji reste dans les textes (notifications, listes de choix).

// EmojiFidelite : emoji de secours des cartes de fidélité (aucun emoji ne
// montre une carte à tampons ; 💳 est une carte bancaire).
const EmojiFidelite = "🪪"

// carteFidelite : une carte à tampons, trois tampons faits, une étoile.
const carteFidelite = template.HTML(`<svg class="icone" viewBox="0 0 64 64" aria-hidden="true" focusable="false">` +
	`<rect x="3" y="11" width="58" height="42" rx="7" fill="#5DAE48"/>` +
	`<path d="M3 18a7 7 0 0 1 7-7h44a7 7 0 0 1 7 7v6H3z" fill="#3F8A31"/>` +
	`<path d="M51 13.6l1.6 3.3 3.6.5-2.6 2.5.6 3.6-3.2-1.7-3.2 1.7.6-3.6-2.6-2.5 3.6-.5z" fill="#FFD54A"/>` +
	`<rect x="9" y="15.5" width="22" height="4" rx="2" fill="#CFEAC3"/>` +
	`<circle cx="14" cy="38" r="5" fill="#fff"/><circle cx="26" cy="38" r="5" fill="#fff"/><circle cx="38" cy="38" r="5" fill="#fff"/>` +
	`<circle cx="50" cy="38" r="4.2" fill="none" stroke="#fff" stroke-width="1.8" stroke-dasharray="2.6 2"/>` +
	`<path d="M11.6 38.2l1.7 1.7 3.2-3.4M23.6 38.2l1.7 1.7 3.2-3.4M35.6 38.2l1.7 1.7 3.2-3.4" fill="none" stroke="#3F8A31" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round"/>` +
	`</svg>`)

// icones : emoji → dessin.
var icones = map[string]template.HTML{EmojiFidelite: carteFidelite}

// Icone : le dessin de l'emoji s'il y en a un, sinon l'emoji (échappé).
// Dans les gabarits : {{emoji .Emoji}}.
func Icone(emoji string) template.HTML {
	if d, ok := icones[emoji]; ok {
		return d
	}
	return template.HTML(template.HTMLEscapeString(emoji)) // #nosec G203 -- texte échappé juste avant
}
