// SPDX-License-Identifier: GPL-3.0-or-later

package budget

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
)

// Deux budgets :
//   - le compte commun (perso_de = 0) : vu par tous les parents, comme
//     avant ;
//   - le budget perso de chaque parent (perso_de = son id) : privé, lui
//     seul le voit (pas même l'autre parent), comme une note privée.
//
// Chacun a ses opérations, ses opérations qui reviennent et ses plafonds.
// Un virement du budget perso vers le compte commun écrit deux opérations
// liées : une dépense côté perso, un revenu côté commun.

// Paramètre de l'adresse et cookie qui retient le dernier budget ouvert.
const (
	ComptePerso  = "perso"
	CompteCommun = "commun"
	cookieCompte = "fc_compte"
)

// Catégories des virements (jamais proposées dans les formulaires).
var CategoriesVirement = []Categorie{
	{"virement", "Virement vers le commun", "🔁", "depense"},
	{"virement-recu", "Virement reçu", "🔁", "revenu"},
}

// estVirement : opération ou récurrence de virement.
func estVirement(categorie string) bool {
	return categorie == "virement" || categorie == "virement-recu"
}

// compteDe : le budget demandé : 0 (commun) ou l'id du membre (son budget
// perso). Choisi par « ?compte=perso|commun » (ou le champ du formulaire),
// retenu par un cookie pour les pages suivantes.
func (m *Module) compteDe(w http.ResponseWriter, r *http.Request, c *appli.Contexte) int64 {
	v := r.URL.Query().Get("compte")
	if v == "" && r.Method == http.MethodPost {
		v = r.PostFormValue("compte")
	}
	if v == ComptePerso || v == CompteCommun {
		http.SetCookie(w, &http.Cookie{Name: cookieCompte, Value: v, Path: "/", MaxAge: 400 * 24 * 3600,
			HttpOnly: true, Secure: m.a.Config.CookieSecurise(), SameSite: http.SameSiteLaxMode})
	} else if ck, err := r.Cookie(cookieCompte); err == nil {
		v = ck.Value
	}
	if v == ComptePerso {
		return c.Membre.ID
	}
	return 0
}

// nomCompte : « perso » ou « commun » (pour les adresses et les formulaires).
func nomCompte(compte int64) string {
	if compte > 0 {
		return ComptePerso
	}
	return CompteCommun
}

// visible : ce budget est-il celui de ce membre (commun, ou son perso) ?
func visible(c *appli.Contexte, persoDe int64) bool {
	return persoDe == 0 || persoDe == c.Membre.ID
}

// supprimerAvecMoitie : l'opération et, pour un virement, son autre moitié.
func supprimerAvecMoitie(ctx context.Context, q *donnees.Queries, famille int64, o donnees.Operation) error {
	if o.VirementID.Valid {
		if err := q.SupprimerOperation(ctx, donnees.SupprimerOperationParams{ID: o.VirementID.Int64, FamilleID: famille}); err != nil {
			return err
		}
	}
	return q.SupprimerOperation(ctx, donnees.SupprimerOperationParams{ID: o.ID, FamilleID: famille})
}

// libelleVirement : « Virement de Marie » (côté commun).
func libelleVirement(nom string) string { return "Virement de " + nom }

// ecrireVirement : les deux moitiés d'un virement, liées l'une à l'autre.
func ecrireVirement(ctx context.Context, q *donnees.Queries, famille, membre int64, nom string, montant int64, jour, libelle string,
	recurrence sql.NullInt64, auteur sql.NullInt64) error {
	qui := sql.NullInt64{Int64: membre, Valid: true}
	if libelle == "" {
		libelle = "Virement vers le commun"
	}
	cree := func(perso int64, sens, cat, lib string) (donnees.Operation, error) {
		if recurrence.Valid {
			return q.CreerOccurrence(ctx, donnees.CreerOccurrenceParams{FamilleID: famille, PersoDe: perso, Sens: sens, Montant: montant,
				Categorie: cat, Libelle: lib, Jour: jour, MembreID: qui, RecurrenceID: recurrence, CreePar: auteur, CreeLe: time.Now().Unix()})
		}
		return q.CreerOperation(ctx, donnees.CreerOperationParams{FamilleID: famille, PersoDe: perso, Sens: sens, Montant: montant,
			Categorie: cat, Libelle: lib, Jour: jour, MembreID: qui, CreePar: auteur, CreeLe: time.Now().Unix()})
	}
	perso, err := cree(membre, "depense", "virement", libelle)
	if err != nil {
		return err
	}
	commun, err := cree(0, "revenu", "virement-recu", libelleVirement(nom))
	if err != nil {
		return err
	}
	if err := q.LierVirement(ctx, donnees.LierVirementParams{VirementID: sql.NullInt64{Int64: commun.ID, Valid: true}, ID: perso.ID, FamilleID: famille}); err != nil {
		return err
	}
	return q.LierVirement(ctx, donnees.LierVirementParams{VirementID: sql.NullInt64{Int64: perso.ID, Valid: true}, ID: commun.ID, FamilleID: famille})
}

// virer : POST /budget/virement : du budget perso vers le compte commun,
// une fois ou chaque mois (une opération qui revient, qui écrit les deux
// moitiés à chaque fois).
func (m *Module) virer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	montant, err := LireMontant(r.PostFormValue("montant"))
	if err != nil {
		m.a.Echec(w, r, http.StatusBadRequest, err.Error())
		return
	}
	jour, ok := LireJour(r.PostFormValue("jour"), c.Maintenant())
	libelle := strings.TrimSpace(r.PostFormValue("libelle"))
	if !ok || utf8.RuneCountInString(libelle) > 80 {
		m.a.Echec(w, r, http.StatusBadRequest, "Date ou libellé invalide.")
		return
	}
	if libelle == "" {
		libelle = "Virement vers le commun"
	}
	auteur := sql.NullInt64{Int64: c.Membre.ID, Valid: true}
	if f := r.PostFormValue("frequence"); FrequenceValide(f) {
		if msg := ValiderProchaine(jour, nil, c.Maintenant()); msg != "" {
			m.a.Echec(w, r, http.StatusBadRequest, msg)
			return
		}
		if msg, err := PlaceRecurrence(ctx, m.a, c.Famille.ID); err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		} else if msg != "" {
			m.a.Echec(w, r, http.StatusBadRequest, msg)
			return
		}
		if _, err := m.a.Q.CreerRecurrence(ctx, donnees.CreerRecurrenceParams{FamilleID: c.Famille.ID, PersoDe: c.Membre.ID, Virement: 1,
			Sens: "depense", Montant: montant, Categorie: "virement", Libelle: libelle, MembreID: auteur, Frequence: f, Debut: jour,
			CreePar: auteur, CreeLe: time.Now().Unix()}); err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
		if err := Generer(ctx, m.a, c.Famille.ID, c.Maintenant()); err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
	} else if err := m.a.Transaction(ctx, func(q *donnees.Queries) error {
		return ecrireVirement(ctx, q, c.Famille.ID, c.Membre.ID, c.Membre.Nom, montant, jour, libelle, sql.NullInt64{}, auteur)
	}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Virement enregistré ✓ (aussi au compte commun)")
	m.a.Rediriger(w, r, "/budget?compte=perso&mois="+jour[:7])
}
