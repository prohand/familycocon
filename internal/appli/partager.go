// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"
)

// Partager : un texte partagé depuis une autre appli du téléphone (Android :
// « Partager › FamilyCocon », share_target du manifeste) arrive sur
// /partager, qui propose de l'envoyer aux courses, en note, à l'agenda…
// Chaque module fournit son action.

// Partage : une action de la page « Partager ».
//   - Poster faux : lien vers URL?titre=…&texte=… (formulaire prérempli) ;
//   - Poster vrai : formulaire envoyé à URL (champs titre et texte).
type Partage struct {
	Module string
	Emoji  string
	Texte  string
	URL    string
	Poster bool
	Lien   string // calculé : URL avec le texte (Poster faux)
}

// AjouterPartage : le module propose une action pour un texte partagé.
func (a *App) AjouterPartage(p Partage) { a.partages = append(a.partages, p) }

// partageMax : texte partagé gardé (le reste est coupé).
const partageMax = 2000

type donneesPartager struct {
	Titre, Texte string
	Actions      []Partage
}

func (a *App) pagePartager(w http.ResponseWriter, r *http.Request, c *Contexte) {
	q := r.URL.Query()
	d := donneesPartager{Titre: couper(strings.TrimSpace(q.Get("titre")), 80)}
	// Le lien partagé (page web) va à la fin du texte.
	morceaux := []string{}
	for _, x := range []string{q.Get("texte"), q.Get("lien")} {
		if x = strings.TrimSpace(x); x != "" && !strings.Contains(strings.Join(morceaux, "\n"), x) {
			morceaux = append(morceaux, x)
		}
	}
	d.Texte = couper(strings.Join(morceaux, "\n"), partageMax)
	if d.Titre == "" && d.Texte == "" {
		a.Rediriger(w, r, "/ajouter")
		return
	}
	v := url.Values{"titre": {d.Titre}, "texte": {d.Texte}}.Encode()
	for _, p := range a.partages {
		m, ok := ModuleParCle(p.Module)
		if !ok || !c.Actif(p.Module) || (m.Parents && !c.EstParent()) {
			continue
		}
		if !p.Poster {
			p.Lien = p.URL + "?" + v
		}
		d.Actions = append(d.Actions, p)
	}
	a.Afficher(w, r, c, "partager", "Partager", "", d)
}

// couper : au plus n caractères.
func couper(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// LirePrerempli : titre et texte passés à un formulaire par la page
// « Partager » (bornés).
func LirePrerempli(r *http.Request, maxTitre, maxTexte int) (titre, texte string) {
	q := r.URL.Query()
	return couper(strings.TrimSpace(q.Get("titre")), maxTitre), couper(strings.TrimSpace(q.Get("texte")), maxTexte)
}
