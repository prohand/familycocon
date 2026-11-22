// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"context"
	"time"
)

// Demarrer lance les tâches de fond : ménage des sessions, invitations et
// appareils périmés (toutes les heures).
func (a *App) Demarrer(ctx context.Context) {
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

// Menage supprime ce qui a expiré.
func (a *App) Menage(ctx context.Context) {
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
func (a *App) Arreter() {}
