// SPDX-License-Identifier: GPL-3.0-or-later

package foyer

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"familycocon/internal/appli"
	"familycocon/internal/budget"
	"familycocon/internal/essai"
	"familycocon/internal/suivi"
)

// Dépenses d'un véhicule au budget : réglage par véhicule, rétroactif sur
// le carnet (reprise Fuelio comprise), jamais en double, budget perso d'un
// autre parent laissé tranquille, « Non » qui retire.
func TestBudgetVehicule(t *testing.T) {
	a, srv := essai.NouvelleApp(t, func(a *appli.App) {
		s := suivi.Brancher(a)
		Brancher(a, s)
		budget.Brancher(a)
	})
	ctx := context.Background()
	marie := essai.Famille(t, srv)
	paul := essai.Parent(t, srv, marie, "Paul")
	_, page := marie.Post("/vehicules", map[string]string{"nom": "Clio", "genre": "voiture"})
	id := essai.Trouver(t, page, `action="/vehicules/(\d+)/km"`)
	if !strings.Contains(page, "data-fuelio-budget") {
		t.Fatal("choix du budget absent de la reprise Fuelio")
	}
	envoyer := func(n *essai.Navigateur, chemin string) {
		t.Helper()
		req, _ := http.NewRequest("POST", srv.URL+chemin, bytes.NewReader(sauvegardeFuelio(t)))
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("HX-Request", "true")
		req.Header.Set("X-Jeton", n.Jeton)
		rep, err := n.C.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		rep.Body.Close()
		if rep.StatusCode != 204 {
			t.Fatalf("reprise Fuelio : %d", rep.StatusCode)
		}
	}
	// ops : les dépenses du budget (tous comptes), par budget (0 : commun).
	ops := func() map[int64]int {
		t.Helper()
		rows, err := a.DB.QueryContext(ctx, "SELECT perso_de, count(*) FROM operations GROUP BY perso_de")
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		n := map[int64]int{}
		for rows.Next() {
			var k int64
			var v int
			if err := rows.Scan(&k, &v); err != nil {
				t.Fatal(err)
			}
			n[k] = v
		}
		return n
	}
	total := func() int {
		n := 0
		for _, v := range ops() {
			n += v
		}
		return n
	}
	// Reprise sans choix : rien au budget (comme avant).
	envoyer(marie, "/vehicules/"+id+"/fuelio")
	if n := total(); n != 0 {
		t.Fatalf("%d dépenses au budget sans le demander", n)
	}
	modifier := func(n *essai.Navigateur, choix string) string {
		t.Helper()
		_, page := n.Post("/vehicules/"+id, map[string]string{"nom": "Clio", "genre": "voiture", "budget": choix})
		return page
	}
	// Compte commun : les 3 lignes reprises avant y vont (rétroactif).
	page = modifier(marie, "commun")
	if o := ops(); o[0] != 3 || len(o) != 1 {
		t.Fatalf("compte commun : %v", o)
	}
	if !strings.Contains(page, "3 dépenses ajoutées au budget") || !strings.Contains(page, "Dépenses au budget : Compte commun") {
		t.Fatalf("message ou réglage absent :\n%s", page)
	}
	// Le plein suivant est pré-réglé sur le compte commun.
	if !strings.Contains(page, `<option value="1" selected>Compte commun</option>`) {
		t.Fatal("« Ajouter aussi au budget » non pré-réglé")
	}
	// Reprendre la même sauvegarde : rien en double.
	envoyer(marie, "/vehicules/"+id+"/fuelio?budget=garder")
	if n := total(); n != 3 {
		t.Fatalf("%d dépenses après une 2e reprise, attendu 3", n)
	}
	// Mon budget perso : les dépenses changent de budget.
	modifier(marie, "perso")
	moi := int64(0)
	for k, v := range ops() {
		if k == 0 || v != 3 {
			t.Fatalf("budget perso : %v", ops())
		}
		moi = k
	}
	// Paul (autre parent) voit « Budget perso de Marie » ; enregistrer la
	// fiche ou reprendre Fuelio sans changer le choix n'y touche pas.
	if _, f := paul.Get("/vehicules/" + id + "/modifier"); !strings.Contains(f, "Budget perso de Marie") {
		t.Fatalf("budget perso de Marie non montré à Paul :\n%s", f)
	}
	modifier(paul, "garder")
	envoyer(paul, "/vehicules/"+id+"/fuelio?budget=garder")
	if o := ops(); o[moi] != 3 || len(o) != 1 {
		t.Fatalf("budget perso de Marie touché par Paul : %v", o)
	}
	// Non : retirées du budget, toujours au carnet.
	modifier(marie, "non")
	if n := total(); n != 0 {
		t.Fatalf("%d dépenses restées au budget", n)
	}
	if _, p := marie.Get("/vehicules/" + id); !strings.Contains(p, "Vidange") {
		t.Fatal("ligne du carnet perdue")
	}
	// Reprise de tous les véhicules avec « Compte commun » : le réglage et
	// les dépenses suivent.
	envoyer(marie, "/vehicules/fuelio?budget="+url.QueryEscape("commun"))
	if o := ops(); o[0] != 3 || len(o) != 1 {
		t.Fatalf("reprise de tous les véhicules au compte commun : %v", o)
	}
	if _, p := marie.Get("/vehicules/" + id); !strings.Contains(p, "Dépenses au budget : Compte commun") {
		t.Fatal("réglage non gardé après la reprise")
	}
}
