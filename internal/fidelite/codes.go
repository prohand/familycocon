// SPDX-License-Identifier: GPL-3.0-or-later

package fidelite

import (
	"errors"
	"fmt"
	"html/template"
	"strings"
)

// Codes-barres dessinés par le serveur, en SVG (aucune bibliothèque) :
//   - EAN-13 : 13 chiffres (la plupart des cartes de supermarché) ;
//   - UPC-A : 12 chiffres (dessiné en EAN-13 précédé d'un 0, lu pareil) ;
//   - EAN-8 : 8 chiffres ;
//   - Code 128 : tout le reste (lettres, chiffres, longueur libre).
// Le SVG est en noir sur blanc, même en mode sombre : les lecteurs de caisse
// en ont besoin.

// motifs128 : largeurs barre/espace des 107 symboles du Code 128 (le dernier
// est le symbole de fin).
var motifs128 = [...]string{
	"212222", "222122", "222221", "121223", "121322", "131222", "122213", "122312", "132212", "221213",
	"221312", "231212", "112232", "122132", "122231", "113222", "123122", "123221", "223211", "221132",
	"221231", "213212", "223112", "312131", "311222", "321122", "321221", "312212", "322112", "322211",
	"212123", "212321", "232121", "111323", "131123", "131321", "112313", "132113", "132311", "211313",
	"231113", "231311", "112133", "112331", "132131", "113123", "113321", "133121", "313121", "211331",
	"231131", "213113", "213311", "213131", "311123", "311321", "331121", "312113", "312311", "332111",
	"314111", "221411", "431111", "111224", "111422", "121124", "121421", "141122", "141221", "112214",
	"112412", "122114", "122411", "142112", "142211", "241211", "221114", "413111", "241112", "134111",
	"111242", "121142", "121241", "114212", "124112", "124211", "411212", "421112", "421211", "212141",
	"214121", "412121", "111143", "111341", "131141", "114113", "114311", "411113", "411311", "113141",
	"114131", "311141", "411131", "211412", "211214", "211232", "2331112",
}

const (
	debutB = 104
	debutC = 105
	fin128 = 106
)

// ErrCode : contenu impossible à coder.
var ErrCode = errors.New("numéro impossible à mettre en code-barres")

// Code128 : les largeurs (barre, espace, barre…) du code, en modules.
// Uniquement des chiffres en nombre pair : jeu C (deux chiffres par
// symbole, code plus court) ; sinon jeu B (caractères ASCII imprimables).
func Code128(texte string) ([]int, error) {
	if texte == "" || len(texte) > 80 {
		return nil, ErrCode
	}
	var valeurs []int
	chiffres := len(texte)%2 == 0
	for _, r := range texte {
		if r < '0' || r > '9' {
			chiffres = false
		}
		if r < 32 || r > 126 {
			return nil, ErrCode
		}
	}
	if chiffres {
		valeurs = append(valeurs, debutC)
		for i := 0; i < len(texte); i += 2 {
			valeurs = append(valeurs, int(texte[i]-'0')*10+int(texte[i+1]-'0'))
		}
	} else {
		valeurs = append(valeurs, debutB)
		for _, r := range texte {
			valeurs = append(valeurs, int(r)-32)
		}
	}
	// Clé de contrôle : début + somme(position × valeur), modulo 103.
	somme := valeurs[0]
	for i, v := range valeurs[1:] {
		somme += (i + 1) * v
	}
	valeurs = append(valeurs, somme%103, fin128)
	var largeurs []int
	for _, v := range valeurs {
		for _, c := range motifs128[v] {
			largeurs = append(largeurs, int(c-'0'))
		}
	}
	return largeurs, nil
}

var (
	eanL = [10]string{"0001101", "0011001", "0010011", "0111101", "0100011", "0110001", "0101111", "0111011", "0110111", "0001011"}
	eanG = [10]string{"0100111", "0110011", "0011011", "0100001", "0011101", "0111001", "0000101", "0010001", "0001001", "0010111"}
	eanR = [10]string{"1110010", "1100110", "1101100", "1000010", "1011100", "1001110", "1010000", "1000100", "1001000", "1110100"}
	// Parité des 6 premiers chiffres codés, selon le premier chiffre.
	eanParite = [10]string{"LLLLLL", "LLGLGG", "LLGGLG", "LLGGGL", "LGLLGG", "LGGLLG", "LGGGLL", "LGLGLG", "LGLGGL", "LGGLGL"}
)

// CleEAN : chiffre de contrôle des 12 premiers chiffres d'un EAN-13.
func CleEAN(douze string) int {
	somme := 0
	for i, c := range douze[:12] {
		d := int(c - '0')
		if i%2 == 1 {
			d *= 3
		}
		somme += d
	}
	return (10 - somme%10) % 10
}

// EstEAN13 : 13 chiffres avec une clé de contrôle juste.
func EstEAN13(s string) bool {
	if len(s) != 13 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return CleEAN(s) == int(s[12]-'0')
}

// chiffres : que des chiffres, n en tout.
func chiffres(s string, n int) bool {
	if len(s) != n {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// EstUPCA : 12 chiffres avec une clé juste (un EAN-13 qui commence par 0).
func EstUPCA(s string) bool { return chiffres(s, 12) && EstEAN13("0"+s) }

// EstEAN8 : 8 chiffres avec une clé juste (poids 3, 1, 3… de la gauche).
func EstEAN8(s string) bool {
	if !chiffres(s, 8) {
		return false
	}
	somme := 0
	for i := 0; i < 7; i++ {
		d := int(s[i] - '0')
		if i%2 == 0 {
			d *= 3
		}
		somme += d
	}
	return (10-somme%10)%10 == int(s[7]-'0')
}

// EAN8 : la suite de modules d'un EAN-8 (4 chiffres L, 4 chiffres R).
func EAN8(s string) (string, error) {
	if !EstEAN8(s) {
		return "", ErrCode
	}
	var b strings.Builder
	b.WriteString("101")
	for i := 0; i < 4; i++ {
		b.WriteString(eanL[s[i]-'0'])
	}
	b.WriteString("01010")
	for i := 4; i < 8; i++ {
		b.WriteString(eanR[s[i]-'0'])
	}
	b.WriteString("101")
	return b.String(), nil
}

// EAN13 : la suite de modules (1 = barre, 0 = espace).
func EAN13(s string) (string, error) {
	if !EstEAN13(s) {
		return "", ErrCode
	}
	var b strings.Builder
	b.WriteString("101")
	parite := eanParite[s[0]-'0']
	for i := 1; i <= 6; i++ {
		d := s[i] - '0'
		if parite[i-1] == 'L' {
			b.WriteString(eanL[d])
		} else {
			b.WriteString(eanG[d])
		}
	}
	b.WriteString("01010")
	for i := 7; i <= 12; i++ {
		b.WriteString(eanR[s[i]-'0'])
	}
	b.WriteString("101")
	return b.String(), nil
}

// svgBarres dessine des barres à partir d'une suite de modules (« 1 » =
// barre), avec une marge blanche de 10 modules de chaque côté.
func svgBarres(modules, legende string) template.HTML {
	const marge, haut = 10, 70
	l := len(modules) + 2*marge
	var s strings.Builder
	fmt.Fprintf(&s, `<svg class="code-barres" viewBox="0 0 %d %d" role="img" aria-label="Code-barres %s">`, l, haut+14, template.HTMLEscapeString(legende))
	fmt.Fprintf(&s, `<rect class="fond-code" width="%d" height="%d"/>`, l, haut+14)
	for i := 0; i < len(modules); {
		if modules[i] != '1' {
			i++
			continue
		}
		j := i
		for j < len(modules) && modules[j] == '1' {
			j++
		}
		fmt.Fprintf(&s, `<rect class="barre" x="%d" y="4" width="%d" height="%d"/>`, marge+i, j-i, haut-4)
		i = j
	}
	fmt.Fprintf(&s, `<text class="legende-code" x="%d" y="%d" text-anchor="middle">%s</text></svg>`, l/2, haut+11, template.HTMLEscapeString(legende))
	return template.HTML(s.String()) // #nosec G203 -- construit ici, texte échappé
}

// largeursEnModules : 2,1,2… (barre, espace…) → « 1101… ».
func largeursEnModules(largeurs []int) string {
	var b strings.Builder
	for i, w := range largeurs {
		c := "1"
		if i%2 == 1 {
			c = "0"
		}
		b.WriteString(strings.Repeat(c, w))
	}
	return b.String()
}

// SVG : le code-barres du numéro dans le format demandé (ean13, code128).
func SVG(numero, format string) (template.HTML, error) {
	switch format {
	case "ean13":
		m, err := EAN13(numero)
		if err != nil {
			return "", err
		}
		return svgBarres(m, numero), nil
	case "upca":
		if !EstUPCA(numero) {
			return "", ErrCode
		}
		m, err := EAN13("0" + numero)
		if err != nil {
			return "", err
		}
		return svgBarres(m, numero), nil
	case "ean8":
		m, err := EAN8(numero)
		if err != nil {
			return "", err
		}
		return svgBarres(m, numero), nil
	case "code128":
		l, err := Code128(numero)
		if err != nil {
			return "", err
		}
		return svgBarres(largeursEnModules(l), numero), nil
	}
	return "", ErrCode
}

// FormatAuto : le type de code d'après le numéro : EAN-13, UPC-A ou EAN-8
// (chiffres avec une clé de contrôle juste), sinon Code 128 (texte court),
// sinon QR code (long, ou caractères hors ASCII).
func FormatAuto(numero string) string {
	switch {
	case EstEAN13(numero):
		return "ean13"
	case EstUPCA(numero):
		return "upca"
	case EstEAN8(numero):
		return "ean8"
	}
	if _, err := Code128(numero); err == nil {
		return "code128"
	}
	return "qr"
}

// NomFormat : « EAN-13 », « QR code »…
func NomFormat(f string) string {
	for _, x := range Formats {
		if x.Cle == f {
			return x.Nom
		}
	}
	return f
}
