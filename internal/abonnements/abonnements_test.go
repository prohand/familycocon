// SPDX-License-Identifier: GPL-3.0-or-later

package abonnements

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/budget"
	"familycocon/internal/donnees"
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

// Bugs de la revue : un abonnement résilié modifié reste résilié et rien
// n'est écrit deux fois ; une date trop ancienne est refusée ; le
// renouvellement du jour reste dans « À venir ».
func TestAbonnementsRevue(t *testing.T) {
	a, srv := essai.NouvelleApp(t, brancher)
	marie := essai.Famille(t, srv)
	paris, _ := time.LoadLocation("Europe/Paris")
	auj := time.Now().In(paris)
	jour := func(n int) string { return auj.AddDate(0, 0, n).Format("2006-01-02") }
	marie.Get("/abonnements/nouveau")
	if code, _ := marie.Post("/abonnements", map[string]string{"nom": "Vieux", "montant": "5", "frequence": "semaine", "prochain": "0001-01-01"}); code != 400 {
		t.Fatalf("date de l'an 1 acceptée : %d", code)
	}
	// Commencé il y a 40 jours : 2 prélèvements écrits.
	marie.Post("/abonnements", map[string]string{"nom": "Vidéo", "montant": "10", "frequence": "mois", "prochain": jour(-40)})
	_, page := marie.Get("/abonnements")
	id := essai.Trouver(t, page, `href="/abonnements/(\d+)"`)
	marie.Post("/abonnements/"+id+"/resilier", nil)
	_, form := marie.Get("/abonnements/" + id)
	prochain := essai.Trouver(t, form, `name="prochain" value="([0-9-]*)"`)
	// Modifié (les notes) : il reste résilié, pas de prélèvement en double.
	if code, _ := marie.Post("/abonnements/"+id, map[string]string{"nom": "Vidéo", "montant": "10", "frequence": "mois", "prochain": prochain, "notes": "résilié en ligne"}); code != 200 {
		t.Fatalf("abonnement résilié non modifiable : %d", code)
	}
	// Même avec une date (ancienne page) : rien ne repart.
	marie.Post("/abonnements/"+id, map[string]string{"nom": "Vidéo", "montant": "10", "frequence": "mois", "prochain": jour(0)})
	if _, page := marie.Get("/abonnements"); !strings.Contains(page, "Résiliés (1)") {
		t.Fatal("réactivé par une simple modification")
	}
	ctx := context.Background()
	f, _ := a.Q.ToutesFamilles(ctx)
	ops, _ := a.Q.OperationsPeriode(ctx, donnees.OperationsPeriodeParams{FamilleID: f[0].ID, Du: "2000-01-01", Au: "2999-01-01"})
	if len(ops) != 2 {
		t.Fatalf("%d prélèvements, attendu 2", len(ops))
	}
	// Payé depuis le début : les 2 prélèvements.
	if _, fiche := marie.Get("/abonnements/" + id); !strings.Contains(fiche, "Payé depuis le début") || !strings.Contains(fiche, "2 prélèvements") {
		t.Fatalf("payé depuis le début :\n%s", fiche)
	}
	// Repris (réabonné) : de nouveau actif, à partir de la date donnée.
	if code, _ := marie.Post("/abonnements/"+id+"/reprendre", map[string]string{"prochain": jour(5)}); code != 200 {
		t.Fatalf("reprise : %d", code)
	}
	if _, page := marie.Get("/abonnements"); strings.Contains(page, "Résiliés") || !strings.Contains(page, "Vidéo") {
		t.Fatal("abonnement repris toujours résilié")
	}
	ops, _ = a.Q.OperationsPeriode(ctx, donnees.OperationsPeriodeParams{FamilleID: f[0].ID, Du: "2000-01-01", Au: "2999-01-01"})
	if len(ops) != 2 {
		t.Fatalf("%d prélèvements après la reprise, attendu 2", len(ops))
	}
	// Renouvellement annuel aujourd'hui : écrit au budget ET dans « À venir ».
	marie.Post("/abonnements", map[string]string{"nom": "Assurance", "montant": "240", "frequence": "annee", "prochain": jour(0)})
	if _, page := marie.Get("/"); !strings.Contains(page, "Renouvellement : Assurance") {
		t.Fatal("renouvellement du jour absent de « À venir »")
	}
	// Total annuel exact (100 €/an et non 99,96 €).
	marie.Post("/abonnements", map[string]string{"nom": "Cent", "montant": "100", "frequence": "annee", "prochain": jour(30)})
	if _, page := marie.Get("/abonnements"); !strings.Contains(page, "460,00") { // 240 + 100 + 120 (Vidéo, repris)
		t.Fatal("total annuel inexact")
	}
}

// Un abonnement reste une dépense, même modifié par la page des opérations
// qui reviennent (revue du 23/11/2026).
func TestAbonnementJamaisRevenu(t *testing.T) {
	a, srv := essai.NouvelleApp(t, brancher)
	marie := essai.Famille(t, srv)
	jour := time.Now().AddDate(0, 0, 5).Format("2006-01-02")
	marie.Get("/abonnements/nouveau")
	marie.Post("/abonnements", map[string]string{"nom": "Musique", "montant": "9,99", "frequence": "mois", "prochain": jour})
	recs, _ := a.Q.RecurrencesFamille(context.Background(), 1)
	if len(recs) != 1 {
		t.Fatalf("%d récurrences", len(recs))
	}
	id := strconv.FormatInt(recs[0].ID, 10)
	champs := map[string]string{"montant": "9,99", "categorie": "salaire", "libelle": "Musique", "frequence": "mois", "prochaine": jour}
	if code, _ := marie.Post("/recurrences/"+id, champs); code != http.StatusBadRequest {
		t.Fatalf("abonnement passé en revenu : %d", code)
	}
	champs["categorie"] = "loisirs"
	if code, _ := marie.Post("/recurrences/"+id, champs); code != 200 {
		t.Fatalf("changement de catégorie de dépense : %d", code)
	}
	if x, _ := a.Q.RecurrenceParID(context.Background(), donnees.RecurrenceParIDParams{ID: recs[0].ID, FamilleID: 1}); x.Sens != "depense" || x.Categorie != "loisirs" {
		t.Fatalf("récurrence : %+v", x)
	}
	// Date de fin d'engagement absurde refusée.
	if code, _ := marie.Post("/abonnements", map[string]string{"nom": "X", "montant": "1", "frequence": "mois", "prochain": jour, "engagement_fin": "9999-01-01"}); code != http.StatusBadRequest {
		t.Fatalf("engagement en 9999 : %d", code)
	}
}
