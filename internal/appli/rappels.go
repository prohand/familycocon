// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"context"
	"fmt"
	"html/template"
	"sort"
	"strconv"
	"time"

	"familycocon/internal/donnees"
)

// Rappels : les dates à ne pas oublier (anniversaires, contrôle technique,
// vaccin du chien, fin d'un projet…). Chaque module fournit ses échéances ;
// l'accueil montre « À venir » (30 jours), et chaque matin une notification
// rappelle ce qui tombe aujourd'hui et dans 7 jours.

// Echeance : une date à venir.
type Echeance struct {
	Jour    time.Time // à minuit, dans le fuseau de la famille
	Titre   string    // « Anniversaire de Mamie (80 ans) »
	Emoji   string
	URL     string
	Module  string
	Parents bool // module réservé aux parents (budget, abonnements) : rien pour les enfants
}

// EcheancesFunc : échéances d'un module pour une famille, du jour debut
// (inclus) au jour fin (exclu).
type EcheancesFunc func(ctx context.Context, f donnees.Famille, debut, fin time.Time) ([]Echeance, error)

// AjouterEcheances : le module fournit ses échéances.
func (a *App) AjouterEcheances(module string, f EcheancesFunc) { a.echeances[module] = f }

// moduleRappels : clé des notifications de rappel (page Moi › Notifications).
const moduleRappels = "rappels"

// PushParDefaut : modules qui notifient un nouveau membre.
const PushParDefaut = "agenda,taches,menus,enfants," + moduleRappels

// lieuFamille : le fuseau de la famille.
func lieuFamille(f donnees.Famille) *time.Location {
	if l, err := time.LoadLocation(f.Fuseau); err == nil {
		return l
	}
	return time.UTC
}

// minuitDe : le jour t à 00:00.
func minuitDe(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// Echeances : les échéances des modules activés, triées par date.
func (a *App) Echeances(ctx context.Context, f donnees.Famille, debut, fin time.Time) []Echeance {
	actifs := map[string]bool{}
	for _, m := range splitModules(f.Modules) {
		actifs[m] = true
	}
	var out []Echeance
	for module, fn := range a.echeances {
		if !actifs[module] {
			continue
		}
		l, err := fn(ctx, f, debut, fin)
		if err != nil {
			a.Journal.Error("échéances", "module", module, "erreur", err)
			continue
		}
		if def, ok := ModuleParCle(module); ok && def.Parents {
			for i := range l {
				l[i].Parents = true
			}
		}
		out = append(out, l...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Jour.Equal(out[j].Jour) {
			return out[i].Jour.Before(out[j].Jour)
		}
		return out[i].Titre < out[j].Titre
	})
	return out
}

func splitModules(s string) []string {
	var out []string
	debut := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			if i > debut {
				out = append(out, s[debut:i])
			}
			debut = i + 1
		}
	}
	return out
}

// Quand : « Aujourd'hui », « Demain », « Dans 5 jours », « Samedi 12 décembre ».
func Quand(jour, aujourdhui time.Time) string {
	n := int(minuitDe(jour).Sub(minuitDe(aujourdhui)).Hours()/24 + 0.5)
	switch {
	case n == 0:
		return "Aujourd'hui"
	case n == 1:
		return "Demain"
	case n > 1 && n <= 7:
		return "Dans " + strconv.Itoa(n) + " jours"
	}
	return DateLongue(jour)
}

type ligneAVenir struct {
	Echeance
	Quand  string
	Proche bool // aujourd'hui ou demain
}

// blocAVenir : bloc de l'accueil, les 30 prochains jours.
func (a *App) blocAVenir(ctx context.Context, c *Contexte) (template.HTML, error) {
	auj := minuitDe(c.Maintenant())
	var ech []Echeance
	for _, e := range a.Echeances(ctx, c.Famille, auj, auj.AddDate(0, 0, 31)) {
		if !e.Parents || c.EstParent() {
			ech = append(ech, e)
		}
	}
	if len(ech) == 0 {
		return "", nil
	}
	if len(ech) > 6 {
		ech = ech[:6]
	}
	var lignes []ligneAVenir
	for _, e := range ech {
		lignes = append(lignes, ligneAVenir{Echeance: e, Quand: Quand(e.Jour, auj), Proche: e.Jour.Sub(auj) < 48*time.Hour})
	}
	return a.RendreBloc("accueil", "a-venir", c, lignes)
}

// heureRappels : l'heure des rappels, dans le fuseau de chaque famille.
const heureRappels = 9

// finRattrapage : après cette heure, un rappel manqué n'est plus envoyé
// (pas de notification tard le soir).
const finRattrapage = 20

// rappelsQuotidiens : au démarrage puis à chaque heure pile, les familles
// pour qui il est 9 h passées (dans leur fuseau) et qui n'ont pas encore eu
// leurs rappels du jour reçoivent une notification pour ce qui tombe
// aujourd'hui et dans 7 jours. Le jour envoyé est noté en base : un serveur
// arrêté à 9 h rattrape à son retour, et rien n'est envoyé deux fois.
func (a *App) rappelsQuotidiens(ctx context.Context) {
	a.envoyerRappelsA(ctx, time.Now(), true)
	for {
		maintenant := time.Now()
		minuteur := time.NewTimer(maintenant.Truncate(time.Hour).Add(time.Hour).Sub(maintenant))
		select {
		case <-ctx.Done():
			minuteur.Stop()
			return
		case t := <-minuteur.C:
			a.envoyerRappelsA(ctx, t, true)
		}
	}
}

// cleRappels : nom de la ligne « rappels envoyés le » d'une famille (table serveur).
func cleRappels(famille int64) string { return "rappels:" + strconv.FormatInt(famille, 10) }

// EnvoyerRappels : les notifications de rappel du jour (toutes les familles).
func (a *App) EnvoyerRappels(ctx context.Context, maintenant time.Time) int {
	return a.envoyerRappelsA(ctx, maintenant, false)
}

// envoyerRappelsA : si seule9h, seulement aux familles pour qui il est 9 h
// passées et qui n'ont pas encore eu leurs rappels du jour.
func (a *App) envoyerRappelsA(ctx context.Context, maintenant time.Time, seule9h bool) int {
	defer a.rattraper("rappels")
	if a.Push == nil {
		return 0
	}
	familles, err := a.Q.ToutesFamilles(ctx)
	if err != nil {
		a.Journal.Error("rappels : familles", "erreur", err)
		return 0
	}
	n := 0
	for _, f := range familles {
		local := maintenant.Add(time.Minute).In(lieuFamille(f)) // minuteur un peu en avance : arrondi
		auj := minuitDe(local)
		if seule9h {
			if local.Hour() < heureRappels || local.Hour() >= finRattrapage {
				continue
			}
			jour := auj.Format("2006-01-02")
			if dernier, err := a.Q.LireServeur(ctx, cleRappels(f.ID)); err == nil && string(dernier) == jour {
				continue // déjà envoyés aujourd'hui
			}
			if err := a.Q.NoterServeur(ctx, donnees.NoterServeurParams{Nom: cleRappels(f.ID), Valeur: []byte(jour)}); err != nil {
				a.Journal.Error("rappels : jour non noté", "erreur", err)
				continue // plutôt aucun rappel que le même à chaque heure
			}
		}
		for _, decalage := range []int{0, 7} {
			jour := auj.AddDate(0, 0, decalage)
			for _, e := range a.Echeances(ctx, f, jour, jour.AddDate(0, 0, 1)) {
				quand := "Aujourd'hui"
				if decalage == 7 {
					quand = "Dans 7 jours"
				}
				texte := fmt.Sprintf("%s %s : %s", e.Emoji, quand, e.Titre)
				// En attendant une place dans la file : aucun rappel perdu.
				a.Push.annoncerEnAttendant(ctx, annonce{famille: f.ID, module: moduleRappels, texte: texte, parents: e.Parents})
				n++
			}
		}
	}
	if n > 0 {
		a.Journal.Info("rappels envoyés", "nombre", n)
	}
	return n
}
