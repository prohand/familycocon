// SPDX-License-Identifier: GPL-3.0-or-later

package menus

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/courses"
	"familycocon/internal/essai"
)

func brancher(a *appli.App) {
	courses.Brancher(a)
	Brancher(a)
}

func TestMenusEtCourses(t *testing.T) {
	_, srv := essai.NouvelleApp(t, brancher)
	marie := essai.Famille(t, srv)
	paris, _ := time.LoadLocation("Europe/Paris")
	jour := time.Now().In(paris).Format("2006-01-02")

	marie.Get("/menus/" + jour + "/soir")
	code, page := marie.Post("/menus/"+jour+"/soir", map[string]string{
		"nom": "Gratin dauphinois", "ingredients": "1 kg de pommes de terre\n- 50 cl de crème\n\nGruyère râpé",
	})
	if code != 200 || !strings.Contains(page, "Gratin dauphinois") || !strings.Contains(page, "3 ingrédients") {
		t.Fatalf("enregistrement : %d", code)
	}
	if code, _ := marie.Post("/menus/"+jour+"/gouter", map[string]string{"nom": "X"}); code != http.StatusNotFound {
		t.Fatalf("créneau inconnu accepté : %d", code)
	}
	// Accueil (le soir) ou « à prévoir » (le midi) : la tuile parle des menus.
	if _, page := marie.Get("/"); !strings.Contains(page, "Ce soir : Gratin dauphinois") && !strings.Contains(page, "Ce midi : à prévoir") {
		t.Fatal("résumé de la tuile Menus")
	}
	// Ingrédients aux courses.
	marie.Htmx = true
	code, frag := marie.Post("/menus/"+jour+"/soir/courses", nil)
	if code != 200 || !strings.Contains(frag, "🛒✓") {
		t.Fatalf("aux courses : %d\n%s", code, frag)
	}
	marie.Htmx = false
	_, page = marie.Get("/courses")
	for _, p := range []string{"Pommes de terre", "Crème", "Gruyère râpé"} {
		if !strings.Contains(page, p) {
			t.Errorf("%s absent des courses", p)
		}
	}
	if !strings.Contains(page, "1 kg") {
		t.Error("quantité perdue")
	}

	// Recette retenue : le lendemain, sans ingrédients, ils reviennent.
	demain := time.Now().In(paris).AddDate(0, 0, 1).Format("2006-01-02")
	marie.Get("/menus/" + demain + "/midi")
	marie.Post("/menus/"+demain+"/midi", map[string]string{"nom": "gratin dauphinois"})
	_, page = marie.Get("/menus/" + demain + "/midi")
	if !strings.Contains(page, "50 cl de crème") {
		t.Fatal("ingrédients de la recette non repris")
	}
	// Toute la semaine aux courses : seul le repas pas encore ajouté compte.
	marie.Htmx = true
	if _, frag := marie.Post("/menus/semaine/courses", map[string]string{"date": demain}); strings.Contains(frag, "Tous les ingrédients de la semaine") {
		t.Fatal("bouton « semaine » encore là après ajout")
	}
}

func TestEnfantNeVidePas(t *testing.T) {
	_, srv := essai.NouvelleApp(t, brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	jour := time.Now().Format("2006-01-02")
	lea.Get("/menus/" + jour + "/midi")
	if code, _ := lea.Post("/menus/"+jour+"/midi", map[string]string{"nom": "Pâtes au pesto"}); code != 200 {
		t.Fatalf("un enfant peut proposer un plat : %d", code)
	}
	if code, _ := lea.Post("/menus/"+jour+"/midi/vider", nil); code != http.StatusForbidden {
		t.Fatalf("vidage par un enfant : %d", code)
	}
}

// Revue : un ingrédient trop long est refusé à l'enregistrement, et un
// double envoi de « aux courses » n'ajoute les ingrédients qu'une fois.
func TestIngredientsLongsEtDoubleEnvoi(t *testing.T) {
	a, srv := essai.NouvelleApp(t, brancher)
	marie := essai.Famille(t, srv)
	jour := "2026-11-24"
	marie.Get("/menus/" + jour + "/soir")
	long := strings.Repeat("a", 81)
	if code, _ := marie.Post("/menus/"+jour+"/soir", map[string]string{"nom": "Soupe", "ingredients": "Poireaux\n" + long}); code != http.StatusBadRequest {
		t.Fatalf("ingrédient de 81 caractères accepté : %d", code)
	}
	if code, _ := marie.Post("/menus/"+jour+"/soir", map[string]string{"nom": "Soupe", "ingredients": "Poireaux\nCarottes"}); code != 200 {
		t.Fatalf("enregistrement : %d", code)
	}
	// Ancienne donnée (enregistrée avant la limite) : la ligne trop longue
	// est ignorée, sans erreur ni doublon.
	if _, err := a.DB.Exec(`UPDATE repas SET ingredients = ?`, "Poireaux\n"+long+"\nCarottes"); err != nil {
		t.Fatal(err)
	}
	marie.Htmx = true
	for i := range 2 {
		if code, _ := marie.Post("/menus/"+jour+"/soir/courses", nil); code != 200 {
			t.Fatalf("aux courses (envoi %d) : %d", i+1, code)
		}
	}
	// Bouton « semaine » juste après : rien de plus.
	marie.Post("/menus/semaine/courses", map[string]string{"date": jour})
	var n int
	if err := a.DB.QueryRow(`SELECT count(*) FROM produits`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("produits aux courses : %d (attendu 2) %v", n, err)
	}
	// Année hors bornes : créneau inconnu.
	marie.Htmx = false
	if code, _ := marie.Get("/menus/9999-01-01/soir"); code != http.StatusNotFound {
		t.Fatalf("année 9999 acceptée : %d", code)
	}
}
