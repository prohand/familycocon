// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"context"
	"net/http"
	"strings"

	"familycocon/internal/donnees"
)

// ModuleDef : un module de l'application (une tuile de l'accueil).
type ModuleDef struct {
	Cle     string
	Nom     string
	Emoji   string
	URL     string
	Parents bool // réservé aux parents (budget)
}

// Catalogue : tous les modules, dans l'ordre de l'accueil.
var Catalogue = []ModuleDef{
	{Cle: "courses", Nom: "Courses", Emoji: "🛒", URL: "/courses"},
	{Cle: "agenda", Nom: "Agenda", Emoji: "🗓️", URL: "/agenda"},
	{Cle: "taches", Nom: "Tâches", Emoji: "✅", URL: "/taches"},
	{Cle: "menus", Nom: "Menus", Emoji: "🍽️", URL: "/menus"},
	{Cle: "budget", Nom: "Budget", Emoji: "💶", URL: "/budget", Parents: true},
	{Cle: "abonnements", Nom: "Abonnements", Emoji: "📺", URL: "/abonnements", Parents: true},
	{Cle: "enfants", Nom: "Espace enfants", Emoji: "⭐", URL: "/enfants"},
	{Cle: "notes", Nom: "Notes", Emoji: "📝", URL: "/notes"},
	{Cle: "contacts", Nom: "Contacts", Emoji: "👥", URL: "/contacts"},
	{Cle: "anniversaires", Nom: "Anniversaires", Emoji: "🎂", URL: "/anniversaires"},
	{Cle: "adresses", Nom: "Adresses", Emoji: "📇", URL: "/adresses"},
	{Cle: "fidelite", Nom: "Cartes de fidélité", Emoji: "💳", URL: "/fidelite"},
	{Cle: "vehicules", Nom: "Véhicules", Emoji: "🚗", URL: "/vehicules"},
	{Cle: "animaux", Nom: "Animaux", Emoji: "🐾", URL: "/animaux"},
	{Cle: "prestataires", Nom: "Prestataires", Emoji: "🧰", URL: "/prestataires"},
	{Cle: "projets", Nom: "Projets", Emoji: "🎯", URL: "/projets"},
}

// ModulesParDefaut : modules activés pour une nouvelle famille (tous).
func ModulesParDefaut() string {
	var cles []string
	for _, m := range Catalogue {
		cles = append(cles, m.Cle)
	}
	return strings.Join(cles, ",")
}

// ModuleParCle : définition d'un module (ok = faux si la clé est inconnue).
func ModuleParCle(cle string) (ModuleDef, bool) {
	for _, m := range Catalogue {
		if m.Cle == cle {
			return m, true
		}
	}
	return ModuleDef{}, false
}

// Raccourci : une case de la barre du bas.
type Raccourci struct {
	Cle   string
	Nom   string
	Emoji string
	URL   string
}

// raccourcisFixes : pages qui ne sont pas des modules.
var raccourcisFixes = []Raccourci{
	{Cle: "accueil", Nom: "Accueil", Emoji: "🏠", URL: "/"},
	{Cle: "moi", Nom: "Moi", Emoji: "👤", URL: "/moi"},
	{Cle: "famille", Nom: "Famille", Emoji: "🏡", URL: "/famille"},
}

// RaccourcisParDefaut : la barre d'un nouveau membre.
const RaccourcisParDefaut = "accueil,agenda,courses,moi"

// RaccourcisEnfant : la barre d'un nouvel enfant.
const RaccourcisEnfant = "accueil,enfants,taches,moi"

// RaccourcisPossibles : ce qu'un membre peut mettre dans sa barre
// (modules activés et permis, plus Accueil, Moi et, pour un parent, Famille).
func RaccourcisPossibles(c *Contexte) []Raccourci {
	var out []Raccourci
	out = append(out, raccourcisFixes[0], raccourcisFixes[1])
	if c.EstParent() {
		out = append(out, raccourcisFixes[2])
	}
	for _, m := range ModulesVisibles(c) {
		out = append(out, Raccourci{Cle: m.Cle, Nom: m.Nom, Emoji: m.Emoji, URL: m.URL})
	}
	return out
}

// ModulesVisibles : modules activés que ce membre peut ouvrir.
func ModulesVisibles(c *Contexte) []ModuleDef {
	var out []ModuleDef
	for _, m := range Catalogue {
		if c.Actif(m.Cle) && (!m.Parents || c.EstParent()) {
			out = append(out, m)
		}
	}
	return out
}

// Raccourcis : les 4 cases de la barre du bas du membre. Un raccourci vers
// un module désactivé est remplacé (par les cases par défaut, puis par les
// autres possibilités) : la barre a toujours 4 cases.
func Raccourcis(c *Contexte) []Raccourci {
	possibles := map[string]Raccourci{}
	ordre := RaccourcisPossibles(c)
	for _, r := range ordre {
		possibles[r.Cle] = r
	}
	var out []Raccourci
	pris := map[string]bool{}
	ajouter := func(cles []string) {
		for _, k := range cles {
			if r, ok := possibles[k]; ok && !pris[k] && len(out) < 4 {
				out = append(out, r)
				pris[k] = true
			}
		}
	}
	ajouter(strings.Split(c.Membre.Raccourcis, ","))
	ajouter(strings.Split(RaccourcisParDefaut, ","))
	for _, r := range ordre {
		ajouter([]string{r.Cle})
	}
	return out
}

// Vue : données de la mise en page commune, plus celles de la page.
type Vue struct {
	Titre   string
	Module  string // module de la page (onglet actif)
	C       *Contexte
	Membres []donnees.Membre // pour l'en-tête (avatars)
	Barre   []Raccourci      // 4 raccourcis : 2 à gauche du « + », 2 à droite
	Flash   string           // notification à afficher au chargement
	Chemin  string
	Donnees any
}

// Gauche, Droite : les deux moitiés de la barre du bas.
func (v *Vue) Gauche() []Raccourci { return v.Barre[:min(2, len(v.Barre))] }

// Droite : voir Gauche.
func (v *Vue) Droite() []Raccourci { return v.Barre[min(2, len(v.Barre)):] }

// Actif : ce raccourci correspond-il à la page affichée ?
func (v *Vue) Actif(r Raccourci) bool {
	if r.URL == "/" {
		return v.Chemin == "/"
	}
	return v.Chemin == r.URL || strings.HasPrefix(v.Chemin, r.URL+"/")
}

// NouvelleVue prépare la mise en page. w peut être nil (fragment : pas de
// notification « flash » à lire).
func (a *App) NouvelleVue(w http.ResponseWriter, r *http.Request, c *Contexte, titre, module string) *Vue {
	v := &Vue{Titre: titre, Module: module, C: c, Chemin: r.URL.Path}
	if w == nil {
		return v // fragment : ni en-tête ni barre
	}
	v.Flash = a.lireFlash(w, r)
	if c != nil {
		v.Barre = Raccourcis(c)
		v.Membres, _ = a.Membres(context.WithoutCancel(r.Context()), c)
	}
	return v
}
