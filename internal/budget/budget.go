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
	a.Route("POST /budget/fixes", mod, appli.Parent, m.recopierFixes)
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
	Fixes     int // opérations fixes du mois précédent absentes de ce mois
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

	// Opérations fixes du mois précédent pas encore recopiées.
	deja := map[string]bool{}
	for _, o := range ops {
		if o.Fixe == 1 {
			deja[o.Categorie+"|"+o.Libelle] = true
		}
	}
	for _, o := range prec {
		if o.Fixe == 1 && !deja[o.Categorie+"|"+o.Libelle] {
			v.Fixes++
		}
	}
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
	f := formOp{Nouvelle: r.PathValue("id") == "", Categories: Categories, Devise: c.Famille.Devise}
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
		f.Montant = strings.Replace(fmt.Sprintf("%d.%02d", f.Op.Montant/100, f.Op.Montant%100), ".", ",", 1)
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
	fixe                           int64
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
	if r.PostFormValue("fixe") == "1" {
		s.fixe = 1
	}
	return s, ""
}

func (m *Module) creer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	s, msg := m.lire(r, c)
	if msg != "" {
		m.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	if _, err := m.a.Q.CreerOperation(r.Context(), donnees.CreerOperationParams{
		FamilleID: c.Famille.ID, Sens: s.sens, Montant: s.montant, Categorie: s.categorie, Libelle: s.libelle,
		Jour: s.jour, MembreID: s.membre, Fixe: s.fixe, CreePar: sql.NullInt64{Int64: c.Membre.ID, Valid: true}, CreeLe: time.Now().Unix(),
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
		MembreID: s.membre, Fixe: s.fixe, ID: o.ID, FamilleID: c.Famille.ID,
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

// recopierFixes : les opérations « fixes » du mois précédent sont recopiées
// dans le mois affiché (même jour du mois, ou le dernier jour s'il n'existe pas).
func (m *Module) recopierFixes(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	ref := c.Maintenant()
	if t, err := time.ParseInLocation("2006-01", r.PostFormValue("mois"), c.Lieu); err == nil {
		ref = t
	}
	premier, dernier := mois(ref)
	ppremier, pdernier := mois(premier.AddDate(0, -1, 0))
	ops, err1 := m.operations(ctx, c, premier, dernier)
	prec, err2 := m.operations(ctx, c, ppremier, pdernier)
	if err1 != nil || err2 != nil {
		m.a.ErreurInterne(w, r, fmt.Errorf("%v %v", err1, err2))
		return
	}
	deja := map[string]bool{}
	for _, o := range ops {
		if o.Fixe == 1 {
			deja[o.Categorie+"|"+o.Libelle] = true
		}
	}
	n := 0
	err := m.a.Transaction(ctx, func(q *donnees.Queries) error {
		for _, o := range prec {
			if o.Fixe != 1 || deja[o.Categorie+"|"+o.Libelle] {
				continue
			}
			jour, _ := strconv.Atoi(o.Jour[8:])
			jour = min(jour, dernier.Day())
			if _, err := q.CreerOperation(ctx, donnees.CreerOperationParams{
				FamilleID: c.Famille.ID, Sens: o.Sens, Montant: o.Montant, Categorie: o.Categorie, Libelle: o.Libelle,
				Jour: fmt.Sprintf("%s-%02d", premier.Format("2006-01"), jour), MembreID: o.MembreID, Fixe: 1,
				CreePar: sql.NullInt64{Int64: c.Membre.ID, Valid: true}, CreeLe: time.Now().Unix(),
			}); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, appli.Pluriel(n, "opération fixe recopiée", "opérations fixes recopiées"))
	m.a.Rediriger(w, r, "/budget?mois="+premier.Format("2006-01"))
}

func (m *Module) resume(ctx context.Context, c *appli.Contexte) appli.Resume {
	premier, dernier := mois(c.Maintenant())
	t, err := m.a.Q.TotauxPeriode(ctx, donnees.TotauxPeriodeParams{FamilleID: c.Famille.ID, Du: repetition.CleJour(premier), Au: repetition.CleJour(dernier)})
	if err != nil || t.Revenus+t.Depenses == 0 {
		return appli.Resume{Info: "Rien ce mois-ci"}
	}
	return appli.Resume{Info: "Reste " + appli.Montant(t.Revenus-t.Depenses, c.Famille.Devise)}
}
