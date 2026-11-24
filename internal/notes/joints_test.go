// SPDX-License-Identifier: GPL-3.0-or-later

package notes

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"familycocon/internal/essai"
)

// Pièces jointes d'une note : envoi, droits, type vérifié, téléchargement,
// suppression, ménage quand la note disparaît.
func TestJointsNotes(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	marie.Post("/notes", map[string]string{"titre": "Assurance", "texte": "contrat"})
	marie.Post("/notes", map[string]string{"titre": "Secret", "texte": "x", "privee": "1"})
	_, p := marie.Get("/notes")
	assurance := essai.Trouver(t, p, `href="/notes/(\d+)">\s*<h3>Assurance</h3>`)
	secret := essai.Trouver(t, p, `href="/notes/(\d+)">\s*<h3>Secret</h3>`)

	pdf := []byte("%PDF-1.4\n% contrat\n" + strings.Repeat("x", 3000))
	_, p = marie.Get("/notes/" + assurance)
	if !strings.Contains(p, `action="/joints/notes/`+assurance+`"`) {
		t.Fatalf("bouton « Joindre un fichier » absent :\n%s", p)
	}
	code, _ := marie.PostFichiers("/joints/notes/"+assurance, map[string]string{"retour": "/notes/" + assurance},
		essai.Fichier{Champ: "fichier", Nom: "Contrat 2026.pdf", Contenu: pdf},
		essai.Fichier{Champ: "fichier", Nom: "liste.csv", Contenu: []byte("nom;prix\npain;1\n")})
	if code != http.StatusOK {
		t.Fatalf("envoi : %d", code)
	}
	_, p = marie.Get("/notes/" + assurance)
	id := essai.Trouver(t, p, `href="/joints/(\d+)/Contrat%202026.pdf"`)
	if !strings.Contains(p, "liste.csv") || !strings.Contains(p, "📄") {
		t.Fatalf("pièces non montrées :\n%s", p)
	}
	if _, p = marie.Get("/notes"); !strings.Contains(p, "📎 2") {
		t.Fatal("nombre de pièces absent de la carte")
	}
	// Refusés : une page web déguisée, un programme, un faux PDF.
	for _, f := range []essai.Fichier{
		{Champ: "fichier", Nom: "piege.html", Contenu: []byte("<script>alert(1)</script>")},
		{Champ: "fichier", Nom: "jeu.exe", Contenu: []byte("MZ\x90\x00")},
		{Champ: "fichier", Nom: "faux.pdf", Contenu: []byte("<html><script>x</script>")},
		{Champ: "fichier", Nom: "page.txt", Contenu: []byte("<!DOCTYPE html><html><script>x</script></html>")},
	} {
		if code, _ := marie.PostFichiers("/joints/notes/"+assurance, nil, f); code != http.StatusBadRequest {
			t.Errorf("%s accepté : %d", f.Nom, code)
		}
	}
	// Trop gros (10 Mo au plus).
	gros := append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("x"), 10<<20)...)
	if code, _ := marie.PostFichiers("/joints/notes/"+assurance, nil, essai.Fichier{Champ: "fichier", Nom: "gros.pdf", Contenu: gros}); code != http.StatusRequestEntityTooLarge {
		t.Errorf("fichier de plus de 10 Mo accepté : %d", code)
	}

	// Téléchargement : PDF ouvert dans le navigateur, type choisi par le serveur.
	rep, err := marie.C.Get(srv.URL + "/joints/" + id + "/x")
	if err != nil {
		t.Fatal(err)
	}
	rep.Body.Close()
	if rep.StatusCode != 200 || rep.Header.Get("Content-Type") != "application/pdf" || !strings.HasPrefix(rep.Header.Get("Content-Disposition"), "inline") ||
		rep.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("téléchargement : %d %v", rep.StatusCode, rep.Header)
	}
	// Léa voit la note partagée (elle télécharge), mais ne peut ni ajouter ni retirer.
	if code, _ := lea.Get("/joints/" + id + "/x"); code != 200 {
		t.Fatalf("Léa ne télécharge pas : %d", code)
	}
	if code, _ := lea.PostFichiers("/joints/notes/"+assurance, nil, essai.Fichier{Champ: "fichier", Nom: "a.pdf", Contenu: pdf}); code != http.StatusForbidden {
		t.Fatalf("Léa ajoute une pièce : %d", code)
	}
	if code, _ := lea.Post("/joints/"+id+"/supprimer", nil); code != http.StatusForbidden {
		t.Fatalf("Léa retire une pièce : %d", code)
	}
	// Note privée de Marie : Léa ne voit pas ses pièces.
	marie.PostFichiers("/joints/notes/"+secret, nil, essai.Fichier{Champ: "fichier", Nom: "journal.pdf", Contenu: pdf})
	_, p = marie.Get("/notes/" + secret)
	idSecret := essai.Trouver(t, p, `href="/joints/(\d+)/journal.pdf"`)
	if code, _ := lea.Get("/joints/" + idSecret + "/x"); code != http.StatusNotFound {
		t.Fatalf("pièce d'une note privée téléchargée par Léa : %d", code)
	}
	// Un module inconnu : refusé.
	if code, _ := marie.PostFichiers("/joints/inconnu/1", nil, essai.Fichier{Champ: "fichier", Nom: "a.pdf", Contenu: pdf}); code != http.StatusForbidden {
		t.Fatalf("module inconnu : %d", code)
	}
	// Retirée par Marie.
	if code, _ := marie.Post("/joints/"+id+"/supprimer", map[string]string{"retour": "/notes/" + assurance}); code != 200 {
		t.Fatalf("retrait : %d", code)
	}
	if code, _ := marie.Get("/joints/" + id + "/x"); code != http.StatusNotFound {
		t.Fatal("pièce retirée encore là")
	}
	// Note supprimée : ses pièces partent au ménage.
	marie.Post("/notes/"+secret+"/supprimer", nil)
	a.EffacerJointsOrphelins(context.Background())
	if code, _ := marie.Get("/joints/" + idSecret + "/x"); code != http.StatusNotFound {
		t.Fatal("pièce d'une note supprimée encore là")
	}
}
