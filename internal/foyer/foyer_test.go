// SPDX-License-Identifier: GPL-3.0-or-later

package foyer

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/contacts"
	"familycocon/internal/essai"
	"familycocon/internal/suivi"
)

func brancher(a *appli.App) {
	contacts.Brancher(a)
	Brancher(a, suivi.Brancher(a))
}

func TestVehicule(t *testing.T) {
	_, srv := essai.NouvelleApp(t, brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")

	if _, page := marie.Get("/vehicules"); !strings.Contains(page, "contrôle technique") {
		t.Fatal("liste vide sans texte d'aide")
	}
	marie.Get("/vehicules/nouveau")
	code, page := marie.Post("/vehicules", map[string]string{"nom": "La Clio", "genre": "voiture", "immatriculation": "ab-123-cd", "kilometrage": "85 000"})
	if code != 200 || !strings.Contains(page, "AB-123-CD") || !strings.Contains(page, "85\u202f000 km") {
		t.Fatalf("création : %d\n%s", code, page)
	}
	id := essai.Trouver(t, page, `action="/vehicules/(\d+)/km"`)

	// Échéance dans 10 jours, chaque année → alerte « proche ».
	jour := time.Now().AddDate(0, 0, 10).Format("2006-01-02")
	if code, _ := marie.Post("/suivi/vehicules/"+id+"/echeances", map[string]string{"libelle": "Assurance", "jour": jour, "repetition_mois": "12"}); code != 200 {
		t.Fatalf("échéance : %d", code)
	}
	if _, page := marie.Get("/vehicules"); !strings.Contains(page, "⏰ Assurance") {
		t.Fatalf("alerte absente :\n%s", page)
	}
	if _, page := marie.Get("/"); !strings.Contains(page, "À venir") || !strings.Contains(page, "Assurance") {
		t.Fatal("échéance absente de « À venir »")
	}
	// Objet d'un autre module : refusé.
	if code, _ := marie.Post("/suivi/animaux/"+id+"/echeances", map[string]string{"libelle": "X", "jour": jour}); code != http.StatusNotFound {
		t.Fatalf("mauvais module accepté : %d", code)
	}

	// « C'est fait » : carnet, kilométrage, et report d'un an.
	_, page = marie.Get("/vehicules/" + id)
	ech := essai.Trouver(t, page, `action="/suivi/echeances/(\d+)/faite"`)
	aujourdhui := time.Now().Format("2006-01-02")
	code, page = lea.Post("/suivi/echeances/"+ech+"/faite", map[string]string{"jour": aujourdhui, "montant": "420,50", "km": "86000"})
	if code != 200 || !strings.Contains(page, "86\u202f000 km") || !strings.Contains(page, "tous les 12 mois") {
		t.Fatalf("faite : %d\n%s", code, page)
	}
	if !strings.Contains(page, "420,50") {
		t.Fatal("coût absent du carnet")
	}
	if _, page := marie.Get("/vehicules"); strings.Contains(page, "⏰") {
		t.Fatal("alerte toujours là après report")
	}
	// Le compteur ne recule pas.
	marie.Post("/vehicules/"+id+"/km", map[string]string{"km": "1000"})
	if _, page := marie.Get("/vehicules/" + id); !strings.Contains(page, "86\u202f000 km") {
		t.Fatal("le kilométrage a reculé")
	}

	// Suppression : enfant refusé, parent accepté (avec le suivi).
	if code, _ := lea.Post("/vehicules/"+id+"/supprimer", nil); code != http.StatusForbidden {
		t.Fatalf("suppression enfant : %d", code)
	}
	if code, _ := marie.Post("/vehicules/"+id+"/supprimer", nil); code != 200 {
		t.Fatalf("suppression : %d", code)
	}
	if code, _ := marie.Get("/vehicules/" + id); code != http.StatusNotFound {
		t.Fatalf("toujours là : %d", code)
	}
	if _, page := marie.Get("/"); strings.Contains(page, "Assurance") {
		t.Fatal("échéance restée après suppression")
	}
}

func TestAnimalEtPrestataire(t *testing.T) {
	_, srv := essai.NouvelleApp(t, brancher)
	marie := essai.Famille(t, srv)

	marie.Get("/animaux/nouveau")
	code, page := marie.Post("/animaux", map[string]string{"nom": "Filou", "espece": "chat", "race": "Européen",
		"naissance": time.Now().AddDate(-3, -1, 0).Format("2006-01-02"), "puce": "250269812345678"})
	if code != 200 || !strings.Contains(page, "🐈 Filou") || !strings.Contains(page, "3 ans") || !strings.Contains(page, "Carnet de santé") {
		t.Fatalf("animal : %d\n%s", code, page)
	}
	id := essai.Trouver(t, page, `action="/suivi/animaux/(\d+)/carnet"`)
	if code, _ := marie.Post("/animaux", map[string]string{"nom": "X", "naissance": "2999-01-01"}); code != http.StatusBadRequest {
		t.Fatalf("naissance future acceptée : %d", code)
	}
	// Ajout au carnet, puis un objet inconnu est refusé.
	if code, page := marie.Post("/suivi/animaux/"+id+"/carnet", map[string]string{"libelle": "Vaccin", "jour": "2026-01-15", "montant": "65"}); code != 200 || !strings.Contains(page, "Vaccin") {
		t.Fatalf("carnet : %d", code)
	}
	if code, _ := marie.Post("/suivi/animaux/999/carnet", map[string]string{"libelle": "X", "jour": "2026-01-01"}); code != http.StatusNotFound {
		t.Fatalf("objet inconnu : %d", code)
	}
	// Sans session : rien.
	x := essai.NouveauNavigateur(t, srv)
	if _, page := x.Get("/animaux/" + id); strings.Contains(page, "Filou") {
		t.Fatal("fiche visible sans session")
	}

	// Prestataire = contact « pro » du carnet.
	marie.Get("/contacts/nouveau?pro=1")
	code, page = marie.Post("/contacts", map[string]string{"nom": "Plomberie Martin", "metier": "Plombier", "pro": "1",
		"telephone": "06 12 34 56 78", "site": "plomberie.example", "adresse": "3 rue des Lilas", "relation": "pro"})
	if code != 200 {
		t.Fatalf("création du contact pro : %d", code)
	}
	idPro := essai.Trouver(t, page, `href="/prestataires/(\d+)"`)
	_, page = marie.Get("/prestataires/" + idPro)
	if !strings.Contains(page, `href="tel:0612345678"`) || !strings.Contains(page, `href="https://plomberie.example"`) ||
		!strings.Contains(page, "query=3%20rue%20des%20Lilas") || !strings.Contains(page, "Interventions") {
		t.Fatalf("fiche prestataire :\n%s", page)
	}
	if _, page := marie.Get("/prestataires"); !strings.Contains(page, "Plomberie Martin") {
		t.Fatal("prestataire absent de la liste")
	}
	if code, _ := marie.Post("/contacts", map[string]string{"nom": "X", "site": "javascript:alert(1)"}); code != http.StatusBadRequest {
		t.Fatalf("site javascript: accepté : %d", code)
	}

	// Vétérinaire choisi dans le carnet : appel direct depuis la fiche de l'animal.
	_, page = marie.Post("/contacts", map[string]string{"nom": "Dr Duval", "metier": "Vétérinaire", "pro": "1", "telephone": "02 99 00 11 22"})
	idVeto := essai.Trouver(t, page, `action="/contacts/(\d+)/favori"`)
	marie.Get("/animaux/" + id + "/modifier")
	_, page = marie.Post("/animaux/"+id, map[string]string{"nom": "Filou", "espece": "chat", "veterinaire_id": idVeto})
	if !strings.Contains(page, "Dr Duval") || !strings.Contains(page, `href="tel:0299001122"`) || !strings.Contains(page, `href="/contacts/`+idVeto+`"`) {
		t.Fatalf("vétérinaire absent de la fiche :\n%s", page)
	}
	// Contact d'une autre famille ou inconnu : ignoré.
	if _, page := marie.Post("/animaux/"+id, map[string]string{"nom": "Filou", "espece": "chat", "veterinaire_id": "9999"}); strings.Contains(page, "Dr Duval") {
		t.Fatal("contact inconnu accepté")
	}
}
