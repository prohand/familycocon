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

// Les jetons secrets d'un chemin ne vont pas au journal.
func TestCheminJournal(t *testing.T) {
	for v, attendu := range map[string]string{
		"/invitation/abcdef": "/invitation/…",
		"/ical/xyz":          "/ical/…",
		"/annuler/123":       "/annuler/…",
		"/notes/3":           "/notes/3",
		"/invitation/":       "/invitation/",
	} {
		if r := cheminJournal(v); r != attendu {
			t.Errorf("cheminJournal(%q) = %q, attendu %q", v, r, attendu)
		}
	}
}
