// SPDX-License-Identifier: GPL-3.0-or-later

package foyer

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"familycocon/internal/appli"
	"familycocon/internal/essai"
	"familycocon/internal/suivi"
)

// Documents des fiches (animal, véhicule, garantie) : joints par un parent,
// vus par tout le monde, effacés avec la fiche.
func TestDocumentsFiches(t *testing.T) {
	a, srv := essai.NouvelleApp(t, func(a *appli.App) {
		s := suivi.Brancher(a)
		Brancher(a, s)
	})
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	_, page := marie.Post("/animaux", map[string]string{"nom": "Rex", "espece": "chien"})
	id := essai.Trouver(t, page, `href="/animaux/(\d+)/modifier"`)
	if !strings.Contains(page, "📎 Documents") || !strings.Contains(page, `action="/joints/animaux/`+id+`"`) {
		t.Fatalf("bloc Documents absent de la fiche :\n%s", page)
	}
	pdf := essai.Fichier{Champ: "fichier", Nom: "vaccins.pdf", Contenu: []byte("%PDF-1.4\n%%EOF\n")}
	if code, _ := marie.PostFichiers("/joints/animaux/"+id, map[string]string{"retour": "/animaux/" + id}, pdf); code != 200 {
		t.Fatalf("envoi du document : %d", code)
	}
	// Léa (enfant) voit le document, sans pouvoir en ajouter.
	_, page = lea.Get("/animaux/" + id)
	if !strings.Contains(page, "vaccins.pdf") || strings.Contains(page, `action="/joints/animaux/`) {
		t.Fatalf("vue de l'enfant :\n%s", page)
	}
	if code, _ := lea.PostFichiers("/joints/animaux/"+id, nil, pdf); code != http.StatusForbidden {
		t.Fatalf("envoi par un enfant : %d", code)
	}
	// Fiche supprimée : le document part au ménage.
	marie.Post("/animaux/"+id+"/supprimer", nil)
	a.EffacerJointsOrphelins(context.Background())
	var n int
	if err := a.DB.QueryRow("SELECT count(*) FROM pieces_jointes").Scan(&n); err != nil || n != 0 {
		t.Fatalf("%d document(s) restés après la suppression (%v)", n, err)
	}
}
