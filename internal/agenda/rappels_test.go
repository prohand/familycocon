// SPDX-License-Identifier: GPL-3.0-or-later

package agenda

import (
	"context"
	"strings"
	"testing"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
	"familycocon/internal/essai"
)

// Un événement « à rappeler » apparaît dans « À venir » ; un événement sans
// rappel, non. Les échéances des autres modules sont visibles dans l'agenda.
func TestRappelEvenementEtEcheancesDansAgenda(t *testing.T) {
	_, srv := essai.NouvelleApp(t, func(a *appli.App) {
		Brancher(a)
		// Un module qui fournit une échéance (comme le contrôle technique).
		a.AjouterEcheances("vehicules", func(_ context.Context, f donnees.Famille, debut, fin time.Time) ([]appli.Echeance, error) {
			j := debut.AddDate(0, 0, 2)
			if !j.Before(fin) {
				return nil, nil
			}
			return []appli.Echeance{{Jour: j, Titre: "Contrôle technique", Emoji: "🚗", URL: "/vehicules/1", Module: "vehicules"}}, nil
		})
	})
	marie := essai.Famille(t, srv)
	paris, _ := time.LoadLocation("Europe/Paris")
	dans3 := time.Now().In(paris).AddDate(0, 0, 3).Format("2006-01-02")
	marie.Get("/agenda/nouveau")
	marie.Post("/agenda", map[string]string{"titre": "Dentiste", "date": dans3, "heure_debut": "10:30", "rappel": "1"})
	marie.Post("/agenda", map[string]string{"titre": "Piscine", "date": dans3, "heure_debut": "17:00"})
	_, page := marie.Get("/")
	if !strings.Contains(page, "Dentiste à 10 h 30") {
		t.Fatalf("événement à rappeler absent de « À venir » :\n%s", page)
	}
	if strings.Contains(page, "Piscine") {
		t.Fatal("événement sans rappel dans « À venir »")
	}
	// La case reste cochée à la modification.
	_, page = marie.Get("/agenda?date=" + dans3)
	id := essai.Trouver(t, page, `href="/agenda/(\d+)[?"][^>]*>\s*<span class="heure">10:30`)
	if _, fiche := marie.Get("/agenda/" + id); !strings.Contains(fiche, `name="rappel" value="1" checked`) {
		t.Fatal("case « Me le rappeler » non cochée")
	}
	// L'échéance d'un autre module est dans l'agenda.
	if !strings.Contains(page, "🚗 Contrôle technique") {
		t.Fatalf("échéance absente de l'agenda :\n%s", page)
	}
}
