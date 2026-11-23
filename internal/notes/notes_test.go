// SPDX-License-Identifier: GPL-3.0-or-later

package notes

import (
	"database/sql"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"familycocon/internal/donnees"
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

// Revue : recherche avec « & » et « % », liste bornée au début du texte,
// passage en privé réservé au parent auteur, plafond, membre retiré.
func TestRevueNotes(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	ctx := t.Context()

	marie.Get("/notes/nouvelle")
	marie.Post("/notes", map[string]string{"titre": "Soldes", "texte": "Pain & beurre à 50% #1"})
	marie.Post("/notes", map[string]string{"titre": "Autre", "texte": "50 euros"})
	marie.Post("/notes", map[string]string{"titre": "Longue", "texte": strings.Repeat("é", 2000)})

	// « 50% » ne trouve que la note qui contient « 50% ».
	_, page := marie.Get("/notes?q=" + url.QueryEscape("50%"))
	if !strings.Contains(page, "Soldes") || strings.Contains(page, "Autre") {
		t.Fatal("recherche de « 50% »")
	}
	// L'adresse de rafraîchissement garde la recherche entière, encodée.
	_, page = marie.Get("/notes?q=" + url.QueryEscape("pain & beurre"))
	if !strings.Contains(page, `hx-get="/notes/contenu?q=pain&#43;%26&#43;beurre"`) || !strings.Contains(page, "Soldes") {
		t.Fatalf("adresse de rafraîchissement mal encodée :\n%s", page)
	}
	// La liste ne porte que le début d'une longue note.
	if _, page = marie.Get("/notes"); strings.Count(page, "é") > 400 || !strings.Contains(page, "…") {
		t.Fatal("texte entier envoyé dans la liste")
	}

	// Léa partage une note : elle ne peut plus la rendre privée (cela
	// reviendrait à la supprimer pour la famille).
	lea.Get("/notes/nouvelle")
	lea.Post("/notes", map[string]string{"titre": "Liste au Père Noël", "texte": "un vélo"})
	var idLea string
	if err := a.DB.QueryRow(`SELECT id FROM notes WHERE titre = 'Liste au Père Noël'`).Scan(&idLea); err != nil {
		t.Fatal(err)
	}
	if _, p := lea.Get("/notes/" + idLea); strings.Contains(p, `name="privee"`) {
		t.Fatal("case « Privée » proposée à un enfant sur une note partagée")
	}
	lea.Post("/notes/"+idLea, map[string]string{"titre": "Liste au Père Noël", "texte": "un vélo", "privee": "1"})
	if _, p := marie.Get("/notes"); !strings.Contains(p, "Liste au Père Noël") {
		t.Fatal("un enfant a fait disparaître sa note partagée")
	}
	// Marie (parent, auteur) le peut pour ses propres notes.
	var idSoldes string
	if err := a.DB.QueryRow(`SELECT id FROM notes WHERE titre = 'Soldes'`).Scan(&idSoldes); err != nil {
		t.Fatal(err)
	}
	marie.Post("/notes/"+idSoldes, map[string]string{"titre": "Soldes", "texte": "x", "privee": "1"})
	if _, p := lea.Get("/notes"); strings.Contains(p, "Soldes") {
		t.Fatal("note rendue privée par son auteur parent encore visible")
	}

	// Membre retiré : ses notes privées sont effacées, les partagées restent.
	lea.Post("/notes", map[string]string{"titre": "Journal secret", "texte": "…", "privee": "1"})
	if err := a.Q.SupprimerNotesPriveesMembre(ctx, donnees.SupprimerNotesPriveesMembreParams{FamilleID: 1, AuteurID: sql.NullInt64{Int64: 2, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	var restantes int
	if err := a.DB.QueryRow(`SELECT count(*) FROM notes WHERE auteur_id = 2`).Scan(&restantes); err != nil || restantes != 1 {
		t.Fatalf("notes de Léa après retrait : %d (attendu 1, la partagée) %v", restantes, err)
	}

	// Plafond de notes par famille.
	if _, err := a.DB.Exec(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < ?)
		INSERT INTO notes (famille_id, titre, cree_le, modifie_le) SELECT 1, 'n' || i, 0, 0 FROM n`, notesMax); err != nil {
		t.Fatal(err)
	}
	if code, _ := marie.Post("/notes", map[string]string{"titre": "De trop"}); code != http.StatusBadRequest {
		t.Fatalf("note au-delà du plafond acceptée : %d", code)
	}
	if _, page = marie.Get("/notes"); !strings.Contains(page, "les plus récentes") {
		t.Fatal("liste coupée sans le dire")
	}
}
