// SPDX-License-Identifier: GPL-3.0-or-later

package fuelio

import (
	"archive/zip"
	"bytes"
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
	if b, err := s.Images[0].Lire(); err != nil || string(b) != "jpeg" {
		t.Fatalf("image : %q %v", b, err)
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
