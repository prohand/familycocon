// SPDX-License-Identifier: GPL-3.0-or-later

package web

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

// Un bloc qui se recharge lui-même (hx-select="#maj") ne doit pas passer
// ses réglages à ses liens : htmx fait hériter hx-select, et un lien
// (navigation hx-boost) ne garderait de la page suivante que son bloc
// « #maj » (page vide s'il n'y en a pas). D'où hx-disinherit="*".
func TestHxSelectSansHeritage(t *testing.T) {
	balise := regexp.MustCompile(`<[a-z]+[^>]*hx-select=[^>]*>`)
	err := fs.WalkDir(Fichiers, "gabarits", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(Fichiers, p)
		if err != nil {
			return err
		}
		for _, m := range balise.FindAllString(string(b), -1) {
			if !strings.Contains(m, `hx-disinherit="*"`) {
				t.Errorf("%s : hx-select sans hx-disinherit : %.90s…", p, m)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
