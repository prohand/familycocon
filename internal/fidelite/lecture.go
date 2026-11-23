// SPDX-License-Identifier: GPL-3.0-or-later

package fidelite

import (
	"image"
	"sort"
	"strings"
)

// Lecture d'un code-barres sur une photo (scan de la carte avec le
// téléphone), sans bibliothèque : EAN-13, UPC-A, EAN-8 et Code 128, les
// codes des cartes de fidélité. Les QR codes ne sont pas lus ici (le
// navigateur sait souvent le faire lui-même, voir app.js).
//
// Méthode : l'image est passée en niveaux de gris, puis lue le long de
// quelques dizaines de lignes, horizontales et verticales. Chaque ligne
// devient une suite de largeurs (barre, espace, barre…), comparées aux
// motifs de chaque code. La clé de contrôle du code écarte les erreurs ;
// le numéro trouvé le plus souvent l'emporte.

// gris : une image en niveaux de gris (0 noir … 255 blanc).
type gris struct {
	l, h int
	pix  []uint8
}

func versGris(img image.Image) gris {
	b := img.Bounds()
	g := gris{l: b.Dx(), h: b.Dy(), pix: make([]uint8, b.Dx()*b.Dy())}
	for y := 0; y < g.h; y++ {
		for x := 0; x < g.l; x++ {
			r, v, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			// Luminance (poids habituels), sur 16 bits → 8 bits.
			g.pix[y*g.l+x] = uint8((299*r + 587*v + 114*bl) / 1000 >> 8)
		}
	}
	return g
}

// ligne : la ligne n (horizontale si vertical est faux), moyennée avec ses
// deux voisines (moins de bruit).
func (g gris) ligne(n int, vertical bool) []int {
	longueur, max := g.l, g.h
	if vertical {
		longueur, max = g.h, g.l
	}
	out := make([]int, longueur)
	for _, k := range []int{n - 1, n, n + 1} {
		if k < 0 || k >= max {
			k = n
		}
		for i := 0; i < longueur; i++ {
			if vertical {
				out[i] += int(g.pix[i*g.l+k])
			} else {
				out[i] += int(g.pix[k*g.l+i])
			}
		}
	}
	return out
}

// largeurs : la ligne en largeurs alternées ; noir : couleur du premier
// morceau. Seuil : milieu entre le plus clair et le plus foncé de la ligne
// (en écartant les extrêmes), ou moyenne locale si local.
func largeurs(v []int, local bool) (l []int, noir bool) {
	if len(v) < 20 {
		return nil, false
	}
	tri := append([]int(nil), v...)
	sort.Ints(tri)
	bas, haut := tri[len(tri)/20], tri[len(tri)*19/20]
	if haut-bas < 3*30 { // trop peu de contraste (×3 : lignes additionnées)
		return nil, false
	}
	seuil := func(int) int { return (bas + haut) / 2 }
	if local {
		somme := make([]int, len(v)+1)
		for i, x := range v {
			somme[i+1] = somme[i] + x
		}
		f := max(8, len(v)/12)
		seuil = func(i int) int {
			a, b := max(0, i-f), min(len(v), i+f)
			m := (somme[b] - somme[a]) / (b - a)
			return min(m, (bas+haut)/2+(haut-bas)/4) // pas de seuil dans le blanc pur
		}
	}
	courant := v[0] < seuil(0)
	noir = courant
	n := 0
	for i, x := range v {
		c := x < seuil(i)
		if c != courant {
			l = append(l, n)
			n, courant = 0, c
		}
		n++
	}
	return append(l, n), noir
}

// motif : les largeurs d'une suite de modules (« 0001101 » → 3,2,1,1).
func motif(modules string) []float64 {
	var out []float64
	for i := 0; i < len(modules); {
		j := i
		for j < len(modules) && modules[j] == modules[i] {
			j++
		}
		out = append(out, float64(j-i))
		i = j
	}
	return out
}

// ecart : différence entre des largeurs mesurées (ramenées à total
// modules) et un motif.
func ecart(w []int, m []float64, total float64) float64 {
	s := 0
	for _, x := range w {
		s += x
	}
	if s == 0 || len(w) != len(m) {
		return 99
	}
	e := 0.0
	for i, x := range w {
		d := float64(x)*total/float64(s) - m[i]
		if d < 0 {
			d = -d
		}
		e += d
	}
	return e
}

var (
	motifsL, motifsG, motifsR [10][]float64
	motifs128Lus              [107][]float64
)

func init() {
	for d := 0; d < 10; d++ {
		motifsL[d], motifsG[d], motifsR[d] = motif(eanL[d]), motif(eanG[d]), motif(eanR[d])
	}
	for i, m := range motifs128 {
		for _, c := range m {
			motifs128Lus[i] = append(motifs128Lus[i], float64(c-'0'))
		}
	}
}

// meilleur : l'indice du motif le plus proche (et son écart).
func meilleur(w []int, motifs [][]float64, total float64) (int, float64) {
	best, e := -1, 99.0
	for i, m := range motifs {
		if x := ecart(w, m, total); x < e {
			best, e = i, x
		}
	}
	return best, e
}

// ecartMax : écart accepté pour un chiffre EAN (7 modules) ; ecartMax128
// pour un symbole Code 128 (11 modules). La clé de contrôle et les marges
// blanches rattrapent le reste.
const (
	ecartMax    = 2.2
	ecartMax128 = 3.2
)

// moduleProche : chaque largeur vaut à peu près un module.
func garde(w []int, module float64) bool {
	for _, x := range w {
		if float64(x) < module*0.4 || float64(x) > module*2.2 {
			return false
		}
	}
	return true
}

// lireEAN : un EAN-13 (ou UPC-A) ou un EAN-8 qui commence à la barre i.
func lireEAN(l []int, i int) (string, string, bool) {
	for _, n := range []int{8, 13} {
		runs := 59
		modules := 95.0
		if n == 8 {
			runs, modules = 43, 67
		}
		if i+runs > len(l) {
			continue
		}
		w := l[i : i+runs]
		s := 0
		for _, x := range w {
			s += x
		}
		module := float64(s) / modules
		// Marges blanches avant et après le code, gardes de début et de fin
		// (sans elles, un bout d'un autre code passerait pour un EAN-8).
		if (i > 0 && float64(l[i-1]) < module*3) || (i+runs < len(l) && float64(l[i+runs]) < module*3) {
			continue
		}
		if !garde(w[:3], module) || !garde(w[runs-3:], module) {
			continue
		}
		moitie := (n - 1) / 2 // 6 chiffres à gauche (EAN-13), 4 (EAN-8)
		if n == 8 {
			moitie = 4
		}
		var b strings.Builder
		parite := ""
		ok := true
		for d := 0; d < moitie && ok; d++ {
			x := w[3+4*d : 7+4*d]
			kl, el := meilleur(x, motifsL[:], 7)
			kg, eg := 99, 99.0
			if n == 13 {
				kg, eg = meilleur(x, motifsG[:], 7)
			}
			switch {
			case el <= eg && el < ecartMax:
				b.WriteByte(byte('0' + kl))
				parite += "L"
			case eg < ecartMax:
				b.WriteByte(byte('0' + kg))
				parite += "G"
			default:
				ok = false
			}
		}
		milieu := 3 + 4*moitie
		if !ok || !garde(w[milieu:milieu+5], module) {
			continue
		}
		for d := 0; d < moitie && ok; d++ {
			x := w[milieu+5+4*d : milieu+9+4*d]
			k, e := meilleur(x, motifsR[:], 7)
			if e >= ecartMax {
				ok = false
				break
			}
			b.WriteByte(byte('0' + k))
		}
		if !ok {
			continue
		}
		if n == 8 {
			if EstEAN8(b.String()) {
				return b.String(), "ean8", true
			}
			continue
		}
		premier := -1
		for k, p := range eanParite {
			if p == parite {
				premier = k
			}
		}
		if premier < 0 {
			continue
		}
		num := string(rune('0'+premier)) + b.String()
		if EstEAN13(num) {
			if num[0] == '0' {
				return num[1:], "upca", true
			}
			return num, "ean13", true
		}
	}
	return "", "", false
}

// lire128 : un Code 128 qui commence à la barre i.
func lire128(l []int, i int) (string, bool) {
	if i+6 > len(l) {
		return "", false
	}
	s := 0
	for _, x := range l[i : i+6] {
		s += x
	}
	if i > 0 && float64(l[i-1]) < float64(s)/11*5 { // marge blanche
		return "", false
	}
	debut, e := meilleur(l[i:i+6], motifs128Lus[103:106], 11)
	if e >= ecartMax128 {
		return "", false
	}
	valeurs := []int{103 + debut}
	for j := i + 6; ; j += 6 {
		if j+7 <= len(l) {
			// Fin : le motif de fin suivi d'une marge blanche (sinon c'est
			// peut-être un symbole ordinaire qui lui ressemble).
			if ecart(l[j:j+7], motifs128Lus[106], 13) < ecartMax128 &&
				(j+7 >= len(l) || float64(l[j+7]) >= float64(s)/11*5) {
				break
			}
		}
		if j+6 > len(l) || len(valeurs) > 82 {
			return "", false
		}
		v, e := meilleur(l[j:j+6], motifs128Lus[:106], 11)
		if e >= ecartMax128 {
			return "", false
		}
		valeurs = append(valeurs, v)
	}
	if len(valeurs) < 3 {
		return "", false
	}
	// Clé : (début + somme des valeurs × leur rang) modulo 103.
	somme := valeurs[0]
	for k := 1; k < len(valeurs)-1; k++ {
		somme += k * valeurs[k]
	}
	if somme%103 != valeurs[len(valeurs)-1] {
		return "", false
	}
	return texte128(valeurs[:len(valeurs)-1])
}

// texte128 : les valeurs (début compris, clé exclue) en texte.
func texte128(v []int) (string, bool) {
	jeu := map[int]byte{103: 'A', 104: 'B', 105: 'C'}[v[0]]
	var b strings.Builder
	decale := false
	for _, x := range v[1:] {
		j := jeu
		if decale {
			j = map[byte]byte{'A': 'B', 'B': 'A'}[jeu]
			decale = false
		}
		switch {
		case j == 'C' && x < 100:
			b.WriteByte(byte('0' + x/10))
			b.WriteByte(byte('0' + x%10))
		case x == 98 && j != 'C':
			decale = true
		case x == 99:
			jeu = 'C'
		case x == 100:
			if j != 'B' {
				jeu = 'B'
			}
		case x == 101:
			if j != 'A' {
				jeu = 'A'
			}
		case x == 102: // FNC1 : ignoré
		case j == 'B' && x < 96:
			b.WriteByte(byte(32 + x))
		case j == 'A' && x < 64:
			b.WriteByte(byte(32 + x))
		default:
			return "", false // caractère de contrôle : pas un numéro de carte
		}
	}
	return b.String(), b.Len() > 0
}

// lireLargeurs : tous les codes trouvés sur une ligne (dans les deux sens).
func lireLargeurs(l []int, noir bool) [][2]string {
	var out [][2]string
	essayer := func(l []int, noir bool) {
		debut := 0
		if !noir {
			debut = 1
		}
		for i := debut; i < len(l); i += 2 { // chaque barre
			if num, f, ok := lireEAN(l, i); ok {
				out = append(out, [2]string{num, f})
			}
			if num, ok := lire128(l, i); ok {
				out = append(out, [2]string{num, "code128"})
			}
		}
	}
	essayer(l, noir)
	inverse := make([]int, len(l))
	for i, x := range l {
		inverse[len(l)-1-i] = x
	}
	essayer(inverse, noir == (len(l)%2 == 1))
	return out
}

// Lire : le code-barres de la photo. ok = faux si aucun code n'est lu.
func Lire(img image.Image) (numero, format string, ok bool) {
	g := versGris(img)
	votes := map[[2]string]int{}
	for _, vertical := range []bool{false, true} {
		max := g.h
		if vertical {
			max = g.l
		}
		for k := 1; k < 40; k++ {
			v := g.ligne(max*k/40, vertical)
			for _, local := range []bool{false, true} {
				l, noir := largeurs(v, local)
				for _, r := range lireLargeurs(l, noir) {
					votes[r]++
				}
			}
		}
	}
	best := 0
	for r, n := range votes {
		if n > best || (n == best && r[0] < numero) {
			numero, format, best = r[0], r[1], n
		}
	}
	return numero, format, best > 0
}

// pixelsMax : une photo plus grande est refusée (le navigateur la réduit
// avant l'envoi ; ceci protège d'une image « bombe »).
const pixelsMax = 4000 * 4000
