// SPDX-License-Identifier: GPL-3.0-or-later

// Package riche : la mise en forme des textes longs (notes, descriptions,
// notes des fiches), écrits par l'éditeur de app.js.
package riche

import (
	"fmt"
	"html/template"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Mise en forme des notes. Le texte enregistré reste du texte simple, une
// ligne par ligne de la note, avec quelques signes (proches de Markdown) :
//
//	# Titre, ## Sous-titre, ### Petit titre
//	> citation
//	- puce        1. liste numérotée
//	[ ] case      [x] case cochée
//	---           (ligne de séparation)
//	[photo 12]    (une photo de la note)
//	**gras**  _italique_  ++souligné++  ~~barré~~  `code`
//	[texte](https://…)   et les adresses http(s) seules, cliquables
//
// Un « \ » devant un signe le garde tel quel (« \* », « \# » en début de
// ligne). L'éditeur de la page (app.js) écrit ce texte ; ici, il devient
// du HTML (tout le texte est échappé : aucune balise ne passe), pour la
// lecture ou pour l'éditeur.

// Mode : façon de rendre le texte.
type Mode int

const (
	Lecture Mode = iota // page qui montre le texte
	Edition             // contenu de l'éditeur (app.js)
)

// Options du rendu.
type Options struct {
	Mode Mode
	// Note : numéro de la note ; ses cases se cochent d'un toucher (htmx)
	// et ses photos (« [photo N] ») s'affichent. 0 ailleurs : cases
	// figées, repères de photo laissés en texte.
	Note int64
	// Notes, en lecture : cases cochées rangées après les autres (dans
	// chaque suite de cases ; le texte, lui, ne change pas).
	CocheesEnBas bool
	// Notes, en lecture : « Ranger la liste » : chaque case devient un
	// champ (✓ enregistrer, ↑ ↓ déplacer, ✕ supprimer). Jeton : CSRF.
	Ranger bool
	Jeton  string
}

var reMarque = regexp.MustCompile(`^\[photo (\d+)\]$`)

// Marque : le numéro de la photo si la ligne est un repère « [photo N] », sinon 0.
func Marque(ligne string) int64 {
	m := reMarque.FindStringSubmatch(strings.TrimSpace(ligne))
	if m == nil {
		return 0
	}
	id, _ := strconv.ParseInt(m[1], 10, 64)
	return id
}

// LireCase : la ligne est-elle une case ? (préfixe, texte, cochée). Une
// ligne qui commence par « [ ] » (ou « - [ ] ») est une case ; « [x] »
// une case cochée.
func LireCase(l string) (prefixe, texte string, cochee, ok bool) {
	t := strings.TrimLeft(l, " \t")
	debut := l[:len(l)-len(t)]
	if strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") {
		debut += t[:2]
		t = t[2:]
	}
	switch {
	case strings.HasPrefix(t, "[ ]"):
		return debut, strings.TrimSpace(t[3:]), false, true
	case strings.HasPrefix(t, "[x]"), strings.HasPrefix(t, "[X]"):
		return debut, strings.TrimSpace(t[3:]), true, true
	}
	return "", "", false, false
}

var (
	reNumero  = regexp.MustCompile(`^(\d{1,6})\. (.*)$`)
	reSepare  = regexp.MustCompile(`^-{3,}$`)
	reTitre   = regexp.MustCompile(`^(#{1,3}) (.*)$`)
	reCitPuce = regexp.MustCompile(`^(?:[-*•]) (.*)$`)
)

// bloc : une ligne de la note, reconnue.
type bloc struct {
	sorte  string // "p", "titre", "cit", "puce", "num", "case", "photo", "hr", "vide"
	niveau int    // titre : 1 à 3 ; num : le numéro
	texte  string // ce qui suit le signe (mise en forme comprise)
	cochee bool
	photo  int64
	ligne  int // numéro de la ligne dans le texte
}

// lireBloc : la sorte d'une ligne.
func lireBloc(l string, i int) bloc {
	l = strings.TrimRight(l, "\r")
	t := strings.TrimSpace(l)
	b := bloc{ligne: i}
	switch {
	case t == "":
		b.sorte = "vide"
	case strings.HasPrefix(t, `\`) && len(t) > 1 && strings.ContainsRune(`#>-*•0123456789[`, rune(t[1])):
		b.sorte, b.texte = "p", t // « \ » gardé : enLigne l'enlève
	case Marque(t) > 0:
		b.sorte, b.photo = "photo", Marque(t)
	case reSepare.MatchString(t):
		b.sorte = "hr"
	default:
		if _, x, cochee, ok := LireCase(l); ok {
			b.sorte, b.texte, b.cochee = "case", x, cochee
		} else if m := reTitre.FindStringSubmatch(t); m != nil {
			b.sorte, b.niveau, b.texte = "titre", len(m[1]), m[2]
		} else if t == ">" || strings.HasPrefix(t, "> ") {
			b.sorte, b.texte = "cit", strings.TrimPrefix(strings.TrimPrefix(t, ">"), " ")
		} else if m := reCitPuce.FindStringSubmatch(t); m != nil {
			b.sorte, b.texte = "puce", m[1]
		} else if m := reNumero.FindStringSubmatch(t); m != nil {
			n, _ := strconv.Atoi(m[1])
			b.sorte, b.niveau, b.texte = "num", n, m[2]
		} else {
			b.sorte, b.texte = "p", l
		}
	}
	return b
}

// HTML : le texte mis en forme (tout le texte est échappé).
func HTML(texte string, o Options) template.HTML {
	mode, note := o.Mode, o.Note
	var b strings.Builder
	lignes := strings.Split(texte, "\n")
	ouvert := "" // liste ou citation en cours : "ul", "ol", "blockquote"
	fermer := func() {
		if ouvert != "" {
			b.WriteString("</" + ouvert + ">")
			ouvert = ""
		}
	}
	ouvrir := func(tag, attrs string) {
		if ouvert != tag {
			fermer()
			b.WriteString("<" + tag + attrs + ">")
			ouvert = tag
		}
	}
	blocs := make([]bloc, len(lignes))
	ordre := make([]int, len(lignes)) // ordre d'affichage des lignes
	for i, l := range lignes {
		blocs[i], ordre[i] = lireBloc(l, i), i
	}
	if mode == Lecture && note != 0 && o.CocheesEnBas && !o.Ranger {
		for i := 0; i < len(blocs); {
			if blocs[i].sorte != "case" {
				i++
				continue
			}
			j := i
			for j < len(blocs) && blocs[j].sorte == "case" {
				j++
			}
			suite := ordre[i:j]
			sort.SliceStable(suite, func(a, b int) bool { return !blocs[suite[a]].cochee && blocs[suite[b]].cochee })
			i = j
		}
	}
	for _, i := range ordre {
		l, x := lignes[i], blocs[i]
		if x.sorte == "photo" && note == 0 {
			x.sorte, x.texte = "p", `\`+strings.TrimSpace(l) // hors des notes : un texte
		}
		switch x.sorte {
		case "puce":
			ouvrir("ul", "")
			b.WriteString("<li>" + enLigne(x.texte, true) + "</li>")
			continue
		case "num":
			if ouvert != "ol" && x.niveau != 1 {
				ouvrir("ol", fmt.Sprintf(` start="%d"`, x.niveau))
			} else {
				ouvrir("ol", "")
			}
			b.WriteString("<li>" + enLigne(x.texte, true) + "</li>")
			continue
		case "cit":
			ouvrir("blockquote", "")
			b.WriteString("<div>" + videOuTexte(x.texte) + "</div>")
			continue
		}
		fermer()
		switch x.sorte {
		case "vide":
			if mode == Edition {
				b.WriteString("<div><br></div>")
			} else {
				b.WriteString(`<div class="ligne-vide"></div>`)
			}
		case "p":
			if mode == Edition {
				b.WriteString("<div>" + videOuTexte(x.texte) + "</div>")
			} else {
				b.WriteString(`<p class="texte-note">` + enLigne(x.texte, true) + "</p>")
			}
		case "titre":
			b.WriteString(fmt.Sprintf("<h%d>%s</h%d>", x.niveau, enLigne(x.texte, true), x.niveau))
		case "hr":
			if mode == Edition {
				b.WriteString(`<hr contenteditable="false">`)
			} else {
				b.WriteString("<hr>")
			}
		case "photo":
			src := fmt.Sprintf("/notes/photos/%d", x.photo)
			if mode == Edition {
				// Dans l'éditeur : l'image ne se glisse pas (le navigateur la
				// sortirait de son cadre) ; elle se déplace avec ↑ ↓ (app.js).
				b.WriteString(fmt.Sprintf(`<figure class="photo-texte" contenteditable="false" draggable="false" data-photo="%d"><img src="%s" alt="Photo de la note" draggable="false">`+
					`<span class="outils-photo"><button type="button" class="monter-photo" aria-label="Monter la photo">↑</button>`+
					`<button type="button" class="descendre-photo" aria-label="Descendre la photo">↓</button>`+
					`<button type="button" class="retirer-photo" aria-label="Retirer la photo du texte">✕</button></span></figure>`, x.photo, src))
			} else {
				b.WriteString(fmt.Sprintf(`<figure class="photo-texte"><a href="%s" target="_blank" rel="noopener"><img src="%s" alt="Photo de la note" loading="lazy"></a></figure>`, src, src))
			}
		case "case":
			coche := ""
			if x.cochee {
				coche = " checked"
			}
			if mode == Edition {
				// Case dessinée en CSS (pas d'élément figé dans le texte : tout
				// s'efface et se sélectionne normalement) ; un toucher sur la
				// case la coche (app.js).
				etat := "0"
				if x.cochee {
					etat = "1"
				}
				b.WriteString(`<div class="case-note" data-case data-cochee="` + etat + `">` + videOuTexte(x.texte) + `</div>`)
			} else if o.Ranger && note != 0 {
				b.WriteString(rangee(note, x, strings.TrimRight(l, "\r"), o.Jeton))
			} else if note == 0 {
				b.WriteString(fmt.Sprintf(`<label class="case-a-cocher"><input type="checkbox"%s disabled> %s</label>`, coche, enLigne(x.texte, true)))
			} else {
				classe := "case-a-cocher"
				vers := "1"
				if x.cochee {
					classe, vers = "case-a-cocher cochee", "0"
				}
				b.WriteString(fmt.Sprintf(`<label class="%s"><input type="checkbox"%s hx-post="/notes/%d/cocher" hx-vals='{"ligne": "%d", "cochee": "%s"}' hx-trigger="change" hx-target="#cases-note" hx-swap="outerHTML"> %s</label>`,
					classe, coche, note, x.ligne, vers, enLigne(x.texte, true)))
			}
		}
	}
	fermer()
	return template.HTML(b.String()) // #nosec G203 -- tout le texte est échappé par enLigne
}

// videOuTexte : une ligne de l'éditeur (vide : <br>, pour qu'elle existe).
func videOuTexte(s string) string {
	if strings.TrimSpace(s) == "" {
		return "<br>"
	}
	return enLigne(s, true)
}

// Simple : le texte sans ses signes, lignes gardées (aperçus, export).
func Simple(texte string) string {
	l := strings.Split(texte, "\n")
	for i := range l {
		l[i] = LigneSimple(l[i])
	}
	return strings.Join(l, "\n")
}

// LigneSimple : une ligne sans ses signes.
func LigneSimple(l string) string {
	x := lireBloc(l, 0)
	switch x.sorte {
	case "photo":
		return "📷"
	case "hr":
		return "—"
	case "vide":
		return ""
	case "case":
		if x.cochee {
			return "☑ " + enLigne(x.texte, false)
		}
		return "☐ " + enLigne(x.texte, false)
	case "puce":
		return "• " + enLigne(x.texte, false)
	case "num":
		return strconv.Itoa(x.niveau) + ". " + enLigne(x.texte, false)
	}
	return enLigne(x.texte, false)
}

// Echapper : du texte simple (un élément ajouté à une liste) gardé tel
// quel : les signes de mise en forme sont précédés d'un « \ » (comme le
// fait l'éditeur).
func Echapper(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `*`, `\*`, `_`, `\_`, "`", "\\`", `[`, `\[`, `~~`, `\~\~`, `++`, `\+\+`)
	return r.Replace(s)
}

// adresseOK : un lien sûr (pas de javascript:…).
func adresseOK(u string) bool {
	return (strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "mailto:")) &&
		!strings.ContainsAny(u, " \t\"'<>")
}

// lien : un lien qui s'ouvre à part.
func lien(u, texte string) string {
	return `<a href="` + template.HTMLEscapeString(u) + `" target="_blank" rel="noopener noreferrer">` + texte + `</a>`
}

// enLigne : la mise en forme d'une ligne (gras, italique, liens…), en HTML
// (tout le texte échappé) ou en texte simple (signes enlevés).
func enLigne(s string, html bool) string {
	var b strings.Builder
	esc := func(t string) {
		if html {
			b.WriteString(template.HTMLEscapeString(t))
		} else {
			b.WriteString(t)
		}
	}
	balise := func(tag, dedans string) {
		if html {
			b.WriteString("<" + tag + ">" + enLigne(dedans, true) + "</" + tag + ">")
		} else {
			b.WriteString(enLigne(dedans, false))
		}
	}
	// Recherches qui ont déjà échoué : un signe sans fermeture plus loin
	// n'en a pas non plus à partir d'une position suivante. Sans cela, une
	// ligne de milliers de « [ » ou de « ** » coûtait un temps énorme (chaque
	// signe relisait toute la fin de la ligne).
	sansFin := map[string]bool{}
	ferme := func(reste string, debut int, signe string) int {
		if sansFin[signe] {
			return -1
		}
		j := fermeture(reste, debut, signe)
		if j < 0 {
			sansFin[signe] = true
		}
		return j
	}
	chercher := func(reste string, c byte) int {
		if sansFin[string(c)] {
			return -1
		}
		j := strings.IndexByte(reste, c)
		if j < 0 {
			sansFin[string(c)] = true
		}
		return j
	}
	prochainEspace := -1 // position (dans s) du prochain blanc, ou len(s)
	for i := 0; i < len(s); {
		reste := s[i:]
		avant, _ := utf8.DecodeLastRuneInString(s[:i])
		debutMot := i == 0 || !(unicode.IsLetter(avant) || unicode.IsDigit(avant))
		switch {
		case reste[0] == '\\' && len(reste) > 1:
			_, n := utf8.DecodeRuneInString(reste[1:])
			esc(reste[1 : 1+n])
			i += 1 + n
			continue
		case debutMot && (strings.HasPrefix(reste, "https://") || strings.HasPrefix(reste, "http://")):
			if prochainEspace < i {
				if j := strings.IndexFunc(reste, unicode.IsSpace); j < 0 {
					prochainEspace = len(s)
				} else {
					prochainEspace = i + j
				}
			}
			fin := prochainEspace - i
			u := strings.TrimRight(reste[:fin], ".,;:!?")
			if strings.HasSuffix(u, ")") && !strings.Contains(u, "(") {
				u = strings.TrimSuffix(u, ")")
			}
			if adresseOK(u) {
				if html {
					b.WriteString(lien(u, template.HTMLEscapeString(u)))
				} else {
					b.WriteString(u)
				}
				i += len(u)
				continue
			}
		case reste[0] == '`':
			if j := chercher(reste[1:], '`'); j >= 0 {
				if html {
					b.WriteString("<code>" + template.HTMLEscapeString(reste[1:1+j]) + "</code>")
				} else {
					b.WriteString(reste[1 : 1+j])
				}
				i += j + 2
				continue
			}
		case strings.HasPrefix(reste, "**"):
			if j := ferme(reste, 2, "**"); j > 2 {
				balise("strong", reste[2:j])
				i += j + 2
				continue
			}
		case strings.HasPrefix(reste, "++"):
			if j := ferme(reste, 2, "++"); j > 2 {
				balise("u", reste[2:j])
				i += j + 2
				continue
			}
		case strings.HasPrefix(reste, "~~"):
			if j := ferme(reste, 2, "~~"); j > 2 {
				balise("s", reste[2:j])
				i += j + 2
				continue
			}
		case reste[0] == '_' && debutMot:
			if j := ferme(reste, 1, "_"); j > 1 {
				apres, _ := utf8.DecodeRuneInString(reste[j+1:])
				if j+1 == len(reste) || !(unicode.IsLetter(apres) || unicode.IsDigit(apres)) {
					balise("em", reste[1:j])
					i += j + 1
					continue
				}
			}
		case reste[0] == '[':
			if j := ferme(reste, 1, "]("); j > 1 {
				if k := chercher(reste[j+2:], ')'); k > 0 {
					u := reste[j+2 : j+2+k]
					if adresseOK(u) {
						if html {
							b.WriteString(lien(u, enLigne(reste[1:j], true)))
						} else {
							b.WriteString(enLigne(reste[1:j], false))
						}
						i += j + 3 + k
						continue
					}
				}
			}
		}
		_, n := utf8.DecodeRuneInString(reste)
		esc(reste[:n])
		i += n
	}
	return b.String()
}

// fermeture : où se ferme le signe ouvert (cherché à partir de debut, les
// caractères précédés d'un « \ » sautés) ; -1 si nulle part.
func fermeture(s string, debut int, signe string) int {
	for i := debut; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if strings.HasPrefix(s[i:], signe) {
			return i
		}
	}
	return -1
}

// rangee : une case de la liste en mode « Ranger » : son texte dans un
// champ, et les boutons. « avant » : la ligne telle qu'elle était (le
// serveur vérifie qu'elle n'a pas changé entre-temps).
func rangee(note int64, x bloc, ligne, jeton string) string {
	base := fmt.Sprintf("/notes/elements/%d/%d", note, x.ligne)
	cible := ` hx-target="#cases-note" hx-swap="outerHTML"`
	esc := template.HTMLEscapeString
	classe := "rangee-element"
	if x.cochee {
		classe += " cochee"
	}
	return `<form class="` + classe + `" method="post" action="` + base + `" hx-post="` + base + `"` + cible + `>` +
		`<input type="hidden" name="_jeton" value="` + esc(jeton) + `"><input type="hidden" name="avant" value="` + esc(ligne) + `">` +
		`<input name="texte" value="` + esc(enLigne(x.texte, false)) + `" maxlength="200" aria-label="Élément" required autocomplete="off">` +
		`<button class="bouton petit" aria-label="Enregistrer">✓</button>` +
		`<button type="button" class="crayon" hx-post="` + base + `/monter"` + cible + ` aria-label="Monter">↑</button>` +
		`<button type="button" class="crayon" hx-post="` + base + `/descendre"` + cible + ` aria-label="Descendre">↓</button>` +
		`<button type="button" class="crayon" hx-post="` + base + `/supprimer"` + cible + ` aria-label="Supprimer">✕</button></form>`
}
