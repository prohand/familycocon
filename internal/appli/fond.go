// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"familycocon/internal/base"
)

// Demarrer lance les tâches de fond : ménage des sessions, invitations et
// appareils périmés (toutes les heures).
func (a *App) Demarrer(ctx context.Context) {
	if err := a.DemarrerPush(ctx); err != nil {
		a.Journal.Error("notifications push indisponibles", "erreur", err)
	}
	go a.rappelsQuotidiens(ctx)
	if a.Config.SauvegardeA != "" {
		go a.sauvegardesAuto(ctx)
	}
	go func() {
		minuteur := time.NewTicker(time.Hour)
		defer minuteur.Stop()
		for {
			a.Menage(ctx)
			select {
			case <-ctx.Done():
				return
			case <-minuteur.C:
			}
		}
	}()
}

// rattraper : à appeler avec defer dans une tâche de fond. Une panique y est
// écrite au journal au lieu d'arrêter tout le serveur (les requêtes HTTP ont
// la même protection dans Envelopper).
func (a *App) rattraper(tache string) {
	if v := recover(); v != nil {
		a.Journal.Error("panique dans une tâche de fond", "tache", tache, "erreur", v, "pile", string(debug.Stack()))
	}
}

// Menage supprime ce qui a expiré.
func (a *App) Menage(ctx context.Context) {
	defer a.rattraper("ménage")
	t := time.Now()
	s, err1 := a.Q.PurgerSessions(ctx, t.Unix())
	i, err2 := a.Q.PurgerInvitations(ctx, t.Unix())
	// Appareil de confiance oublié après 400 jours sans visite.
	ap, err3 := a.Q.PurgerAppareils(ctx, t.Add(-400*24*time.Hour).Unix())
	for _, err := range []error{err1, err2, err3} {
		if err != nil && ctx.Err() == nil {
			a.Journal.Error("ménage", "erreur", err)
		}
	}
	if s+i+ap > 0 {
		a.Journal.Debug("ménage", "sessions", s, "invitations", i, "appareils", ap)
	}
}

// Arreter ferme proprement ce qui doit l'être avant l'arrêt du serveur.
func (a *App) Arreter() { a.Direct.fermer() }

// prochaine : la prochaine heure « HH:MM » (heure du serveur, variable TZ).
func prochaine(maintenant time.Time, hhmm string) time.Time {
	h, _ := time.Parse("15:04", hhmm)
	t := time.Date(maintenant.Year(), maintenant.Month(), maintenant.Day(), h.Hour(), h.Minute(), 0, 0, maintenant.Location())
	if !t.After(maintenant) {
		t = t.AddDate(0, 0, 1)
	}
	return t
}

// sauvegardesAuto : une sauvegarde vérifiée chaque jour à FC_SAUVEGARDE_HEURE,
// dans FC_DONNEES/sauvegardes ; seules les FC_SAUVEGARDE_GARDER dernières
// sont gardées.
func (a *App) sauvegardesAuto(ctx context.Context) {
	dossier := filepath.Join(a.Config.Donnees, "sauvegardes")
	for {
		attente := time.Until(prochaine(time.Now(), a.Config.SauvegardeA))
		a.Journal.Debug("prochaine sauvegarde automatique", "dans", attente.Round(time.Minute).String())
		minuteur := time.NewTimer(attente)
		select {
		case <-ctx.Done():
			minuteur.Stop()
			return
		case <-minuteur.C:
		}
		func() {
			defer a.rattraper("sauvegarde automatique")
			debut := time.Now()
			chemin, err := base.SauvegardeAuto(ctx, a.DB, dossier, a.Config.SauvegardeNb)
			if err != nil {
				a.Journal.Error("sauvegarde automatique ratée", "erreur", err)
				// Montrée aux parents et à la supervision (sans le chemin).
				a.noterSauvegarde(ctx, EtatSauvegarde{Le: time.Now().Unix(), Erreur: "la sauvegarde a échoué : voir le journal du serveur"})
				return
			}
			etat := EtatSauvegarde{Le: time.Now().Unix(), OK: true}
			if fi, err := os.Stat(chemin); err == nil {
				etat.Taille = fi.Size()
			}
			a.noterSauvegarde(ctx, etat)
			// Les photos jointes (tickets) sont des fichiers à côté de la base :
			// les nouvelles sont copiées dans sauvegardes/pieces.
			if n, err := CopierNouveaux(a.DossierPieces(), filepath.Join(dossier, "pieces")); err != nil {
				a.Journal.Error("sauvegarde des photos ratée", "erreur", err)
			} else if n > 0 {
				a.Journal.Info("photos sauvegardées", "nouvelles", n)
			}
			a.Journal.Info("sauvegarde automatique faite", "fichier", chemin, "duree_ms", time.Since(debut).Milliseconds())
		}()
	}
}

// CopierNouveaux : copie de src vers dst les fichiers qui n'y sont pas
// encore (les photos ne changent jamais une fois écrites). Renvoie le
// nombre de fichiers copiés. src absent : rien à faire.
func CopierNouveaux(src, dst string) (int, error) {
	n := 0
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) && p == src {
			return fs.SkipAll
		}
		if err != nil || d.IsDir() || strings.HasSuffix(p, ".tmp") {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		cible := filepath.Join(dst, rel)
		if _, err := os.Stat(cible); err == nil {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(cible), 0o750); err != nil {
			return err
		}
		entree, err := os.Open(p)
		if err != nil {
			return err
		}
		defer entree.Close()
		sortie, err := os.OpenFile(cible+".tmp", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
		if err != nil {
			return err
		}
		if _, err := io.Copy(sortie, entree); err != nil {
			sortie.Close()
			return err
		}
		if err := sortie.Close(); err != nil {
			return err
		}
		n++
		return os.Rename(cible+".tmp", cible)
	})
	return n, err
}

// DossierPieces : le dossier des photos gardées (tickets du carnet, photos
// des notes), à côté de la base : pieces/<famille>/<nom>.jpg.
func (a *App) DossierPieces() string { return filepath.Join(a.Config.Donnees, "pieces") }
