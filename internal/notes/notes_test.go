// SPDX-License-Identifier: GPL-3.0-or-later

package notes

import (
	"net/http"
	"strings"
	"testing"

	"familycocon/internal/essai"
)

func TestNotesPartageesEtPrivees(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")

	marie.Get("/notes/nouvelle")
	if code, _ := marie.Post("/notes", map[string]string{"titre": "", "texte": ""}); code != 400 {
		t.Fatalf("note vide acceptée : %d", code)
	}
	marie.Post("/notes", map[string]string{"titre": "Code du wifi", "texte": "cocon-2026", "couleur": "4", "epinglee": "1"})
	marie.Post("/notes", map[string]string{"titre": "Idées cadeaux Léa", "texte": "vélo, livres", "privee": "1"})

	// Léa voit la note partagée, pas la note privée de Marie.
	_, page := lea.Get("/notes")
	if !strings.Contains(page, "Code du wifi") || strings.Contains(page, "Idées cadeaux") || !strings.Contains(page, "Épinglées") {
		t.Fatalf("visibilité côté enfant :\n%s", page)
	}
	idPrivee := essai.Trouver(t, mustGet(marie, "/notes?voir=privees"), `href="/notes/(\d+)"`)
	if code, _ := lea.Get("/notes/" + idPrivee); code != http.StatusNotFound {
		t.Fatalf("note privée ouverte par un autre membre : %d", code)
	}
	// Recherche.
	if _, p := marie.Get("/notes?q=wifi"); !strings.Contains(p, "Code du wifi") || strings.Contains(p, "Idées cadeaux") {
		t.Fatal("recherche")
	}
	// Léa ne modifie pas la note de sa mère, ni ne supprime.
	idWifi := essai.Trouver(t, page, `href="/notes/(\d+)"`)
	if code, _ := lea.Post("/notes/"+idWifi, map[string]string{"titre": "Piraté", "texte": "x"}); code != http.StatusForbidden {
		t.Fatalf("modification par un enfant : %d", code)
	}
	if code, _ := lea.Post("/notes/"+idWifi+"/supprimer", nil); code != http.StatusForbidden {
		t.Fatalf("suppression par un enfant : %d", code)
	}
	// Léa écrit sa propre note privée ; Marie (parent) ne la voit pas.
	lea.Get("/notes/nouvelle")
	lea.Post("/notes", map[string]string{"titre": "Journal secret", "texte": "…", "privee": "1"})
	if _, p := marie.Get("/notes"); strings.Contains(p, "Journal secret") {
		t.Fatal("note privée d'un enfant visible par un parent")
	}
	// Épingler / désépingler (htmx).
	marie.Get("/notes")
	marie.Htmx = true
	if _, frag := marie.Post("/notes/"+idWifi+"/epingler", nil); strings.Contains(frag, "Épinglées") {
		t.Fatal("désépinglage sans effet")
	}
	marie.Htmx = false
	// Suppression par un parent.
	if code, p := marie.Post("/notes/"+idWifi+"/supprimer", nil); code != 200 || strings.Contains(p, "Code du wifi") {
		t.Fatalf("suppression : %d", code)
	}
}

func mustGet(n *essai.Navigateur, chemin string) string {
	_, p := n.Get(chemin)
	return p
}
