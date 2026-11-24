// SPDX-License-Identifier: GPL-3.0-or-later

package fidelite

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"familycocon/internal/essai"
)

func TestTrouverEnseigne(t *testing.T) {
	for nom, cle := range map[string]string{
		"Carrefour":              "carrefour",
		"Carte Carrefour Market": "carrefour",
		"E.Leclerc":              "leclerc",
		"Intermarché":            "intermarche",
		"Super U":                "u",
		"Hyper U Lyon":           "u",
		"H&M":                    "hm",
		"Décathlon":              "decathlon",
		"Leroy Merlin":           "leroymerlin",
		"BP":                     "bp",
		"Pharmacie du centre":    "pharmacie",
		"Boulangerie Paul":       "",
		"Décoration Cora":        "cora", // « cora » en mot entier
		"Décoration":             "",     // pas « cora » au milieu d'un mot
		"Club U":                 "",
		"":                       "",
		"C&A":                    "ca",
		"Ça c'est Paris":         "",
		"Carte Action Logement":  "action",
		"Station BP":             "",
	} {
		got := Marque(nom, 1)
		if got != cle {
			t.Errorf("Marque(%q) = %q, attendu %q", nom, got, cle)
		}
	}
	if Marque("Carrefour", 0) != "" {
		t.Error("couleur de l'enseigne alors que la carte garde la sienne")
	}
	// La feuille de style contient chaque enseigne.
	rec := httptest.NewRecorder()
	feuilleEnseignes(rec, httptest.NewRequest(http.MethodGet, "/fidelite/enseignes.css", nil))
	css := rec.Body.String()
	if !strings.Contains(css, ".marque-carrefour{--c:#004E9F;--c2:#E30613;--t:#fff}") || !strings.Contains(css, ".marque-fnac{--c:#E1A925;--c2:#1D1D1B;--t:#111}") {
		t.Fatalf("feuille des enseignes :\n%s", css)
	}
}

// Lecteur de codes du navigateur (zxing-wasm) : servi avec WebAssembly
// permis (sa propre politique), mis en cache ; pas les .gz bruts.
func TestLecteurZxingServi(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	for _, c := range []struct {
		chemin string
		code   int
	}{{"/ocr/zxing316/lecteur.js", 200}, {"/ocr/zxing316/index.js", 200}, {"/ocr/zxing316/zxing_reader.wasm", 200},
		{"/ocr/zxing316/index.js.gz", 404}} {
		rep, err := http.Get(srv.URL + c.chemin)
		if err != nil {
			t.Fatal(err)
		}
		rep.Body.Close()
		if rep.StatusCode != c.code {
			t.Errorf("%s : %d, attendu %d", c.chemin, rep.StatusCode, c.code)
		}
		if c.code == 200 && !strings.Contains(rep.Header.Get("Content-Security-Policy"), "'wasm-unsafe-eval'") {
			t.Errorf("%s : WebAssembly non permis", c.chemin)
		}
	}
}
