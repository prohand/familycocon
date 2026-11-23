// SPDX-License-Identifier: GPL-3.0-or-later

package projets

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/essai"
	"familycocon/internal/suivi"
)

func brancher(a *appli.App) { Brancher(a, suivi.Brancher(a)) }

func TestProjet(t *testing.T) {
	_, srv := essai.NouvelleApp(t, brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")

	objectif := time.Now().AddDate(0, 0, 20).Format("2006-01-02")
	marie.Get("/projets/nouveau")
	code, page := marie.Post("/projets", map[string]string{"nom": "Vacances en Bretagne", "emoji": "🏖️", "couleur": "4",
		"objectif": objectif, "budget": "1 200,00", "description": "Une semaine à la mer"})
	if code != 200 || !strings.Contains(page, "🏖️ Vacances en Bretagne") || !strings.Contains(page, "Objectif") {
		t.Fatalf("création : %d\n%s", code, page)
	}
	id := essai.Trouver(t, page, `action="/projets/(\d+)/etapes"`)
	// Emoji inconnu : remplacé.
	if _, page := marie.Post("/projets", map[string]string{"nom": "X", "emoji": "<b>"}); !strings.Contains(page, "🎯 X") {
		t.Fatal("emoji libre accepté")
	}

	// Étapes : une datée pour Léa, une sans date.
	demain := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	_, page = marie.Get("/projets/" + id)
	idLea := essai.Trouver(t, page, `<option value="(\d+)">Léa</option>`)
	marie.Post("/projets/"+id+"/etapes", map[string]string{"texte": "Réserver le gîte", "jour": demain, "membre": idLea})
	_, page = marie.Post("/projets/"+id+"/etapes", map[string]string{"texte": "Faire les valises"})
	if !strings.Contains(page, "Réserver le gîte") || !strings.Contains(page, `max="2" value="0"`) {
		t.Fatalf("étapes :\n%s", page)
	}
	// « À venir » : l'étape et l'objectif.
	if _, page := marie.Get("/"); !strings.Contains(page, "Réserver le gîte · Vacances en Bretagne") || !strings.Contains(page, "Objectif : Vacances en Bretagne") {
		t.Fatal("rappels absents de l'accueil")
	}

	// Léa coche une étape ; elle ne peut pas en supprimer.
	etape := essai.Trouver(t, page, `action="/projets/etapes/(\d+)/cocher"`)
	if code, page := lea.Post("/projets/etapes/"+etape+"/cocher", map[string]string{"faite": "1"}); code != 200 || !strings.Contains(page, `max="2" value="1"`) {
		t.Fatalf("cocher : %d", code)
	}
	// Double clic (même état demandé) : reste cochée.
	if _, page := lea.Post("/projets/etapes/"+etape+"/cocher", map[string]string{"faite": "1"}); !strings.Contains(page, `max="2" value="1"`) {
		t.Fatal("double clic : décochée")
	}
	if code, _ := lea.Post("/projets/etapes/"+etape+"/supprimer", nil); code != http.StatusForbidden {
		t.Fatalf("suppression enfant : %d", code)
	}

	// Dépense au carnet : budget mis à jour.
	_, page = marie.Post("/suivi/projets/"+id+"/carnet", map[string]string{"libelle": "Acompte du gîte", "jour": time.Now().Format("2006-01-02"), "montant": "300"})
	if !strings.Contains(page, "Acompte du gîte") || !strings.Contains(page, "Reste 900,00") {
		t.Fatalf("dépense :\n%s", page)
	}
	// La liste montre le dépensé et le budget.
	if _, liste := marie.Get("/projets"); !strings.Contains(liste, "300,00") || !strings.Contains(liste, "1 200,00") && !strings.Contains(liste, "1\u202f200,00") {
		t.Fatalf("budget absent de la liste :\n%s", liste)
	}
	// Pas d'échéances pour les projets.
	if strings.Contains(page, "Ajouter une échéance") {
		t.Fatal("bloc échéances affiché")
	}
	if code, _ := marie.Post("/suivi/projets/"+id+"/echeances", map[string]string{"libelle": "X", "jour": demain}); code != http.StatusNotFound {
		t.Fatalf("échéance de projet acceptée : %d", code)
	}

	// Terminer : le projet passe dans « Terminés », les rappels disparaissent.
	marie.Post("/projets/"+id+"/terminer", nil)
	if _, page := marie.Get("/projets"); !strings.Contains(page, "Terminés (1)") {
		t.Fatalf("terminés :\n%s", page)
	}
	if _, page := marie.Get("/"); strings.Contains(page, "Vacances en Bretagne") {
		t.Fatal("rappel d'un projet terminé")
	}

	// Suppression : parent seulement.
	if code, _ := lea.Post("/projets/"+id+"/supprimer", nil); code != http.StatusForbidden {
		t.Fatalf("suppression enfant : %d", code)
	}
	if code, _ := marie.Post("/projets/"+id+"/supprimer", nil); code != 200 {
		t.Fatalf("suppression : %d", code)
	}
	if code, _ := marie.Get("/projets/" + id); code != http.StatusNotFound {
		t.Fatalf("toujours là : %d", code)
	}
}
