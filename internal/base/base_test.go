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
