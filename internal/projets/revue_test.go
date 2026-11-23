// SPDX-License-Identifier: GPL-3.0-or-later

package projets

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"familycocon/internal/donnees"
	"familycocon/internal/essai"
)

// Revue du 23/11/2026.
func TestRevueProjets(t *testing.T) {
	a, srv := essai.NouvelleApp(t, brancher)
	marie := essai.Famille(t, srv)
	ctx := context.Background()
	auj := time.Now().Format("2006-01-02")

	marie.Get("/projets/nouveau")
	_, page := marie.Post("/projets", map[string]string{"nom": "Travaux", "budget": "100"})
	id := essai.Trouver(t, page, `action="/projets/(\d+)/etapes"`)

	// 150 dépenses de 1 € : le total compte tout (avant : les 100 dernières).
	for range 150 {
		if _, err := a.Q.AjouterCarnet(ctx, donnees.AjouterCarnetParams{FamilleID: 1, Module: mod, ObjetID: 1, Jour: auj, Libelle: "Vis",
			Montant: 100, CreeLe: time.Now().Unix()}); err != nil {
			t.Fatal(err)
		}
	}
	if _, page := marie.Get("/projets/" + id); !strings.Contains(page, "Dépensé : <b>150,00") || !strings.Contains(page, "Dépassé de 50,00") {
		t.Fatalf("total des dépenses faux :\n%s", page)
	}

	// Budget « 0 » : accepté, veut dire « pas de budget ».
	if code, _ := marie.Post("/projets/"+id, map[string]string{"nom": "Travaux", "budget": "0"}); code != 200 {
		t.Fatalf("budget 0 : %d", code)
	}
	if p, _ := a.Q.ProjetParID(ctx, donnees.ProjetParIDParams{ID: 1, FamilleID: 1}); p.Budget != 0 {
		t.Fatalf("budget : %d", p.Budget)
	}

	// Terminer deux fois (double clic) : reste terminé.
	marie.Post("/projets/"+id+"/terminer", map[string]string{"termine": "1"})
	marie.Post("/projets/"+id+"/terminer", map[string]string{"termine": "1"})
	if p, _ := a.Q.ProjetParID(ctx, donnees.ProjetParIDParams{ID: 1, FamilleID: 1}); p.TermineLe == 0 {
		t.Fatal("double clic : projet rouvert")
	}
	if _, page := marie.Get("/"); !strings.Contains(page, "Aucun projet en cours") {
		t.Fatal("résumé : projet terminé compté en cours")
	}
	marie.Post("/projets/"+id+"/terminer", map[string]string{"termine": "0"})

	// Dates absurdes refusées (objectif, étape, dépense).
	if code, _ := marie.Post("/projets/"+id, map[string]string{"nom": "Travaux", "objectif": "9999-01-01"}); code != http.StatusBadRequest {
		t.Fatalf("objectif en 9999 : %d", code)
	}
	if code, _ := marie.Post("/projets/"+id+"/etapes", map[string]string{"texte": "Peindre", "jour": "9999-01-01"}); code != http.StatusBadRequest {
		t.Fatalf("étape en 9999 : %d", code)
	}
	if code, _ := marie.Post("/suivi/projets/"+id+"/carnet", map[string]string{"libelle": "Pot", "jour": "9999-01-01"}); code != http.StatusBadRequest {
		t.Fatalf("dépense en 9999 : %d", code)
	}

	// Dépense envoyée deux fois : une seule ligne ; coût « 0 » accepté.
	n0, _ := a.Q.CompterCarnetObjet(ctx, donnees.CompterCarnetObjetParams{FamilleID: 1, Module: mod, ObjetID: 1})
	ligne := map[string]string{"libelle": "Coup de main du voisin", "jour": auj, "montant": "0"}
	if code, _ := marie.Post("/suivi/projets/"+id+"/carnet", ligne); code != 200 {
		t.Fatalf("coût 0 : %d", code)
	}
	marie.Post("/suivi/projets/"+id+"/carnet", ligne)
	if n, _ := a.Q.CompterCarnetObjet(ctx, donnees.CompterCarnetObjetParams{FamilleID: 1, Module: mod, ObjetID: 1}); n != n0+1 {
		t.Fatalf("double envoi : %d lignes ajoutées", n-n0)
	}

	// Plafond d'étapes par projet.
	for range EtapesMax {
		if _, err := a.Q.AjouterEtape(ctx, donnees.AjouterEtapeParams{ProjetID: 1, FamilleID: 1, Texte: "x", CreeLe: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if code, _ := marie.Post("/projets/"+id+"/etapes", map[string]string{"texte": "De trop"}); code != http.StatusBadRequest {
		t.Fatalf("plafond d'étapes : %d", code)
	}
}
