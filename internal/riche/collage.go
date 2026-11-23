// SPDX-License-Identifier: GPL-3.0-or-later

package riche

import (
	"html"
	"regexp"
	"strings"
)

// Texte collé depuis une autre appli (Méli-Mélo, Joplin, un éditeur
// Markdown…) : souvent du Markdown mêlé de balises HTML
// (« **<u>Pneu :</u>** », « ![](attachment:680) »). DepuisCollage le
// ramène à nos signes ; HTML le met ensuite en forme.

var (
	reImageMD   = regexp.MustCompile(`!\[([^\]\n]*)\]\(([^)\s]*)[^)]*\)`)
	reLienHTML  = regexp.MustCompile(`(?is)<a\s[^>]*href\s*=\s*["']([^"']+)["'][^>]*>(.*?)</a\s*>`)
	reTitreHTML = regexp.MustCompile(`(?i)<h([1-6])\b[^>]*>`)
	reBalise    = regexp.MustCompile(`(?s)<[^>]{0,500}>`)
	// *italique* (une étoile, pas « ** », pas une puce « * »).
	reEtoile = regexp.MustCompile(`(^|[^*\w])\*([^*\s\n](?:[^*\n]*[^*\s\n])?)\*([^*\w]|$)`)
	// Balises de mise en forme → nos signes.
	balises = []struct {
		re    *regexp.Regexp
		signe string
	}{
		{regexp.MustCompile(`(?i)</?(b|strong)\b[^>]*>`), "**"},
		{regexp.MustCompile(`(?i)</?(i|em)\b[^>]*>`), "_"},
		{regexp.MustCompile(`(?i)</?(u|ins)\b[^>]*>`), "++"},
		{regexp.MustCompile(`(?i)</?(s|strike|del)\b[^>]*>`), "~~"},
		{regexp.MustCompile(`(?i)</?code\b[^>]*>`), "`"},
	}
	finsDeLigne = regexp.MustCompile(`(?i)<br\s*/?>|</(p|div|li|h[1-6]|blockquote|tr)\s*>`)
)

// DepuisCollage : le texte collé, avec nos signes à la place des balises.
func DepuisCollage(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	// Images : une adresse web devient un lien ; une pièce jointe d'une
	// autre appli (« attachment:… ») ne peut pas venir : on le dit.
	s = reImageMD.ReplaceAllStringFunc(s, func(m string) string {
		p := reImageMD.FindStringSubmatch(m)
		if adresseOK(p[2]) {
			nom := strings.TrimSpace(p[1])
			if nom == "" {
				nom = "image"
			}
			return "[🖼️ " + nom + "](" + p[2] + ")"
		}
		return "🖼️ (image à coller à part)"
	})
	s = reLienHTML.ReplaceAllString(s, "[$2]($1)")
	s = reTitreHTML.ReplaceAllStringFunc(s, func(m string) string {
		n := int(reTitreHTML.FindStringSubmatch(m)[1][0] - '0')
		return "\n" + strings.Repeat("#", min(n, 3)) + " "
	})
	s = regexp.MustCompile(`(?i)<li\b[^>]*>`).ReplaceAllString(s, "- ")
	s = regexp.MustCompile(`(?i)<blockquote\b[^>]*>`).ReplaceAllString(s, "> ")
	s = regexp.MustCompile(`(?i)<hr\s*/?>`).ReplaceAllString(s, "\n---\n")
	s = finsDeLigne.ReplaceAllString(s, "\n")
	for _, b := range balises {
		s = b.re.ReplaceAllString(s, b.signe)
	}
	s = reBalise.ReplaceAllString(s, "") // le reste (span, p, ul…) : seulement le texte
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, " ", " ")
	// *italique* → _italique_ (deux fois : deux mots qui se suivent).
	for range 2 {
		s = reEtoile.ReplaceAllString(s, "${1}_${2}_${3}")
	}
	return strings.Trim(s, "\n")
}
