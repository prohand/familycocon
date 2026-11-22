// SPDX-License-Identifier: GPL-3.0-or-later

package fidelite

import (
	"net/http"
	"strings"
	"testing"

	"familycocon/internal/essai"
)

func TestCartes(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	marie.Get("/fidelite/nouvelle")
	// EAN-13 tapé avec des espaces : reconnu tout seul.
	code, page := marie.Post("/fidelite", map[string]string{"enseigne": "Supermarché", "numero": "3017 6204 2200 3", "format": "auto", "couleur": "3"})
	if code != 200 || !strings.Contains(page, `class="code-barres"`) || !strings.Contains(page, "3017620422003") {
		t.Fatalf("carte EAN : %d", code)
	}
	// Numéro avec lettres en EAN-13 : refusé.
	if code, _ := marie.Post("/fidelite", map[string]string{"enseigne": "X", "numero": "AB12", "format": "ean13"}); code != 400 {
		t.Fatalf("EAN impossible accepté : %d", code)
	}
	// QR code.
	if _, page := marie.Post("/fidelite", map[string]string{"enseigne": "Pharmacie", "numero": "PH-778899", "format": "qr"}); !strings.Contains(page, "data:image/png;base64,") {
		t.Fatal("QR code absent")
	}
	// Liste : les 4 derniers chiffres seulement.
	_, page = lea.Get("/fidelite")
	if !strings.Contains(page, "Supermarché") || !strings.Contains(page, "…2003") {
		t.Fatalf("liste :\n%s", page)
	}
	id := essai.Trouver(t, page, `href="/fidelite/(\d+)"`)
	if code, _ := lea.Post("/fidelite/"+id+"/supprimer", nil); code != http.StatusForbidden {
		t.Fatalf("suppression par un enfant : %d", code)
	}
}
