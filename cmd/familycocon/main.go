// SPDX-License-Identifier: GPL-3.0-or-later

// FamilyCocon : application web d'organisation familiale, auto-hébergée.
//
// Utilisation :
//
//	familycocon                       lance le serveur
//	familycocon -sante                vérifie que le serveur répond (HEALTHCHECK Docker)
//	familycocon -sauvegarde FICHIER   copie la base à chaud (VACUUM INTO)
//	familycocon -restaurer FICHIER    remet une sauvegarde en place (serveur ARRÊTÉ)
//	familycocon -version              affiche la version
//
// La configuration est lue dans les variables d'environnement (voir .env.exemple).
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata" // fuseaux horaires embarqués : l'image « scratch » n'en a pas

	"familycocon/internal/appli"
	"familycocon/internal/base"
	"familycocon/internal/config"
	"familycocon/internal/version"
)

func main() {
	sante := flag.Bool("sante", false, "vérifie que le serveur répond (code de sortie 0 = ok)")
	sauvegarde := flag.String("sauvegarde", "", "copie la base dans ce fichier (un nom seul : dans FC_DONNEES/sauvegardes)")
	restaurer := flag.String("restaurer", "", "remet cette sauvegarde en place (le serveur doit être arrêté)")
	afficherVersion := flag.Bool("version", false, "affiche la version")
	flag.Parse()

	if *afficherVersion {
		fmt.Println("familycocon", version.Version, version.Commit)
		return
	}
	cfg, err := config.Lire()
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration :", err)
		os.Exit(2)
	}
	switch {
	case *sante:
		os.Exit(verifierSante(cfg))
	case *restaurer != "":
		ancienne, err := base.Restaurer(context.Background(), *restaurer, cfg.Donnees)
		if err != nil {
			fmt.Fprintln(os.Stderr, "restauration :", err)
			os.Exit(1)
		}
		fmt.Println("base restaurée depuis", *restaurer)
		if ancienne != "" {
			fmt.Println("ancienne base gardée :", ancienne)
		}
		return
	case *sauvegarde != "":
		if err := sauvegarder(cfg, *sauvegarde); err != nil {
			fmt.Fprintln(os.Stderr, "sauvegarde :", err)
			os.Exit(1)
		}
		return
	}
	if err := servir(cfg); err != nil {
		slog.Error("arrêt sur erreur", "erreur", err)
		os.Exit(1)
	}
}

func nouveauJournal(cfg *config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.NiveauLogs}
	if cfg.FormatLogs == "json" {
		return slog.New(slog.NewJSONHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, opts))
}

func ouvrirBase(cfg *config.Config) (*sql.DB, error) {
	return base.Ouvrir(filepath.Join(cfg.Donnees, base.NomFichier))
}

func servir(cfg *config.Config) error {
	journal := nouveauJournal(cfg)
	slog.SetDefault(journal)
	journal.Info("démarrage de FamilyCocon", "version", version.Version, "adresse", cfg.Adresse,
		"url", cfg.URLPublique.String(), "donnees", cfg.Donnees)

	db, err := ouvrirBase(cfg)
	if err != nil {
		return fmt.Errorf("ouverture de la base : %w", err)
	}
	defer db.Close()
	ctx, arreter := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer arreter()

	copie, err := base.Migrer(ctx, db, filepath.Join(cfg.Donnees, "sauvegardes"))
	if err != nil {
		return err
	}
	if copie != "" {
		journal.Info("base mise à jour ; copie d'avant la mise à jour gardée", "copie", copie)
	}

	a, err := appli.Nouvelle(cfg, db, journal)
	if err != nil {
		return err
	}
	if err := brancherModules(a); err != nil {
		return err
	}
	if err := a.ChargerGabarits(); err != nil {
		return fmt.Errorf("gabarits : %w", err)
	}
	a.Demarrer(ctx) // tâches de fond (ménage des sessions, sauvegardes…)

	srv := &http.Server{
		Addr:              cfg.Adresse,
		Handler:           a.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second, // levé pour le flux temps réel (SSE)
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          slog.NewLogLogger(journal.Handler(), slog.LevelWarn),
	}
	erreurs := make(chan error, 1)
	go func() { erreurs <- srv.ListenAndServe() }()
	journal.Info("serveur prêt", "adresse", cfg.Adresse)

	select {
	case err := <-erreurs:
		return err
	case <-ctx.Done():
	}
	journal.Info("arrêt demandé")
	a.Arreter() // ferme les flux temps réel pour que Shutdown n'attende pas
	fin, annuler := context.WithTimeout(context.Background(), 10*time.Second)
	defer annuler()
	if err := srv.Shutdown(fin); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	journal.Info("arrêt terminé")
	return nil
}

// verifierSante interroge /health sur la machine même (HEALTHCHECK Docker :
// l'image n'a ni curl ni wget).
func verifierSante(cfg *config.Config) int {
	hote, port, err := net.SplitHostPort(cfg.Adresse)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if hote == "" || hote == "0.0.0.0" || hote == "::" {
		hote = "127.0.0.1"
	}
	client := &http.Client{Timeout: 4 * time.Second}
	rep, err := client.Get("http://" + net.JoinHostPort(hote, port) + "/health")
	if err != nil {
		fmt.Fprintln(os.Stderr, "santé :", err)
		return 1
	}
	defer rep.Body.Close()
	if rep.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "santé : statut", rep.StatusCode)
		return 1
	}
	return 0
}

// sauvegarder copie la base à chaud, puis vérifie la copie. Cible « - » :
// la copie est écrite sur la sortie standard (docker exec … > fichier.db).
func sauvegarder(cfg *config.Config, cible string) error {
	flux := cible == "-"
	if flux {
		cible = filepath.Join(cfg.Donnees, "sauvegardes", fmt.Sprintf(".flux-%d.db", time.Now().UnixNano()))
	} else if !strings.ContainsRune(cible, '/') {
		cible = filepath.Join(cfg.Donnees, "sauvegardes", cible)
	}
	if err := os.MkdirAll(filepath.Dir(cible), 0o700); err != nil {
		return err
	}
	db, err := ouvrirBase(cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, annuler := context.WithTimeout(context.Background(), 5*time.Minute)
	defer annuler()
	if _, err := base.CopierVers(ctx, db, cible); err != nil {
		return err
	}
	if flux {
		defer os.Remove(cible)
	}
	copie, err := base.OuvrirLecture(cible)
	if err != nil {
		return err
	}
	err = base.Verifier(ctx, copie)
	copie.Close()
	if err != nil {
		return err
	}
	if flux {
		f, err := os.Open(cible)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(os.Stdout, f)
		return err
	}
	fmt.Println("sauvegarde faite :", cible)
	return nil
}
