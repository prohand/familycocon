// SPDX-License-Identifier: GPL-3.0-or-later

// Package photo : préparer une photo envoyée (ticket, facture) avant de la
// garder : vérifiée, réduite (1 600 px au plus de côté) et réécrite en JPEG.
// La réécriture retire aussi les informations cachées de l'appareil (lieu
// GPS, modèle du téléphone…). Bibliothèque standard seulement.
package photo

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png" // captures d'écran
	"sync"
)

// Limites : côté le plus long après réduction ; pixels acceptés à
// l'entrée (une image « bombe » de plusieurs gigapixels est refusée avant
// d'être décodée).
const (
	CoteMax   = 1600
	PixelsMax = 24_000_000 // ~96 Mio une fois décodée : la limite de mémoire
	qualite   = 80
)

// une : une seule photo préparée à la fois (une grande photo décodée prend
// beaucoup de mémoire ; deux envois en même temps attendent leur tour).
var une sync.Mutex

// ErrPhoto : fichier qui n'est pas une photo lisible.
var ErrPhoto = errors.New("photo illisible")

// Preparer : la photo (JPEG ou PNG) réduite et réécrite en JPEG.
func Preparer(donnees []byte) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(donnees))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > PixelsMax {
		return nil, ErrPhoto
	}
	une.Lock()
	defer une.Unlock()
	img, _, err := image.Decode(bytes.NewReader(donnees))
	if err != nil {
		return nil, ErrPhoto
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, Reduire(img, CoteMax), &jpeg.Options{Quality: qualite}); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Reduire : l'image dont le plus grand côté fait au plus cote pixels
// (moyenne des pixels de chaque case : net et sans crénelage). Plus petite :
// recopiée telle quelle (sur fond blanc pour la transparence).
func Reduire(img image.Image, cote int) *image.RGBA {
	b := img.Bounds()
	l, h := b.Dx(), b.Dy()
	nl, nh := l, h
	if l > cote || h > cote {
		if l >= h {
			nl, nh = cote, max(1, h*cote/l)
		} else {
			nl, nh = max(1, l*cote/h), cote
		}
	}
	out := image.NewRGBA(image.Rect(0, 0, nl, nh))
	for y := 0; y < nh; y++ {
		y0, y1 := y*h/nh, max((y+1)*h/nh, y*h/nh+1)
		for x := 0; x < nl; x++ {
			x0, x1 := x*l/nl, max((x+1)*l/nl, x*l/nl+1)
			var r, g, bl, n uint64
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					cr, cg, cb, ca := img.At(b.Min.X+xx, b.Min.Y+yy).RGBA()
					// Transparent → blanc.
					blanc := uint64(0xffff - ca)
					r += uint64(cr) + blanc
					g += uint64(cg) + blanc
					bl += uint64(cb) + blanc
					n++
				}
			}
			out.SetRGBA(x, y, color.RGBA{uint8(r / n >> 8), uint8(g / n >> 8), uint8(bl / n >> 8), 255})
		}
	}
	return out
}
