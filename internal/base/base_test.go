// SPDX-License-Identifier: GPL-3.0-or-later

package base

import (
	"context"
	"errors"
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

// 0015 : les anciennes opérations « fixes » deviennent des récurrences
// mensuelles qui partent de leur dernière occurrence.
func TestMigrationFixes(t *testing.T) {
	ctx := context.Background()
	db, err := Ouvrir(filepath.Join(t.TempDir(), NomFichier))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	migs, err := lireMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Version(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, m := range migs[:14] {
		if err := appliquer(ctx, db, m); err != nil {
			t.Fatal(m.nom, err)
		}
	}
	for _, q := range []string{
		`INSERT INTO familles (id, nom, cree_le) VALUES (1, 'Martin', 0)`,
		`INSERT INTO operations (famille_id, sens, montant, categorie, libelle, jour, fixe, cree_le) VALUES
		  (1, 'revenu', 250000, 'salaire', 'Salaire', '2026-10-28', 1, 0),
		  (1, 'revenu', 260000, 'salaire', 'Salaire', '2026-11-28', 1, 0),
		  (1, 'depense', 8000, 'courses', '', '2026-11-03', 0, 0)`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Migrer(ctx, db, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	var n, montant, faites, liees int
	var debut, freq string
	if err := db.QueryRowContext(ctx, `SELECT count(*), max(montant), max(debut), max(frequence), max(faites) FROM recurrences`).
		Scan(&n, &montant, &debut, &freq, &faites); err != nil {
		t.Fatal(err)
	}
	if n != 1 || montant != 260000 || debut != "2026-11-28" || freq != "mois" || faites != 1 {
		t.Fatalf("récurrence : %d %d %s %s %d", n, montant, debut, freq, faites)
	}
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM operations WHERE recurrence_id IS NOT NULL`).Scan(&liees)
	if liees != 2 {
		t.Fatalf("%d opérations liées, attendu 2", liees)
	}
}

// 0017 : les prestataires deviennent des contacts « pro » ; leurs échéances
// et leur carnet suivent le nouvel identifiant.
func TestMigrationPrestataires(t *testing.T) {
	ctx := context.Background()
	db, err := Ouvrir(filepath.Join(t.TempDir(), NomFichier))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	migs, err := lireMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Version(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, m := range migs[:16] {
		if err := appliquer(ctx, db, m); err != nil {
			t.Fatal(m.nom, err)
		}
	}
	for _, q := range []string{
		`INSERT INTO familles (id, nom, cree_le) VALUES (1, 'Martin', 0)`,
		// Un contact existant (id 1) : le prestataire 1 deviendra le contact 2.
		`INSERT INTO contacts (id, famille_id, nom, cree_le, modifie_le) VALUES (1, 1, 'Mamie', 0, 0)`,
		`INSERT INTO prestataires (id, famille_id, nom, metier, telephone, site, cree_le) VALUES (1, 1, 'Plomberie Martin', 'Plombier', '0612', 'https://p.example', 0)`,
		`INSERT INTO echeances (famille_id, module, objet_id, libelle, jour, cree_le) VALUES (1, 'prestataires', 1, 'Entretien', '2027-01-01', 0)`,
		`INSERT INTO carnet (famille_id, module, objet_id, jour, libelle, montant, cree_le) VALUES (1, 'prestataires', 1, '2026-10-01', 'Fuite', 9000, 0)`,
		`INSERT INTO vehicules (id, famille_id, nom, cree_le) VALUES (1, 1, 'Clio', 0)`,
		`INSERT INTO echeances (famille_id, module, objet_id, libelle, jour, cree_le) VALUES (1, 'vehicules', 1, 'CT', '2027-01-01', 0)`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(q, err)
		}
	}
	if _, err := Migrer(ctx, db, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	var id, pro int
	var metier, site string
	if err := db.QueryRowContext(ctx, `SELECT id, pro, metier, site FROM contacts WHERE nom = 'Plomberie Martin'`).Scan(&id, &pro, &metier, &site); err != nil {
		t.Fatal(err)
	}
	if id != 2 || pro != 1 || metier != "Plombier" || site != "https://p.example" {
		t.Fatalf("contact : %d %d %s %s", id, pro, metier, site)
	}
	var e, c, v int
	_ = db.QueryRowContext(ctx, `SELECT objet_id FROM echeances WHERE module = 'prestataires'`).Scan(&e)
	_ = db.QueryRowContext(ctx, `SELECT objet_id FROM carnet WHERE module = 'prestataires'`).Scan(&c)
	_ = db.QueryRowContext(ctx, `SELECT objet_id FROM echeances WHERE module = 'vehicules'`).Scan(&v)
	if e != 2 || c != 2 || v != 1 {
		t.Fatalf("suivi : échéance %d, carnet %d, véhicule %d", e, c, v)
	}
	if _, err := db.ExecContext(ctx, `SELECT 1 FROM prestataires`); err == nil {
		t.Fatal("table prestataires encore là")
	}
}

// 0044 : la table recurrences est refaite (nouvelle fréquence) sans casser
// les liens : opérations et abonnements gardent leur récurrence, et les
// clés étrangères sont rallumées après.
func TestMigrationRecurrencesDixMois(t *testing.T) {
	ctx := context.Background()
	db, err := Ouvrir(filepath.Join(t.TempDir(), NomFichier))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	migs, err := lireMigrations()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Version(ctx, db); err != nil {
		t.Fatal(err)
	}
	for _, m := range migs[:43] {
		if err := appliquer(ctx, db, m); err != nil {
			t.Fatal(m.nom, err)
		}
	}
	for _, q := range []string{
		`INSERT INTO familles (id, nom, cree_le) VALUES (1, 'Martin', 0)`,
		`INSERT INTO recurrences (id, famille_id, sens, montant, categorie, frequence, debut, faites, cree_le, perso_de, virement)
		  VALUES (7, 1, 'depense', 1299, 'abonnements', 'mois', '2026-10-05', 2, 0, 3, 1)`,
		`INSERT INTO operations (famille_id, sens, montant, categorie, jour, cree_le, recurrence_id) VALUES (1, 'depense', 1299, 'abonnements', '2026-10-05', 0, 7)`,
		`INSERT INTO abonnements (famille_id, recurrence_id, cree_le) VALUES (1, 7, 0)`,
	} {
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatal(q, err)
		}
	}
	if _, err := Migrer(ctx, db, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	var ops, abos, perso, vir int
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM operations WHERE recurrence_id = 7`).Scan(&ops)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM abonnements WHERE recurrence_id = 7`).Scan(&abos)
	_ = db.QueryRowContext(ctx, `SELECT perso_de, virement FROM recurrences WHERE id = 7`).Scan(&perso, &vir)
	if ops != 1 || abos != 1 || perso != 3 || vir != 1 {
		t.Fatalf("après migration : %d opération, %d abonnement, perso %d, virement %d", ops, abos, perso, vir)
	}
	if _, err := db.ExecContext(ctx, `UPDATE recurrences SET frequence = 'mois10' WHERE id = 7`); err != nil {
		t.Fatal("mois10 refusé :", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE recurrences SET frequence = 'mois11' WHERE id = 7`); err == nil {
		t.Fatal("fréquence inconnue acceptée")
	}
	// Les clés étrangères marchent encore : supprimer la récurrence
	// supprime l'abonnement et détache l'opération.
	if _, err := db.ExecContext(ctx, `DELETE FROM recurrences WHERE id = 7`); err != nil {
		t.Fatal(err)
	}
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM abonnements`).Scan(&abos)
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM operations WHERE recurrence_id IS NULL`).Scan(&ops)
	if abos != 0 || ops != 1 {
		t.Fatalf("clés étrangères : %d abonnement, %d opération détachée", abos, ops)
	}
}

// Une sauvegarde automatique interrompue ne laisse aucun fichier : elle ne
// prend pas la place d'une bonne copie dans la rotation.
func TestSauvegardeAutoInterrompue(t *testing.T) {
	dossier := t.TempDir()
	db, err := Ouvrir(filepath.Join(dossier, NomFichier))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := Migrer(context.Background(), db, dossier); err != nil {
		t.Fatal(err)
	}
	ctx, annuler := context.WithCancel(context.Background())
	annuler()
	sauv := filepath.Join(dossier, "sauvegardes")
	if _, err := SauvegardeAuto(ctx, db, sauv, 2); err == nil {
		t.Fatal("erreur attendue (contexte annulé)")
	}
	if entrees, _ := os.ReadDir(sauv); len(entrees) != 0 {
		t.Fatalf("%d fichier(s) laissé(s) par une sauvegarde interrompue", len(entrees))
	}
}

// Un seul preneur du verrou : la restauration refuse un serveur en marche.
func TestVerrouDuDossier(t *testing.T) {
	ctx := context.Background()
	dossier := t.TempDir()
	db, err := Ouvrir(filepath.Join(dossier, NomFichier))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Migrer(ctx, db, dossier); err != nil {
		t.Fatal(err)
	}
	copie, err := Copier(ctx, db, dossier, "copie.db")
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	liberer, err := Verrouiller(dossier) // le « serveur »
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verrouiller(dossier); !errors.Is(err, ErrServeurEnMarche) {
		t.Fatalf("second verrou : %v", err)
	}
	if _, err := Restaurer(ctx, copie, dossier); !errors.Is(err, ErrServeurEnMarche) {
		t.Fatalf("restauration sous un serveur en marche : %v", err)
	}
	liberer()
	if _, err := Restaurer(ctx, copie, dossier); err != nil {
		t.Fatalf("restauration, serveur arrêté : %v", err)
	}
}

// Copies « avant-migration » : jamais plus que la limite, les plus récentes.
func TestGarderDernieres(t *testing.T) {
	dossier := t.TempDir()
	for _, nom := range []string{"avant-migration-v3-20260101-000000.db", "avant-migration-v3-20260102-000000.db",
		"avant-migration-v3-20260103-000000.db", "auto-20260101-000000.db"} {
		os.WriteFile(filepath.Join(dossier, nom), []byte("x"), 0o600)
	}
	garderDernieres(dossier, "avant-migration-", 2)
	entrees, _ := os.ReadDir(dossier)
	var noms []string
	for _, e := range entrees {
		noms = append(noms, e.Name())
	}
	if strings.Join(noms, " ") != "auto-20260101-000000.db avant-migration-v3-20260102-000000.db avant-migration-v3-20260103-000000.db" {
		t.Fatalf("fichiers gardés : %v", noms)
	}
}
