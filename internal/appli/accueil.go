// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"bytes"
	"context"
	"html/template"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
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
	// Compte à mot de passe sans double authentification ni passkey : un
	// conseil s'affiche pour le protéger.
	ConseilSecurite bool
}

func (a *App) routesAccueil() {
	a.Route("GET /{$}", "", Membre, a.pageAccueil)
	a.Route("GET /ajouter", "", Membre, a.pageAjouter)
	a.Route("POST /riche/apercu", "", Membre, a.apercuRiche)
	a.Route("GET /partager", "", Membre, a.pagePartager)
	a.Route("GET /chercher", "", Membre, a.pageChercher)
}

func (a *App) pageAccueil(w http.ResponseWriter, r *http.Request, c *Contexte) {
	ctx := r.Context()
	d := donneesAccueil{Date: DateLongue(c.Maintenant())}
	if c.Membre.Mdp.Valid && c.Membre.Totp == "" {
		n, err := a.Q.CompterPasskeys(ctx, c.Membre.ID)
		d.ConseilSecurite = err == nil && n == 0
	}
	// Dans l'ordre choisi par le membre (« Mon accueil »), sans ceux qu'il
	// a masqués.
	for _, m := range ModulesAccueil(c) {
		if m.Masque {
			continue
		}
		t := tuile{ModuleDef: m.ModuleDef}
		if f, ok := a.resumes[m.Cle]; ok {
			t.Resume = f(ctx, c)
		}
		d.Tuiles = append(d.Tuiles, t)
	}
	for _, b := range a.blocs {
		if b.Module != "" && !c.Actif(b.Module) { // "" : bloc commun (« À venir »)
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

// groupesAjout : la page « Ajouter » rangée par groupes (un module absent
// d'ici va dans « Autres »).
var groupesAjout = []struct {
	Nom     string
	Modules []string
}{
	{"🏠 Au quotidien", []string{"courses", "taches", "agenda", "menus", "notes"}},
	{"💶 Argent", []string{"budget", "abonnements", "fidelite"}},
	{"👨‍👩‍👧 Famille", []string{"enfants", "contacts", "anniversaires", "adresses"}},
	{"🔧 Maison et véhicules", []string{"vehicules", "animaux", "prestataires", "garanties", "projets"}},
}

// GroupeAjout : un groupe de la page « Ajouter ».
type GroupeAjout struct {
	Nom     string
	Actions []Ajout
}

func (a *App) pageAjouter(w http.ResponseWriter, r *http.Request, c *Contexte) {
	var actions []Ajout
	for _, aj := range a.ajouts {
		m, ok := ModuleParCle(aj.Module)
		if ok && c.Actif(aj.Module) && (!m.Parents || c.EstParent()) {
			actions = append(actions, aj)
		}
	}
	// Rangées par groupes, dans l'ordre des groupes puis de l'enregistrement.
	pris := make([]bool, len(actions))
	var groupes []GroupeAjout
	for _, g := range groupesAjout {
		ga := GroupeAjout{Nom: g.Nom}
		for i, aj := range actions {
			if !pris[i] && slices.Contains(g.Modules, aj.Module) {
				pris[i] = true
				ga.Actions = append(ga.Actions, aj)
			}
		}
		if len(ga.Actions) > 0 {
			groupes = append(groupes, ga)
		}
	}
	autres := GroupeAjout{Nom: "Autres"}
	for i, aj := range actions {
		if !pris[i] {
			autres.Actions = append(autres.Actions, aj)
		}
	}
	if len(autres.Actions) > 0 {
		groupes = append(groupes, autres)
	}
	a.Afficher(w, r, c, "ajouter", "Ajouter", "", groupes)
}

var (
	joursFR = []string{"dimanche", "lundi", "mardi", "mercredi", "jeudi", "vendredi", "samedi"}
	moisFR  = []string{"janvier", "février", "mars", "avril", "mai", "juin", "juillet", "août", "septembre", "octobre", "novembre", "décembre"}
)

// JourFR : « lundi ».
func JourFR(t time.Time) string { return joursFR[t.Weekday()] }

// DateCourte : « 28/11 », ou « 14/01/2027 » si ce n'est pas l'année de
// maintenant (un rappel ou une échéance de l'an prochain).
func DateCourte(t, maintenant time.Time) string {
	if t.Year() != maintenant.Year() {
		return t.Format("02/01/2006")
	}
	return t.Format("02/01")
}

// MoisFR : « novembre ».
func MoisFR(m time.Month) string { return moisFR[m-1] }

// DateLongue : « Dimanche 22 novembre ».
func DateLongue(t time.Time) string {
	j := JourFR(t)
	return string(j[0]-32) + j[1:] + " " + strconv.Itoa(t.Day()) + " " + MoisFR(t.Month())
}

// ModuleAccueil : un module de l'accueil d'un membre, masqué ou non.
type ModuleAccueil struct {
	ModuleDef
	Masque bool
}

// ModulesAccueil : les modules visibles du membre, dans l'ordre qu'il a
// choisi (membres.accueil : « agenda,courses,-animaux » ; « - » : masqué).
// Les modules absents de son choix (nouveaux) suivent, dans l'ordre du
// catalogue.
func ModulesAccueil(c *Contexte) []ModuleAccueil {
	visibles := ModulesVisibles(c)
	var out []ModuleAccueil
	pris := map[string]bool{}
	for _, x := range strings.Split(c.Membre.Accueil, ",") {
		cle, masque := strings.CutPrefix(strings.TrimSpace(x), "-")
		for _, m := range visibles {
			if m.Cle == cle && !pris[cle] {
				pris[cle] = true
				out = append(out, ModuleAccueil{ModuleDef: m, Masque: masque})
			}
		}
	}
	for _, m := range visibles {
		if !pris[m.Cle] {
			out = append(out, ModuleAccueil{ModuleDef: m})
		}
	}
	return out
}
