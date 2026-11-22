// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// Gabarits : pages HTML (html/template : tout texte inséré est échappé,
// donc pas d'injection de HTML ou de script par un nom de produit).
//
// Organisation de web/gabarits :
//   - base.html : la mise en page commune (en-tête, barre du bas) ;
//   - partiels/*.html : morceaux réutilisés (avatar, notification…) ;
//   - pages/**.html : une page par fichier ; « pages/courses/liste.html »
//     s'appelle « courses/liste ». Une page définit le bloc « contenu »,
//     et peut définir d'autres blocs renvoyés seuls à htmx (fragments).
type Gabarits struct {
	pages map[string]*template.Template
}

// ChargerGabarits analyse tous les gabarits au démarrage : une erreur de
// syntaxe empêche le serveur de démarrer (et fait échouer les tests).
func ChargerGabarits(racine fs.FS, fonctions template.FuncMap) (*Gabarits, error) {
	commun := template.New("base.html").Funcs(fonctions)
	if _, err := commun.ParseFS(racine, "base.html", "partiels/*.html"); err != nil {
		return nil, err
	}
	g := &Gabarits{pages: map[string]*template.Template{}}
	err := fs.WalkDir(racine, "pages", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path.Ext(p) != ".html" {
			return err
		}
		t, err := commun.Clone()
		if err != nil {
			return err
		}
		if _, err := t.ParseFS(racine, p); err != nil {
			return err
		}
		nom := strings.TrimSuffix(strings.TrimPrefix(p, "pages/"), ".html")
		g.pages[nom] = t
		return nil
	})
	return g, err
}

// Executer écrit le bloc nom de la page dans w. Le rendu se fait d'abord en
// mémoire : une erreur donne une vraie page d'erreur, pas une page coupée.
func (g *Gabarits) Executer(w http.ResponseWriter, statut int, page, nom string, donnees any) error {
	t, ok := g.pages[page]
	if !ok {
		return fmt.Errorf("gabarit inconnu : %s", page)
	}
	var b bytes.Buffer
	if err := t.ExecuteTemplate(&b, nom, donnees); err != nil {
		return fmt.Errorf("gabarit %s/%s : %w", page, nom, err)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statut)
	_, err := b.WriteTo(w)
	return err
}

// Existe : la page est-elle connue ? (tests)
func (g *Gabarits) Existe(page string) bool {
	_, ok := g.pages[page]
	return ok
}
