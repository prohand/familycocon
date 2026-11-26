// SPDX-License-Identifier: GPL-3.0-or-later

package foyer

import (
	"context"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
	"familycocon/internal/suivi"
)

// Dépenses d'un véhicule au budget (parents, budget activé) : un réglage
// par véhicule, « Non », « Compte commun » ou « Mon budget perso ».
//   - changé (fiche « Modifier » ou reprise Fuelio) : il vaut aussi pour
//     tout ce qui est déjà au carnet (rétroactif) ;
//   - il pré-règle « Ajouter aussi au budget » des pleins et entretiens ;
//   - la reprise Fuelio ajoute au budget les lignes reprises.

// Valeurs du choix dans les formulaires (« garder » : ne rien changer).
const (
	choixGarder = "garder"
	choixNon    = "non"
)

// OptionBudget : une option de la liste « 💶 Dépenses au budget ».
type OptionBudget struct {
	Valeur, Texte string
	Actif         bool
}

// peutBudget : le membre peut-il régler le budget d'un véhicule ?
func peutBudget(c *appli.Contexte) bool { return c.EstParent() && c.Actif("budget") }

// optionsBudget : les choix pour ce véhicule (x nil : la reprise Fuelio de
// tous les véhicules, où « garder » laisse le réglage de chacun).
func (m *Module) optionsBudget(ctx context.Context, c *appli.Contexte, x *donnees.Vehicule) []OptionBudget {
	if x == nil {
		return []OptionBudget{
			{choixGarder, "Selon le réglage de chaque véhicule", true},
			{choixNon, "Non", false},
			{suivi.BudgetCommun, "Compte commun", false},
			{suivi.BudgetPerso, "🔒 Mon budget perso", false},
		}
	}
	var out []OptionBudget
	actuel := x.Budget
	if actuel == "" {
		actuel = choixNon
	}
	// Dans le budget perso d'un autre parent : il reste le sien tant
	// qu'on ne choisit pas autre chose.
	if x.Budget == suivi.BudgetPerso && x.BudgetPersoDe != c.Membre.ID {
		nom := "d'un autre parent"
		if l, err := m.a.Membres(ctx, c); err == nil {
			for _, mb := range l {
				if mb.ID == x.BudgetPersoDe {
					nom = "de " + mb.Nom
				}
			}
		}
		out = append(out, OptionBudget{choixGarder, "🔒 Budget perso " + nom, true})
		actuel = choixGarder
	}
	for _, o := range []OptionBudget{{choixNon, "Non", false}, {suivi.BudgetCommun, "Compte commun", false}, {suivi.BudgetPerso, "🔒 Mon budget perso", false}} {
		o.Actif = o.Valeur == actuel
		out = append(out, o)
	}
	return out
}

// texteBudget : « Compte commun »… pour la fiche (vide : nulle part).
func (m *Module) texteBudget(ctx context.Context, c *appli.Contexte, x donnees.Vehicule) string {
	for _, o := range m.optionsBudget(ctx, c, &x) {
		if o.Actif && o.Valeur != choixNon {
			return o.Texte
		}
	}
	return ""
}

// reglerBudget : applique le choix du formulaire au véhicule x, puis range
// ses dépenses (déjà au carnet comprises) dans ce budget. Choix vide ou
// « garder » : le réglage du véhicule ne change pas (les lignes qui
// manquent au budget y sont ajoutées, s'il en a un).
func (m *Module) reglerBudget(ctx context.Context, c *appli.Contexte, x donnees.Vehicule, choix string) (suivi.Rangement, error) {
	if !peutBudget(c) {
		return suivi.Rangement{}, nil
	}
	compte, change := x.Budget, false
	switch choix {
	case choixNon, suivi.BudgetCommun, suivi.BudgetPerso:
		compte = suivi.BudgetValide(choix)
		change = compte != x.Budget || (compte == suivi.BudgetPerso && x.BudgetPersoDe != c.Membre.ID)
	}
	if change {
		var perso int64
		if compte == suivi.BudgetPerso {
			perso = c.Membre.ID
		}
		if err := m.a.Q.ChangerBudgetVehicule(ctx, donnees.ChangerBudgetVehiculeParams{Budget: compte, BudgetPersoDe: perso,
			ID: x.ID, FamilleID: c.Famille.ID}); err != nil {
			return suivi.Rangement{}, err
		}
	} else if compte == suivi.BudgetNon || (compte == suivi.BudgetPerso && x.BudgetPersoDe != c.Membre.ID) {
		// Rien de réglé (les lignes ajoutées une à une restent) ou budget
		// perso d'un autre parent (le sien).
		return suivi.Rangement{}, nil
	}
	return m.s.RangerBudget(ctx, c, modVehicules, x.ID, x.Nom, compte)
}
