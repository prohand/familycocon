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

// Outils du budget : recherche dans les opérations, vue de l'année.

type vueRecherche struct {
	Q        string
	Jours    []jourOps
	Total    int64 // dépenses trouvées
	Revenus  int64
	Nombre   int
	Tronquee bool
	Devise   string
	Perso    bool // budget perso
}

// chercher : les opérations des 12 derniers mois dont le libellé contient
// le texte (« dentiste »), avec le total.
func (m *Module) chercher(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	compte := m.compteDe(w, r, c)
	v := vueRecherche{Q: strings.TrimSpace(r.URL.Query().Get("q")), Devise: c.Famille.Devise, Perso: compte > 0}
	if utf8.RuneCountInString(v.Q) > 60 {
		v.Q = string([]rune(v.Q)[:60])
	}
	if v.Q != "" {
		depuis := repetition.CleJour(c.Maintenant().AddDate(-1, 0, 0))
		ops, err := m.a.Q.ChercherOperations(ctx, donnees.ChercherOperationsParams{FamilleID: c.Famille.ID, Compte: compte, Depuis: depuis, Motif: v.Q})
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
	Perso                  bool // budget perso
}

// annee : les 12 mois (revenus, dépenses) et les dépenses par catégorie,
// comparées à l'année d'avant.
func (m *Module) annee(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	an := c.Maintenant().Year()
	if n, err := strconv.Atoi(r.URL.Query().Get("an")); err == nil && n >= 1900 && n <= 2200 {
		an = n
	}
	compte := m.compteDe(w, r, c)
	v := vueAnnee{An: an, Precedent: an - 1, Suivant: an + 1, Devise: c.Famille.Devise, Perso: compte > 0}
	lire := func(a int) ([]donnees.TotauxParMoisRow, error) {
		return m.a.Q.TotauxParMois(ctx, donnees.TotauxParMoisParams{FamilleID: c.Famille.ID, Compte: compte,
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

// chercherGlobal : recherche globale (libellés des 12 derniers mois), dans
// le compte commun puis dans le budget perso du membre.
func (m *Module) chercherGlobal(ctx context.Context, c *appli.Contexte, q string) ([]appli.Resultat, error) {
	var ops []donnees.Operation
	for _, compte := range []int64{0, c.Membre.ID} {
		l, err := m.a.Q.ChercherOperations(ctx, donnees.ChercherOperationsParams{FamilleID: c.Famille.ID, Compte: compte,
			Depuis: repetition.CleJour(c.Maintenant().AddDate(-1, 0, 0)), Motif: q})
		if err != nil {
			return nil, err
		}
		ops = append(ops, l...)
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
