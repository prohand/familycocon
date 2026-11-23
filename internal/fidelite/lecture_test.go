// SPDX-License-Identifier: GPL-3.0-or-later

package fidelite

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"testing"

	"familycocon/internal/essai"
)

// photo : une fausse photo du code : modules de « taille » pixels (pas
// forcément entière), marges, lumière inégale, bruit, et tournée d'un quart
// de tour si demandé.
func photo(modules string, taille float64, tourne bool, graine int64) image.Image {
	alea := rand.New(rand.NewSource(graine))
	marge := 60
	l := int(float64(len(modules))*taille) + 2*marge
	h := 160
	if tourne {
		l, h = h, l
	}
	img := image.NewGray(image.Rect(0, 0, l, h))
	for y := 0; y < h; y++ {
		for x := 0; x < l; x++ {
			pos, autre := x, y
			if tourne {
				pos, autre = y, x
			}
			v := 235.0 - float64(pos)*40/float64(max(l, h)) // lumière inégale
			k := int((float64(pos) - float64(marge)) / taille)
			if pos >= marge && k < len(modules) && modules[k] == '1' && autre > 20 && autre < 140 {
				v = 40
			}
			v += alea.Float64()*30 - 15 // bruit
			img.SetGray(x, y, color.Gray{Y: uint8(min(255, max(0, v)))})
		}
	}
	return img
}

func TestLireCodes(t *testing.T) {
	ean13, _ := EAN13("3017620422003")
	ean8, _ := EAN8("96385074")
	upca, _ := EAN13("0036000291452")
	c1, _ := Code128("ABC-1234")
	c2, _ := Code128("12345678")
	cas := []struct {
		modules, numero, format string
	}{
		{ean13, "3017620422003", "ean13"},
		{ean8, "96385074", "ean8"},
		{upca, "036000291452", "upca"},
		{largeursEnModules(c1), "ABC-1234", "code128"},
		{largeursEnModules(c2), "12345678", "code128"},
	}
	for _, c := range cas {
		for i, taille := range []float64{2, 2.7, 4.3} {
			for _, tourne := range []bool{false, true} {
				num, f, ok := Lire(photo(c.modules, taille, tourne, int64(i)))
				if !ok || num != c.numero || f != c.format {
					t.Errorf("%s (module %.1f px, tourné %v) : lu %q %q %v", c.numero, taille, tourne, num, f, ok)
				}
			}
		}
	}
	// Image sans code : rien.
	vide := image.NewGray(image.Rect(0, 0, 300, 200))
	if _, _, ok := Lire(vide); ok {
		t.Error("code lu sur une image vide")
	}
}

func TestFormatAuto(t *testing.T) {
	cas := map[string]string{"3017620422003": "ean13", "036000291452": "upca", "96385074": "ean8",
		"1234567890123": "code128", "ABC-1234": "code128", "Carte n° 12": "qr"}
	for num, attendu := range cas {
		if f := FormatAuto(num); f != attendu {
			t.Errorf("FormatAuto(%q) = %q, attendu %q", num, f, attendu)
		}
	}
}

// Route /fidelite/scanner : la photo envoyée par le navigateur est lue.
func TestRouteScanner(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	_, form := marie.Get("/fidelite/nouvelle")
	if !strings.Contains(form, "data-scanner") {
		t.Fatal("bouton Scanner absent")
	}
	m, _ := EAN13("3017620422003")
	var b bytes.Buffer
	_ = png.Encode(&b, photo(m, 3, false, 1))
	envoyer := func(corps []byte) (int, string) {
		req, _ := http.NewRequest("POST", srv.URL+"/fidelite/scanner", bytes.NewReader(corps))
		req.Header.Set("Content-Type", "image/png")
		req.Header.Set("X-Jeton", marie.Jeton)
		rep, err := marie.C.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer rep.Body.Close()
		r, _ := io.ReadAll(rep.Body)
		return rep.StatusCode, string(r)
	}
	if code, r := envoyer(b.Bytes()); code != 200 || !strings.Contains(r, `"numero":"3017620422003"`) || !strings.Contains(r, `"format":"ean13"`) {
		t.Fatalf("lecture : %d %s", code, r)
	}
	if code, _ := envoyer([]byte("pas une image")); code != 400 {
		t.Fatalf("image illisible : %d", code)
	}
	var vide bytes.Buffer
	_ = png.Encode(&vide, image.NewGray(image.Rect(0, 0, 200, 100)))
	if code, r := envoyer(vide.Bytes()); code != 422 || !strings.Contains(r, "Aucun code-barres") {
		t.Fatalf("sans code : %d %s", code, r)
	}
}
