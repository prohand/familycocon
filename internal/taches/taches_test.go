// SPDX-License-Identifier: GPL-3.0-or-later

package taches

import (
	"net/http"
	"strings"
	"testing"

	"familycocon/internal/essai"
)

func TestTachesDuJour(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	marie.Get("/taches/nouvelle")
	if code, page := marie.Post("/taches", map[string]string{"titre": "Sortir les poubelles", "repetition": "jour"}); code != 200 || !strings.Contains(page, "Sortir les poubelles") || !strings.Contains(page, "🗑️") {
		t.Fatalf("création : %d", code)
	}
	if code, _ := marie.Post("/taches", map[string]string{"titre": "Arroser", "repetition": "jours"}); code != 400 {
		t.Fatalf("« certains jours » sans jour accepté : %d", code)
	}
	if code, _ := marie.Post("/taches", map[string]string{"titre": "", "repetition": "jour"}); code != 400 {
		t.Fatalf("titre vide accepté : %d", code)
	}
	marie.Post("/taches", map[string]string{"titre": "Ranger le garage", "repetition": "unique"})

	// Accueil : bloc « Tâches du jour » et résumé.
	_, page := marie.Get("/")
	if !strings.Contains(page, "Tâches du jour") || !strings.Contains(page, "2 à faire") {
		t.Fatalf("accueil sans les tâches :\n%s", page)
	}

	_, page = marie.Get("/taches")
	id := essai.Trouver(t, page, `/taches/(\d+)/faire"[^>]*Sortir les poubelles`)
	marie.Htmx = true
	code, frag := marie.Post("/taches/"+id+"/faire", map[string]string{"pour": "0"})
	if code != 200 || !strings.Contains(frag, "par Marie") || !strings.Contains(frag, "Qui a fait quoi") {
		t.Fatalf("faire : %d\n%s", code, frag)
	}
	// Deux fois : pas de doublon dans l'historique.
	_, frag = marie.Post("/taches/"+id+"/faire", map[string]string{"pour": "0"})
	if strings.Count(frag, "Sortir les poubelles</span>") != 1 {
		t.Fatalf("doublon dans l'historique :\n%s", frag)
	}
	if !strings.Contains(frag, "🏆 Cette semaine") {
		t.Fatal("scores absents")
	}
	if _, frag = marie.Post("/taches/"+id+"/defaire", map[string]string{"pour": "0"}); strings.Contains(frag, "par Marie") {
		t.Fatal("décochage sans effet")
	}
}

func TestDroitsEnfant(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	marie.Get("/taches/nouvelle")
	marie.Post("/taches", map[string]string{"titre": "Mettre la table", "repetition": "jour"})
	_, page := marie.Get("/taches")
	id := essai.Trouver(t, page, `/taches/(\d+)/faire`)

	// Marie coche ; Léa ne peut pas décocher, ni supprimer, ni modifier.
	marie.Htmx = true
	marie.Post("/taches/"+id+"/faire", map[string]string{"pour": "0"})
	lea.Get("/taches")
	lea.Htmx = true
	if code, _ := lea.Post("/taches/"+id+"/defaire", nil); code != http.StatusForbidden {
		t.Fatalf("décochage par un enfant : %d", code)
	}
	if code, _ := lea.Post("/taches/"+id+"/supprimer", nil); code != http.StatusForbidden {
		t.Fatalf("suppression par un enfant : %d", code)
	}
	if code, _ := lea.Post("/taches/"+id, map[string]string{"titre": "Rien", "repetition": "jour"}); code != http.StatusForbidden {
		t.Fatalf("modification par un enfant : %d", code)
	}
	// Léa crée sa propre tâche et peut la modifier.
	lea.Htmx = false
	lea.Get("/taches/nouvelle")
	if code, page := lea.Post("/taches", map[string]string{"titre": "Nourrir le chat", "repetition": "jours", "jour1": "1", "jour3": "1"}); code != 200 || !strings.Contains(page, "🐈") {
		t.Fatalf("création par un enfant : %d", code)
	}
	// Suppression par un parent : la tâche disparaît, l'historique reste.
	marie.Htmx = false
	marie.Get("/taches")
	if code, page := marie.Post("/taches/"+id+"/supprimer", nil); code != 200 || !strings.Contains(page, "supprimée") || !strings.Contains(page, "Mettre la table</span>") {
		t.Fatalf("suppression : %d", code)
	}
}
