// SPDX-License-Identifier: GPL-3.0-or-later

// Package base : base SQLite de FamilyCocon (repris de mirador-stack,
// internal/server/base).
//
// Choix (SPEC.md § Base) : modernc.org/sqlite (pur Go, pas de cgo : binaire
// statique et compilation croisée arm64 triviale), mode WAL, une seule
// connexion (SQLite n'écrit qu'une transaction à la fois ; une famille ne
// fait que quelques requêtes par seconde).
//
// Migrations : fichiers SQL numérotés, embarqués dans le binaire, appliqués
// au démarrage dans une transaction chacun. Avant toute migration d'une base
// existante, une copie cohérente est faite (VACUUM INTO) : revenir en arrière
// = reprendre cette copie.
package base

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pilote SQLite pur Go
)

//go:embed migrations/*.sql
var fichiersMigrations embed.FS

// NomFichier : nom de la base dans le dossier des données.
const NomFichier = "familycocon.db"

// Ouvrir ouvre (ou crée) la base avec les réglages de sécurité et de
// performance : WAL, clés étrangères, attente en cas de verrou.
func Ouvrir(chemin string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(chemin), 0o750); err != nil {
		return nil, err
	}
	// temp_store(2) : tables temporaires en mémoire. L'image Docker est en
	// lecture seule et sans /tmp : SQLite ne doit jamais y écrire.
	// cache_size(-2000) : 2 Mio de cache de pages, largement assez.
	dsn := "file:" + chemin + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)" +
		"&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=temp_store(2)" +
		"&_pragma=cache_size(-2000)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// Une seule connexion : pas de conflit de verrou, mémoire minimale.
	// Attention : toujours fermer un *sql.Rows avant la requête suivante.
	db.SetMaxOpenConns(1)
	db.SetConnMaxIdleTime(0)
	if err := db.Ping(); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	if err := os.Chmod(chemin, 0o600); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return db, nil
}

// OuvrirLecture ouvre une base en lecture seule, sans toucher à son mode de
// journal (vérification d'une sauvegarde : aucun fichier -wal créé à côté).
func OuvrirLecture(chemin string) (*sql.DB, error) {
	if _, err := os.Stat(chemin); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+chemin+"?mode=ro&_pragma=query_only(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, db.Ping()
}

// migration : un fichier NNNN_nom.sql.
type migration struct {
	version int
	nom     string
	sql     string
}

func lireMigrations() ([]migration, error) {
	entrees, err := fs.ReadDir(fichiersMigrations, "migrations")
	if err != nil {
		return nil, err
	}
	var out []migration
	for _, e := range entrees {
		num, _, ok := strings.Cut(e.Name(), "_")
		v, err := strconv.Atoi(num)
		if !ok || err != nil || v <= 0 {
			return nil, fmt.Errorf("nom de migration invalide %q (NNNN_nom.sql)", e.Name())
		}
		contenu, err := fichiersMigrations.ReadFile("migrations/" + e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, migration{version: v, nom: e.Name(), sql: string(contenu)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	for i, m := range out {
		if m.version != i+1 {
			return nil, fmt.Errorf("migrations non consécutives : %s", m.nom)
		}
	}
	return out, nil
}

// VersionAttendue : version du schéma de ce binaire (nombre de migrations).
func VersionAttendue() int {
	migs, err := lireMigrations()
	if err != nil {
		return -1
	}
	return len(migs)
}

// Version renvoie la version du schéma (0 = base vide).
func Version(ctx context.Context, db *sql.DB) (int, error) {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY, nom TEXT NOT NULL, appliquee_le INTEGER NOT NULL)`); err != nil {
		return 0, err
	}
	var v sql.NullInt64
	err := db.QueryRowContext(ctx, `SELECT max(version) FROM schema_version`).Scan(&v)
	return int(v.Int64), err
}

// Migrer applique les migrations manquantes. Si la base contient déjà des
// données, une copie est faite avant dans dossierSauvegardes. Renvoie le
// chemin de la copie (vide si aucune n'a été nécessaire).
func Migrer(ctx context.Context, db *sql.DB, dossierSauvegardes string) (string, error) {
	migs, err := lireMigrations()
	if err != nil {
		return "", err
	}
	actuelle, err := Version(ctx, db)
	if err != nil {
		return "", err
	}
	if actuelle > len(migs) {
		return "", fmt.Errorf("base en version %d, plus récente que ce binaire (%d) : mettez FamilyCocon à jour", actuelle, len(migs))
	}
	if actuelle == len(migs) {
		return "", nil
	}
	copie := ""
	if actuelle > 0 {
		if copie, err = Copier(ctx, db, dossierSauvegardes,
			fmt.Sprintf("avant-migration-v%d-%s.db", actuelle, time.Now().UTC().Format("20060102-150405"))); err != nil {
			return "", fmt.Errorf("copie avant migration impossible, migration annulée : %w", err)
		}
	}
	for _, m := range migs[actuelle:] {
		if err := appliquer(ctx, db, m); err != nil {
			return copie, fmt.Errorf("migration %s : %w (copie de la base avant migration : %s)", m.nom, err, copie)
		}
	}
	return copie, nil
}

// appliquer exécute une migration et note sa version, dans une transaction :
// en cas d'erreur, rien n'est modifié.
func appliquer(ctx context.Context, db *sql.DB, m migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // sans effet après Commit
	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_version(version, nom, appliquee_le) VALUES (?, ?, ?)`,
		m.version, m.nom, time.Now().Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

// Copier fait une copie cohérente de la base pendant qu'elle tourne
// (VACUUM INTO : compacte et ne bloque pas les lecteurs). Sert aux migrations
// et aux sauvegardes.
func Copier(ctx context.Context, db *sql.DB, dossier, nom string) (string, error) {
	if strings.ContainsAny(nom, `/\'`) {
		return "", errors.New("nom de copie invalide")
	}
	if err := os.MkdirAll(dossier, 0o700); err != nil {
		return "", err
	}
	return CopierVers(ctx, db, filepath.Join(dossier, nom))
}

// CopierVers : comme Copier, vers un chemin complet (sous-commande
// -sauvegarde). Le fichier ne doit pas déjà exister.
func CopierVers(ctx context.Context, db *sql.DB, chemin string) (string, error) {
	if strings.ContainsAny(chemin, `'`) {
		return "", errors.New("chemin de copie invalide (apostrophe)")
	}
	if _, err := os.Stat(chemin); err == nil {
		return "", fmt.Errorf("%s existe déjà", chemin)
	}
	// VACUUM INTO n'accepte pas de paramètre lié : le chemin vient de la
	// configuration ou de la ligne de commande, et ne contient pas d'apostrophe.
	if _, err := db.ExecContext(ctx, `VACUUM INTO '`+chemin+`'`); err != nil { // #nosec G202 -- chemin contrôlé
		return "", err
	}
	return chemin, os.Chmod(chemin, 0o600)
}

// Verifier contrôle l'intégrité d'une base (sert après une sauvegarde et
// avant une restauration).
func Verifier(ctx context.Context, db *sql.DB) error {
	var r string
	if err := db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&r); err != nil {
		return err
	}
	if r != "ok" {
		return fmt.Errorf("base abîmée : %s", r)
	}
	return nil
}

// Restaurer remplace la base du dossier par la sauvegarde source, après
// l'avoir vérifiée (intégrité, version du schéma pas plus récente que ce
// binaire). L'ancienne base est gardée à côté (« .avant-restauration-… »).
// Le serveur doit être ARRÊTÉ.
func Restaurer(ctx context.Context, source, dossier string) (string, error) {
	src, err := OuvrirLecture(source)
	if err != nil {
		return "", fmt.Errorf("sauvegarde illisible : %w", err)
	}
	defer src.Close()
	if err := Verifier(ctx, src); err != nil {
		return "", err
	}
	var v sql.NullInt64
	if err := src.QueryRowContext(ctx, `SELECT max(version) FROM schema_version`).Scan(&v); err != nil {
		return "", fmt.Errorf("ce fichier n'est pas une sauvegarde de FamilyCocon : %w", err)
	}
	if int(v.Int64) > VersionAttendue() {
		return "", fmt.Errorf("sauvegarde en version %d, plus récente que ce binaire (%d) : mettez FamilyCocon à jour d'abord", v.Int64, VersionAttendue())
	}
	contenu, err := os.ReadFile(source)
	if err != nil {
		return "", err
	}
	cible := filepath.Join(dossier, NomFichier)
	temp := cible + ".restauration"
	if err := os.WriteFile(temp, contenu, 0o600); err != nil {
		return "", err
	}
	ancienne := ""
	if _, err := os.Stat(cible); err == nil {
		ancienne = cible + ".avant-restauration-" + time.Now().UTC().Format("20060102-150405")
		if err := os.Rename(cible, ancienne); err != nil {
			return "", err
		}
	}
	// Les fichiers du journal WAL de l'ancienne base ne doivent pas être
	// rejoués sur la nouvelle.
	for _, suffixe := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(cible + suffixe); err == nil {
			if ancienne != "" {
				_ = os.Rename(cible+suffixe, ancienne+suffixe)
			} else {
				_ = os.Remove(cible + suffixe)
			}
		}
	}
	return ancienne, os.Rename(temp, cible)
}

// Sauvegardes automatiques : « auto-AAAAMMJJ-HHMMSS.db » dans le dossier.

// SauvegardeAuto fait une sauvegarde vérifiée et ne garde que les garder
// plus récentes sauvegardes automatiques.
func SauvegardeAuto(ctx context.Context, db *sql.DB, dossier string, garder int) (string, error) {
	nom := "auto-" + time.Now().Format("20060102-150405") + ".db"
	chemin, err := Copier(ctx, db, dossier, nom)
	if err != nil {
		return "", err
	}
	copie, err := OuvrirLecture(chemin)
	if err != nil {
		return chemin, err
	}
	err = Verifier(ctx, copie)
	copie.Close()
	if err != nil {
		return chemin, err
	}
	entrees, err := os.ReadDir(dossier)
	if err != nil {
		return chemin, err
	}
	var autos []string
	for _, e := range entrees {
		if strings.HasPrefix(e.Name(), "auto-") && strings.HasSuffix(e.Name(), ".db") {
			autos = append(autos, e.Name())
		}
	}
	sort.Strings(autos) // la date dans le nom : ordre chronologique
	for len(autos) > garder {
		_ = os.Remove(filepath.Join(dossier, autos[0]))
		autos = autos[1:]
	}
	return chemin, nil
}
