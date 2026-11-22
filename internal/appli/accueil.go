// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"bytes"
	"context"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"time"
)

// Accueil : une grande tuile par module activé, avec un petit résumé
// (« 8 produits à prendre ») fourni par le module, puis les blocs du jour
// (agenda, tâches…) fournis eux aussi par les modules.

// Resume : ce qu'affiche une tuile.
type Resume struct {
	Info  string // sous le nom du module
	Badge int    // pastille en haut à droite (0 : pas de pastille)
}

// ResumeFunc calcule le résumé d'un module pour ce membre.
type ResumeFunc func(ctx context.Context, c *Contexte) Resume

// BlocAccueil : un bloc sous les tuiles (« Aujourd'hui », « Tâches du jour »).
type BlocAccueil struct {
	Module string
	Ordre  int
	// Rendre renvoie le HTML du bloc (vide : pas de bloc).
	Rendre func(ctx context.Context, c *Contexte) (template.HTML, error)
}

// Ajout : une action du bouton « + ».
type Ajout struct {
	Module string
	Emoji  string
	Texte  string
	URL    string
}

// AjouterResume : le module fournit le résumé de sa tuile.
func (a *App) AjouterResume(module string, f ResumeFunc) { a.resumes[module] = f }

// AjouterBloc : le module ajoute un bloc à l'accueil.
func (a *App) AjouterBloc(b BlocAccueil) {
	a.blocs = append(a.blocs, b)
	sort.SliceStable(a.blocs, func(i, j int) bool { return a.blocs[i].Ordre < a.blocs[j].Ordre })
}

// AjouterAction : le module ajoute une action au bouton « + ».
func (a *App) AjouterAction(aj Ajout) { a.ajouts = append(a.ajouts, aj) }

// RendreBloc : HTML d'un bloc de gabarit (pour les blocs de l'accueil).
func (a *App) RendreBloc(page, bloc string, c *Contexte, donnees any) (template.HTML, error) {
	t, ok := a.Gabarits.pages[page]
	if !ok {
		return "", nil
	}
	var b bytes.Buffer
	v := &Vue{C: c, Donnees: donnees}
	if err := t.ExecuteTemplate(&b, bloc, v); err != nil {
		return "", err
	}
	return template.HTML(b.String()), nil // #nosec G203 -- produit par html/template (déjà échappé)
}

type tuile struct {
	ModuleDef
	Resume
}

type donneesAccueil struct {
	Date   string
	Tuiles []tuile
	Blocs  []template.HTML
}

func (a *App) routesAccueil() {
	a.Route("GET /{$}", "", Membre, a.pageAccueil)
	a.Route("GET /ajouter", "", Membre, a.pageAjouter)
}

func (a *App) pageAccueil(w http.ResponseWriter, r *http.Request, c *Contexte) {
	ctx := r.Context()
	d := donneesAccueil{Date: DateLongue(c.Maintenant())}
	for _, m := range ModulesVisibles(c) {
		t := tuile{ModuleDef: m}
		if f, ok := a.resumes[m.Cle]; ok {
			t.Resume = f(ctx, c)
		}
		d.Tuiles = append(d.Tuiles, t)
	}
	for _, b := range a.blocs {
		if !c.Actif(b.Module) {
			continue
		}
		h, err := b.Rendre(ctx, c)
		if err != nil {
			a.Journal.Error("bloc de l'accueil", "module", b.Module, "erreur", err)
			continue
		}
		if h != "" {
			d.Blocs = append(d.Blocs, h)
		}
	}
	a.Afficher(w, r, c, "accueil", "Accueil", "accueil", d)
}

func (a *App) pageAjouter(w http.ResponseWriter, r *http.Request, c *Contexte) {
	var actions []Ajout
	for _, aj := range a.ajouts {
		m, ok := ModuleParCle(aj.Module)
		if ok && c.Actif(aj.Module) && (!m.Parents || c.EstParent()) {
			actions = append(actions, aj)
		}
	}
	a.Afficher(w, r, c, "ajouter", "Ajouter", "", actions)
}

var (
	joursFR = []string{"dimanche", "lundi", "mardi", "mercredi", "jeudi", "vendredi", "samedi"}
	moisFR  = []string{"janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"}
)

// JourFR : « lundi ».
func JourFR(t time.Time) string { return joursFR[t.Weekday()] }

// MoisFR : « novembre ».
func MoisFR(m time.Month) string { return moisFR[m-1] }

// DateLongue : « Dimanche 22 novembre ».
func DateLongue(t time.Time) string {
	j := JourFR(t)
	return string(j[0]-32) + j[1:] + " " + strconv.Itoa(t.Day()) + " " + MoisFR(t.Month())
}
