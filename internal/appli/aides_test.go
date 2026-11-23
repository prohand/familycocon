// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"testing"
	"time"

	"familycocon/internal/donnees"
)

func TestAidesCommunes(t *testing.T) {
	membres := []donnees.Membre{{ID: 3, Nom: "Marie"}, {ID: 7, Nom: "Léa"}}
	parID := MembresParID(membres)
	if parID[7].Nom != "Léa" || parID[9] != nil {
		t.Fatal("MembresParID")
	}
	if got := TitreJour(time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC)); got != "Lundi 16" {
		t.Fatalf("TitreJour : %q", got)
	}
	parent := &Contexte{Membre: donnees.Membre{ID: 1, Role: RoleParent}}
	enfant := &Contexte{Membre: donnees.Membre{ID: 2, Role: RoleEnfant}}
	if !PeutModifier(parent, 9, true) || !PeutModifier(enfant, 2, true) || PeutModifier(enfant, 9, true) || PeutModifier(enfant, 2, false) {
		t.Fatal("PeutModifier")
	}
}

// Une requête (un Contexte) : un seul calcul ; une autre requête : recalcul.
func TestMemoRequete(t *testing.T) {
	var memo MemoRequete[int]
	appels := 0
	calcul := func() (int, error) { appels++; return appels, nil }
	c1, c2 := &Contexte{}, &Contexte{}
	if v, _ := memo.Lire(c1, calcul); v != 1 {
		t.Fatal("premier calcul")
	}
	if v, _ := memo.Lire(c1, calcul); v != 1 || appels != 1 {
		t.Fatal("même requête : recalculé")
	}
	if v, _ := memo.Lire(c2, calcul); v != 2 {
		t.Fatal("autre requête : pas recalculé")
	}
	if _, _ = memo.Lire(nil, calcul); appels != 3 {
		t.Fatal("sans contexte : toujours calculé")
	}
	if _, _ = memo.Lire(nil, calcul); appels != 4 {
		t.Fatal("sans contexte : toujours calculé")
	}
}
