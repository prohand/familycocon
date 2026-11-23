// SPDX-License-Identifier: GPL-3.0-or-later

// Package photo : préparer une photo envoyée (ticket, facture) avant de la
// garder : vérifiée, réduite (1 600 px au plus de côté) et réécrite en JPEG.
// La réécriture retire aussi les informations cachées de l'appareil (lieu
// GPS, modèle du téléphone…). Bibliothèque standard seulement.
package photo

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	_ "image/png" // captures d'écran
	"io"
	"os"
	"path/filepath"
	"strconv"
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
	return PreparerFlux(func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(donnees)), nil })
}

// fluxMax : taille lue au plus pour une photo (fichier compressé).
const fluxMax = 64 << 20

// PreparerFlux : comme Preparer, mais la photo est lue par morceaux (dans
// un ZIP, par exemple) au lieu d'être d'abord gardée en mémoire. ouvrir est
// appelée deux fois : une pour la taille, une pour l'image.
func PreparerFlux(ouvrir func() (io.ReadCloser, error)) ([]byte, error) {
	r, err := ouvrir()
	if err != nil {
		return nil, err
	}
	cfg, _, err := image.DecodeConfig(bufio.NewReader(io.LimitReader(r, fluxMax)))
	_ = r.Close()
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > PixelsMax {
		return nil, ErrPhoto
	}
	une.Lock()
	defer une.Unlock()
	r, err = ouvrir()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	img, _, err := image.Decode(bufio.NewReader(io.LimitReader(r, fluxMax)))
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

// Rangement des photos gardées (tickets du carnet, photos des notes) :
// <racine>/<famille>/<nom aléatoire>.jpg. La racine est le dossier
// « pieces » des données ; la base ne garde que le nom.

// Ranger : écrit la photo (déjà préparée) sous un nom aléatoire et rend ce
// nom. Écrite sous un nom provisoire puis renommée : jamais de fichier coupé.
func Ranger(racine string, famille int64, jpeg []byte) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	nom := hex.EncodeToString(b) + ".jpg"
	dossier := filepath.Join(racine, strconv.FormatInt(famille, 10))
	if err := os.MkdirAll(dossier, 0o750); err != nil {
		return "", err
	}
	chemin := filepath.Join(dossier, nom)
	if err := os.WriteFile(chemin+".tmp", jpeg, 0o640); err != nil {
		return "", err
	}
	if err := os.Rename(chemin+".tmp", chemin); err != nil {
		_ = os.Remove(chemin + ".tmp")
		return "", err
	}
	return nom, nil
}

// Chemin : le fichier d'une photo. Le nom vient de la base (créé par
// Ranger) : filepath.Base empêche toute sortie du dossier.
func Chemin(racine string, famille int64, nom string) string {
	return filepath.Join(racine, strconv.FormatInt(famille, 10), filepath.Base(nom))
}

// Effacer : supprime des photos ; rend la première erreur (hors fichier
// déjà absent).
func Effacer(racine string, famille int64, noms ...string) error {
	var premiere error
	for _, n := range noms {
		if err := os.Remove(Chemin(racine, famille, n)); err != nil && !errors.Is(err, os.ErrNotExist) && premiere == nil {
			premiere = err
		}
	}
	return premiere
}
