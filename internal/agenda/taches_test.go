// SPDX-License-Identifier: GPL-3.0-or-later

package agenda

import (
	"strings"
	"testing"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/essai"
	"familycocon/internal/taches"
)

// Les tâches quotidiennes apparaissent dans l'agenda, en petit, avec un filtre
// « Toutes / Les miennes / Masquées » gardé par le navigateur.
func TestTachesDansAgenda(t *testing.T) {
	_, srv := essai.NouvelleApp(t, func(a *appli.App) {
		Brancher(a)
		taches.Brancher(a)
	})
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	_, page := marie.Get("/taches/nouvelle")
	idLea := essai.Trouver(t, page, `<option value="(\d+)">Léa</option>`)
	marie.Post("/taches", map[string]string{"titre": "Sortir les poubelles", "repetition": "jour"})
	marie.Post("/taches", map[string]string{"titre": "Ranger sa chambre", "repetition": "jour", "assigne": idLea})
	marie.Post("/taches", map[string]string{"titre": "Ranger le garage", "repetition": "unique"})

	_, page = lea.Get("/agenda")
	if !strings.Contains(page, `class="elements-jour"`) || !strings.Contains(page, "Sortir les poubelles") || !strings.Contains(page, "Ranger sa chambre") {
		t.Fatalf("tâches absentes de l'agenda :\n%s", page)
	}
	if strings.Contains(page, "Ranger le garage") {
		t.Fatal("tâche sans jour montrée dans l'agenda")
	}
	// Pas avant sa création : la semaine dernière, rien.
	hier := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	if _, page := lea.Get("/agenda?date=" + hier); strings.Contains(page, "Sortir les poubelles") {
		t.Fatal("tâche montrée avant sa création")
	}
	// Marie : « Les miennes » (les tâches de tout le monde restent, pas celles de Léa).
	if _, page := marie.Get("/agenda?taches=moi"); !strings.Contains(page, "Sortir les poubelles") || strings.Contains(page, "Ranger sa chambre") {
		t.Fatal("filtre « les miennes »")
	}
	// Le choix est retenu.
	if _, page := marie.Get("/agenda?vue=mois"); strings.Contains(page, "Ranger sa chambre") {
		t.Fatal("filtre non retenu")
	}
	if _, page := marie.Get("/agenda?taches=non"); strings.Contains(page, "elements-jour") {
		t.Fatal("tâches non masquées")
	}
}

// Tâches avec rappel : « une fois » à sa date ; chaque semaine le lundi ;
// l'heure du rappel suit le titre.
func TestRappelsTachesDansAgenda(t *testing.T) {
	_, srv := essai.NouvelleApp(t, func(a *appli.App) {
		Brancher(a)
		taches.Brancher(a)
	})
	marie := essai.Famille(t, srv)
	dans10 := time.Now().AddDate(0, 0, 10)
	marie.Post("/taches", map[string]string{"titre": "Tondre", "repetition": "unique", "rappel": "10:00", "date_rappel": dans10.Format("2006-01-02")})
	marie.Post("/taches", map[string]string{"titre": "Draps", "repetition": "semaine", "rappel": "09:00"})
	marie.Post("/taches", map[string]string{"titre": "Vitres", "repetition": "semaine"}) // sans rappel : pas de jour
	_, page := marie.Get("/agenda?vue=semaine&date=" + dans10.Format("2006-01-02"))
	if !strings.Contains(page, "Tondre · ⏰ 10:00") || !strings.Contains(page, "Draps · ⏰ 09:00") || strings.Contains(page, "Vitres") {
		t.Fatalf("rappels des tâches dans l'agenda :\n%s", page)
	}
	// La tâche de la semaine n'y est qu'une fois (le lundi).
	if n := strings.Count(page, "Draps · ⏰ 09:00"); n != 1 {
		t.Fatalf("tâche de la semaine montrée %d fois", n)
	}
}
