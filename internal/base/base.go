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
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
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
	return VersionLue(ctx, db)
}

// VersionLue : la version du schéma, en lecture seule (sonde /health : rien
// n'est écrit, la table existe depuis le démarrage).
func VersionLue(ctx context.Context, db *sql.DB) (int, error) {
	var v sql.NullInt64
	err := db.QueryRowContext(ctx, `SELECT max(version) FROM schema_version`).Scan(&v)
	return int(v.Int64), err
}

// copiesMigrationMax : copies « avant-migration » gardées. Une migration qui
// échoue fait redémarrer le conteneur en boucle : sans limite, chaque essai
// ajouterait une copie complète de la base, jusqu'à remplir le disque.
const copiesMigrationMax = 3

// garderDernieres supprime les fichiers du dossier qui commencent par
// prefixe, sauf les garder plus récents (la date est dans le nom).
func garderDernieres(dossier, prefixe string, garder int) {
	entrees, err := os.ReadDir(dossier)
	if err != nil {
		return
	}
	var noms []string
	for _, e := range entrees {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefixe) {
			noms = append(noms, e.Name())
		}
	}
	sort.Strings(noms)
	for len(noms) > garder {
		_ = os.Remove(filepath.Join(dossier, noms[0]))
		noms = noms[1:]
	}
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
		// Place faite d'abord : avec la nouvelle, copiesMigrationMax au plus.
		garderDernieres(dossierSauvegardes, "avant-migration-", copiesMigrationMax-1)
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
//
// Une migration qui commence par « -- sans cles etrangeres » tourne avec
// foreign_keys=OFF (méthode conseillée par SQLite pour refaire une table :
// sinon, supprimer l'ancienne table déclencherait les ON DELETE des tables
// qui pointent vers elle). Les liens sont vérifiés à la fin
// (foreign_key_check) : un lien cassé annule tout.
func appliquer(ctx context.Context, db *sql.DB, m migration) error {
	// Une seule connexion : le PRAGMA ne vaut que pour elle.
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	sansCles := strings.HasPrefix(m.sql, "-- sans cles etrangeres")
	if sansCles {
		// Hors transaction : SQLite ignore ce PRAGMA dans une transaction.
		if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
			return err
		}
		defer func() { _, _ = conn.ExecContext(context.Background(), "PRAGMA foreign_keys = ON") }()
	}
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // sans effet après Commit
	// Liens déjà cassés avant la migration (base réparée à la main…) : ils
	// ne la bloquent pas ; seuls ceux qu'elle casserait l'annulent.
	var avant map[string]bool
	if sansCles {
		if avant, err = liensCasses(ctx, tx); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return err
	}
	if sansCles {
		apres, err := liensCasses(ctx, tx)
		if err != nil {
			return err
		}
		for lien := range apres {
			if !avant[lien] {
				return fmt.Errorf("lien cassé par la migration : %s", lien)
			}
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_version(version, nom, appliquee_le) VALUES (?, ?, ?)`,
		m.version, m.nom, time.Now().Unix()); err != nil {
		return err
	}
	return tx.Commit()
}

// liensCasses : les liens cassés de la base (PRAGMA foreign_key_check),
// « table, ligne n → table visée ».
func liensCasses(ctx context.Context, tx *sql.Tx) (map[string]bool, error) {
	lignes, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return nil, err
	}
	defer lignes.Close()
	out := map[string]bool{}
	for lignes.Next() {
		var table, parent string
		var id, fk sql.NullInt64
		if err := lignes.Scan(&table, &id, &parent, &fk); err != nil {
			return nil, err
		}
		out[fmt.Sprintf("%s, ligne %d → %s (lien %d)", table, id.Int64, parent, fk.Int64)] = true
	}
	return out, lignes.Err()
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
		_ = os.Remove(chemin) // copie interrompue : pas de fichier à moitié écrit
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
	// Le serveur tient ce verrou tant qu'il tourne : restaurer sous lui ne
	// servirait à rien (il continuerait sur l'ancien fichier).
	liberer, err := Verrouiller(dossier)
	if err != nil {
		return "", err
	}
	defer liberer()
	cible := filepath.Join(dossier, NomFichier)
	temp := cible + ".restauration"
	if err := copierFichier(source, temp); err != nil {
		_ = os.Remove(temp)
		return "", err
	}
	// Anciennes bases « avant-restauration » : seules les plus récentes sont
	// gardées (6 fichiers au plus, journaux -wal et -shm compris).
	garderDernieres(dossier, NomFichier+".avant-restauration-", 6)
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

// copierFichier copie source vers cible (droits 600) sans tout charger en
// mémoire, et attend que le disque ait tout écrit.
func copierFichier(source, cible string) error {
	src, err := os.Open(source) // #nosec G304 -- chemin donné par l'administrateur
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(cible, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) // #nosec G304 -- dossier des données
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		return errors.Join(err, dst.Close())
	}
	if err := dst.Sync(); err != nil {
		return errors.Join(err, dst.Close())
	}
	return dst.Close()
}

// ErrServeurEnMarche : le dossier des données est utilisé par un serveur.
var ErrServeurEnMarche = errors.New("FamilyCocon tourne sur ce dossier de données : arrêtez-le d'abord")

// Verrouiller prend le verrou du dossier des données (fichier
// « familycocon.verrou », flock). Le serveur le garde tant qu'il tourne ;
// la restauration le prend le temps de remplacer la base. Un second
// preneur reçoit ErrServeurEnMarche. liberer rend le verrou.
func Verrouiller(dossier string) (liberer func(), err error) {
	if err := os.MkdirAll(dossier, 0o750); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dossier, "familycocon.verrou"), os.O_RDWR|os.O_CREATE, 0o600) // #nosec G304 -- dossier des données
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil { // #nosec G115 -- descripteur de fichier
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrServeurEnMarche
		}
		return nil, err
	}
	return func() { _ = f.Close() }, nil // fermer rend le verrou
}

// Sauvegardes automatiques : « auto-AAAAMMJJ-HHMMSS.db » dans le dossier.

// SauvegardeAuto fait une sauvegarde vérifiée et ne garde que les garder
// plus récentes sauvegardes automatiques.
func SauvegardeAuto(ctx context.Context, db *sql.DB, dossier string, garder int) (string, error) {
	// Écrite sous un nom provisoire, renommée une fois vérifiée : une copie
	// interrompue ou abîmée ne prend jamais la place d'une bonne sauvegarde.
	nom := "auto-" + time.Now().Format("20060102-150405") + ".db"
	provisoire, err := Copier(ctx, db, dossier, nom+".partiel")
	if err != nil {
		return "", err
	}
	copie, err := OuvrirLecture(provisoire)
	if err == nil {
		err = Verifier(ctx, copie)
		_ = copie.Close()
	}
	if err != nil {
		_ = os.Remove(provisoire)
		return "", err
	}
	chemin := filepath.Join(dossier, nom)
	if err := os.Rename(provisoire, chemin); err != nil {
		_ = os.Remove(provisoire)
		return "", err
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
