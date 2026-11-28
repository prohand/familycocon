// SPDX-License-Identifier: GPL-3.0-or-later

package photo

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"testing"
)

// Un PNG 16 bits de 4 000 × 4 000 pèse peu mais prend 122 Mio une fois
// décodé : refusé avant tout décodage.
func TestControlerPNG16Bits(t *testing.T) {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewNRGBA64(image.Rect(0, 0, 4000, 4000))); err != nil {
		t.Fatal(err)
	}
	if b.Len() > 1<<20 {
		t.Fatalf("PNG d'essai trop lourd : %d octets", b.Len())
	}
	if err := Controler(bytes.NewReader(b.Bytes()), PixelsMax); err != ErrTropGrande {
		t.Fatalf("PNG 16 bits accepté : %v", err)
	}
	if _, err := Preparer(b.Bytes()); err != ErrTropGrande {
		t.Fatalf("Preparer : %v", err)
	}
	// Le même en 8 bits gris (16 Mio) passe.
	b.Reset()
	_ = png.Encode(&b, image.NewGray(image.Rect(0, 0, 4000, 4000)))
	if err := Controler(bytes.NewReader(b.Bytes()), PixelsMax); err != nil {
		t.Fatalf("PNG gris refusé : %v", err)
	}
}

// Une photo JPEG ordinaire (4:2:0, de base) passe ; le même en-tête marqué
// « progressif » prend 5 fois plus de mémoire et est refusé.
func TestControlerJPEG(t *testing.T) {
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewYCbCr(image.Rect(0, 0, 4000, 3000), image.YCbCrSubsampleRatio420), nil); err != nil {
		t.Fatal(err)
	}
	if err := Controler(bytes.NewReader(b.Bytes()), PixelsMax); err != nil {
		t.Fatalf("photo ordinaire refusée : %v", err)
	}
	// Remplace le marqueur SOF0 (FF C0) par SOF2 (FF C2).
	d := bytes.Clone(b.Bytes())
	i := bytes.Index(d, []byte{0xFF, 0xC0})
	if i < 0 {
		t.Fatal("SOF0 introuvable")
	}
	d[i+1] = 0xC2
	if err := Controler(bytes.NewReader(d), PixelsMax); err != ErrTropGrande {
		t.Fatalf("JPEG progressif de 12 Mpx accepté : %v", err)
	}
	// Trop de pixels.
	if err := Controler(bytes.NewReader(b.Bytes()), 1000); err != ErrTropGrande {
		t.Fatalf("limite de pixels ignorée : %v", err)
	}
	// Ni JPEG ni PNG, ou coupé.
	for _, x := range [][]byte{[]byte("GIF89a......"), b.Bytes()[:20], {0xFF, 0xD8}} {
		if err := Controler(bytes.NewReader(x), PixelsMax); err != ErrPhoto {
			t.Fatalf("%q : %v", x[:min(len(x), 6)], err)
		}
	}
}

func TestDecoder(t *testing.T) {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewGray(image.Rect(0, 0, 30, 20)))
	var l int
	if err := Decoder(b.Bytes(), func(img image.Image) { l = img.Bounds().Dx() }); err != nil || l != 30 {
		t.Fatalf("Decoder : %v %d", err, l)
	}
	if err := Decoder([]byte("rien"), func(image.Image) {}); err != ErrPhoto {
		t.Fatal("fichier quelconque décodé")
	}
}
