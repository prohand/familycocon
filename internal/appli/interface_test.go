// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Thème choisi dans Moi : posé sur la page (data-theme), gardé par l'appareil.
func TestThemeChoisi(t *testing.T) {
	_, _, marie := familleTest(t)
	if _, page := marie.get("/moi"); strings.Contains(page, "data-theme=") {
		t.Fatal("thème forcé sans choix")
	}
	marie.post("/moi/theme", map[string]string{"theme": "sombre"})
	if _, page := marie.get("/"); !strings.Contains(page, `<html lang="fr" data-theme="sombre">`) {
		t.Fatal("thème sombre non appliqué")
	}
	if code, _ := marie.post("/moi/theme", map[string]string{"theme": "violet"}); code != 400 {
		t.Fatalf("thème inconnu accepté : %d", code)
	}
	marie.post("/moi/theme", map[string]string{"theme": ""})
	if _, page := marie.get("/"); strings.Contains(page, "data-theme=") {
		t.Fatal("retour à l'automatique sans effet")
	}
}

// Avatar : choisi dans Moi (liste fermée), affiché à la place de l'initiale.
func TestAvatar(t *testing.T) {
	_, _, marie := familleTest(t)
	if _, page := marie.get("/moi"); !strings.Contains(page, `name="avatar" value="🦊"`) {
		t.Fatal("choix de l'avatar absent")
	}
	marie.post("/moi/profil", map[string]string{"prenom": "Marie", "couleur": "2", "avatar": "🦊"})
	if _, page := marie.get("/"); !strings.Contains(page, `title="Marie">🦊</span>`) {
		t.Fatal("avatar absent de l'en-tête")
	}
	// Un emoji hors de la liste n'est pas accepté (l'ancien reste).
	marie.post("/moi/profil", map[string]string{"prenom": "Marie", "couleur": "2", "avatar": "<b>x</b>"})
	if _, page := marie.get("/"); !strings.Contains(page, `title="Marie">🦊</span>`) {
		t.Fatal("avatar hors liste accepté")
	}
	// Retour à l'initiale.
	marie.post("/moi/profil", map[string]string{"prenom": "Marie", "couleur": "2", "avatar": ""})
	if _, page := marie.get("/"); !strings.Contains(page, `title="Marie">M</span>`) {
		t.Fatal("retour à l'initiale impossible")
	}
}

// Photos effacées de l'appli : rangées à part dans la sauvegarde, puis
// effacées après « garder » jours.
func TestRangerEffaces(t *testing.T) {
	src, dst, corbeille := t.TempDir(), t.TempDir(), filepath.Join(t.TempDir(), "effacees")
	_ = os.MkdirAll(filepath.Join(src, "1"), 0o750)
	_ = os.WriteFile(filepath.Join(src, "1", "garde.jpg"), []byte("g"), 0o640)
	_ = os.WriteFile(filepath.Join(src, "1", "efface.jpg"), []byte("e"), 0o640)
	if _, err := CopierNouveaux(src, dst); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(filepath.Join(src, "1", "efface.jpg"))
	j1 := time.Date(2026, 11, 1, 3, 0, 0, 0, time.UTC)
	if n, err := RangerEffaces(src, dst, corbeille, 7, j1); err != nil || n != 1 {
		t.Fatalf("rangement : %d %v", n, err)
	}
	if _, err := os.Stat(filepath.Join(dst, "1", "garde.jpg")); err != nil {
		t.Fatal("photo encore utilisée retirée de la sauvegarde")
	}
	if _, err := os.Stat(filepath.Join(corbeille, "2026-11-01", "1", "efface.jpg")); err != nil {
		t.Fatal("photo effacée pas rangée à part")
	}
	// 5 jours après : encore gardée ; 8 jours après : effacée.
	_, _ = RangerEffaces(src, dst, corbeille, 7, j1.AddDate(0, 0, 5))
	if _, err := os.Stat(filepath.Join(corbeille, "2026-11-01")); err != nil {
		t.Fatal("effacée trop tôt")
	}
	_, _ = RangerEffaces(src, dst, corbeille, 7, j1.AddDate(0, 0, 8))
	if _, err := os.Stat(filepath.Join(corbeille, "2026-11-01")); err == nil {
		t.Fatal("jamais effacée")
	}
}

// Sauvegarde des photos : seules les nouvelles sont copiées.
func TestCopierNouveaux(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	_ = os.MkdirAll(filepath.Join(src, "1"), 0o750)
	_ = os.WriteFile(filepath.Join(src, "1", "a.jpg"), []byte("a"), 0o640)
	_ = os.WriteFile(filepath.Join(src, "1", "b.jpg.tmp"), []byte("b"), 0o640)
	if n, err := CopierNouveaux(src, dst); err != nil || n != 1 {
		t.Fatalf("première copie : %d %v", n, err)
	}
	if n, _ := CopierNouveaux(src, dst); n != 0 {
		t.Fatalf("recopie : %d", n)
	}
	if b, _ := os.ReadFile(filepath.Join(dst, "1", "a.jpg")); string(b) != "a" {
		t.Fatal("contenu")
	}
	if n, err := CopierNouveaux(filepath.Join(src, "absent"), dst); err != nil || n != 0 {
		t.Fatalf("dossier absent : %d %v", n, err)
	}
}
