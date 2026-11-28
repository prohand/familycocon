// SPDX-License-Identifier: GPL-3.0-or-later

package photo

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"io"
)

// MemoireMax : mémoire que peut prendre au plus une image une fois décodée.
// Le conteneur a 128 Mo : compter le nombre de pixels ne suffit pas, car un
// petit PNG 16 bits (8 octets par pixel) ou un JPEG « progressif » (5 octets
// par point de chaque couleur) prend bien plus qu'une photo ordinaire.
// Exemple : un PNG 16 bits de 4 000 × 4 000 pèse 120 Kio, mais 122 Mio une
// fois décodé : il est refusé.
const MemoireMax = 64 << 20

// ErrTropGrande : image qui prendrait trop de mémoire une fois décodée.
var ErrTropGrande = errors.New("photo trop grande")

// Controler lit le début de l'image (JPEG ou PNG) et refuse, avant tout
// décodage, une image trop grande en pixels ou en mémoire.
func Controler(r io.Reader, pixelsMax int) error {
	b := bufio.NewReader(r)
	debut, err := b.Peek(8)
	if err != nil {
		return ErrPhoto
	}
	var largeur, hauteur, memoire int64
	switch {
	case string(debut) == "\x89PNG\r\n\x1a\n":
		largeur, hauteur, memoire, err = memoirePNG(b)
	case debut[0] == 0xFF && debut[1] == 0xD8:
		largeur, hauteur, memoire, err = memoireJPEG(b)
	default:
		return ErrPhoto // ni JPEG ni PNG
	}
	if err != nil || largeur <= 0 || hauteur <= 0 {
		return ErrPhoto
	}
	if largeur*hauteur > int64(pixelsMax) || memoire > MemoireMax {
		return ErrTropGrande
	}
	return nil
}

// memoirePNG : taille et mémoire prise une fois décodé (selon le type de
// pixels que donne le décodeur de Go), d'après l'en-tête IHDR.
func memoirePNG(b *bufio.Reader) (largeur, hauteur, memoire int64, err error) {
	// signature (8) + longueur (4) + « IHDR » (4) + 13 octets de données
	h, err := b.Peek(29)
	if err != nil || string(h[12:16]) != "IHDR" {
		return 0, 0, 0, ErrPhoto
	}
	largeur = int64(binary.BigEndian.Uint32(h[16:20]))
	hauteur = int64(binary.BigEndian.Uint32(h[20:24]))
	profondeur, typeCouleur, entrelace := h[24], h[25], h[28]
	var octets int64
	switch {
	case typeCouleur == 3 || (typeCouleur == 0 && profondeur <= 8):
		octets = 1 // palette, ou gris
	case typeCouleur == 0:
		octets = 2 // gris 16 bits
	case profondeur == 16:
		octets = 8 // couleur 16 bits (RGBA64, NRGBA64)
	default:
		octets = 4 // couleur 8 bits (RGBA, NRGBA)
	}
	memoire = largeur * hauteur * octets
	if entrelace != 0 {
		memoire *= 2 // image de chaque passe, plus l'image finale
	}
	return largeur, hauteur, memoire, nil
}

// lectureMaxJPEG : l'en-tête d'un JPEG (avec les données de l'appareil,
// la petite image d'aperçu…) se lit dans ce nombre d'octets.
const lectureMaxJPEG = 4 << 20

// memoireJPEG : taille et mémoire prise une fois décodé, d'après le
// marqueur SOF (découpage des couleurs, JPEG « progressif » ou non).
func memoireJPEG(b *bufio.Reader) (largeur, hauteur, memoire int64, err error) {
	if _, err := b.Discard(2); err != nil { // FF D8
		return 0, 0, 0, ErrPhoto
	}
	lu := 2
	for lu < lectureMaxJPEG {
		// Marqueur : FF, puis son type (les FF en trop sont permis).
		m, err := b.ReadByte()
		if err != nil || m != 0xFF {
			return 0, 0, 0, ErrPhoto
		}
		t, err := b.ReadByte()
		for err == nil && t == 0xFF {
			t, err = b.ReadByte()
		}
		if err != nil {
			return 0, 0, 0, ErrPhoto
		}
		lu += 2
		if t == 0xD8 || t == 0x01 || (t >= 0xD0 && t <= 0xD7) {
			continue // marqueurs sans données
		}
		var l [2]byte
		if _, err := io.ReadFull(b, l[:]); err != nil {
			return 0, 0, 0, ErrPhoto
		}
		n := int(binary.BigEndian.Uint16(l[:])) - 2
		if n < 0 {
			return 0, 0, 0, ErrPhoto
		}
		// SOF0 (de base), SOF1 (étendu), SOF2 (progressif) : les seuls lus par Go.
		if t == 0xC0 || t == 0xC1 || t == 0xC2 {
			return lireSOF(b, n, t == 0xC2)
		}
		if t == 0xD9 || t == 0xDA { // fin, ou début des données sans SOF
			return 0, 0, 0, ErrPhoto
		}
		if _, err := b.Discard(n); err != nil {
			return 0, 0, 0, ErrPhoto
		}
		lu += 2 + n
	}
	return 0, 0, 0, ErrPhoto
}

// lireSOF : le marqueur SOF (n octets) : précision, hauteur, largeur, puis
// pour chaque couleur son découpage (facteurs horizontal et vertical).
func lireSOF(b *bufio.Reader, n int, progressif bool) (largeur, hauteur, memoire int64, err error) {
	if n < 6 || n > 6+3*4 {
		return 0, 0, 0, ErrPhoto
	}
	d := make([]byte, n)
	if _, err := io.ReadFull(b, d); err != nil {
		return 0, 0, 0, ErrPhoto
	}
	hauteur = int64(binary.BigEndian.Uint16(d[1:3]))
	largeur = int64(binary.BigEndian.Uint16(d[3:5]))
	nb := int(d[5])
	if nb != 1 && nb != 3 && nb != 4 || n != 6+3*nb {
		return 0, 0, 0, ErrPhoto
	}
	hmax, vmax := int64(1), int64(1)
	fh, fv := make([]int64, nb), make([]int64, nb)
	for i := range nb {
		fh[i], fv[i] = int64(d[7+3*i]>>4), int64(d[7+3*i]&0x0F)
		if fh[i] < 1 || fv[i] < 1 || fh[i] > 4 || fv[i] > 4 {
			return 0, 0, 0, ErrPhoto
		}
		hmax, vmax = max(hmax, fh[i]), max(vmax, fv[i])
	}
	// Un plan par couleur, à sa résolution (chaque côté arrondi au bloc de 8).
	var plans int64
	for i := range nb {
		l := (largeur*fh[i]/hmax + 15) / 8 * 8
		h := (hauteur*fv[i]/vmax + 15) / 8 * 8
		plans += l * h
	}
	memoire = plans
	if progressif {
		memoire += 4 * plans // coefficients gardés pendant le décodage (int32)
	}
	// Quatre couleurs (CMJN) ou découpage inhabituel : l'image est recopiée
	// en 4 octets par pixel à la fin du décodage.
	if nb == 4 || (nb == 3 && (fh[1] != 1 || fv[1] != 1 || fh[2] != 1 || fv[2] != 1)) {
		memoire += 4 * largeur * hauteur
	}
	return largeur, hauteur, memoire, nil
}

// Decoder décode l'image (déjà passée par Controler) et la donne à f, en
// gardant le verrou « une » jusqu'au retour de f : jamais deux grandes
// images en mémoire en même temps.
func Decoder(donnees []byte, f func(image.Image)) error {
	une.Lock()
	defer une.Unlock()
	img, _, err := image.Decode(bytes.NewReader(donnees))
	if err != nil {
		return ErrPhoto
	}
	f(img)
	return nil
}
