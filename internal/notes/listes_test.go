// SPDX-License-Identifier: GPL-3.0-or-later

package notes

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"familycocon/internal/essai"
)

func TestAjouterCaseEtNettoyer(t *testing.T) {
	cas := []struct{ texte, element, attendu string }{
		{"", "lait", "[ ] lait"},
		{"Courses :", "lait", "Courses :\n[ ] lait"},
		{"Courses :\n- [ ] pain\n- [x] oeufs\nMerci", "lait", "Courses :\n- [ ] pain\n- [x] oeufs\n- [ ] lait\nMerci"},
	}
	for _, c := range cas {
		if got := ajouterCase(c.texte, c.element); got != c.attendu {
			t.Errorf("ajouterCase(%q) = %q, attendu %q", c.texte, got, c.attendu)
		}
	}
	if got, n := retirerCochees("A\n[ ] pain\n[x] oeufs\n[X] lait"); got != "A\n[ ] pain" || n != 2 {
		t.Errorf("retirerCochees : %q, %d", got, n)
	}
	if got := sansCasesVides("[ ] pain\n[ ] \n[ ]"); got != "[ ] pain" {
		t.Errorf("sansCasesVides : %q", got)
	}
	if got := apercuCases("Valise\n- [ ] pulls\n[x] maillot"); got != "Valise\n☐ pulls\n☑ maillot" {
		t.Errorf("apercuCases : %q", got)
	}
}

func TestListes(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")

	// « ☑️ Liste » : le formulaire commence par une case.
	_, page := marie.Get("/notes/nouvelle?liste=1")
	if !strings.Contains(page, "Nouvelle liste") || !strings.Contains(page, ">[ ] </textarea>") {
		t.Fatalf("formulaire de liste :\n%s", page)
	}
	// La liste enregistrée s'ouvre aussitôt ; la case vide du bout est retirée.
	_, page = marie.Post("/notes", map[string]string{"titre": "Valise", "texte": "[ ] pulls\n[ ] "})
	id := essai.Trouver(t, page, `action="/notes/(\d+)/element"`)
	// Léa (enfant) ajoute un élément et coche ; elle ne retire pas les cochés.
	lea.Get("/notes/" + id)
	if code, _ := lea.Post("/notes/"+id+"/element", map[string]string{"element": "  maillot   de bain "}); code != 200 {
		t.Fatalf("ajout par un enfant : %d", code)
	}
	lea.Post("/notes/"+id+"/cocher", map[string]string{"ligne": "0", "cochee": "1"})
	if code, _ := lea.Post("/notes/"+id+"/nettoyer", nil); code != http.StatusForbidden {
		t.Fatalf("nettoyage par un enfant : %d", code)
	}
	if code, _ := lea.Post("/notes/"+id+"/element", map[string]string{"element": " "}); code != 400 {
		t.Fatalf("élément vide : %d", code)
	}
	_, page = marie.Get("/notes/" + id)
	if !strings.Contains(page, "maillot de bain") || !strings.Contains(page, "Retirer les éléments cochés (1)") {
		t.Fatalf("liste après ajout :\n%s", page)
	}
	// La carte de la liste dessine les cases.
	if _, p := marie.Get("/notes"); !strings.Contains(p, "☑ pulls") || !strings.Contains(p, "☐ maillot de bain") {
		t.Fatal("aperçu des cases")
	}
	marie.Post("/notes/"+id+"/nettoyer", nil)
	if _, p := marie.Get("/notes/" + id); strings.Contains(p, "pulls") || !strings.Contains(p, "maillot de bain") {
		t.Fatal("éléments cochés non retirés")
	}
}

func TestPhotosNotes(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	marie.Get("/notes/nouvelle")
	_, page := marie.Post("/notes", map[string]string{"titre": "Plan du camping", "texte": "allée B", "suite": "ouvrir"})
	id := essai.Trouver(t, page, `data-piece="/notes/(\d+)/photos"`)
	marie.Post("/notes", map[string]string{"titre": "Secret", "texte": "x", "privee": "1", "suite": "ouvrir"})
	_, page = marie.Get("/notes?voir=privees")
	idPrivee := essai.Trouver(t, page, `href="/notes/(\d+)"`)

	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2000, 1000)))
	envoyer := func(n *essai.Navigateur, note string, corps []byte) int {
		req, _ := http.NewRequest("POST", srv.URL+"/notes/"+note+"/photos", bytes.NewReader(corps))
		req.Header.Set("Content-Type", "image/png")
		req.Header.Set("X-Jeton", n.Jeton)
		rep, err := n.C.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		rep.Body.Close()
		return rep.StatusCode
	}
	lea.Get("/notes")
	if code := envoyer(lea, id, b.Bytes()); code != http.StatusForbidden {
		t.Fatalf("photo ajoutée par un enfant à la note d'un parent : %d", code)
	}
	if code := envoyer(marie, id, []byte("pas une image")); code != 400 {
		t.Fatalf("fichier quelconque : %d", code)
	}
	if code := envoyer(marie, id, b.Bytes()); code != 200 {
		t.Fatalf("envoi : %d", code)
	}
	if code := envoyer(marie, idPrivee, b.Bytes()); code != 200 {
		t.Fatalf("envoi (note privée) : %d", code)
	}
	_, page = marie.Get("/notes/" + id)
	ph := essai.Trouver(t, page, `<img src="/notes/photos/(\d+)"`)
	_, page = marie.Get("/notes/" + idPrivee)
	phPrivee := essai.Trouver(t, page, `<img src="/notes/photos/(\d+)"`)
	// Léa voit la photo de la note partagée, pas celle de la note privée.
	if code, corps := lea.Get("/notes/photos/" + ph); code != 200 || !strings.HasPrefix(corps, "\xff\xd8") {
		t.Fatalf("photo d'une note partagée : %d", code)
	}
	if code, _ := lea.Get("/notes/photos/" + phPrivee); code != 404 {
		t.Fatalf("photo d'une note privée vue par un autre : %d", code)
	}
	if _, p := marie.Get("/notes"); !strings.Contains(p, "📷 1") {
		t.Fatal("nombre de photos sur la carte")
	}
	fichiers := func() int {
		l, _ := filepath.Glob(filepath.Join(a.DossierPieces(), "*", "*.jpg"))
		return len(l)
	}
	if n := fichiers(); n != 2 {
		t.Fatalf("%d photos sur le disque", n)
	}
	if code, _ := lea.Post("/notes/photos/"+ph+"/supprimer", nil); code != http.StatusForbidden {
		t.Fatalf("photo retirée par un enfant : %d", code)
	}
	marie.Post("/notes/photos/"+ph+"/supprimer", nil)
	marie.Post("/notes/"+idPrivee+"/supprimer", nil)
	if n := fichiers(); n != 0 {
		t.Fatalf("%d photos restées sur le disque", n)
	}
}
