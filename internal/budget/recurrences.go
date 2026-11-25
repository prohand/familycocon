// SPDX-License-Identifier: GPL-3.0-or-later

package budget

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
	"familycocon/internal/repetition"
)

// Opérations qui reviennent (salaire, loyer, abonnements…) : chaque
// récurrence écrit elle-même ses occurrences dans le budget, à leur date.
// L'occurrence n°k tombe à debut + k périodes ; « faites » compte celles
// déjà écrites. Rien ne tourne en tâche de fond : les occurrences dues sont
// écrites quand on ouvre le budget, l'accueil ou les abonnements.

// Frequence : tous les combien.
type Frequence struct {
	Cle, Nom string
	Mois     int // 0 : chaque semaine
	ParAn    int // nombre de fois par an
}

// Dix mois : chaque mois de janvier à octobre, rien en novembre et
// décembre (taxe foncière, taxe d'habitation mensualisées).
const dixMois = "mois10"

// Frequences proposées.
var Frequences = []Frequence{
	{"semaine", "Chaque semaine", 0, 52}, {"mois", "Chaque mois", 1, 12},
	{dixMois, "Chaque mois, de janvier à octobre (10 fois par an)", 1, 10},
	{"2mois", "Tous les 2 mois", 2, 6},
	{"trimestre", "Tous les 3 mois (trimestre)", 3, 4}, {"semestre", "Tous les 6 mois (semestre)", 6, 2},
	{"annee", "Chaque année", 12, 1},
}

func frequence(cle string) (Frequence, bool) {
	for _, f := range Frequences {
		if f.Cle == cle {
			return f, true
		}
	}
	return Frequence{}, false
}

// FrequenceValide : vrai si la clé est connue.
func FrequenceValide(cle string) bool { _, ok := frequence(cle); return ok }

// NomFrequence : « Chaque mois »…
func NomFrequence(cle string) string { f, _ := frequence(cle); return f.Nom }

// Occurrence : la date de l'occurrence n°k (k = 0 : debut). Pour les mois,
// même jour du mois, ou le dernier jour du mois s'il n'existe pas (le 31).
func Occurrence(debut, freq string, k int64) (time.Time, error) {
	d, err := time.Parse("2006-01-02", debut)
	if err != nil {
		return d, err
	}
	f, ok := frequence(freq)
	if !ok {
		return d, errors.New("fréquence inconnue")
	}
	if f.Mois == 0 {
		return d.AddDate(0, 0, int(7*k)), nil
	}
	premier := time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, int(k)*f.Mois, 0)
	if f.Cle == dixMois {
		// Les mois comptés de 0 à 9 dans chaque année (janvier à octobre) ;
		// un début en novembre ou décembre part du janvier suivant.
		an, mois := d.Year(), int(d.Month())
		if mois > 10 {
			an, mois = an+1, 1
		}
		n := int64(an)*10 + int64(mois-1) + k
		premier = time.Date(int(n/10), time.Month(n%10+1), 1, 0, 0, 0, 0, time.UTC)
	}
	dernierJour := premier.AddDate(0, 1, -1).Day()
	return premier.AddDate(0, 0, min(d.Day(), dernierJour)-1), nil
}

// ParAn : le montant sur une année (exact, sans arrondi).
func ParAn(montant int64, freq string) int64 {
	f, _ := frequence(freq)
	return montant * int64(f.ParAn)
}

// RecurrencesMax : par famille (garde-fou).
const RecurrencesMax = 300

// ValiderProchaine : la date de la prochaine fois est-elle acceptable ?
// Pas plus d'un an dans le passé (sinon des centaines d'opérations seraient
// écrites d'un coup), pas plus de 50 ans plus tard, et après la dernière
// occurrence déjà écrite (sinon elle serait écrite deux fois). rec : la
// récurrence modifiée (nil pour une nouvelle). « » si tout va bien.
func ValiderProchaine(prochaine string, rec *donnees.Recurrence, aujourdhui time.Time) string {
	j, err := time.Parse("2006-01-02", prochaine)
	switch {
	case err != nil:
		return "Date invalide."
	case j.Before(aujourdhui.AddDate(-1, 0, -1)):
		return "La première date ne peut pas être plus d'un an en arrière."
	case j.After(aujourdhui.AddDate(50, 0, 0)):
		return "Date trop lointaine."
	}
	if rec != nil && rec.Faites > 0 {
		if derniere, err := Occurrence(rec.Debut, rec.Frequence, rec.Faites-1); err == nil && prochaine <= repetition.CleJour(derniere) {
			return "La prochaine date doit suivre la dernière déjà passée (" + appli.DateLongue(derniere) + ")."
		}
	}
	return ""
}

// ParMois : le montant ramené à un mois (pour les totaux).
func ParMois(montant int64, freq string) int64 {
	f, _ := frequence(freq)
	return montant * int64(f.ParAn) / 12
}

// Prochaine : la prochaine occurrence pas encore écrite (ok = faux si la
// récurrence est finie).
func Prochaine(r donnees.Recurrence) (time.Time, bool) {
	j, err := Occurrence(r.Debut, r.Frequence, r.Faites)
	if err != nil || (r.Fin != "" && repetition.CleJour(j) > r.Fin) {
		return j, false
	}
	return j, true
}

// errDejaFaite : une autre requête a écrit l'occurrence en même temps.
var errDejaFaite = errors.New("occurrence déjà écrite")

// dejaGenere : le dernier jour où tout a été écrit, par serveur et par
// famille. L'accueil, le budget et les abonnements demandent tous « écris
// ce qui est dû » : une fois par jour suffit, puisque toute création ou
// modification d'une récurrence appelle Generer elle-même.
var dejaGenere sync.Map // cleGenere → jour (« 2026-11-23 »)

type cleGenere struct {
	a       *appli.App
	famille int64
}

// GenererSiBesoin : comme Generer, mais rien à faire si c'est déjà fait
// aujourd'hui pour cette famille (pages d'affichage).
func GenererSiBesoin(ctx context.Context, a *appli.App, famille int64, aujourdhui time.Time) error {
	if j, ok := dejaGenere.Load(cleGenere{a, famille}); ok && j == repetition.CleJour(aujourdhui) {
		return nil
	}
	return Generer(ctx, a, famille, aujourdhui)
}

// PlaceRecurrence : « » s'il reste de la place pour une récurrence de plus,
// sinon le message à afficher. L'erreur SQL est rendue à part (500).
func PlaceRecurrence(ctx context.Context, a *appli.App, famille int64) (string, error) {
	n, err := a.Q.CompterRecurrences(ctx, famille)
	if err != nil {
		return "", err
	}
	if n >= RecurrencesMax {
		return "Trop d'opérations qui reviennent : arrêtez-en d'abord.", nil
	}
	return "", nil
}

// Replanifier : la date de départ et le compteur à enregistrer quand on
// modifie une récurrence. Même prochaine date et même fréquence : rien ne
// bouge. Sinon on repart de la date donnée (compteur à zéro), après contrôle.
// msg : « » si tout va bien.
func Replanifier(rec donnees.Recurrence, prochaine, freq string, aujourdhui time.Time) (debut string, faites int64, msg string) {
	if j, ok := Prochaine(rec); ok && repetition.CleJour(j) == prochaine && freq == rec.Frequence {
		return rec.Debut, rec.Faites, ""
	}
	if msg := ValiderProchaine(prochaine, &rec, aujourdhui); msg != "" {
		return "", 0, msg
	}
	return prochaine, 0, ""
}

// MessageModifiee : la récurrence a changé entre la lecture et l'écriture.
const MessageModifiee = "Cette opération vient de changer : rechargez la page et recommencez."

// Generer écrit les occurrences dues jusqu'à aujourd'hui (inclus).
func Generer(ctx context.Context, a *appli.App, famille int64, aujourdhui time.Time) error {
	recs, err := a.Q.RecurrencesFamille(ctx, famille)
	if err != nil {
		return err
	}
	limite := repetition.CleJour(aujourdhui)
	for _, r := range recs {
		// 600 au plus d'un coup : plus de 10 ans d'une récurrence hebdomadaire.
		for n := 0; n < 600; n++ {
			j, ok := Prochaine(r)
			if !ok || repetition.CleJour(j) > limite {
				break
			}
			err := a.Transaction(ctx, func(q *donnees.Queries) error {
				// Avancer d'abord : si une autre requête l'a déjà fait, rien n'est écrit deux fois.
				if n, err := q.AvancerRecurrence(ctx, donnees.AvancerRecurrenceParams{ID: r.ID, FamilleID: famille, Faites: r.Faites}); err != nil || n == 0 {
					return errors.Join(err, errDejaFaite)
				}
				// Virement qui revient : les deux moitiés (perso et commun).
				if r.Virement == 1 {
					qui, err := q.MembreParID(ctx, donnees.MembreParIDParams{ID: r.PersoDe, FamilleID: famille})
					if err != nil {
						return err
					}
					return ecrireVirement(ctx, q, famille, r.PersoDe, qui.Nom, r.Montant, repetition.CleJour(j), r.Libelle,
						sql.NullInt64{Int64: r.ID, Valid: true}, r.CreePar)
				}
				_, err := q.CreerOccurrence(ctx, donnees.CreerOccurrenceParams{
					FamilleID: r.FamilleID, PersoDe: r.PersoDe, Sens: r.Sens, Montant: r.Montant, Categorie: r.Categorie, Libelle: r.Libelle,
					Jour: repetition.CleJour(j), MembreID: r.MembreID, RecurrenceID: sql.NullInt64{Int64: r.ID, Valid: true},
					CreePar: r.CreePar, CreeLe: time.Now().Unix(),
				})
				return err
			})
			if errors.Is(err, errDejaFaite) {
				break
			}
			if err != nil {
				return err
			}
			r.Faites++
		}
	}
	dejaGenere.Store(cleGenere{a, famille}, limite)
	return nil
}

// Prevues : les occurrences à venir entre premier et dernier (inclus),
// après aujourd'hui, comme des opérations (non enregistrées).
func Prevues(recs []donnees.Recurrence, premier, dernier, aujourdhui time.Time) []donnees.Operation {
	du, au, auj := repetition.CleJour(premier), repetition.CleJour(dernier), repetition.CleJour(aujourdhui)
	var out []donnees.Operation
	for _, r := range recs {
		for n := 0; n < 600; n++ {
			j, ok := Prochaine(r)
			cle := repetition.CleJour(j)
			if !ok || cle > au {
				break
			}
			if cle >= du && cle > auj {
				out = append(out, donnees.Operation{FamilleID: r.FamilleID, Sens: r.Sens, Montant: r.Montant, Categorie: r.Categorie,
					Libelle: r.Libelle, Jour: cle, MembreID: r.MembreID, RecurrenceID: sql.NullInt64{Int64: r.ID, Valid: true}})
			}
			r.Faites++
		}
	}
	// Par date.
	for i := 1; i < len(out); i++ {
		for k := i; k > 0 && out[k].Jour < out[k-1].Jour; k-- {
			out[k], out[k-1] = out[k-1], out[k]
		}
	}
	return out
}

// ---------- Page « Opérations qui reviennent » ----------

// LigneRecurrence : une récurrence et sa prochaine date.
type LigneRecurrence struct {
	donnees.Recurrence
	Cat        Categorie
	Frequence  string
	Prochaine  string // « Vendredi 5 décembre », ou « Terminée »
	Abonnement bool
}

type vueRecurrences struct {
	Revenus, Depenses         []LigneRecurrence
	RevenusMois, DepensesMois int64 // ramenés à un mois
	NetMois                   int64
	Devise                    string
	Perso                     bool // budget perso
}

// abonnees : les récurrences de la famille qui sont des abonnements.
func (m *Module) abonnees(ctx context.Context, c *appli.Contexte) (map[int64]bool, error) {
	ids, err := m.a.Q.RecurrencesAbonnees(ctx, c.Famille.ID)
	out := make(map[int64]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, err
}

func ligneRecurrence(r donnees.Recurrence, abonnees map[int64]bool) LigneRecurrence {
	cat, ok := CategorieParCle(r.Categorie)
	if !ok {
		cat, _ = CategorieParCle("autre")
	}
	l := LigneRecurrence{Recurrence: r, Cat: cat, Frequence: NomFrequence(r.Frequence), Prochaine: "Terminée"}
	if j, ok := Prochaine(r); ok {
		l.Prochaine = appli.DateLongue(j)
	}
	l.Abonnement = abonnees[r.ID]
	return l
}

func (m *Module) pageRecurrences(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	if err := GenererSiBesoin(ctx, m.a, c.Famille.ID, c.Maintenant()); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	compte := m.compteDe(w, r, c)
	recs, err := m.recurrencesCompte(ctx, c, compte)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	abonnees, err := m.abonnees(ctx, c)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	v := vueRecurrences{Devise: c.Famille.Devise, Perso: compte > 0}
	for _, x := range recs {
		l := ligneRecurrence(x, abonnees)
		_, active := Prochaine(x)
		if x.Sens == "revenu" {
			v.Revenus = append(v.Revenus, l)
			if active {
				v.RevenusMois += ParMois(x.Montant, x.Frequence)
			}
		} else {
			v.Depenses = append(v.Depenses, l)
			if active {
				v.DepensesMois += ParMois(x.Montant, x.Frequence)
			}
		}
	}
	v.NetMois = v.RevenusMois - v.DepensesMois
	m.a.Afficher(w, r, c, "budget/recurrences", "Opérations qui reviennent", mod, v)
}

// recurrence : la récurrence {id}, si elle est dans un budget de ce membre.
func (m *Module) recurrence(r *http.Request, c *appli.Contexte) (donnees.Recurrence, bool) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	x, err := m.a.Q.RecurrenceParID(r.Context(), donnees.RecurrenceParIDParams{ID: id, FamilleID: c.Famille.ID})
	return x, err == nil && visible(c, x.PersoDe)
}

type formRecurrence struct {
	R          LigneRecurrence
	Montant    string
	ProchaineJ string // date du champ « Prochaine fois »
	Categories []Categorie
	Frequences []Frequence
	Membres    []donnees.Membre
	Devise     string
}

func (m *Module) formRecurrence(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	x, ok := m.recurrence(r, c)
	if !ok {
		m.a.PageErreur(w, r, c, http.StatusNotFound, "Introuvable", "Cette opération qui revient n'existe plus.")
		return
	}
	abonnees, err := m.abonnees(r.Context(), c)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	f := formRecurrence{R: ligneRecurrence(x, abonnees), Montant: MontantSaisie(x.Montant), Categories: Categories,
		Frequences: Frequences, Devise: c.Famille.Devise}
	if j, ok := Prochaine(x); ok {
		f.ProchaineJ = repetition.CleJour(j)
	}
	if f.Membres, err = m.a.Membres(r.Context(), c); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Afficher(w, r, c, "budget/recurrence", "Opération qui revient", mod, f)
}

// modifierRecurrence : change les prochaines occurrences (pas celles déjà écrites).
func (m *Module) modifierRecurrence(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	x, ok := m.recurrence(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Introuvable.")
		return
	}
	montant, err := LireMontant(r.PostFormValue("montant"))
	if err != nil {
		m.a.Echec(w, r, http.StatusBadRequest, err.Error())
		return
	}
	cat, ok := CategorieParCle(r.PostFormValue("categorie"))
	if x.Virement == 1 || estVirement(x.Categorie) { // un virement le reste
		cat, ok = CategorieParCle(x.Categorie)
	}
	libelle := strings.TrimSpace(r.PostFormValue("libelle"))
	freq := r.PostFormValue("frequence")
	prochaine, fin := r.PostFormValue("prochaine"), r.PostFormValue("fin")
	_, okP := LireJour(prochaine, c.Maintenant())
	_, okF := LireJour(fin, c.Maintenant())
	switch {
	case !ok:
		m.a.Echec(w, r, http.StatusBadRequest, "Catégorie inconnue.")
		return
	case utf8.RuneCountInString(libelle) > 80:
		m.a.Echec(w, r, http.StatusBadRequest, "Libellé : 80 caractères au plus.")
		return
	case !FrequenceValide(freq) || !okP:
		m.a.Echec(w, r, http.StatusBadRequest, "Indiquez la fréquence et la prochaine date.")
		return
	case fin != "" && (!okF || fin < prochaine):
		m.a.Echec(w, r, http.StatusBadRequest, "La date de fin doit suivre la prochaine date.")
		return
	}
	var membre sql.NullInt64
	if id, _ := strconv.ParseInt(r.PostFormValue("membre"), 10, 64); id > 0 {
		if _, err := m.a.Q.MembreParID(r.Context(), donnees.MembreParIDParams{ID: id, FamilleID: c.Famille.ID}); err == nil {
			membre = sql.NullInt64{Int64: id, Valid: true}
		}
	}
	// Un abonnement reste une dépense, quelle que soit la catégorie choisie
	// (sinon il serait un revenu dans le budget et un coût dans Abonnements).
	if _, err := m.a.Q.AbonnementDeRecurrence(r.Context(), donnees.AbonnementDeRecurrenceParams{RecurrenceID: x.ID, FamilleID: c.Famille.ID}); err == nil {
		if cat.Sens != "depense" {
			m.a.Echec(w, r, http.StatusBadRequest, "Un abonnement est une dépense : choisissez une catégorie de dépense.")
			return
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		m.a.ErreurInterne(w, r, err)
		return
	}
	// Nouvelle date ou nouvelle fréquence : on repart de la date donnée.
	debut, faites, msg := Replanifier(x, prochaine, freq, c.Maintenant())
	if msg != "" {
		m.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	n, err := m.a.Q.ModifierRecurrence(r.Context(), donnees.ModifierRecurrenceParams{
		Sens: cat.Sens, Montant: montant, Categorie: cat.Cle, Libelle: libelle, MembreID: membre, Frequence: freq,
		Debut: debut, Fin: fin, Faites: faites, ID: x.ID, FamilleID: c.Famille.ID, FaitesAvant: x.Faites,
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	if n == 0 { // une occurrence a été écrite entre la lecture et l'écriture
		m.a.Echec(w, r, http.StatusConflict, MessageModifiee)
		return
	}
	if err := Generer(r.Context(), m.a, c.Famille.ID, c.Maintenant()); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Enregistré ✓ (pour les prochaines fois)")
	m.a.Rediriger(w, r, "/budget/recurrences")
}

// arreterRecurrence : plus d'occurrence après aujourd'hui (l'historique reste).
func (m *Module) arreterRecurrence(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	x, ok := m.recurrence(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Introuvable.")
		return
	}
	if err := Generer(r.Context(), m.a, c.Famille.ID, c.Maintenant()); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	if err := m.a.Q.ArreterRecurrence(r.Context(), donnees.ArreterRecurrenceParams{Fin: repetition.CleJour(c.Maintenant()), ID: x.ID, FamilleID: c.Famille.ID}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Arrêtée : elle ne reviendra plus")
	m.a.Rediriger(w, r, "/budget/recurrences")
}

// supprimerRecurrence : la récurrence (et son abonnement) ; les opérations
// déjà passées restent dans le budget.
func (m *Module) supprimerRecurrence(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	x, ok := m.recurrence(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Introuvable.")
		return
	}
	ctx := r.Context()
	avecPassees := r.PostFormValue("passees") == "1"
	var n int64
	err := m.a.Transaction(ctx, func(q *donnees.Queries) error {
		// Sur demande : aussi les opérations déjà ajoutées automatiquement
		// (par exemple une ancienne opération « fixe » devenue récurrente).
		if avecPassees {
			var err error
			if n, err = q.SupprimerOccurrences(ctx, donnees.SupprimerOccurrencesParams{RecurrenceID: sql.NullInt64{Int64: x.ID, Valid: true}, FamilleID: c.Famille.ID}); err != nil {
				return err
			}
		}
		return q.SupprimerRecurrence(ctx, donnees.SupprimerRecurrenceParams{ID: x.ID, FamilleID: c.Famille.ID})
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	if avecPassees {
		m.a.Flash(w, "Supprimée, avec "+appli.Pluriel(n, "opération ajoutée", "opérations ajoutées"))
	} else {
		m.a.Flash(w, "Supprimée (les opérations passées restent)")
	}
	m.a.Rediriger(w, r, "/budget/recurrences")
}
