// SPDX-License-Identifier: GPL-3.0-or-later

package photo

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestPreparer(t *testing.T) {
	grande := image.NewRGBA(image.Rect(0, 0, 4000, 3000))
	for y := 0; y < 3000; y++ {
		for x := 0; x < 4000; x++ {
			grande.Set(x, y, color.RGBA{uint8(x), uint8(y), 100, 255})
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, grande)
	out, err := Preparer(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(out))
	if err != nil || cfg.Width != 1600 || cfg.Height != 1200 {
		t.Fatalf("réduction : %+v %v", cfg, err)
	}
	if _, err := Preparer([]byte("pas une image")); err != ErrPhoto {
		t.Fatal("fichier quelconque accepté")
	}
	petite := image.NewGray(image.Rect(0, 0, 300, 200))
	b.Reset()
	_ = png.Encode(&b, petite)
	if out, err := Preparer(b.Bytes()); err != nil {
		t.Fatal(err)
	} else if cfg, _ := jpeg.DecodeConfig(bytes.NewReader(out)); cfg.Width != 300 {
		t.Fatal("petite image agrandie ou rognée")
	}
}
