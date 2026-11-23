// SPDX-License-Identifier: GPL-3.0-or-later

package budget

import (
	"context"
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

// Outils du budget : recherche dans les opérations, vue de l'année,
// plafonds par catégorie (enveloppes).

// ---------- Plafonds ----------

// seuilAlerte : part du plafond (en %) à partir de laquelle on prévient.
const seuilAlerte = 90

// plafonds : les plafonds de la famille, par catégorie (centimes par mois).
func (m *Module) plafonds(ctx context.Context, c *appli.Contexte) (map[string]int64, error) {
	l, err := m.a.Q.PlafondsFamille(ctx, c.Famille.ID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(l))
	for _, p := range l {
		out[p.Categorie] = p.Montant
	}
	return out, nil
}

// jauge : la ligne « par catégorie », comparée au plafond s'il y en a un.
func jauge(cat Categorie, depense, plafond int64) parCategorie {
	p := parCategorie{Cat: cat, Montant: depense, Plafond: plafond}
	if plafond > 0 {
		p.Max = plafond
		p.Pour100 = depense * 100 / plafond
		p.Alerte = p.Pour100 >= seuilAlerte
	}
	return p
}

// alertePlafond : « Courses : 92 % du plafond » pour la catégorie la plus
// proche de son plafond (au-delà de 90 %), « » sinon.
func (m *Module) alertePlafond(ctx context.Context, c *appli.Contexte, premier, dernier time.Time) string {
	plafonds, err := m.plafonds(ctx, c)
	if err != nil || len(plafonds) == 0 {
		return ""
	}
	totaux, err := m.a.Q.TotauxParMois(ctx, donnees.TotauxParMoisParams{FamilleID: c.Famille.ID,
		Du: repetition.CleJour(premier), Au: repetition.CleJour(dernier)})
	if err != nil {
		return ""
	}
	pire := parCategorie{}
	for _, t := range totaux {
		cat, ok := CategorieParCle(t.Categorie)
		if !ok || t.Sens != "depense" || plafonds[t.Categorie] == 0 {
			continue
		}
		if j := jauge(cat, t.Total, plafonds[t.Categorie]); j.Alerte && j.Pour100 > pire.Pour100 {
			pire = j
		}
	}
	if pire.Pour100 == 0 {
		return ""
	}
	return pire.Cat.Nom + " : " + strconv.FormatInt(pire.Pour100, 10) + " % du plafond"
}

type lignePlafond struct {
	Cat     Categorie
	Montant string // saisie (vide : pas de plafond)
}

func (m *Module) pagePlafonds(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	p, err := m.plafonds(r.Context(), c)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	var lignes []lignePlafond
	for _, cat := range Categories {
		if cat.Sens != "depense" {
			continue
		}
		l := lignePlafond{Cat: cat}
		if n := p[cat.Cle]; n > 0 {
			l.Montant = MontantSaisie(n)
		}
		lignes = append(lignes, l)
	}
	m.a.Afficher(w, r, c, "budget/plafonds", "Plafonds", mod, map[string]any{"Lignes": lignes, "Devise": c.Famille.Devise})
}

func (m *Module) enregistrerPlafonds(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	if err := r.ParseForm(); err != nil {
		m.a.Echec(w, r, http.StatusBadRequest, "Formulaire invalide.")
		return
	}
	err := m.a.Transaction(ctx, func(q *donnees.Queries) error {
		for _, cat := range Categories {
			if cat.Sens != "depense" {
				continue
			}
			var n int64
			var err error
			if saisie := strings.TrimSpace(r.PostForm.Get("plafond_" + cat.Cle)); saisie != "" {
				if n, err = LireMontantOuZero(saisie); err != nil {
					return errPlafond{cat.Nom}
				}
			}
			if n == 0 {
				err = q.RetirerPlafond(ctx, donnees.RetirerPlafondParams{FamilleID: c.Famille.ID, Categorie: cat.Cle})
			} else {
				err = q.PoserPlafond(ctx, donnees.PoserPlafondParams{FamilleID: c.Famille.ID, Categorie: cat.Cle, Montant: n})
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
	if e, ok := err.(errPlafond); ok {
		m.a.Echec(w, r, http.StatusBadRequest, "Montant invalide pour « "+e.nom+" ».")
		return
	}
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Plafonds enregistrés ✓")
	m.a.Rediriger(w, r, "/budget")
}

type errPlafond struct{ nom string }

func (e errPlafond) Error() string { return "plafond invalide : " + e.nom }

// ---------- Recherche ----------

type vueRecherche struct {
	Q        string
	Jours    []jourOps
	Total    int64 // dépenses trouvées
	Revenus  int64
	Nombre   int
	Tronquee bool
	Devise   string
}

// chercher : les opérations des 12 derniers mois dont le libellé contient
// le texte (« dentiste »), avec le total.
func (m *Module) chercher(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	v := vueRecherche{Q: strings.TrimSpace(r.URL.Query().Get("q")), Devise: c.Famille.Devise}
	if utf8.RuneCountInString(v.Q) > 60 {
		v.Q = string([]rune(v.Q)[:60])
	}
	if v.Q != "" {
		depuis := repetition.CleJour(c.Maintenant().AddDate(-1, 0, 0))
		ops, err := m.a.Q.ChercherOperations(ctx, donnees.ChercherOperationsParams{FamilleID: c.Famille.ID, Depuis: depuis, Motif: v.Q})
		if err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
		v.Nombre, v.Tronquee = len(ops), len(ops) == 200
		for _, o := range ops {
			if o.Sens == "revenu" {
				v.Revenus += o.Montant
			} else {
				v.Total += o.Montant
			}
			cat, ok := CategorieParCle(o.Categorie)
			if !ok {
				cat, _ = CategorieParCle("autre")
			}
			d, _ := time.ParseInLocation("2006-01-02", o.Jour, c.Lieu)
			titre := appli.DateLongue(d) + " " + strconv.Itoa(d.Year())
			if len(v.Jours) == 0 || v.Jours[len(v.Jours)-1].Titre != titre {
				v.Jours = append(v.Jours, jourOps{Titre: titre})
			}
			v.Jours[len(v.Jours)-1].Ops = append(v.Jours[len(v.Jours)-1].Ops, ligneOp{Operation: o, Cat: cat})
		}
	}
	m.a.Afficher(w, r, c, "budget/chercher", "Chercher", mod, v)
}

// ---------- Année ----------

type moisAnnee struct {
	Mois     string // 2026-03
	Nom      string // Mars
	Revenus  int64
	Depenses int64
}

type catAnnee struct {
	Cat       Categorie
	Montant   int64
	Precedent int64 // l'année d'avant
	Ecart     int64 // Montant − Precedent
}

type vueAnnee struct {
	An, Precedent, Suivant int
	Mois                   []moisAnnee
	Max                    int64 // échelle des barres
	Revenus, Depenses      int64
	Reste                  int64
	Categories             []catAnnee
	Devise                 string
}

// annee : les 12 mois (revenus, dépenses) et les dépenses par catégorie,
// comparées à l'année d'avant.
func (m *Module) annee(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	an := c.Maintenant().Year()
	if n, err := strconv.Atoi(r.URL.Query().Get("an")); err == nil && n >= 1900 && n <= 2200 {
		an = n
	}
	v := vueAnnee{An: an, Precedent: an - 1, Suivant: an + 1, Devise: c.Famille.Devise}
	lire := func(a int) ([]donnees.TotauxParMoisRow, error) {
		return m.a.Q.TotauxParMois(ctx, donnees.TotauxParMoisParams{FamilleID: c.Famille.ID,
			Du: strconv.Itoa(a) + "-01-01", Au: strconv.Itoa(a) + "-12-31"})
	}
	cette, err := lire(an)
	if err == nil {
		var avant []donnees.TotauxParMoisRow
		if avant, err = lire(an - 1); err == nil {
			v.remplir(cette, avant)
		}
	}
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Afficher(w, r, c, "budget/annee", "Budget "+strconv.Itoa(an), mod, v)
}

func (v *vueAnnee) remplir(cette, avant []donnees.TotauxParMoisRow) {
	parMois := map[string]*moisAnnee{}
	for i := 1; i <= 12; i++ {
		nom := appli.MoisFR(time.Month(i))
		cle := strconv.Itoa(v.An) + "-" + twoDigits(i)
		v.Mois = append(v.Mois, moisAnnee{Mois: cle, Nom: strings.ToUpper(nom[:1]) + nom[1:]})
	}
	for i := range v.Mois {
		parMois[v.Mois[i].Mois] = &v.Mois[i]
	}
	parCat := map[string]*catAnnee{}
	cat := func(cle string) *catAnnee {
		if x, ok := parCat[cle]; ok {
			return x
		}
		c, ok := CategorieParCle(cle)
		if !ok {
			c, _ = CategorieParCle("autre")
		}
		x := &catAnnee{Cat: c}
		parCat[cle] = x
		return x
	}
	for _, t := range cette {
		if mo := parMois[t.Mois]; mo != nil {
			if t.Sens == "revenu" {
				mo.Revenus += t.Total
				v.Revenus += t.Total
			} else {
				mo.Depenses += t.Total
				v.Depenses += t.Total
			}
		}
		if t.Sens == "depense" {
			cat(t.Categorie).Montant += t.Total
		}
	}
	for _, t := range avant {
		if t.Sens == "depense" {
			cat(t.Categorie).Precedent += t.Total
		}
	}
	v.Reste = v.Revenus - v.Depenses
	for _, mo := range v.Mois {
		v.Max = max(v.Max, mo.Revenus, mo.Depenses)
	}
	for _, x := range parCat {
		x.Ecart = x.Montant - x.Precedent
		v.Categories = append(v.Categories, *x)
	}
	sort.Slice(v.Categories, func(i, j int) bool {
		if v.Categories[i].Montant != v.Categories[j].Montant {
			return v.Categories[i].Montant > v.Categories[j].Montant
		}
		return v.Categories[i].Cat.Cle < v.Categories[j].Cat.Cle
	})
}

func twoDigits(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}

// chercherGlobal : recherche globale (libellés des 12 derniers mois).
func (m *Module) chercherGlobal(ctx context.Context, c *appli.Contexte, q string) ([]appli.Resultat, error) {
	ops, err := m.a.Q.ChercherOperations(ctx, donnees.ChercherOperationsParams{FamilleID: c.Famille.ID,
		Depuis: repetition.CleJour(c.Maintenant().AddDate(-1, 0, 0)), Motif: q})
	if err != nil {
		return nil, err
	}
	var out []appli.Resultat
	for i, o := range ops {
		if i == 8 {
			break
		}
		cat, ok := CategorieParCle(o.Categorie)
		if !ok {
			cat, _ = CategorieParCle("autre")
		}
		signe := "−"
		if o.Sens == "revenu" {
			signe = "+"
		}
		detail := signe + appli.Montant(o.Montant, c.Famille.Devise)
		if d, err := time.Parse("2006-01-02", o.Jour); err == nil {
			detail += " · " + appli.DateLongue(d) + " " + strconv.Itoa(d.Year())
		}
		out = append(out, appli.Resultat{Emoji: cat.Emoji, Titre: o.Libelle, Detail: detail, URL: "/budget/" + strconv.FormatInt(o.ID, 10)})
	}
	return out, nil
}
