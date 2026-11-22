// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import "context"

// Demarrer lance les tâches de fond (ménage, sauvegardes automatiques…).
func (a *App) Demarrer(ctx context.Context) {
	_ = ctx
}

// Arreter ferme proprement ce qui doit l'être avant l'arrêt du serveur.
func (a *App) Arreter() {}
