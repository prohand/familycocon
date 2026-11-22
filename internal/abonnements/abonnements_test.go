// SPDX-License-Identifier: GPL-3.0-or-later

package abonnements

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/budget"
	"familycocon/internal/essai"
)

func brancher(a *appli.App) {
	budget.Brancher(a)
	Brancher(a)
}

func TestAbonnements(t *testing.T) {
	_, srv := essai.NouvelleApp(t, brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	paris, _ := time.LoadLocation("Europe/Paris")
	auj := time.Now().In(paris)
	jour := func(n int) string { return auj.AddDate(0, 0, n).Format("2006-01-02") }

	marie.Get("/abonnements/nouveau")
	// Box : prélevée aujourd'hui (écrite au budget), engagement qui finit bientôt.
	code, page := marie.Post("/abonnements", map[string]string{"nom": "Box internet", "emoji": "🌐", "montant": "29,99",
		"frequence": "mois", "prochain": jour(0), "engagement_fin": jour(20), "site": "box.example", "categorie": "abonnements"})
	if code != 200 || !strings.Contains(page, "Box internet") || !strings.Contains(page, "Engagé jusqu") {
		t.Fatalf("création : %d\n%s", code, page)
	}
	// Assurance annuelle dans 5 jours.
	marie.Post("/abonnements", map[string]string{"nom": "Assurance habitation", "emoji": "🛡️", "montant": "240", "frequence": "annee", "prochain": jour(5)})
	_, page = marie.Get("/abonnements")
	// 29,99 + 240/12 = 49,99 par mois.
	if !strings.Contains(page, "49,99") {
		t.Fatalf("total par mois :\n%s", page)
	}
	if code, _ := marie.Post("/abonnements", map[string]string{"nom": "X", "montant": "1", "frequence": "mois", "prochain": jour(1), "site": "javascript:alert(1)"}); code != http.StatusBadRequest {
		t.Fatalf("site javascript accepté : %d", code)
	}
	// Le prélèvement du jour est dans le budget.
	if _, page := marie.Get("/budget"); !strings.Contains(page, "Box internet") {
		t.Fatal("prélèvement absent du budget")
	}
	// Rappels (parents seulement) : renouvellement et fin d'engagement.
	if _, page := marie.Get("/"); !strings.Contains(page, "Renouvellement : Assurance habitation") || !strings.Contains(page, "Fin d&#39;engagement : Box internet") {
		t.Fatal("rappels absents pour un parent")
	}
	if _, page := lea.Get("/"); strings.Contains(page, "Assurance habitation") || strings.Contains(page, "Box internet") {
		t.Fatal("rappel d'abonnement montré à un enfant")
	}
	if code, _ := lea.Get("/abonnements"); code != http.StatusForbidden {
		t.Fatalf("abonnements ouverts à un enfant : %d", code)
	}

	// Résilier : plus de prélèvement prévu.
	id := essai.Trouver(t, page, `href="/abonnements/(\d+)"`)
	_, fiche := marie.Get("/abonnements/" + id)
	if !strings.Contains(fiche, `href="https://box.example"`) {
		t.Fatal("lien de gestion absent")
	}
	marie.Post("/abonnements/"+id+"/resilier", nil)
	if _, page := marie.Get("/abonnements"); !strings.Contains(page, "Résiliés (1)") {
		t.Fatalf("résilié :\n%s", page)
	}
}
