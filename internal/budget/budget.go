// SPDX-License-Identifier: GPL-3.0-or-later

// Package budget : revenus et dépenses de la famille, par catégorie.
// En haut, le « reste à vivre » du mois (revenus − dépenses) ; une courbe
// des dépenses cumulées, comparée au mois précédent ; des filtres par
// catégorie. Réservé aux parents.
package budget

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
	"familycocon/internal/repetition"
)

// Module : le module Budget.
type Module struct{ a *appli.App }

const mod = "budget"

// Categorie : une catégorie de revenus ou de dépenses.
type Categorie struct {
	Cle   string
	Nom   string
	Emoji string
	Sens  string // depense | revenu
}

// Categories, dans l'ordre des formulaires.
var Categories = []Categorie{
	{"courses", "Courses", "🛒", "depense"},
	{"logement", "Logement", "🏠", "depense"},
	{"energie", "Énergie", "⚡", "depense"},
	{"transport", "Transport", "🚗", "depense"},
	{"sante", "Santé", "🩺", "depense"},
	{"ecole", "École", "🎒", "depense"},
	{"enfants", "Enfants", "🧸", "depense"},
	{"loisirs", "Loisirs", "🎡", "depense"},
	{"restaurants", "Restaurants", "🍕", "depense"},
	{"vetements", "Vêtements", "👕", "depense"},
	{"abonnements", "Abonnements", "📱", "depense"},
	{"cadeaux", "Cadeaux", "🎁", "depense"},
	{"animaux", "Animaux", "🐾", "depense"},
	{"autre", "Autre dépense", "💸", "depense"},
	{"salaire", "Salaire", "💼", "revenu"},
	{"aides", "Aides, allocations", "🏛️", "revenu"},
	{"autre-revenu", "Autre revenu", "💰", "revenu"},
}

// CategorieParCle : la catégorie (ok = faux si inconnue).
func CategorieParCle(cle string) (Categorie, bool) {
	for _, c := range Categories {
		if c.Cle == cle {
			return c, true
		}
	}
	return Categorie{}, false
}

// Brancher ajoute les routes du module.
func Brancher(a *appli.App) {
	m := &Module{a: a}
	a.Route("GET /budget", mod, appli.Parent, m.page)
	a.Route("GET /budget/nouvelle", mod, appli.Parent, m.formulaire)
	a.Route("POST /budget", mod, appli.Parent, m.creer)
	a.Route("GET /budget/recurrences", mod, appli.Parent, m.pageRecurrences)
	a.Route("GET /recurrences/{id}", mod, appli.Parent, m.formRecurrence)
	a.Route("POST /recurrences/{id}", mod, appli.Parent, m.modifierRecurrence)
	a.Route("POST /recurrences/{id}/arreter", mod, appli.Parent, m.arreterRecurrence)
	a.Route("POST /recurrences/{id}/supprimer", mod, appli.Parent, m.supprimerRecurrence)
	a.Route("GET /budget/{id}", mod, appli.Parent, m.formulaire)
	a.Route("POST /budget/{id}", mod, appli.Parent, m.modifier)
	a.Route("POST /budget/{id}/supprimer", mod, appli.Parent, m.supprimer)

	a.AjouterResume(mod, m.resume)
	a.AjouterAction(appli.Ajout{Module: mod, Emoji: "💶", Texte: "Une dépense", URL: "/budget/nouvelle"})
}

// mois : premier et dernier jour du mois de t.
func mois(t time.Time) (time.Time, time.Time) {
	premier := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	return premier, premier.AddDate(0, 1, -1)
}

type ligneOp struct {
	donnees.Operation
	Cat    Categorie
	Membre *donnees.Membre
}

type jourOps struct {
	Titre string
	Ops   []ligneOp
}

type parCategorie struct {
	Cat     Categorie
	Montant int64
}

type vueBudget struct {
	Mois      string // 2026-11
	Titre     string // Novembre 2026
	Precedent string
	Suivant   string
	Filtre    string
	Revenus   int64
	Depenses  int64
	Reste     int64
	Courbe    template.HTML
	ParCat    []parCategorie
	MaxCat    int64
	Jours     []jourOps
	Pilules   []Categorie
	Prevues   []ligneOp // opérations qui reviennent, pas encore passées ce mois-ci
	PrevuNet  int64     // revenus − dépenses prévus
	ResteFin  int64     // reste à vivre prévu en fin de mois
	NbRecur   int       // opérations qui reviennent (lien vers leur page)
	Devise    string
}

func lireMois(r *http.Request, c *appli.Contexte) time.Time {
	if t, err := time.ParseInLocation("2006-01", r.URL.Query().Get("mois"), c.Lieu); err == nil {
		return t
	}
	return c.Maintenant()
}

func (m *Module) operations(ctx context.Context, c *appli.Contexte, du, au time.Time) ([]donnees.Operation, error) {
	return m.a.Q.OperationsPeriode(ctx, donnees.OperationsPeriodeParams{FamilleID: c.Famille.ID, Du: repetition.CleJour(du), Au: repetition.CleJour(au)})
}

// cumuls : dépenses cumulées par jour du mois (jusqu'au jour « jusqua » inclus).
func cumuls(ops []donnees.Operation, premier time.Time, jours, jusqua int, filtre string) []int64 {
	parJour := make([]int64, jours)
	for _, o := range ops {
		if o.Sens != "depense" || (filtre != "" && o.Categorie != filtre) {
			continue
		}
		d, err := time.Parse("2006-01-02", o.Jour)
		if err != nil || d.Day() > jours {
			continue
		}
		parJour[d.Day()-1] += o.Montant
	}
	out := make([]int64, 0, jusqua)
	var total int64
	for i := 0; i < jusqua && i < jours; i++ {
		total += parJour[i]
		out = append(out, total)
	}
	return out
}

func (m *Module) vue(ctx context.Context, c *appli.Contexte, ref time.Time, filtre string) (*vueBudget, error) {
	premier, dernier := mois(ref)
	ppremier, pdernier := mois(premier.AddDate(0, -1, 0))
	nom := appli.MoisFR(premier.Month())
	v := &vueBudget{
		Mois: premier.Format("2006-01"), Titre: strings.ToUpper(nom[:1]) + nom[1:] + " " + strconv.Itoa(premier.Year()),
		Precedent: ppremier.Format("2006-01"), Suivant: premier.AddDate(0, 1, 0).Format("2006-01"),
		Devise: c.Famille.Devise,
	}
	if _, ok := CategorieParCle(filtre); ok {
		v.Filtre = filtre
	}
	// Les opérations qui reviennent sont d'abord écrites jusqu'à aujourd'hui.
	if err := Generer(ctx, m.a, c.Famille.ID, c.Maintenant()); err != nil {
		return nil, err
	}
	ops, err := m.operations(ctx, c, premier, dernier)
	if err != nil {
		return nil, err
	}
	prec, err := m.operations(ctx, c, ppremier, pdernier)
	if err != nil {
		return nil, err
	}
	membres, err := m.a.Membres(ctx, c)
	if err != nil {
		return nil, err
	}
	parID := map[int64]*donnees.Membre{}
	for i := range membres {
		parID[membres[i].ID] = &membres[i]
	}
	totaux := map[string]int64{}
	presentes := map[string]bool{}
	for _, o := range ops {
		if o.Sens == "revenu" {
			v.Revenus += o.Montant
		} else {
			v.Depenses += o.Montant
			totaux[o.Categorie] += o.Montant
		}
		presentes[o.Categorie] = true
		if v.Filtre != "" && o.Categorie != v.Filtre {
			continue
		}
		cat, ok := CategorieParCle(o.Categorie)
		if !ok {
			cat, _ = CategorieParCle("autre")
		}
		l := ligneOp{Operation: o, Cat: cat}
		if o.MembreID.Valid {
			l.Membre = parID[o.MembreID.Int64]
		}
		d, _ := time.ParseInLocation("2006-01-02", o.Jour, c.Lieu)
		titre := strings.ToUpper(appli.JourFR(d)[:1]) + appli.JourFR(d)[1:] + " " + strconv.Itoa(d.Day())
		if len(v.Jours) == 0 || v.Jours[len(v.Jours)-1].Titre != titre {
			v.Jours = append(v.Jours, jourOps{Titre: titre})
		}
		v.Jours[len(v.Jours)-1].Ops = append(v.Jours[len(v.Jours)-1].Ops, l)
	}
	v.Reste = v.Revenus - v.Depenses
	for _, cat := range Categories {
		if presentes[cat.Cle] {
			v.Pilules = append(v.Pilules, cat)
		}
		if n := totaux[cat.Cle]; n > 0 {
			v.ParCat = append(v.ParCat, parCategorie{Cat: cat, Montant: n})
			v.MaxCat = max(v.MaxCat, n)
		}
	}
	sort.SliceStable(v.ParCat, func(i, j int) bool { return v.ParCat[i].Montant > v.ParCat[j].Montant })

	// Courbe : jusqu'à aujourd'hui pour le mois en cours, le mois entier sinon.
	jours := dernier.Day()
	jusqua := jours
	maintenant := c.Maintenant()
	if premier.Year() == maintenant.Year() && premier.Month() == maintenant.Month() {
		jusqua = maintenant.Day()
	} else if premier.After(maintenant) {
		jusqua = 0
	}
	v.Courbe = Courbe{
		Actuel:    cumuls(ops, premier, jours, jusqua, v.Filtre),
		Precedent: cumuls(prec, ppremier, pdernier.Day(), pdernier.Day(), v.Filtre),
		Jours:     jours, Devise: c.Famille.Devise,
	}.SVG()

	// Ce qui reviendra d'ici la fin du mois (après aujourd'hui).
	recs, err := m.a.Q.RecurrencesFamille(ctx, c.Famille.ID)
	if err != nil {
		return nil, err
	}
	v.NbRecur = len(recs)
	for _, p := range Prevues(recs, premier, dernier, maintenant) {
		cat, ok := CategorieParCle(p.Categorie)
		if !ok {
			cat, _ = CategorieParCle("autre")
		}
		l := ligneOp{Operation: p, Cat: cat}
		if p.MembreID.Valid {
			l.Membre = parID[p.MembreID.Int64]
		}
		v.Prevues = append(v.Prevues, l)
		if p.Sens == "revenu" {
			v.PrevuNet += p.Montant
		} else {
			v.PrevuNet -= p.Montant
		}
	}
	v.ResteFin = v.Reste + v.PrevuNet
	return v, nil
}

func (m *Module) page(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	v, err := m.vue(r.Context(), c, lireMois(r, c), r.URL.Query().Get("cat"))
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Afficher(w, r, c, "budget/budget", "Budget", mod, v)
}

type formOp struct {
	Nouvelle   bool
	Op         donnees.Operation
	Montant    string
	Categories []Categorie
	Membres    []donnees.Membre
	Frequences []Frequence
	Recurrence string // « Chaque mois », si l'opération fait partie d'une récurrence
	Devise     string
}

func (m *Module) operation(r *http.Request, c *appli.Contexte) (donnees.Operation, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return donnees.Operation{}, sql.ErrNoRows
	}
	return m.a.Q.OperationParID(r.Context(), donnees.OperationParIDParams{ID: id, FamilleID: c.Famille.ID})
}

func (m *Module) formulaire(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	f := formOp{Nouvelle: r.PathValue("id") == "", Categories: Categories, Frequences: Frequences, Devise: c.Famille.Devise}
	var err error
	if f.Membres, err = m.a.Membres(r.Context(), c); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	if f.Nouvelle {
		f.Op = donnees.Operation{Sens: "depense", Categorie: "courses", Jour: repetition.CleJour(c.Maintenant()),
			MembreID: sql.NullInt64{Int64: c.Membre.ID, Valid: true}}
		if r.URL.Query().Get("sens") == "revenu" {
			f.Op.Sens, f.Op.Categorie = "revenu", "salaire"
		}
	} else {
		if f.Op, err = m.operation(r, c); err != nil {
			m.a.PageErreur(w, r, c, http.StatusNotFound, "Opération introuvable", "Cette opération n'existe plus.")
			return
		}
		f.Montant = montantSaisie(f.Op.Montant)
		if f.Op.RecurrenceID.Valid {
			if rec, err := m.a.Q.RecurrenceParID(r.Context(), donnees.RecurrenceParIDParams{ID: f.Op.RecurrenceID.Int64, FamilleID: c.Famille.ID}); err == nil {
				f.Recurrence = NomFrequence(rec.Frequence)
			}
		}
	}
	titre := "Nouvelle opération"
	if !f.Nouvelle {
		titre = "Modifier l'opération"
	}
	m.a.Afficher(w, r, c, "budget/operation", titre, mod, f)
}

type saisieOp struct {
	sens, categorie, libelle, jour string
	montant                        int64
	membre                         sql.NullInt64
	frequence, fin                 string // nouvelle opération qui revient
}

func (m *Module) lire(r *http.Request, c *appli.Contexte) (saisieOp, string) {
	var s saisieOp
	var err error
	if s.montant, err = LireMontant(r.PostFormValue("montant")); err != nil {
		return s, err.Error()
	}
	cat, ok := CategorieParCle(r.PostFormValue("categorie"))
	if !ok {
		return s, "Catégorie inconnue."
	}
	s.categorie, s.sens = cat.Cle, cat.Sens
	s.libelle = strings.TrimSpace(r.PostFormValue("libelle"))
	if utf8.RuneCountInString(s.libelle) > 80 {
		return s, "Libellé : 80 caractères au plus."
	}
	if _, err := time.Parse("2006-01-02", r.PostFormValue("jour")); err != nil {
		return s, "Date invalide."
	}
	s.jour = r.PostFormValue("jour")
	if id, err := strconv.ParseInt(r.PostFormValue("membre"), 10, 64); err == nil && id > 0 {
		if _, err := m.a.Q.MembreParID(r.Context(), donnees.MembreParIDParams{ID: id, FamilleID: c.Famille.ID}); err == nil {
			s.membre = sql.NullInt64{Int64: id, Valid: true}
		}
	}
	if f := r.PostFormValue("frequence"); FrequenceValide(f) {
		s.frequence = f
		s.fin = r.PostFormValue("fin")
		if _, err := time.Parse("2006-01-02", s.fin); s.fin != "" && (err != nil || s.fin < s.jour) {
			return s, "La date de fin doit suivre la première date."
		}
	}
	return s, ""
}

func (m *Module) creer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	s, msg := m.lire(r, c)
	if msg != "" {
		m.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	ctx := r.Context()
	auteur := sql.NullInt64{Int64: c.Membre.ID, Valid: true}
	if s.frequence != "" {
		if msg := ValiderProchaine(s.jour, nil, c.Maintenant()); msg != "" {
			m.a.Echec(w, r, http.StatusBadRequest, msg)
			return
		}
		if recs, err := m.a.Q.RecurrencesFamille(ctx, c.Famille.ID); err != nil || len(recs) >= RecurrencesMax {
			m.a.Echec(w, r, http.StatusBadRequest, "Trop d'opérations qui reviennent : arrêtez-en d'abord.")
			return
		}
		// Opération qui revient : la récurrence écrit elle-même les occurrences
		// passées ou du jour ; les suivantes viendront à leur date.
		if _, err := m.a.Q.CreerRecurrence(ctx, donnees.CreerRecurrenceParams{
			FamilleID: c.Famille.ID, Sens: s.sens, Montant: s.montant, Categorie: s.categorie, Libelle: s.libelle,
			MembreID: s.membre, Frequence: s.frequence, Debut: s.jour, Fin: s.fin, CreePar: auteur, CreeLe: time.Now().Unix(),
		}); err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
		if err := Generer(ctx, m.a, c.Famille.ID, c.Maintenant()); err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
	} else if _, err := m.a.Q.CreerOperation(ctx, donnees.CreerOperationParams{
		FamilleID: c.Famille.ID, Sens: s.sens, Montant: s.montant, Categorie: s.categorie, Libelle: s.libelle,
		Jour: s.jour, MembreID: s.membre, CreePar: auteur, CreeLe: time.Now().Unix(),
	}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Enregistré ✓")
	m.a.Rediriger(w, r, "/budget?mois="+s.jour[:7])
}

func (m *Module) modifier(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	o, err := m.operation(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Opération introuvable.")
		return
	}
	s, msg := m.lire(r, c)
	if msg != "" {
		m.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	if err := m.a.Q.ModifierOperation(r.Context(), donnees.ModifierOperationParams{
		Sens: s.sens, Montant: s.montant, Categorie: s.categorie, Libelle: s.libelle, Jour: s.jour,
		MembreID: s.membre, ID: o.ID, FamilleID: c.Famille.ID,
	}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Enregistré ✓")
	m.a.Rediriger(w, r, "/budget?mois="+s.jour[:7])
}

func (m *Module) supprimer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	o, err := m.operation(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Opération introuvable.")
		return
	}
	if err := m.a.Q.SupprimerOperation(r.Context(), donnees.SupprimerOperationParams{ID: o.ID, FamilleID: c.Famille.ID}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Opération supprimée")
	m.a.Rediriger(w, r, "/budget?mois="+o.Jour[:7])
}

func (m *Module) resume(ctx context.Context, c *appli.Contexte) appli.Resume {
	_ = Generer(ctx, m.a, c.Famille.ID, c.Maintenant())
	premier, dernier := mois(c.Maintenant())
	t, err := m.a.Q.TotauxPeriode(ctx, donnees.TotauxPeriodeParams{FamilleID: c.Famille.ID, Du: repetition.CleJour(premier), Au: repetition.CleJour(dernier)})
	if err != nil || t.Revenus+t.Depenses == 0 {
		return appli.Resume{Info: "Rien ce mois-ci"}
	}
	return appli.Resume{Info: "Reste " + appli.Montant(t.Revenus-t.Depenses, c.Famille.Devise)}
}

// montantSaisie : 1234 centimes → « 12,34 » (pour un champ de formulaire).
func montantSaisie(n int64) string {
	return fmt.Sprintf("%d,%02d", n/100, n%100)
}
