// SPDX-License-Identifier: GPL-3.0-or-later

// Package budget : revenus et dépenses de la famille, par catégorie : le
// compte commun, et le budget perso (privé) de chaque parent (comptes.go).
// En haut, le « reste à vivre » du mois (revenus − dépenses) ; une courbe
// des dépenses cumulées, comparée au mois précédent ; des filtres par
// catégorie. Réservé aux parents.
package budget

import (
	"context"
	"database/sql"
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
	{"maison", "Maison, travaux", "🔨", "depense"},
	{"energie", "Énergie", "⚡", "depense"},
	{"transport", "Transport", "🚗", "depense"},
	{"sante", "Santé", "🩺", "depense"},
	{"beaute", "Beauté, soins", "💇", "depense"},
	{"ecole", "École", "🎒", "depense"},
	{"enfants", "Enfants", "🧸", "depense"},
	{"garde", "Garde d'enfants", "👶", "depense"},
	{"loisirs", "Loisirs", "🎡", "depense"},
	{"sport", "Sport", "⚽", "depense"},
	{"vacances", "Vacances, voyages", "✈️", "depense"},
	{"restaurants", "Restaurants", "🍕", "depense"},
	{"vetements", "Vêtements", "👕", "depense"},
	{"abonnements", "Abonnements", "📱", "depense"},
	{"cadeaux", "Cadeaux", "🎁", "depense"},
	{"dons", "Dons", "❤️", "depense"},
	{"animaux", "Animaux", "🐾", "depense"},
	{"impots", "Impôts", "🧾", "depense"},
	{"assurances", "Assurances", "🛡️", "depense"},
	{"credit", "Crédit", "🏦", "depense"},
	{"frais-bancaires", "Frais bancaires", "💳", "depense"},
	{"epargne", "Épargne", "🐷", "depense"},
	{"autre", "Autre dépense", "💸", "depense"},
	{"salaire", "Salaire", "💼", "revenu"},
	{"aides", "Aides, allocations", "🏛️", "revenu"},
	{"remboursements", "Remboursements", "🔄", "revenu"},
	{"ventes", "Ventes", "🏷️", "revenu"},
	{"loyers", "Loyers perçus", "🔑", "revenu"},
	{"retraite", "Retraite, pension", "👴", "revenu"},
	{"autre-revenu", "Autre revenu", "💰", "revenu"},
}

// CategorieParCle : la catégorie (ok = faux si inconnue).
func CategorieParCle(cle string) (Categorie, bool) {
	for _, c := range append(Categories, CategoriesVirement...) {
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
	a.Route("POST /budget/virement", mod, appli.Parent, m.virer)
	a.Route("GET /budget/recurrences", mod, appli.Parent, m.pageRecurrences)
	a.Route("GET /budget/chercher", mod, appli.Parent, m.chercher)
	a.Route("GET /budget/annee", mod, appli.Parent, m.annee)
	a.Route("GET /budget/plafonds", mod, appli.Parent, m.pagePlafonds)
	a.Route("POST /budget/plafonds", mod, appli.Parent, m.enregistrerPlafonds)
	a.Route("GET /recurrences/{id}", mod, appli.Parent, m.formRecurrence)
	a.Route("POST /recurrences/{id}", mod, appli.Parent, m.modifierRecurrence)
	a.Route("POST /recurrences/{id}/arreter", mod, appli.Parent, m.arreterRecurrence)
	a.Route("POST /recurrences/{id}/supprimer", mod, appli.Parent, m.supprimerRecurrence)
	a.Route("GET /budget/{id}", mod, appli.Parent, m.formulaire)
	a.Route("POST /budget/{id}", mod, appli.Parent, m.modifier)
	a.Route("POST /budget/{id}/supprimer", mod, appli.Parent, m.supprimer)

	a.AjouterResume(mod, m.resume)
	a.AjouterRecherche(mod, m.chercherGlobal)
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
	Plafond int64 // 0 : pas de plafond
	Max     int64 // échelle de la jauge : le plafond, ou la plus grosse catégorie
	Alerte  bool  // 90 % du plafond atteints
	Pour100 int64 // part du plafond dépensée
}

// payePar : dépenses du mois payées par un parent (« qui a payé quoi »).
type payePar struct {
	Membre  *donnees.Membre
	Montant int64
}

type vueBudget struct {
	Compte     string // « commun » ou « perso »
	Perso      bool   // budget perso (privé) du membre
	Aujourdhui string
	Frequences []Frequence
	Mois       string // 2026-11
	Titre      string // Novembre 2026
	Precedent  string
	Suivant    string
	Filtre     string
	// Revenus, Depenses, Reste : le mois entier, ce qui est prévu compris
	// (opérations qui reviennent pas encore passées) ; Aujourdhui... :
	// ce qui est déjà passé.
	Revenus  int64
	Depenses int64
	Reste    int64
	ResteAuj int64 // reste à vivre de ce qui est déjà passé
	PrevuRev int64 // revenus encore à venir ce mois-ci
	PrevuDep int64 // dépenses encore à venir ce mois-ci
	Courbe   template.HTML
	ParCat   []parCategorie
	MaxCat   int64
	PayePar  []payePar // au moins deux parents ont payé ce mois-ci
	Ecart    int64     // différence entre le premier et le second
	Plafonds bool      // la famille a fixé au moins un plafond
	Jours    []jourOps
	Pilules  []Categorie
	Prevues  []ligneOp // opérations qui reviennent, pas encore passées ce mois-ci
	NbRecur  int       // opérations qui reviennent (lien vers leur page)
	Devise   string
}

func lireMois(r *http.Request, c *appli.Contexte) time.Time {
	if t, err := time.ParseInLocation("2006-01", r.URL.Query().Get("mois"), c.Lieu); err == nil {
		return t
	}
	return c.Maintenant()
}

func (m *Module) operations(ctx context.Context, c *appli.Contexte, compte int64, du, au time.Time) ([]donnees.Operation, error) {
	return m.a.Q.OperationsPeriode(ctx, donnees.OperationsPeriodeParams{FamilleID: c.Famille.ID, Compte: compte, Du: repetition.CleJour(du), Au: repetition.CleJour(au)})
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

func (m *Module) vue(ctx context.Context, c *appli.Contexte, compte int64, ref time.Time, filtre string) (*vueBudget, error) {
	premier, dernier := mois(ref)
	ppremier, pdernier := mois(premier.AddDate(0, -1, 0))
	nom := appli.MoisFR(premier.Month())
	v := &vueBudget{
		Mois: premier.Format("2006-01"), Titre: strings.ToUpper(nom[:1]) + nom[1:] + " " + strconv.Itoa(premier.Year()),
		Precedent: ppremier.Format("2006-01"), Suivant: premier.AddDate(0, 1, 0).Format("2006-01"),
		Devise: c.Famille.Devise, Compte: nomCompte(compte), Perso: compte > 0,
		Aujourdhui: repetition.CleJour(c.Maintenant()), Frequences: Frequences,
	}
	if _, ok := CategorieParCle(filtre); ok {
		v.Filtre = filtre
	}
	// Les opérations qui reviennent sont d'abord écrites jusqu'à aujourd'hui.
	if err := GenererSiBesoin(ctx, m.a, c.Famille.ID, c.Maintenant()); err != nil {
		return nil, err
	}
	ops, err := m.operations(ctx, c, compte, premier, dernier)
	if err != nil {
		return nil, err
	}
	prec, err := m.operations(ctx, c, compte, ppremier, pdernier)
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
	parMembre := map[int64]int64{}
	for _, o := range ops {
		if o.Sens == "revenu" {
			v.Revenus += o.Montant
		} else {
			v.Depenses += o.Montant
			totaux[o.Categorie] += o.Montant
			if o.MembreID.Valid {
				parMembre[o.MembreID.Int64] += o.Montant
			}
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
	plafonds, err := m.plafonds(ctx, c, compte)
	if err != nil {
		return nil, err
	}
	v.Plafonds = len(plafonds) > 0
	for _, cat := range Categories {
		if presentes[cat.Cle] {
			v.Pilules = append(v.Pilules, cat)
		}
		if n := totaux[cat.Cle]; n > 0 || plafonds[cat.Cle] > 0 {
			v.ParCat = append(v.ParCat, jauge(cat, n, plafonds[cat.Cle]))
			v.MaxCat = max(v.MaxCat, n)
		}
	}
	for i := range v.ParCat {
		if v.ParCat[i].Plafond == 0 {
			v.ParCat[i].Max = v.MaxCat
		}
	}
	sort.SliceStable(v.ParCat, func(i, j int) bool { return v.ParCat[i].Montant > v.ParCat[j].Montant })
	// Qui a payé : utile quand deux parents partagent les dépenses.
	for _, mb := range membres {
		if n := parMembre[mb.ID]; n > 0 {
			v.PayePar = append(v.PayePar, payePar{Membre: parID[mb.ID], Montant: n})
		}
	}
	if len(v.PayePar) < 2 || compte > 0 {
		v.PayePar = nil
	} else {
		sort.SliceStable(v.PayePar, func(i, j int) bool { return v.PayePar[i].Montant > v.PayePar[j].Montant })
		v.Ecart = v.PayePar[0].Montant - v.PayePar[1].Montant
	}

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
	recs, err := m.recurrencesCompte(ctx, c, compte)
	if err != nil {
		return nil, err
	}
	v.NbRecur = len(recs)
	// Côté commun, les virements qui reviennent des budgets perso sont des
	// revenus à venir.
	if compte == 0 {
		recus, err := m.virementsRecus(ctx, c, parID)
		if err != nil {
			return nil, err
		}
		recs = append(recs, recus...)
	}
	// Le reste à vivre compte le mois entier : ce qui est passé et ce qui
	// est prévu (salaire, virement, loyer… pas encore arrivés).
	v.ResteAuj = v.Reste
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
			v.PrevuRev += p.Montant
		} else {
			v.PrevuDep += p.Montant
		}
	}
	v.Revenus += v.PrevuRev
	v.Depenses += v.PrevuDep
	v.Reste = v.Revenus - v.Depenses
	return v, nil
}

// recurrencesCompte : les opérations qui reviennent de ce budget.
// prevuNet : revenus − dépenses encore à venir du mois (opérations qui
// reviennent, et pour le commun les virements des budgets perso).
func (m *Module) prevuNet(ctx context.Context, c *appli.Contexte, compte int64, premier, dernier time.Time) (int64, error) {
	recs, err := m.recurrencesCompte(ctx, c, compte)
	if err != nil {
		return 0, err
	}
	if compte == 0 {
		recus, err := m.virementsRecus(ctx, c, nil)
		if err != nil {
			return 0, err
		}
		recs = append(recs, recus...)
	}
	var net int64
	for _, p := range Prevues(recs, premier, dernier, c.Maintenant()) {
		if p.Sens == "revenu" {
			net += p.Montant
		} else {
			net -= p.Montant
		}
	}
	return net, nil
}

// virementsRecus : les virements qui reviennent des budgets perso vers le
// commun, vus du côté commun (revenus « Virement de Marie »).
func (m *Module) virementsRecus(ctx context.Context, c *appli.Contexte, parID map[int64]*donnees.Membre) ([]donnees.Recurrence, error) {
	tous, err := m.a.Q.RecurrencesFamille(ctx, c.Famille.ID)
	var out []donnees.Recurrence
	for _, r := range tous {
		if r.Virement == 1 {
			r.Sens, r.Categorie = "revenu", "virement-recu"
			if mb, ok := parID[r.PersoDe]; ok {
				r.Libelle = libelleVirement(mb.Nom)
			}
			out = append(out, r)
		}
	}
	return out, err
}

func (m *Module) recurrencesCompte(ctx context.Context, c *appli.Contexte, compte int64) ([]donnees.Recurrence, error) {
	tous, err := m.a.Q.RecurrencesFamille(ctx, c.Famille.ID)
	var out []donnees.Recurrence
	for _, r := range tous {
		if r.PersoDe == compte {
			out = append(out, r)
		}
	}
	return out, err
}

func (m *Module) page(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	v, err := m.vue(r.Context(), c, m.compteDe(w, r, c), lireMois(r, c), r.URL.Query().Get("cat"))
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Afficher(w, r, c, "budget/budget", "Budget", mod, v)
}

type formOp struct {
	Compte     string   // « commun » ou « perso »
	Virement   bool     // moitié d'un virement : montant et date seulement
	Libelles   []string // déjà tapés (suggestions)
	Nouvelle   bool
	Op         donnees.Operation
	Montant    string
	Categories []Categorie
	Membres    []donnees.Membre
	Frequences []Frequence
	Recurrence string // « Chaque mois », si l'opération fait partie d'une récurrence
	Devise     string
}

// operation : l'opération {id}, si elle est dans un budget de ce membre
// (le commun, ou son perso).
func (m *Module) operation(r *http.Request, c *appli.Contexte) (donnees.Operation, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return donnees.Operation{}, sql.ErrNoRows
	}
	o, err := m.a.Q.OperationParID(r.Context(), donnees.OperationParIDParams{ID: id, FamilleID: c.Famille.ID})
	if err == nil && !visible(c, o.PersoDe) {
		return donnees.Operation{}, sql.ErrNoRows
	}
	return o, err
}

func (m *Module) formulaire(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	f := formOp{Nouvelle: r.PathValue("id") == "", Categories: Categories, Frequences: Frequences, Devise: c.Famille.Devise}
	var err error
	if f.Membres, err = m.a.Membres(r.Context(), c); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	compte := m.compteDe(w, r, c)
	if f.Nouvelle {
		f.Op = donnees.Operation{Sens: "depense", Categorie: "courses", Jour: repetition.CleJour(c.Maintenant()),
			MembreID: sql.NullInt64{Int64: c.Membre.ID, Valid: true}, PersoDe: compte}
		if r.URL.Query().Get("sens") == "revenu" {
			f.Op.Sens, f.Op.Categorie = "revenu", "salaire"
		}
		// Nouvelle opération : seulement les catégories du sens choisi (une
		// tuile de dépense touchée sur « Nouveau revenu » le changeait en
		// dépense sans le dire).
		f.Categories = nil
		for _, cat := range Categories {
			if cat.Sens == f.Op.Sens {
				f.Categories = append(f.Categories, cat)
			}
		}
	} else {
		if f.Op, err = m.operation(r, c); err != nil {
			m.a.PageErreur(w, r, c, http.StatusNotFound, "Opération introuvable", "Cette opération n'existe plus.")
			return
		}
		f.Montant = MontantSaisie(f.Op.Montant)
		compte = f.Op.PersoDe
		f.Virement = estVirement(f.Op.Categorie)
		if f.Op.RecurrenceID.Valid {
			if rec, err := m.a.Q.RecurrenceParID(r.Context(), donnees.RecurrenceParIDParams{ID: f.Op.RecurrenceID.Int64, FamilleID: c.Famille.ID}); err == nil {
				f.Recurrence = NomFrequence(rec.Frequence)
			}
		}
	}
	f.Compte = nomCompte(compte)
	if f.Libelles, err = m.a.Q.LibellesRecents(r.Context(), donnees.LibellesRecentsParams{FamilleID: c.Famille.ID, PersoDe: compte}); err != nil {
		m.a.Journal.Warn("budget : libellés illisibles", "erreur", err) // le formulaire marche sans
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
	var ok2 bool
	if s.jour, ok2 = LireJour(r.PostFormValue("jour"), c.Maintenant()); !ok2 {
		return s, "Date invalide."
	}
	if id, err := strconv.ParseInt(r.PostFormValue("membre"), 10, 64); err == nil && id > 0 {
		if _, err := m.a.Q.MembreParID(r.Context(), donnees.MembreParIDParams{ID: id, FamilleID: c.Famille.ID}); err == nil {
			s.membre = sql.NullInt64{Int64: id, Valid: true}
		}
	}
	if f := r.PostFormValue("frequence"); FrequenceValide(f) {
		s.frequence = f
		s.fin = r.PostFormValue("fin")
		if _, ok := LireJour(s.fin, c.Maintenant()); s.fin != "" && (!ok || s.fin < s.jour) {
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
	compte := m.compteDe(w, r, c)
	if s.frequence != "" {
		if msg := ValiderProchaine(s.jour, nil, c.Maintenant()); msg != "" {
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
		// Opération qui revient : la récurrence écrit elle-même les occurrences
		// passées ou du jour ; les suivantes viendront à leur date.
		if _, err := m.a.Q.CreerRecurrence(ctx, donnees.CreerRecurrenceParams{
			FamilleID: c.Famille.ID, PersoDe: compte, Sens: s.sens, Montant: s.montant, Categorie: s.categorie, Libelle: s.libelle,
			MembreID: s.membre, Frequence: s.frequence, Debut: s.jour, Fin: s.fin, CreePar: auteur, CreeLe: time.Now().Unix(),
		}); err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
		if err := Generer(ctx, m.a, c.Famille.ID, c.Maintenant()); err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
	} else {
		// Double envoi (double clic, réseau lent) : la même opération, écrite
		// par la même personne dans les 10 dernières secondes, n'est pas répétée.
		deja, err := m.a.Q.OperationIdentiqueRecente(ctx, donnees.OperationIdentiqueRecenteParams{
			FamilleID: c.Famille.ID, PersoDe: compte, Sens: s.sens, Montant: s.montant, Categorie: s.categorie, Libelle: s.libelle,
			Jour: s.jour, CreePar: auteur, CreeLe: time.Now().Unix() - 10,
		})
		if err == nil && deja == 0 {
			_, err = m.a.Q.CreerOperation(ctx, donnees.CreerOperationParams{
				FamilleID: c.Famille.ID, PersoDe: compte, Sens: s.sens, Montant: s.montant, Categorie: s.categorie, Libelle: s.libelle,
				Jour: s.jour, MembreID: s.membre, CreePar: auteur, CreeLe: time.Now().Unix(),
			})
		}
		if err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Enregistré ✓")
	m.a.Rediriger(w, r, "/budget?compte="+nomCompte(compte)+"&mois="+s.jour[:7])
}

func (m *Module) modifier(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	o, err := m.operation(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Opération introuvable.")
		return
	}
	// Un virement garde sa catégorie (le formulaire ne la propose pas).
	if estVirement(o.Categorie) {
		_ = r.ParseForm()
		r.PostForm.Set("categorie", o.Categorie)
	}
	s, msg := m.lire(r, c)
	if msg != "" {
		m.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	ctx := r.Context()
	err = m.a.Transaction(ctx, func(q *donnees.Queries) error {
		if err := q.ModifierOperation(ctx, donnees.ModifierOperationParams{
			Sens: s.sens, Montant: s.montant, Categorie: s.categorie, Libelle: s.libelle, Jour: s.jour,
			MembreID: s.membre, ID: o.ID, FamilleID: c.Famille.ID,
		}); err != nil {
			return err
		}
		// L'autre moitié d'un virement suit (montant et date).
		if o.VirementID.Valid {
			return q.ModifierMoitieVirement(ctx, donnees.ModifierMoitieVirementParams{Montant: s.montant, Jour: s.jour,
				ID: o.VirementID.Int64, FamilleID: c.Famille.ID})
		}
		return nil
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Enregistré ✓")
	m.a.Rediriger(w, r, "/budget?compte="+nomCompte(o.PersoDe)+"&mois="+s.jour[:7])
}

func (m *Module) supprimer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	o, err := m.operation(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Opération introuvable.")
		return
	}
	// Un virement part en entier (ses deux moitiés).
	if err := m.a.Transaction(r.Context(), func(q *donnees.Queries) error {
		return supprimerAvecMoitie(r.Context(), q, c.Famille.ID, o)
	}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	if o.VirementID.Valid {
		m.a.Flash(w, "Virement supprimé (des deux côtés)")
	} else {
		m.a.Flash(w, "Opération supprimée")
	}
	m.a.Rediriger(w, r, "/budget?compte="+nomCompte(o.PersoDe)+"&mois="+o.Jour[:7])
}

func (m *Module) resume(ctx context.Context, c *appli.Contexte) appli.Resume {
	if err := GenererSiBesoin(ctx, m.a, c.Famille.ID, c.Maintenant()); err != nil {
		m.a.Journal.Error("budget : opérations qui reviennent", "erreur", err)
	}
	premier, dernier := mois(c.Maintenant())
	totaux := func(compte int64) (donnees.TotauxPeriodeRow, error) {
		return m.a.Q.TotauxPeriode(ctx, donnees.TotauxPeriodeParams{FamilleID: c.Famille.ID, Compte: compte,
			Du: repetition.CleJour(premier), Au: repetition.CleJour(dernier)})
	}
	t, err := totaux(0)
	p, errP := totaux(c.Membre.ID)
	if err != nil || errP != nil || t.Revenus+t.Depenses+p.Revenus+p.Depenses == 0 {
		return appli.Resume{Info: "Rien ce mois-ci"}
	}
	// Un plafond presque atteint passe avant le reste à vivre.
	for _, compte := range []int64{0, c.Membre.ID} {
		if alerte := m.alertePlafond(ctx, c, compte, premier, dernier); alerte != "" {
			return appli.Resume{Info: alerte, Badge: 1}
		}
	}
	// Même reste à vivre que la page : le prévu du mois compris.
	netCommun, err := m.prevuNet(ctx, c, 0, premier, dernier)
	netMoi, errM := m.prevuNet(ctx, c, c.Membre.ID, premier, dernier)
	if err != nil || errM != nil {
		netCommun, netMoi = 0, 0
	}
	info := "Reste " + appli.Montant(t.Revenus-t.Depenses+netCommun, c.Famille.Devise)
	if p.Revenus+p.Depenses > 0 || netMoi != 0 {
		info = "Commun : " + appli.Montant(t.Revenus-t.Depenses+netCommun, c.Famille.Devise) + " · Moi : " + appli.Montant(p.Revenus-p.Depenses+netMoi, c.Famille.Devise)
	}
	return appli.Resume{Info: info}
}
