// SPDX-License-Identifier: GPL-3.0-or-later

package base

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationsEtCopie(t *testing.T) {
	ctx := context.Background()
	dossier := t.TempDir()
	db, err := Ouvrir(filepath.Join(dossier, NomFichier))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var mode string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "wal" {
		t.Fatalf("mode WAL attendu, obtenu %q (%v)", mode, err)
	}
	// Base vide : pas de copie avant migration.
	copie, err := Migrer(ctx, db, filepath.Join(dossier, "sauvegardes"))
	if err != nil || copie != "" {
		t.Fatalf("migration : %v, copie %q", err, copie)
	}
	v, err := Version(ctx, db)
	if err != nil || v != VersionAttendue() || v < 1 {
		t.Fatalf("version %d (attendue %d) : %v", v, VersionAttendue(), err)
	}
	// Une seconde migration ne fait rien.
	if copie, err := Migrer(ctx, db, dossier); err != nil || copie != "" {
		t.Fatalf("seconde migration : %v %q", err, copie)
	}

	// Copie à chaud, puis vérification de la copie en lecture seule.
	if _, err := db.Exec(`INSERT INTO familles(nom, cree_le) VALUES ('Test', 1)`); err != nil {
		t.Fatal(err)
	}
	chemin, err := Copier(ctx, db, filepath.Join(dossier, "sauvegardes"), "copie.db")
	if err != nil {
		t.Fatal(err)
	}
	lecture, err := OuvrirLecture(chemin)
	if err != nil {
		t.Fatal(err)
	}
	defer lecture.Close()
	if err := Verifier(ctx, lecture); err != nil {
		t.Fatal(err)
	}
	var nom string
	if err := lecture.QueryRow(`SELECT nom FROM familles`).Scan(&nom); err != nil || nom != "Test" {
		t.Fatalf("copie incomplète : %q %v", nom, err)
	}
	if _, err := os.Stat(chemin + "-wal"); err == nil {
		t.Fatal("la vérification ne doit pas créer de fichier -wal")
	}
	// Pas d'écrasement, pas de nom dangereux.
	if _, err := Copier(ctx, db, filepath.Join(dossier, "sauvegardes"), "copie.db"); err == nil {
		t.Fatal("écrasement accepté")
	}
	if _, err := Copier(ctx, db, dossier, "a'b.db"); err == nil {
		t.Fatal("apostrophe acceptée")
	}
}

func TestBasePlusRecente(t *testing.T) {
	ctx := context.Background()
	db, err := Ouvrir(filepath.Join(t.TempDir(), NomFichier))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := Migrer(ctx, db, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO schema_version VALUES (999, 'futur', 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := Migrer(ctx, db, t.TempDir()); err == nil || !strings.Contains(err.Error(), "plus récente") {
		t.Fatalf("erreur attendue, obtenu %v", err)
	}
}

func TestRestaurerEtSauvegardeAuto(t *testing.T) {
	ctx := context.Background()
	dossier := t.TempDir()
	db, err := Ouvrir(filepath.Join(dossier, NomFichier))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Migrer(ctx, db, dossier); err != nil {
		t.Fatal(err)
	}
	db.Exec(`INSERT INTO familles(nom, cree_le) VALUES ('Avant', 1)`)
	sauv := filepath.Join(dossier, "sauvegardes")
	// Rotation : 4 sauvegardes, 2 gardées.
	var derniere string
	os.MkdirAll(sauv, 0o700)
	for i := 0; i < 4; i++ {
		os.WriteFile(filepath.Join(sauv, "auto-2020010"+string(rune('1'+i))+"-000000.db"), []byte("x"), 0o600)
	}
	if derniere, err = SauvegardeAuto(ctx, db, sauv, 2); err != nil {
		t.Fatal(err)
	}
	entrees, _ := os.ReadDir(sauv)
	if len(entrees) != 2 {
		t.Fatalf("%d sauvegardes gardées, 2 attendues", len(entrees))
	}
	db.Exec(`UPDATE familles SET nom = 'Après'`)
	db.Close()

	// Fichier qui n'est pas une base : refusé.
	faux := filepath.Join(dossier, "faux.db")
	os.WriteFile(faux, []byte("pas une base"), 0o600)
	if _, err := Restaurer(ctx, faux, dossier); err == nil {
		t.Fatal("faux fichier accepté")
	}
	ancienne, err := Restaurer(ctx, derniere, dossier)
	if err != nil || ancienne == "" {
		t.Fatalf("restauration : %v (%q)", err, ancienne)
	}
	db, err = Ouvrir(filepath.Join(dossier, NomFichier))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var nom string
	if err := db.QueryRow(`SELECT nom FROM familles`).Scan(&nom); err != nil || nom != "Avant" {
		t.Fatalf("base restaurée : %q %v", nom, err)
	}
}
