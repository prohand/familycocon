// SPDX-License-Identifier: GPL-3.0-or-later

package foyer

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
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
	if strings.Contains(page, "Lyon</small>") {
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
	// Léa (enfant) : les véhicules lui sont fermés.
	lea.Get("/")
	if code, _ := lea.Get(suite + "100"); code != http.StatusNotFound {
		t.Fatalf("enfant : %d", code)
	}
	if code, _ := marie.Get("/suivi/vehicules/9999/pleins?depuis=100"); code != 404 {
		t.Fatalf("véhicule inconnu : %d", code)
	}
}

// Scan du ticket : les fichiers du lecteur (servis compressés, WebAssembly
// permis sur ces seules réponses), la lecture du texte (parents) et la
// photo jointe au plein.
func TestScanTicket(t *testing.T) {
	a, srv := essai.NouvelleApp(t, func(a *appli.App) {
		s := suivi.Brancher(a)
		Brancher(a, s)
	})
	// Fichiers du lecteur.
	get := func(adr string, gz bool) *http.Response {
		req, _ := http.NewRequest("GET", srv.URL+adr, nil)
		if gz {
			req.Header.Set("Accept-Encoding", "gzip")
		}
		rep, err := http.DefaultTransport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		return rep
	}
	rep := get("/ocr/v7/tesseract-core-simd-lstm.wasm", true)
	if rep.StatusCode != 200 || rep.Header.Get("Content-Encoding") != "gzip" || rep.Header.Get("Content-Type") != "application/wasm" ||
		!strings.Contains(rep.Header.Get("Content-Security-Policy"), "'wasm-unsafe-eval'") || !strings.Contains(rep.Header.Get("Cache-Control"), "immutable") {
		t.Fatalf("wasm : %d %v", rep.StatusCode, rep.Header)
	}
	rep.Body.Close()
	rep = get("/ocr/v7/tesseract.min.js", false)
	corps, _ := io.ReadAll(rep.Body)
	rep.Body.Close()
	if rep.StatusCode != 200 || rep.Header.Get("Content-Encoding") != "" || len(corps) < 10000 {
		t.Fatalf("js sans gzip : %d, %d octets", rep.StatusCode, len(corps))
	}
	if rep = get("/ocr/v7/fra.traineddata.gz", true); rep.StatusCode != 200 || rep.Header.Get("Content-Encoding") != "" {
		t.Fatalf("modèle : %d %v", rep.StatusCode, rep.Header)
	}
	rep.Body.Close()
	for _, adr := range []string{"/ocr/v7/inconnu.js", "/ocr/LICENCE-tesseract.js.txt", "/ocr/v7/", "/ocr/v7/tesseract.min.js.gz"} {
		if rep = get(adr, true); rep.StatusCode != 404 {
			t.Errorf("%s : %d", adr, rep.StatusCode)
		}
		rep.Body.Close()
	}
	// Le reste du site garde la politique stricte.
	if rep = get("/connexion", false); strings.Contains(rep.Header.Get("Content-Security-Policy"), "wasm") {
		t.Error("WebAssembly permis hors du lecteur")
	}
	rep.Body.Close()

	// Lecture du texte : parents seulement.
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	lire := func(n *essai.Navigateur, texte string) (int, string) {
		b, _ := json.Marshal(map[string]string{"texte": texte})
		req, _ := http.NewRequest("POST", srv.URL+"/suivi/ticket", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Jeton", n.Jeton)
		rep, err := n.C.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer rep.Body.Close()
		c, _ := io.ReadAll(rep.Body)
		return rep.StatusCode, string(c)
	}
	texte := "STATION TEST\nle 03/11/26\nQuantite = 30,00 L\nPrix unit. = 1,750 EUR\nMONTANT 52,50 EUR"
	if code, c := lire(marie, texte); code != 200 || !strings.Contains(c, `"litres":"30,00"`) || !strings.Contains(c, `"montant":"52,50"`) {
		t.Fatalf("lecture : %d %s", code, c)
	}
	lea.Get("/")
	if code, _ := lire(lea, texte); code != http.StatusForbidden {
		t.Fatalf("lecture par un enfant : %d", code)
	}

	// La photo du ticket part avec le plein.
	_, page := marie.Post("/vehicules", map[string]string{"nom": "Clio", "genre": "voiture"})
	id := essai.Trouver(t, page, `action="/vehicules/(\d+)/km"`)
	_, page = marie.Get("/vehicules/" + id + "?onglet=pleins")
	if !strings.Contains(page, "data-scanner-ticket") || !strings.Contains(page, `name="ticket"`) {
		t.Fatal("pas de bouton « Scanner le ticket »")
	}
	var img bytes.Buffer
	_ = jpeg.Encode(&img, image.NewRGBA(image.Rect(0, 0, 900, 1200)), nil)
	_, page = marie.Post("/suivi/vehicules/"+id+"/carnet", map[string]string{"libelle": "Plein", "jour": "2026-11-03", "km": "1000",
		"litres": "30", "montant": "52,50", "ticket": base64.StdEncoding.EncodeToString(img.Bytes())})
	if !strings.Contains(page, "/suivi/pieces/") {
		t.Fatal("photo du ticket non jointe au plein")
	}
	if l, _ := filepath.Glob(filepath.Join(a.DossierPieces(), "*", "*.jpg")); len(l) != 1 {
		t.Fatalf("%d photos sur le disque", len(l))
	}
	// Photo abîmée : le plein est gardé, on le dit.
	_, page = marie.Post("/suivi/vehicules/"+id+"/carnet", map[string]string{"libelle": "Plein", "jour": "2026-11-10", "km": "1500",
		"litres": "20", "ticket": "pas du base64 !"})
	if !strings.Contains(page, "photo du ticket non jointe") || !strings.Contains(page, "1 500 km") {
		t.Fatal("plein avec une photo abîmée")
	}
}

// Fuelio « Full = 2 » (partiel, niveau estimé) : repris comme partiel ; un
// plein déjà repris comme complet est corrigé, pas doublé ; aucune dépense
// dans la sauvegarde : le message le dit.
func TestFuelioPartielEstime(t *testing.T) {
	_, srv := essai.NouvelleApp(t, func(a *appli.App) {
		s := suivi.Brancher(a)
		Brancher(a, s)
	})
	marie := essai.Famille(t, srv)
	_, page := marie.Post("/vehicules", map[string]string{"nom": "Focus", "genre": "voiture"})
	id := essai.Trouver(t, page, `action="/vehicules/(\d+)/km"`)
	carnet := "/suivi/vehicules/" + id + "/carnet"
	// Déjà là, repris avant comme un plein complet.
	marie.Post(carnet, map[string]string{"libelle": "Plein", "jour": "2025-01-30", "km": "105189", "litres": "8,51", "montant": "15,01"})
	csv := "\"## Vehicle\"\n\"Name\",\"DistUnit\",\"FuelUnit\"\n\"Focus\",\"0\",\"0\"\n" +
		"\"## Log\"\n\"Date\",\"Odo (km)\",\"Fuel (L)\",\"Full\",\"Price\",\"UniqueId\"\n" +
		"\"2025-01-20\",\"104800\",\"40\",\"1\",\"70\",\"1\"\n" +
		"\"2025-01-30\",\"105189.0\",\"8.51\",\"2\",\"15.01\",\"2\"\n" +
		"\"## CostCategories\"\n\"CostTypeID\",\"Name\",\"priority\",\"color\"\n" +
		"\"## Costs\"\n\"CostTitle\",\"Date\",\"Odo\",\"CostTypeID\",\"Notes\",\"Cost\",\"isTemplate\",\"isIncome\",\"UniqueId\"\n"
	req, _ := http.NewRequest("POST", srv.URL+"/vehicules/"+id+"/fuelio", strings.NewReader(csv))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("HX-Request", "true")
	req.Header.Set("X-Jeton", marie.Jeton)
	rep, err := marie.C.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	rep.Body.Close()
	_, page = marie.Get("/vehicules/" + id + "?onglet=pleins")
	for _, attendu := range []string{"1 plein repris", "1 plein corrigé (partiel)", "aucune dépense (entretien) dans cette sauvegarde", "8,51 L · Plein partiel"} {
		if !strings.Contains(page, attendu) {
			t.Errorf("sans « %s »", attendu)
		}
	}
	if n := strings.Count(page, `<summary class="ligne">`); n != 2 {
		t.Errorf("plein doublé : %d lignes", n)
	}
}

// Import de tous les véhicules depuis la liste : celui qui existe reçoit
// ses pleins, celui qui manque est créé (immatriculation, modèle) et rangé
// dans les anciens si son dernier plein a plus d'un an.
func TestFuelioTousEtAnciens(t *testing.T) {
	_, srv := essai.NouvelleApp(t, func(a *appli.App) {
		s := suivi.Brancher(a)
		Brancher(a, s)
	})
	marie := essai.Famille(t, srv)
	_, page := marie.Post("/vehicules", map[string]string{"nom": "Mazda 3", "genre": "voiture"})
	mazda := essai.Trouver(t, page, `action="/vehicules/(\d+)/km"`)
	recent := time.Now().AddDate(0, -1, 0).Format("2006-01-02")
	vehicule := func(nom, plaque, marque, modele, annee string, pleins ...string) string {
		s := "\"## Vehicle\"\n\"Name\",\"Plate\",\"Make\",\"Model\",\"Year\"\n\"" + nom + "\",\"" + plaque + "\",\"" + marque + "\",\"" + modele + "\",\"" + annee + "\"\n" +
			"\"## Log\"\n\"Date\",\"Odo (km)\",\"Fuel (L)\",\"Full\",\"Price\",\"UniqueId\"\n"
		for i, j := range pleins {
			s += fmt.Sprintf("\"%s\",\"%d\",\"40\",\"1\",\"70\",\"%d\"\n", j, 1000+i*500, i+1)
		}
		return s
	}
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for nom, contenu := range map[string]string{
		"vehicle-1-local.csv": vehicule("Mazda 3", "fv-320-qa", "Mazda", "3", "2021", recent),
		"vehicle-2-local.csv": vehicule("Kia Rio", "", "Kia", "Rio", "0", "2015-07-04", "2018-07-28"),
	} {
		w, _ := z.Create(nom)
		_, _ = w.Write([]byte(contenu))
	}
	_ = z.Close()
	req, _ := http.NewRequest("POST", srv.URL+"/vehicules/fuelio", bytes.NewReader(b.Bytes()))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("HX-Request", "true")
	req.Header.Set("X-Jeton", marie.Jeton)
	rep, err := marie.C.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	rep.Body.Close()
	_, page = marie.Get("/vehicules")
	for _, attendu := range []string{"Mazda 3 : 1 plein repris", "Kia Rio (ajouté, rangé dans les anciens) : 2 pleins repris",
		"🏁 Anciens véhicules", "plus à nous depuis le 28/07/2018 · Kia Rio"} {
		if !strings.Contains(page, attendu) {
			t.Errorf("liste sans « %s »", attendu)
		}
	}
	kia := essai.Trouver(t, page, `href="/vehicules/(\d+)"><span class="grand-emoji">🚗</span><b>Kia Rio`)
	if _, p := marie.Get("/vehicules/" + kia); !strings.Contains(p, "🏁 Ancien véhicule") {
		t.Error("fiche sans le bandeau « ancien »")
	}
	// Le raccourci « Ajouter un plein » ne propose que les véhicules encore là.
	if _, p := marie.Get("/vehicules/plein"); !strings.Contains(p, `action="/vehicules/`+mazda+`/km"`) {
		t.Error("raccourci : pas sur la Mazda")
	}
	// Rendre la Kia active (case décochée) : elle revient dans la liste.
	marie.Post("/vehicules/"+kia, map[string]string{"nom": "Kia Rio", "genre": "voiture"})
	if _, p := marie.Get("/vehicules"); strings.Contains(p, "Anciens véhicules") {
		t.Error("Kia toujours dans les anciens")
	}
}
