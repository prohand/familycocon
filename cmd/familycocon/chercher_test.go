// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/essai"
)

// Recherche globale : trouve dans plusieurs modules, sans accents ; un
// enfant ne voit ni le budget ni les notes privées des autres.
func TestRechercheGlobale(t *testing.T) {
	_, srv := essai.NouvelleApp(t, func(a *appli.App) {
		if err := brancherModules(a); err != nil {
			t.Fatal(err)
		}
	})
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	paris, _ := time.LoadLocation("Europe/Paris")
	auj := time.Now().In(paris).Format("2006-01-02")
	marie.Get("/agenda/nouveau")
	marie.Post("/agenda", map[string]string{"titre": "Dentiste Hélène", "date": auj})
	if code, p := marie.Post("/notes", map[string]string{"titre": "Numéro d'Hélène", "texte": "06…"}); code != 200 {
		t.Fatalf("note : %d %s", code, p)
	}
	marie.Post("/notes", map[string]string{"titre": "Cadeau Hélène", "texte": "secret", "privee": "1"})
	marie.Post("/budget", map[string]string{"montant": "40", "categorie": "sante", "libelle": "Hélène dentiste", "jour": auj})

	_, page := marie.Get("/chercher?q=" + url.QueryEscape("helene"))
	for _, attendu := range []string{"Dentiste Hélène", "Numéro d&#39;Hélène", "Cadeau Hélène", "Hélène dentiste"} {
		if !strings.Contains(page, attendu) {
			t.Fatalf("%q absent de la recherche :\n%s", attendu, page)
		}
	}
	_, page = lea.Get("/chercher?q=" + url.QueryEscape("HÉLÈNE"))
	if !strings.Contains(page, "Dentiste Hélène") || strings.Contains(page, "Cadeau Hélène") || strings.Contains(page, "Hélène dentiste") {
		t.Fatalf("recherche d'un enfant :\n%s", page)
	}
	// Une lettre : rien n'est cherché.
	if _, page = marie.Get("/chercher?q=h"); strings.Contains(page, "Dentiste") {
		t.Fatal("recherche d'une seule lettre")
	}
}

// Export de la famille : JSON valide, toutes les sections, aucun secret,
// pas les notes privées des autres ; réservé aux parents.
func TestExportFamille(t *testing.T) {
	_, srv := essai.NouvelleApp(t, func(a *appli.App) {
		if err := brancherModules(a); err != nil {
			t.Fatal(err)
		}
	})
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	lea.Get("/notes/nouvelle")
	lea.Post("/notes", map[string]string{"titre": "Journal de Léa", "texte": "secret", "privee": "1"})
	marie.Get("/courses")
	marie.Post("/notes", map[string]string{"titre": "Code wifi", "texte": "nid"})
	if code, _ := lea.Get("/famille/export.json"); code != 403 {
		t.Fatalf("export ouvert à un enfant : %d", code)
	}
	code, corps := marie.Get("/famille/export.json")
	if code != 200 {
		t.Fatalf("export : %d", code)
	}
	var d map[string]json.RawMessage
	if err := json.Unmarshal([]byte(corps), &d); err != nil {
		t.Fatalf("JSON invalide : %v\n%s", err, corps)
	}
	for _, sec := range []string{"famille", "membres", "courses_listes", "agenda_evenements", "budget_operations", "notes", "projets_etapes"} {
		if _, ok := d[sec]; !ok {
			t.Errorf("section %q absente", sec)
		}
	}
	for _, interdit := range []string{"argon2", "Mdp", "Pin", "Totp", "Journal de Léa"} {
		if strings.Contains(corps, interdit) {
			t.Errorf("%q dans l'export", interdit)
		}
	}
	if !strings.Contains(corps, "Code wifi") {
		t.Error("note partagée absente")
	}
}
