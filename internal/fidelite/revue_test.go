// SPDX-License-Identifier: GPL-3.0-or-later

package fidelite

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"familycocon/internal/donnees"
	"familycocon/internal/essai"
)

// Revue du 23/11/2026.
func TestRevueCartes(t *testing.T) {
	for numero, attendu := range map[string]string{"123456789": "6789", "abc": "abc", "carte-éèàù": "éèàù", "": ""} {
		if f := finNumero(numero); f != attendu {
			t.Errorf("finNumero(%q) = %q", numero, f)
		}
	}
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	ctx := context.Background()
	marie.Get("/fidelite/nouvelle")
	marie.Post("/fidelite", map[string]string{"enseigne": "Librairie", "numero": "Carte de Zoé éèàù", "format": "aucun"})
	// La liste montre la fin du numéro sans couper un caractère.
	if _, page := marie.Get("/fidelite"); !strings.Contains(page, "…éèàù") || strings.Contains(page, "�") {
		t.Fatalf("fin du numéro :\n%s", page)
	}
	// « Carte utilisée » : seulement pour une carte de la famille.
	if err := a.Q.CarteUtilisee(ctx, donnees.CarteUtiliseeParams{UtiliseeLe: 99, ID: 1, FamilleID: 2}); err != nil {
		t.Fatal(err)
	}
	if ct, _ := a.Q.CarteParID(ctx, donnees.CarteParIDParams{ID: 1, FamilleID: 1}); ct.UtiliseeLe == 99 {
		t.Fatal("carte marquée utilisée par une autre famille")
	}
	// Plafond de cartes.
	n, _ := a.Q.CompterCartes(ctx, 1)
	for i := n; i < CartesMax; i++ {
		if _, err := a.Q.CreerCarte(ctx, donnees.CreerCarteParams{FamilleID: 1, Enseigne: "x", Numero: "1", Format: "aucun", Couleur: 1, CreeLe: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if code, _ := marie.Post("/fidelite", map[string]string{"enseigne": "De trop", "numero": "1", "format": "aucun"}); code != http.StatusBadRequest {
		t.Fatalf("plafond de cartes : %d", code)
	}
}
