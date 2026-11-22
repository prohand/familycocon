// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"log/slog"
	"testing"
)

func TestNiveauRequete(t *testing.T) {
	cas := []struct {
		chemin  string
		statut  int
		attendu slog.Level
	}{
		{"/", 200, slog.LevelDebug},
		{"/taches/contenu", 200, slog.LevelDebug},
		{"/connexion", 303, slog.LevelDebug},
		{"/budget", 403, slog.LevelInfo},
		{"/wp-login.php", 404, slog.LevelInfo},
		{"/favicon.ico", 404, slog.LevelDebug},
		{"/connexion", 429, slog.LevelWarn},
		{"/courses", 500, slog.LevelError},
	}
	for _, c := range cas {
		if n := niveauRequete(c.chemin, c.statut); n != c.attendu {
			t.Errorf("%s %d : %v, attendu %v", c.chemin, c.statut, n, c.attendu)
		}
	}
}
