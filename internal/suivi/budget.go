// SPDX-License-Identifier: GPL-3.0-or-later

package suivi

import (
	"context"
	"database/sql"
	"time"
	"unicode/utf8"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
)

// Où vont les dépenses du carnet d'un objet dans le budget (réglage d'un
// véhicule) : nulle part, au compte commun, ou au budget perso d'un parent.
const (
	BudgetNon    = ""
	BudgetCommun = "commun"
	BudgetPerso  = "perso"
)

// BudgetValide : le réglage lu dans un formulaire (inconnu : nulle part).
func BudgetValide(v string) string {
	if v == BudgetCommun || v == BudgetPerso {
		return v
	}
	return BudgetNon
}

// Rangement : ce que RangerBudget a fait.
type Rangement struct {
	Ajoutees, Deplacees, Retirees int
}

// Message : « 12 dépenses ajoutées au budget »… (vide : rien n'a changé).
func (r Rangement) Message() string {
	var t string
	ajout := func(s string) {
		if t != "" {
			t += " · "
		}
		t += s
	}
	if r.Ajoutees > 0 {
		ajout(appli.Pluriel(r.Ajoutees, "dépense ajoutée au budget", "dépenses ajoutées au budget"))
	}
	if r.Deplacees > 0 {
		ajout(appli.Pluriel(r.Deplacees, "dépense changée de budget", "dépenses changées de budget"))
	}
	if r.Retirees > 0 {
		ajout(appli.Pluriel(r.Retirees, "dépense retirée du budget", "dépenses retirées du budget"))
	}
	return t
}

// RangerBudget : toutes les dépenses du carnet de l'objet (y compris
// celles déjà notées ou reprises de Fuelio) vont dans ce budget :
//   - BudgetCommun ou BudgetPerso (celui de c.Membre) : les lignes avec un
//     coût pas encore au budget y sont ajoutées, celles déjà au budget
//     changent de budget si besoin ;
//   - BudgetNon : les dépenses ajoutées au budget avec le carnet en sont
//     retirées (les lignes du carnet restent, avec leur coût).
//
// seules (non nil) : n'ajouter que ces lignes-là (reprise Fuelio : les
// lignes qu'elle vient d'écrire), sans rien déplacer ni retirer. Une
// dépense retirée à la main du budget n'y revient donc pas.
//
// Le budget perso d'un autre parent n'est jamais touché, et vers « mon
// budget perso » ne passent que les dépenses ajoutées par ce parent (celles
// d'un autre restent au compte commun, où il les voit). Rien n'est fait
// pour un enfant, si le budget est fermé ou si l'objet n'a pas de catégorie.
// nom : le nom de l'objet (libellé des dépenses : « Plein · Clio »).
func (s *Suivi) RangerBudget(ctx context.Context, c *appli.Contexte, module string, objet int64, nom, compte string, seules []int64) (Rangement, error) {
	var r Rangement
	o, ok := s.objets[module]
	if !ok || o.Categorie == "" || !c.EstParent() || !c.Actif("budget") {
		return r, nil
	}
	compte = BudgetValide(compte)
	if seules != nil && compte == BudgetNon {
		return r, nil
	}
	var cible int64 // budget visé : 0 (commun) ou ce parent (perso)
	if compte == BudgetPerso {
		cible = c.Membre.ID
	}
	garder := map[int64]bool{}
	for _, id := range seules {
		garder[id] = true
	}
	err := s.a.Transaction(ctx, func(q *donnees.Queries) error {
		lies, err := q.CarnetAuBudget(ctx, donnees.CarnetAuBudgetParams{FamilleID: c.Famille.ID, Module: module, ObjetID: objet})
		if err != nil {
			return err
		}
		if seules != nil {
			lies = nil // reprise : rien ne bouge de ce qui est déjà au budget
		}
		for _, l := range lies {
			if l.PersoDe != 0 && l.PersoDe != c.Membre.ID {
				continue // budget perso d'un autre parent : le sien
			}
			if cible != 0 && l.PersoDe == 0 && l.CreePar.Int64 != c.Membre.ID {
				continue // ajoutée au commun par un autre parent : elle y reste
			}
			switch {
			case compte == BudgetNon:
				if err := q.SupprimerOperation(ctx, donnees.SupprimerOperationParams{ID: l.OperationID, FamilleID: c.Famille.ID}); err != nil {
					return err
				}
				r.Retirees++
			case l.PersoDe != cible:
				if err := q.ChangerCompteOperation(ctx, donnees.ChangerCompteOperationParams{PersoDe: cible, ID: l.OperationID, FamilleID: c.Famille.ID}); err != nil {
					return err
				}
				r.Deplacees++
			}
		}
		if compte == BudgetNon {
			return nil
		}
		lignes, err := q.CarnetSansBudget(ctx, donnees.CarnetSansBudgetParams{FamilleID: c.Famille.ID, Module: module, ObjetID: objet})
		if err != nil {
			return err
		}
		maintenant := time.Now().Unix()
		for _, l := range lignes {
			if seules != nil && !garder[l.ID] {
				continue
			}
			op, err := q.CreerOperation(ctx, donnees.CreerOperationParams{FamilleID: c.Famille.ID, PersoDe: cible, Sens: "depense", Montant: l.Montant,
				Categorie: o.Categorie, Libelle: libelleBudget(l.Libelle, nom), Jour: l.Jour, MembreID: memberID(c), CreePar: memberID(c), CreeLe: maintenant})
			if err != nil {
				return err
			}
			if err := q.LierCarnetOperation(ctx, donnees.LierCarnetOperationParams{OperationID: sql.NullInt64{Int64: op.ID, Valid: true}, ID: l.ID, FamilleID: c.Famille.ID}); err != nil {
				return err
			}
			r.Ajoutees++
		}
		return nil
	})
	if err == nil && r != (Rangement{}) {
		s.a.Publier(c, "budget", "")
	}
	return r, err
}

// libelleBudget : « Plein · Clio » (80 caractères au plus).
func libelleBudget(libelle, nom string) string {
	if nom != "" {
		libelle += " · " + nom
	}
	if utf8.RuneCountInString(libelle) > 80 {
		libelle = string([]rune(libelle)[:80])
	}
	return libelle
}

// compteAuto : le budget où va d'elle-même une dépense de cet objet,
// d'après son réglage (véhicule) : perso = 0 (commun) ou ce parent.
// ok faux : nulle part (pas de réglage, budget perso d'un autre parent,
// enfant, budget fermé).
func (s *Suivi) compteAuto(ctx context.Context, c *appli.Contexte, o Objet, objet int64) (perso int64, ok bool) {
	if o.Budget == nil || o.Categorie == "" || !c.EstParent() || !c.Actif("budget") {
		return 0, false
	}
	switch compte, de := o.Budget(ctx, c.Famille.ID, objet); {
	case compte == BudgetCommun:
		return 0, true
	case compte == BudgetPerso && de == c.Membre.ID:
		return c.Membre.ID, true
	}
	return 0, false
}
