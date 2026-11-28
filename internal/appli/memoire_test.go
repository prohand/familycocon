// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"strconv"
	"testing"
	"time"
)

// Un seul membre (ou une seule adresse) ne remplit pas la mémoire courte ;
// pleine, elle oublie l'entrée la plus ancienne au lieu de tout refuser.
func TestMemoireCourte(t *testing.T) {
	m := NouvelleMemoireCourte(10, 3)
	// Un compte qui se connecte en boucle : seules ses 3 dernières attentes restent.
	for i := range 100 {
		m.Mettre("pirate", "p"+strconv.Itoa(i), i, time.Minute)
	}
	if len(m.entrees) != 3 {
		t.Fatalf("un groupe garde %d entrées", len(m.entrees))
	}
	if _, ok := m.Lire("p99"); !ok {
		t.Fatal("dernière entrée du groupe oubliée")
	}
	// Les autres membres passent toujours.
	m.Mettre("marie", "marie", 1, time.Minute)
	if v, ok := m.Lire("marie"); !ok || v.(int) != 1 {
		t.Fatal("entrée d'un autre membre refusée")
	}
	// Pleine (beaucoup de groupes) : la plus ancienne part, la nouvelle entre.
	for i := range 20 {
		m.Mettre("g"+strconv.Itoa(i), "g"+strconv.Itoa(i), i, time.Duration(i+2)*time.Minute)
	}
	if len(m.entrees) != 10 {
		t.Fatalf("mémoire de %d entrées (10 au plus)", len(m.entrees))
	}
	if _, ok := m.Lire("g19"); !ok {
		t.Fatal("nouvelle entrée refusée quand la mémoire est pleine")
	}
	// Remplacer une clé existante ne compte pas comme une entrée de plus.
	m.Mettre("g19", "g19", 42, time.Hour)
	if v, _ := m.Lire("g19"); v.(int) != 42 || len(m.entrees) != 10 {
		t.Fatal("remplacement d'une entrée")
	}
	// Expirée : oubliée.
	m.Mettre("x", "court", 1, -time.Second)
	if _, ok := m.Lire("court"); ok {
		t.Fatal("entrée expirée lue")
	}
}
