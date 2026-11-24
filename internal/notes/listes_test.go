// SPDX-License-Identifier: GPL-3.0-or-later

package notes

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"familycocon/internal/agenda"
	"familycocon/internal/appli"
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

	// Une note avec des cases (bouton ☑ de l'éditeur) se lit comme une liste.
	marie.Get("/notes/nouvelle")
	_, page := marie.Post("/notes", map[string]string{"titre": "Valise", "texte": "[ ] pulls\n[ ] "})
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
	id := essai.Trouver(t, page, `data-editeur="(\d+)"`)
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

// Photo dans le texte : l'éditeur l'envoie (?place=1 : son numéro en
// JSON), écrit « [photo N] » ; elle s'affiche à cet endroit et plus en bas ;
// le repère d'une photo d'une autre note est retiré.
func TestPhotoDansLeTexte(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	marie.Get("/notes/nouvelle")
	_, page := marie.Post("/notes", map[string]string{"titre": "Potager", "texte": "Plan\n[photo 99]", "suite": "ouvrir"})
	id := essai.Trouver(t, page, `data-editeur="(\d+)"`)
	if strings.Contains(page, "[photo 99]") {
		t.Fatal("repère d'une photo inconnue gardé à la création")
	}
	_, page = marie.Post("/notes", map[string]string{"titre": "Autre", "texte": "x", "suite": "ouvrir"})
	autre := essai.Trouver(t, page, `data-editeur="(\d+)"`)
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 400, 300)))
	envoyer := func(note string) string {
		req, _ := http.NewRequest("POST", srv.URL+"/notes/"+note+"/photos?place=1", bytes.NewReader(b.Bytes()))
		req.Header.Set("Content-Type", "image/png")
		req.Header.Set("X-Jeton", marie.Jeton)
		rep, err := marie.C.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer rep.Body.Close()
		var r struct{ ID int64 }
		if err := json.NewDecoder(rep.Body).Decode(&r); err != nil || r.ID == 0 || rep.StatusCode != 200 {
			t.Fatalf("envoi placé : %d %v", rep.StatusCode, err)
		}
		return strconv.FormatInt(r.ID, 10)
	}
	ph, etrangere := envoyer(id), envoyer(autre)
	_, page = marie.Get("/notes/" + id)
	version := essai.Trouver(t, page, `name="version" value="(\d+)"`)
	marie.Post("/notes/"+id, map[string]string{"titre": "Potager", "version": version, "couleur": "1",
		"texte": "Plan\n[photo " + ph + "]\n[photo " + etrangere + "]\n[ ] Terreau"})
	_, page = marie.Get("/notes/" + id)
	if !strings.Contains(page, "[photo "+ph+"]") || strings.Contains(page, "[photo "+etrangere+"]") {
		t.Fatalf("repères mal gardés :\n%s", page)
	}
	// Note avec une case : lue en entier (texte, photo à sa place, case).
	if !strings.Contains(page, `<p class="texte-note">Plan</p>`) || !strings.Contains(page, `<figure class="photo-texte"><a href="/notes/photos/`+ph+`"`) {
		t.Fatalf("photo absente du texte :\n%s", page)
	}
	// Placée dans le texte : plus en bas de la note.
	if strings.Contains(page, `data-placer="`+ph+`"`) {
		t.Error("photo du texte aussi en bas")
	}
	if _, p := marie.Get("/notes"); !strings.Contains(p, "Plan\n📷\n☐ Terreau") {
		t.Errorf("aperçu de la carte :\n%s", p)
	}
	// Retirée du texte : elle revient en bas (pas effacée).
	_, page = marie.Get("/notes/" + id)
	version = essai.Trouver(t, page, `name="version" value="(\d+)"`)
	marie.Post("/notes/"+id, map[string]string{"titre": "Potager", "version": version, "couleur": "1", "texte": "Plan\n[ ] Terreau"})
	if _, p := marie.Get("/notes/" + id); !strings.Contains(p, `data-placer="`+ph+`"`) {
		t.Error("photo retirée du texte : pas revenue en bas")
	}
}

// Catégories : créées par tout le monde, une par note, filtre de la liste,
// étiquette sur la carte ; supprimée : ses notes restent, sans catégorie.
func TestCategories(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	marie.Get("/notes")
	if _, p := marie.Get("/notes"); strings.Contains(p, "＋ Liste") || strings.Contains(p, "voir=listes") {
		t.Fatal("ancien filtre ou bouton « ＋ Liste » encore là")
	}
	// Léa (enfant) crée une catégorie ; un nom vide est refusé.
	lea.Get("/notes/categories")
	if code, _ := lea.Post("/notes/categories", map[string]string{"nom": "  ", "emoji": "🚗"}); code != 400 {
		t.Fatalf("nom vide : %d", code)
	}
	if code, _ := lea.Post("/notes/categories", map[string]string{"nom": "Voiture", "emoji": "🚗"}); code != 200 {
		t.Fatalf("catégorie créée par un enfant : %d", code)
	}
	marie.Post("/notes/categories", map[string]string{"nom": "Maison", "emoji": "🏠"})
	_, p := marie.Get("/notes/categories")
	voiture := essai.Trouver(t, p, `name="id" value="(\d+)"><input type="hidden" name="action" value="modifier">\s*<span class="champ-emoji"><input type="hidden" name="emoji" value="🚗">`)
	// Grille des emojis proposée sur la page.
	if !strings.Contains(p, `<template id="grille-emojis">`) || !strings.Contains(p, `data-emoji="🏖️"`) {
		t.Fatal("grille des emojis absente")
	}
	// Une note rangée dans « Voiture », une sans catégorie.
	_, p = marie.Get("/notes/nouvelle?cat=" + voiture)
	if !strings.Contains(p, `<option value="`+voiture+`" selected>🚗 Voiture</option>`) {
		t.Fatalf("catégorie de la liste non proposée :\n%s", p)
	}
	marie.Post("/notes", map[string]string{"titre": "Pneus", "texte": "Continental", "categorie": voiture})
	marie.Post("/notes", map[string]string{"titre": "Wifi", "texte": "code"})
	// Une catégorie d'une autre famille (inconnue) : sans catégorie.
	marie.Post("/notes", map[string]string{"titre": "Intrus", "texte": "x", "categorie": "999"})
	_, p = marie.Get("/notes?cat=" + voiture)
	if !strings.Contains(p, "Pneus") || strings.Contains(p, "Wifi") || strings.Contains(p, "Intrus") || !strings.Contains(p, `<span class="etiquette-categorie">🚗 Voiture</span>`) {
		t.Fatalf("filtre Voiture :\n%s", p)
	}
	if _, p = marie.Get("/notes?cat=sans"); !strings.Contains(p, "Wifi") || !strings.Contains(p, "Intrus") || strings.Contains(p, "Pneus") {
		t.Fatal("filtre « Sans catégorie »")
	}
	// Renommée, puis supprimée : la note reste, sans catégorie.
	marie.Post("/notes/categories", map[string]string{"action": "modifier", "id": voiture, "nom": "Auto", "emoji": "🚙"})
	if _, p = marie.Get("/notes"); !strings.Contains(p, "🚙 Auto") {
		t.Fatal("catégorie non renommée")
	}
	marie.Post("/notes/categories", map[string]string{"action": "supprimer", "id": voiture})
	if _, p = marie.Get("/notes?cat=sans"); !strings.Contains(p, "Pneus") || strings.Contains(p, "🚙 Auto") {
		t.Fatal("note perdue ou catégorie restée après suppression")
	}
	_ = a
}

// Listes : cochés en bas (affichage), ranger (modifier, déplacer,
// supprimer), rappel envoyé une fois à son heure.
func TestRangerEtRappel(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	marie.Get("/notes/nouvelle")
	_, page := marie.Post("/notes", map[string]string{"titre": "Valise", "texte": "[x] crème\n[ ] maillot\n[ ] livre"})
	id := essai.Trouver(t, page, `/notes/(\d+)/cochees`)
	// Cochés en bas (par défaut) : maillot, livre, puis crème.
	if i, j := strings.Index(page, "maillot"), strings.Index(page, "crème"); i < 0 || j < i || !strings.Contains(page, "<b>1/3</b> faits") {
		t.Fatalf("cochés en bas :\n%s", page)
	}
	_, p := marie.Post("/notes/"+id+"/cochees", map[string]string{"en_bas": "0"})
	if i, j := strings.Index(p, "maillot"), strings.Index(p, "crème"); j > i {
		t.Fatal("cochés à leur place : crème devrait être en tête")
	}
	// Ranger : un enfant ne peut pas.
	if code, _ := lea.Get("/notes/liste/" + id + "?ranger=1"); code != 200 {
		t.Fatalf("liste vue par Léa : %d", code)
	}
	if code, _ := lea.Post("/notes/elements/"+id+"/1", map[string]string{"avant": "[ ] maillot", "texte": "x"}); code != http.StatusForbidden {
		t.Fatalf("élément modifié par un enfant : %d", code)
	}
	texte := func() string {
		_, p := marie.Get("/notes/" + id)
		return essai.Trouver(t, p, `(?s)<textarea name="texte"[^>]*>(.*?)</textarea>`)
	}
	marie.Post("/notes/elements/"+id+"/1", map[string]string{"avant": "[ ] maillot", "texte": "maillot *2"})
	marie.Post("/notes/elements/"+id+"/2/monter", map[string]string{"avant": "[ ] livre"})
	marie.Post("/notes/elements/"+id+"/0/supprimer", map[string]string{"avant": "[x] crème"})
	if got := texte(); got != "[ ] livre\n[ ] maillot \\*2" {
		t.Fatalf("après rangement : %q", got)
	}
	// Ligne qui a changé entre-temps : rien n'est touché.
	if code, _ := marie.Post("/notes/elements/"+id+"/0/supprimer", map[string]string{"avant": "[x] crème"}); code != http.StatusConflict {
		t.Fatalf("ligne périmée : %d", code)
	}
	// Rappel : dans le passé refusé ; à venir accepté, montré, envoyé une fois.
	if code, _ := marie.Post("/notes/"+id+"/rappel", map[string]string{"rappel": "2020-01-01T10:00"}); code != 400 {
		t.Fatalf("rappel passé : %d", code)
	}
	quand := time.Now().Add(2 * time.Hour)
	marie.Post("/notes/"+id+"/rappel", map[string]string{"rappel": quand.In(time.Local).Format("2006-01-02T15:04")})
	if _, p := marie.Get("/notes"); !strings.Contains(p, "⏰ ") {
		t.Fatal("rappel absent de la carte")
	}
	n, _ := strconv.ParseInt(id, 10, 64)
	a.TachesMinute(context.Background(), time.Now())
	if l, _ := a.Q.NotesARappeler(context.Background(), quand.Add(time.Hour).Unix()); len(l) != 1 || l[0].ID != n {
		t.Fatalf("rappel parti trop tôt : %v", l)
	}
	a.TachesMinute(context.Background(), quand.Add(time.Minute))
	if l, _ := a.Q.NotesARappeler(context.Background(), quand.Add(time.Hour).Unix()); len(l) != 0 {
		t.Fatal("rappel pas marqué envoyé")
	}
}

// « Parents seulement » : une note, ou toute une catégorie, cachée aux enfants.
func TestParentsSeulement(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	idNote := func(p, titre string) string {
		t.Helper()
		return essai.Trouver(t, p, `href="/notes/(\d+)">\s*(?:<span class="etiquette-categorie">[^<]*</span>\s*)?<h3>`+titre+`</h3>`)
	}

	// Une note des parents : Léa ne la voit ni dans la liste, ni par la
	// recherche, ni par son adresse.
	marie.Post("/notes", map[string]string{"titre": "Cadeaux de Noel", "texte": "velo", "parents": "1"})
	marie.Post("/notes", map[string]string{"titre": "Wifi", "texte": "code"})
	_, p := marie.Get("/notes")
	cadeaux := idNote(p, "Cadeaux de Noel")
	if !strings.Contains(p, `title="Parents seulement">👥</span>`) {
		t.Fatal("repère « parents seulement » absent de la carte")
	}
	if _, p = lea.Get("/notes"); strings.Contains(p, "Cadeaux") || !strings.Contains(p, "Wifi") {
		t.Fatalf("liste de Léa :\n%s", p)
	}
	if _, p = lea.Get("/notes?q=velo"); strings.Contains(p, "Cadeaux") {
		t.Fatal("trouvée par la recherche d'un enfant")
	}
	if code, _ := lea.Get("/notes/" + cadeaux); code != http.StatusNotFound {
		t.Fatalf("ouverte par un enfant : %d", code)
	}
	// Un enfant n'a pas la case et ne peut pas l'imposer.
	if _, p = lea.Get("/notes/nouvelle"); strings.Contains(p, `name="parents"`) {
		t.Fatal("case « Parents seulement » proposée à un enfant")
	}
	lea.Post("/notes", map[string]string{"titre": "Mon dessin", "texte": "x", "parents": "1"})
	if _, p = marie.Get("/notes"); !strings.Contains(p, "Mon dessin") {
		t.Fatal("note d'un enfant cachée")
	}
	dessin := idNote(p, "Mon dessin")

	// Une catégorie des parents : cachée aux enfants, avec ses notes.
	marie.Post("/notes/categories", map[string]string{"nom": "Papiers", "emoji": "📄", "parents": "1"})
	lea.Post("/notes/categories", map[string]string{"nom": "Jeux", "emoji": "🎮", "parents": "1"}) // case ignorée
	_, p = marie.Get("/notes/categories")
	papiers := essai.Trouver(t, p, `name="id" value="(\d+)"><input type="hidden" name="action" value="modifier">\s*<span class="champ-emoji"><input type="hidden" name="emoji" value="📄">`)
	if !strings.Contains(p, `aria-pressed="true"`) || !strings.Contains(p, "Jeux") {
		t.Fatalf("catégories vues par un parent :\n%s", p)
	}
	marie.Post("/notes", map[string]string{"titre": "Impots", "texte": "avis", "categorie": papiers})
	if _, p = lea.Get("/notes/categories"); strings.Contains(p, "Papiers") || !strings.Contains(p, "Jeux") {
		t.Fatalf("catégories vues par Léa :\n%s", p)
	}
	if _, p = lea.Get("/notes"); strings.Contains(p, "Impots") || strings.Contains(p, "Papiers") {
		t.Fatal("catégorie ou note des parents vue par un enfant")
	}
	// Léa ne peut ni s'en servir, ni la changer, ni la supprimer.
	lea.Post("/notes", map[string]string{"titre": "Devoirs", "texte": "x", "categorie": papiers})
	lea.Post("/notes/categories", map[string]string{"action": "modifier", "id": papiers, "nom": "Pris", "emoji": ""})
	lea.Post("/notes/categories", map[string]string{"action": "supprimer", "id": papiers})
	if code, _ := lea.Post("/notes/categories", map[string]string{"action": "parents", "id": papiers, "parents": "0"}); code != http.StatusForbidden {
		t.Fatalf("réglage changé par un enfant : %d", code)
	}
	if _, p = marie.Get("/notes?cat=" + papiers); !strings.Contains(p, "📄 Papiers") || !strings.Contains(p, "Impots") || strings.Contains(p, "Devoirs") {
		t.Fatalf("catégorie touchée par un enfant :\n%s", p)
	}
	// Une note de Léa rangée par un parent chez les parents : elle la voit
	// toujours, et la garde dans cette catégorie quand elle la modifie.
	marie.Post("/notes/"+dessin, map[string]string{"titre": "Mon dessin", "texte": "x", "categorie": papiers})
	if code, _ := lea.Get("/notes/" + dessin); code != http.StatusOK {
		t.Fatalf("son auteur ne la voit plus : %d", code)
	}
	lea.Post("/notes/"+dessin, map[string]string{"titre": "Mon dessin", "texte": "y", "categorie": "0"})
	if _, p = marie.Get("/notes?cat=" + papiers); !strings.Contains(p, "Mon dessin") {
		t.Fatal("note sortie de la catégorie des parents par un enfant")
	}
	// Rappel d'une note cachée : repéré comme tel (envoyé aux parents seuls).
	quand := time.Now().Add(48 * time.Hour)
	marie.Post("/notes/"+cadeaux+"/rappel", map[string]string{"rappel": quand.Format("2006-01-02T15:04")})
	l, err := a.Q.NotesARappeler(context.Background(), quand.Add(24*time.Hour).Unix())
	if err != nil || len(l) != 1 || strconv.FormatInt(l[0].ID, 10) != cadeaux || l[0].Cachee != 1 {
		t.Fatalf("rappel d'une note cachée : %+v %v", l, err)
	}

	// Rendue à toute la famille : Léa la voit.
	marie.Post("/notes/categories", map[string]string{"action": "parents", "id": papiers, "parents": "0"})
	if _, p = lea.Get("/notes"); !strings.Contains(p, "Impots") {
		t.Fatal("catégorie rendue à tous : note toujours cachée")
	}
}

// Filtre « Partagées / Privées », gardé avec la catégorie ; rappels des
// notes dans l'agenda (seulement ceux qu'on voit).
func TestPartageesPriveesEtAgenda(t *testing.T) {
	_, srv := essai.NouvelleApp(t, func(a *appli.App) { Brancher(a); agenda.Brancher(a) })
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	marie.Post("/notes/categories", map[string]string{"nom": "Maison", "emoji": "🏠"})
	_, p := marie.Get("/notes/categories")
	maison := essai.Trouver(t, p, `name="id" value="(\d+)"><input type="hidden" name="action" value="modifier">`)
	marie.Post("/notes", map[string]string{"titre": "Wifi", "texte": "code", "categorie": maison})
	marie.Post("/notes", map[string]string{"titre": "Journal", "texte": "secret", "privee": "1", "categorie": maison})
	marie.Post("/notes", map[string]string{"titre": "Idees", "texte": "x"})
	if _, p = marie.Get("/notes?voir=partagees"); !strings.Contains(p, "Wifi") || !strings.Contains(p, "Idees") || strings.Contains(p, "Journal") {
		t.Fatalf("filtre Partagées :\n%s", p)
	}
	if _, p = marie.Get("/notes?voir=privees&cat=" + maison); !strings.Contains(p, "Journal") || strings.Contains(p, "Wifi") || strings.Contains(p, "Idees") {
		t.Fatal("filtre Privées + catégorie")
	}
	// « Vues des enfants » (parents seulement) : ni privées, ni réservées aux parents.
	marie.Post("/notes", map[string]string{"titre": "Surprise", "texte": "x", "parents": "1"})
	if _, q := marie.Get("/notes?voir=enfants"); !strings.Contains(q, "Wifi") || !strings.Contains(q, "Idees") || strings.Contains(q, "Journal") || strings.Contains(q, "Surprise") {
		t.Fatalf("filtre « Vues des enfants » :\n%s", q)
	}
	if _, q := lea.Get("/notes"); strings.Contains(q, "voir=enfants") {
		t.Fatal("filtre « Vues des enfants » proposé à un enfant")
	}
	// Les liens gardent l'autre filtre.
	if !strings.Contains(p, `href="/notes?voir=partagees&amp;cat=`+maison+`"`) || !strings.Contains(p, `href="/notes?voir=privees&amp;cat=sans"`) {
		t.Fatalf("liens des filtres :\n%s", p)
	}
	// Rappels : dans l'agenda de chacun, sauf la note privée chez Léa.
	_, p = marie.Get("/notes")
	quand := time.Now().Add(48 * time.Hour)
	for _, titre := range []string{"Wifi", "Journal"} {
		id := essai.Trouver(t, p, `href="/notes/(\d+)">\s*(?:<span class="etiquette-categorie">[^<]*</span>\s*)?<h3>`+titre+`</h3>`)
		marie.Post("/notes/"+id+"/rappel", map[string]string{"rappel": quand.Format("2006-01-02T15:04")})
	}
	if _, p = marie.Get("/agenda?vue=semaine&date=" + quand.Format("2006-01-02")); !strings.Contains(p, "Wifi") || !strings.Contains(p, "🔒") {
		t.Fatalf("rappels absents de l'agenda :\n%s", p)
	}
	if _, p = lea.Get("/agenda?vue=semaine&date=" + quand.Format("2006-01-02")); !strings.Contains(p, "Wifi") || strings.Contains(p, "Journal") {
		t.Fatal("agenda de Léa : note privée de Marie montrée, ou rappel partagé absent")
	}
}
