// SPDX-License-Identifier: GPL-3.0-or-later

package foyer

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/essai"
	"familycocon/internal/suivi"
)

// sauvegardeFuelio : un faux fichier .fuelio (ZIP) : le CSV, la photo du
// ticket du premier plein (nommée par son identifiant) et une photo
// qu'aucune ligne ne réclame.
func sauvegardeFuelio(t *testing.T) []byte {
	t.Helper()
	csv := "\"## Vehicle\"\n\"Name\",\"DistUnit\"\n\"Clio\",\"0\"\n" +
		"\"## Log\"\n\"Data\",\"Odo (km)\",\"Fuel (litres)\",\"Full\",\"Price (optional)\",\"City (optional)\",\"Notes (optional)\",\"UniqueId\"\n" +
		"\"2024-03-15 18:22\",\"45210\",\"42.37\",\"1\",\"76.21\",\"Lyon\",\"\",\"1710523320000\"\n" +
		"\"2024-04-02 08:05\",\"45890\",\"40\",\"1\",\"72\",\"\",\"\",\"1712037900000\"\n" +
		"\"## CostCategories\"\n\"CostTypeID\",\"Name\"\n\"1\",\"Entretien\"\n" +
		"\"## Costs\"\n\"CostTitle\",\"Date\",\"Odo\",\"CostTypeID\",\"Notes\",\"Cost\",\"isTemplate\",\"isIncome\",\"UniqueId\"\n" +
		"\"Vidange\",\"2024-03-20\",\"45500\",\"1\",\"\",\"89.90\",\"0\",\"0\",\"555\"\n"
	var photo bytes.Buffer
	_ = jpeg.Encode(&photo, image.NewRGBA(image.Rect(0, 0, 2400, 1200)), nil)
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for nom, contenu := range map[string][]byte{
		"vehicle-1-sync.csv":       []byte(csv),
		"images/1710523320000.jpg": photo.Bytes(),
		"images/autre.jpg":         photo.Bytes(),
	} {
		w, err := z.Create(nom)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(contenu)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestRepriseFuelio(t *testing.T) {
	a, srv := essai.NouvelleApp(t, func(a *appli.App) {
		s := suivi.Brancher(a)
		Brancher(a, s)
	})
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	_, page := marie.Post("/vehicules", map[string]string{"nom": "Clio", "genre": "voiture"})
	id := essai.Trouver(t, page, `action="/vehicules/(\d+)/km"`)
	if !strings.Contains(page, `data-fuelio="/vehicules/`+id+`/fuelio"`) {
		t.Fatal("pas de reprise Fuelio sur la fiche")
	}
	envoyer := func(n *essai.Navigateur, corps []byte) (int, string) {
		req, _ := http.NewRequest("POST", srv.URL+"/vehicules/"+id+"/fuelio", bytes.NewReader(corps))
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("HX-Request", "true")
		req.Header.Set("X-Jeton", n.Jeton)
		rep, err := n.C.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		rep.Body.Close()
		return rep.StatusCode, rep.Header.Get("HX-Trigger")
	}
	fichier := sauvegardeFuelio(t)
	_, page = lea.Get("/vehicules/" + id)
	if strings.Contains(page, "data-fuelio") {
		t.Fatal("reprise Fuelio proposée à un enfant")
	}
	if code, _ := envoyer(lea, fichier); code != 403 {
		t.Fatalf("reprise par un enfant : %d", code)
	}
	if code, _ := envoyer(marie, []byte("n'importe quoi")); code != 400 {
		t.Fatalf("fichier quelconque : %d", code)
	}
	if code, msg := envoyer(marie, fichier); code != 204 {
		t.Fatalf("reprise : %d %s", code, msg)
	}
	_, page = marie.Get("/vehicules/" + id)
	for _, attendu := range []string{"2 pleins et 1 dépense repris", "1 photo jointe", "1 photo laissée de côté", "Vidange", "45\u202f890 km"} {
		if !strings.Contains(page, attendu) {
			t.Errorf("fiche sans « %s »", attendu)
		}
	}
	// Les pleins sont dans leur onglet, pas dans l'entretien.
	if strings.Contains(page, "Lyon") {
		t.Error("plein affiché dans l'onglet entretien")
	}
	if _, p := marie.Get("/vehicules/" + id + "?onglet=pleins&periode=tout"); !strings.Contains(p, "Lyon") || strings.Contains(p, "Vidange</span>") {
		t.Error("onglet pleins")
	}
	fichiers, _ := filepath.Glob(filepath.Join(a.Config.Donnees, "pieces", "*", "*.jpg"))
	if len(fichiers) != 1 {
		t.Fatalf("%d photos sur le disque", len(fichiers))
	}
	// Le fichier envoyé ne reste pas sur le disque.
	if l, _ := filepath.Glob(filepath.Join(a.Config.Donnees, "fuelio-*")); len(l) != 0 {
		t.Fatalf("fichier temporaire gardé : %v", l)
	}
	// Deuxième fois : rien en double, ni ligne ni photo.
	if code, _ := envoyer(marie, fichier); code != 204 {
		t.Fatalf("deuxième reprise : %d", code)
	}
	_, page = marie.Get("/vehicules/" + id)
	if !strings.Contains(page, "rien de nouveau · 3 déjà là") {
		t.Error("doublons à la deuxième reprise")
	}
	if fichiers, _ = filepath.Glob(filepath.Join(a.Config.Donnees, "pieces", "*", "*.jpg")); len(fichiers) != 1 {
		t.Fatalf("%d photos après la deuxième reprise", len(fichiers))
	}
}

// Plus de 100 pleins : « Voir les pleins plus anciens » charge les pages
// suivantes jusqu'au dernier.
func TestPleinsPlusAnciens(t *testing.T) {
	_, srv := essai.NouvelleApp(t, func(a *appli.App) {
		s := suivi.Brancher(a)
		Brancher(a, s)
	})
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	_, page := marie.Post("/vehicules", map[string]string{"nom": "Clio", "genre": "voiture"})
	id := essai.Trouver(t, page, `action="/vehicules/(\d+)/km"`)
	var csv strings.Builder
	csv.WriteString("\"## Log\"\n\"Date\",\"Odo (km)\",\"Fuel (L)\",\"Full\",\"Price\",\"UniqueId\"\n")
	debut := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 205; i++ {
		fmt.Fprintf(&csv, "\"%s 10:00\",\"%d\",\"40\",\"1\",\"70\",\"%d\"\n", debut.AddDate(0, 0, 7*i).Format("2006-01-02"), 10000+600*i, i+1)
	}
	req, _ := http.NewRequest("POST", srv.URL+"/vehicules/"+id+"/fuelio", strings.NewReader(csv.String()))
	req.Header.Set("HX-Request", "true")
	req.Header.Set("X-Jeton", marie.Jeton)
	rep, err := marie.C.Do(req)
	if err != nil || rep.StatusCode != 204 {
		t.Fatalf("reprise : %v %v", err, rep.StatusCode)
	}
	rep.Body.Close()
	_, page = marie.Get("/vehicules/" + id + "?onglet=pleins")
	suite := "/suivi/vehicules/" + id + "/pleins?depuis="
	if n := strings.Count(page, `class="heure"`); n != 100 || !strings.Contains(page, suite+"100") {
		t.Fatalf("première page : %d pleins", n)
	}
	_, page = marie.Get(suite + "100")
	if n := strings.Count(page, `class="heure"`); n != 100 || !strings.Contains(page, suite+"200") || strings.Contains(page, "<html") {
		t.Fatalf("deuxième page : %d pleins", n)
	}
	_, page = marie.Get(suite + "200")
	if n := strings.Count(page, `class="heure"`); n != 5 || strings.Contains(page, "plus anciens") || !strings.Contains(page, "08/01") {
		t.Fatalf("dernière page : %d pleins", n)
	}
	// Léa voit les pleins plus anciens, sans les prix.
	lea.Get("/vehicules/" + id)
	if code, p := lea.Get(suite + "100"); code != 200 || strings.Contains(p, "70,00") || !strings.Contains(p, "40 L") {
		t.Fatalf("enfant : %d", code)
	}
	if code, _ := marie.Get("/suivi/vehicules/9999/pleins?depuis=100"); code != 404 {
		t.Fatalf("véhicule inconnu : %d", code)
	}
}
