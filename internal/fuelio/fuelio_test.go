// SPDX-License-Identifier: GPL-3.0-or-later

package fuelio

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// csvEssai : une sauvegarde Fuelio inventée, au format du CSV de synchro.
const csvEssai = "\ufeff\"## Vehicle\"\n" +
	"\"Name\",\"Description\",\"DistUnit\",\"FuelUnit\"\n" +
	"\"Clio\",\"\",\"0\",\"0\"\n" +
	"\"## Log\"\n" +
	"\"Data\",\"Odo (km)\",\"Fuel (litres)\",\"Full\",\"Price (optional)\",\"l/100km (optional)\",\"latitude (optional)\",\"longitude (optional)\",\"City (optional)\",\"Notes (optional)\",\"Missed\",\"TankNumber\",\"FuelType\",\"VolumePrice\",\"StationID (optional)\",\"ExcludeDistance\",\"UniqueId\",\"TankCalc\"\n" +
	"\"2024-03-15 18:22\",\"45210\",\"42.37\",\"1\",\"76.21\",\"0\",\"0\",\"0\",\"Lyon\",\"autoroute\",\"0\",\"1\",\"110\",\"1.799\",\"0\",\"0\",\"1710523320000\",\"0\"\n" +
	"\"2024-04-02 08:05\",\"45890\",\"20,5\",\"0\",\"36.9\",\"0\",\"0\",\"0\",\"\",\"\",\"0\",\"1\",\"110\",\"1.8\",\"0\",\"0\",\"1712037900000\",\"0\"\n" +
	"\"pas une date\",\"1\",\"1\",\"1\",\"1\",\"0\",\"0\",\"0\",\"\",\"\",\"0\",\"1\",\"110\",\"1\",\"0\",\"0\",\"9\",\"0\"\n" +
	"\"## CostCategories\"\n" +
	"\"CostTypeID\",\"Name\",\"priority\",\"color\"\n" +
	"\"1\",\"Entretien\",\"0\",\"#FF0000\"\n" +
	"\"2\",\"Assurance\",\"0\",\"#00FF00\"\n" +
	"\"## Costs\"\n" +
	"\"CostTitle\",\"Date\",\"Odo\",\"CostTypeID\",\"Notes\",\"Cost\",\"flag\",\"idR\",\"read\",\"RemindOdo\",\"RemindDate\",\"isTemplate\",\"RepeatOdo\",\"RepeatMonths\",\"isIncome\",\"UniqueId\"\n" +
	"\"Vidange\",\"2024-03-20\",\"45500\",\"1\",\"garage Martin\",\"89.90\",\"0\",\"0\",\"1\",\"0\",\"2011-01-01\",\"0\",\"0\",\"0\",\"0\",\"555\"\n" +
	"\"\",\"2024-05-01\",\"0\",\"2\",\"\",\"420\",\"0\",\"0\",\"1\",\"0\",\"2011-01-01\",\"0\",\"0\",\"12\",\"0\",\"556\"\n" +
	"\"Modele\",\"2024-05-01\",\"0\",\"2\",\"\",\"420\",\"0\",\"0\",\"1\",\"0\",\"2011-01-01\",\"1\",\"0\",\"12\",\"0\",\"557\"\n" +
	"\"Revente\",\"2024-06-01\",\"0\",\"2\",\"\",\"100\",\"0\",\"0\",\"1\",\"0\",\"2011-01-01\",\"0\",\"0\",\"0\",\"1\",\"558\"\n" +
	"\"## FavStations\"\n\"ID\",\"Name\"\n"

func TestLireCSV(t *testing.T) {
	v, err := LireCSV(strings.NewReader(csvEssai))
	if err != nil {
		t.Fatal(err)
	}
	if v.Nom != "Clio" || len(v.Lignes) != 4 {
		t.Fatalf("%q : %d lignes", v.Nom, len(v.Lignes))
	}
	p := v.Lignes[0]
	if !p.Plein || p.Jour != "2024-03-15" || p.Km != 45210 || p.Litres != 4237 || p.Montant != 7621 || p.Note != "Lyon · autoroute" || p.Libelle != "Plein" {
		t.Fatalf("plein : %+v", p)
	}
	if v.Lignes[1].Libelle != "Plein partiel" || v.Lignes[1].Litres != 2050 {
		t.Fatalf("plein partiel : %+v", v.Lignes[1])
	}
	if d := v.Lignes[2]; d.Plein || d.Libelle != "Vidange" || d.Montant != 8990 || d.Km != 45500 || d.Note != "garage Martin" {
		t.Fatalf("dépense : %+v", d)
	}
	// Sans titre : le nom de la catégorie. Modèles et recettes : ignorés.
	if d := v.Lignes[3]; d.Libelle != "Assurance" || d.Montant != 42000 {
		t.Fatalf("dépense sans titre : %+v", d)
	}
}

func TestMiles(t *testing.T) {
	s := strings.Replace(csvEssai, "Odo (km)", "Odo (mi)", 1)
	v, err := LireCSV(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	if v.Lignes[0].Km != 72758 { // 45 210 miles
		t.Fatalf("miles : %d", v.Lignes[0].Km)
	}
}

func TestPasFuelio(t *testing.T) {
	if _, err := LireCSV(strings.NewReader("a,b,c\n1,2,3\n")); err == nil {
		t.Fatal("CSV quelconque accepté")
	}
	chemin := filepath.Join(t.TempDir(), "x.zip")
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, _ := z.Create("notes.txt")
	_, _ = w.Write([]byte("rien"))
	_ = z.Close()
	_ = os.WriteFile(chemin, b.Bytes(), 0o600)
	if _, err := Ouvrir(chemin); err == nil {
		t.Fatal("ZIP sans CSV accepté")
	}
}

func TestOuvrirZip(t *testing.T) {
	// Sauvegarde complète : CSV dans un sous-dossier, ZIP de synchro dans le
	// ZIP, et des photos.
	var interne bytes.Buffer
	zi := zip.NewWriter(&interne)
	w, _ := zi.Create("vehicle-2-sync.csv")
	_, _ = w.Write([]byte(strings.Replace(csvEssai, "\"Clio\"", "\"Kangoo\"", 1)))
	_ = zi.Close()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, _ = z.Create("backup-csv/vehicle-1-sync.csv")
	_, _ = w.Write([]byte(csvEssai))
	w, _ = z.Create("vehicle-2-sync.csv.zip")
	_, _ = w.Write(interne.Bytes())
	w, _ = z.Create("photos/1710523320000.jpg")
	_, _ = w.Write([]byte("jpeg"))
	_ = z.Close()
	chemin := filepath.Join(t.TempDir(), "sauvegarde.fuelio")
	_ = os.WriteFile(chemin, b.Bytes(), 0o600)
	s, err := Ouvrir(chemin)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Fermer()
	if len(s.Vehicules) != 2 || s.Vehicules[1].Nom != "Kangoo" || len(s.Images) != 1 {
		t.Fatalf("%d véhicules, %d images", len(s.Vehicules), len(s.Images))
	}
	r, err := s.Images[0].Ouvrir()
	if err != nil {
		t.Fatal(err)
	}
	contenu, _ := io.ReadAll(r)
	r.Close()
	if string(contenu) != "jpeg" {
		t.Fatalf("image : %q", contenu)
	}
}

func TestRapprocher(t *testing.T) {
	v, _ := LireCSV(strings.NewReader(csvEssai))
	cas := map[string]int{
		"photos/1710523320000.jpg": 0,  // identifiant du plein
		"Fuelio/costs/555_1.jpg":   2,  // identifiant de la dépense
		"IMG_20240320_101500.jpg":  2,  // date : une seule ligne ce jour-là
		"scan-2024-04-02.png":      1,  // date écrite avec des tirets
		"IMG_20240101_101500.jpg":  -1, // aucune ligne ce jour-là
		"photo.jpg":                -1,
		"555_1710523320000.jpg":    -1, // deux lignes citées : dans le doute, rien
	}
	for nom, attendu := range cas {
		if i := Rapprocher(nom, v.Lignes); i != attendu {
			t.Errorf("%s : %d, attendu %d", nom, i, attendu)
		}
	}
}

// csvComplet : un véhicule au format des sauvegardes complètes récentes
// (« Date », « Fuel (L) », guid, partie « ## Pictures »). Les pleins et les
// dépenses ont les mêmes UniqueId (1, 2…), comme dans Fuelio.
func csvComplet(nom string) string {
	return "\"## Vehicle\"\n\"Name\",\"Description\",\"DistUnit\",\"FuelUnit\",\"guid\"\n\"" + nom + "\",\"\",\"0\",\"0\",\"v1\"\n" +
		"\"## Log\"\n\"Date\",\"Odo (km)\",\"Fuel (L)\",\"Full\",\"Price\",\"l/100km\",\"City\",\"Notes\",\"UniqueId\",\"guid\",\"lastupdated\"\n" +
		"\"2025-01-10 10:00\",\"1000\",\"40\",\"1\",\"70\",\"0\",\"\",\"\",\"1\",\"aaaa-log-1\",\"0\"\n" +
		"\"2025-02-10 10:00\",\"1600\",\"41\",\"1\",\"71\",\"0\",\"\",\"\",\"2\",\"aaaa-log-2\",\"0\"\n" +
		"\"2025-03-10 10:00\",\"2200\",\"42\",\"1\",\"72\",\"0\",\"\",\"\",\"3\",\"aaaa-log-3\",\"0\"\n" +
		"\"## CostCategories\"\n\"CostTypeID\",\"Name\",\"priority\",\"color\"\n\"1\",\"Entretien\",\"0\",\"0\"\n" +
		"\"## Costs\"\n\"CostTitle\",\"Date\",\"Odo\",\"CostTypeID\",\"Notes\",\"Cost\",\"isTemplate\",\"isIncome\",\"UniqueId\",\"guid\"\n" +
		"\"Vidange\",\"2025-01-20\",\"1200\",\"1\",\"\",\"90\",\"0\",\"0\",\"1\",\"bbbb-cost-1\"\n" +
		"\"Pneus\",\"2025-02-20\",\"1800\",\"1\",\"\",\"300\",\"0\",\"0\",\"2\",\"bbbb-cost-2\"\n" +
		"\"## Category\"\n\"IdCategory\",\"Name\",\"guid\",\"lastupdated\"\n" +
		"\"## TripLog\"\n\"title\",\"StartName\"\n" +
		"\"## FavStations\"\n\"Name\",\"Latitude\"\n" +
		"\"## Pictures\"\n\"Filename\",\"Note\",\"Type\",\"target_id\"\n" +
		"\"A.png\",\"\",\"1\",\"1\"\n" + // plein 1 ou dépense 1 : le type tranche
		"\"B.png\",\"\",\"1\",\"3\"\n" + // seul un plein porte 3 : type 1 = plein
		"\"C.png\",\"\",\"2\",\"2\"\n" +
		"\"D.png\",\"\",\"2\",\"bbbb-cost-1\"\n" + // guid d'une dépense : type 2 = dépense
		"\"E.png\",\"\",\"3\",\"1\"\n" + // trajet : laissé de côté
		"\"F.png\",\"\",\"1\",\"99\"\n" + // ligne inconnue
		"\"manque.png\",\"\",\"2\",\"1\"\n" // fichier absent
}

// fichierComplet : le .fuelio (ZIP) avec pictures.data (un ZIP dedans).
func fichierComplet(t *testing.T, methode uint16) string {
	t.Helper()
	var images bytes.Buffer
	zi := zip.NewWriter(&images)
	for _, n := range []string{"A.png", "B.png", "C.png", "D.png", "E.png", "F.png", "autre-vehicule.png"} {
		w, _ := zi.Create(n)
		_, _ = w.Write([]byte("image " + n))
	}
	_ = zi.Close()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	w, _ := z.CreateHeader(&zip.FileHeader{Name: "pictures.data", Method: methode})
	_, _ = w.Write(images.Bytes())
	for i, nom := range []string{"Kangoo", "Clio", "Vélo"} {
		w, _ = z.Create(fmt.Sprintf("vehicle-%d-local.csv", i+1))
		_, _ = w.Write([]byte(csvComplet(nom)))
	}
	_ = z.Close()
	chemin := filepath.Join(t.TempDir(), "Fuelio_2026-11-23.fuelio")
	_ = os.WriteFile(chemin, b.Bytes(), 0o600)
	return chemin
}

func TestSauvegardeComplete(t *testing.T) {
	for _, methode := range []uint16{zip.Store, zip.Deflate} {
		chemin := fichierComplet(t, methode)
		s, err := Ouvrir(chemin)
		if err != nil {
			t.Fatal(err)
		}
		if len(s.Vehicules) != 3 || len(s.Images) != 7 {
			t.Fatalf("%d véhicules, %d images", len(s.Vehicules), len(s.Images))
		}
		v := s.Vehicules[1]
		if v.Nom != "Clio" || len(v.Lignes) != 5 || v.Lignes[0].Litres != 4000 || v.Lignes[0].Guid != "aaaa-log-1" || len(v.Photos) != 7 {
			t.Fatalf("véhicule : %q, %d lignes, %d photos, %+v", v.Nom, len(v.Lignes), len(v.Photos), v.Lignes[0])
		}
		liens, sans := Rattacher(v, s.Images)
		obtenu := map[string]string{}
		for _, l := range liens {
			obtenu[l.Image.Nom] = v.Lignes[l.Ligne].Jour + " " + v.Lignes[l.Ligne].Libelle
			r, err := l.Image.Ouvrir()
			if err != nil {
				t.Fatal(err)
			}
			contenu, _ := io.ReadAll(r)
			r.Close()
			if string(contenu) != "image "+l.Image.Nom {
				t.Fatalf("contenu de %s : %q", l.Image.Nom, contenu)
			}
		}
		attendu := map[string]string{"A.png": "2025-01-10 Plein", "B.png": "2025-03-10 Plein", "C.png": "2025-02-20 Pneus", "D.png": "2025-01-20 Vidange"}
		if fmt.Sprint(obtenu) != fmt.Sprint(attendu) || sans != 3 {
			t.Fatalf("méthode %d : %v, %d laissées de côté", methode, obtenu, sans)
		}
		if err := s.Fermer(); err != nil {
			t.Fatal(err)
		}
		// Le fichier provisoire (ZIP compressé) est effacé.
		if l, _ := filepath.Glob(filepath.Join(filepath.Dir(chemin), "fuelio-*")); len(l) != 0 {
			t.Fatalf("fichier provisoire gardé : %v", l)
		}
	}
}

// Dates au format choisi dans Fuelio et lignes écartées comptées.
func TestDatesEtEcarts(t *testing.T) {
	csv := "\"## Vehicle\"\n\"Name\",\"DistUnit\",\"FuelUnit\"\n\"Clio\",\"0\",\"0\"\n" +
		"\"## Log\"\n\"Date\",\"Odo (km)\",\"Fuel (L)\",\"Full\",\"Price\",\"UniqueId\"\n" +
		"\"15/03/2024 18:22\",\"1000\",\"40\",\"1\",\"70\",\"1\"\n" +
		"\"??\",\"1100\",\"40\",\"1\",\"70\",\"2\"\n" +
		"\"## Costs\"\n\"CostTitle\",\"Date\",\"Odo\",\"CostTypeID\",\"Notes\",\"Cost\",\"isTemplate\",\"isIncome\",\"UniqueId\"\n" +
		"\"Vidange\",\"20.03.2024\",\"1200\",\"1\",\"\",\"90\",\"0\",\"0\",\"3\"\n" +
		"\"Modele\",\"2024-03-20\",\"0\",\"1\",\"\",\"90\",\"true\",\"0\",\"4\"\n" +
		"\"Revente\",\"2024-03-20\",\"0\",\"1\",\"\",\"90\",\"0\",\"1\",\"5\"\n"
	v, err := LireCSV(strings.NewReader(csv))
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Lignes) != 2 || v.Lignes[0].Jour != "2024-03-15" || v.Lignes[1].Jour != "2024-03-20" || v.Lignes[1].Libelle != "Vidange" {
		t.Fatalf("lignes : %+v", v.Lignes)
	}
	if e := v.Ecart; e.PleinsSansDate != 1 || e.DepensesLues != 3 || e.Modeles != 1 || e.Recettes != 1 || e.DepensesSansDate != 0 {
		t.Fatalf("écarts : %+v", e)
	}
}
