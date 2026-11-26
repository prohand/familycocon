// SPDX-License-Identifier: GPL-3.0-or-later

package budget

import (
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
)

// « Les prochains mois » : pour chaque mois à venir (le mois en cours et
// les 11 suivants), ce qui rentrera et ce qui sera prélevé, d'après les
// opérations qui reviennent (salaire, loyer, crédit, taxe foncière de
// janvier à octobre, cantine de septembre à juin, assurance une fois par
// an…), et ce qu'il restera. Le mois en cours compte aussi ce qui est déjà
// passé. Un mois touché ouvre la page du mois.

const nbProchains = 12

// moisPrevu : une ligne de « Les prochains mois ».
type moisPrevu struct {
	Mois    string // 2026-12 (adresse de la page du mois)
	Num     int    // 12
	Nom     string // Décembre
	An      int
	Revenus int64
	Prelev  int64
	Reste   int64
	EnCours bool
	Note    string        // ce qui change ce mois-là
	Barre   template.HTML // prélèvements / reste (SVG)
	Charge  bool          // plus chargé que d'habitude (un « + » dans la note)
}

type vueProchains struct {
	Compte string
	Perso  bool
	Mois   []moisPrevu
	Total  struct{ Revenus, Prelev, Reste int64 }
	Graphe template.HTML // grand écran : barres revenus / prélèvements
	Devise string
	An     string
}

func (m *Module) pageProchains(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	if err := GenererSiBesoin(ctx, m.a, c.Famille.ID, c.Maintenant()); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	compte := m.compteDe(w, r, c)
	recs, err := m.recurrencesCompte(ctx, c, compte)
	if err == nil && compte == 0 {
		var recus []donnees.Recurrence
		if recus, err = m.virementsRecus(ctx, c, nil); err == nil {
			recs = append(recs, recus...)
		}
	}
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	maintenant := c.Maintenant()
	v := vueProchains{Compte: nomCompte(compte), Perso: compte > 0, Devise: c.Famille.Devise, An: strconv.Itoa(maintenant.Year())}
	// Combien de mois chaque prélèvement apparaît (pour dire ce qui change).
	presences := map[string]int{}
	parMois := make([]map[string]int64, nbProchains)
	debut, _ := mois(maintenant)
	for i := 0; i < nbProchains; i++ {
		premier, dernier := mois(debut.AddDate(0, i, 0))
		mp := moisPrevu{Mois: premier.Format("2006-01"), Num: int(premier.Month()), An: premier.Year(), EnCours: i == 0}
		nom := appli.MoisFR(premier.Month())
		mp.Nom = strings.ToUpper(nom[:1]) + nom[1:]
		parMois[i] = map[string]int64{}
		ajouter := func(sens, libelle, categorie string, montant int64) {
			if sens == "revenu" {
				mp.Revenus += montant
				return
			}
			mp.Prelev += montant
			cle := libelle
			if cle == "" {
				cat, _ := CategorieParCle(categorie)
				cle = cat.Nom
			}
			parMois[i][cle] += montant
		}
		if i == 0 {
			// Le mois en cours : ce qui est déjà passé compte aussi
			// (revenus, et prélèvements des opérations qui reviennent).
			ops, err := m.operations(ctx, c, compte, premier, dernier)
			if err != nil {
				m.a.ErreurInterne(w, r, err)
				return
			}
			for _, o := range ops {
				if o.Sens == "revenu" || o.RecurrenceID.Valid {
					ajouter(o.Sens, o.Libelle, o.Categorie, o.Montant)
				}
			}
		}
		for _, p := range Prevues(recs, premier, dernier, maintenant) {
			ajouter(p.Sens, p.Libelle, p.Categorie, p.Montant)
		}
		for cle := range parMois[i] {
			presences[cle]++
		}
		mp.Reste = mp.Revenus - mp.Prelev
		mp.Barre = barrePrevu(mp.Prelev, mp.Revenus)
		v.Total.Revenus += mp.Revenus
		v.Total.Prelev += mp.Prelev
		v.Mois = append(v.Mois, mp)
	}
	v.Total.Reste = v.Total.Revenus - v.Total.Prelev
	// Ce qui change : « + Assurance habitation » (rare, présent ce mois-ci),
	// « pas de cantine » (habituel, absent ce mois-ci). Pas pour le mois en
	// cours : ce qui commence plus tard n'y est simplement pas encore.
	for i := range v.Mois {
		var plus, moins []string
		for cle, n := range presences {
			_, ici := parMois[i][cle]
			switch {
			case ici && n <= nbProchains/2:
				plus = append(plus, "+ "+cle)
			case !ici && n > nbProchains/2 && i > 0:
				moins = append(moins, "pas de "+minusculeDebut(cle))
			}
		}
		sort.Strings(plus)
		sort.Strings(moins)
		v.Mois[i].Note = strings.Join(append(plus, moins...), " · ")
		v.Mois[i].Charge = len(plus) > 0
	}
	v.Graphe = graphePrevu(v.Mois)
	m.a.Afficher(w, r, c, "budget/prochains", "Les prochains mois", mod, v)
}

// minusculeDebut : « Électricité » → « électricité » (première lettre, en
// entier : pas l'octet).
func minusculeDebut(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return strings.ToLower(string(r[0])) + string(r[1:])
}

// moisCourts : les noms de mois abrégés du graphique.
var moisCourts = []string{"janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."}

// barrePrevu : la barre d'un mois (prélèvements, puis ce qui reste), en SVG
// (pas de style écrit dans la page).
func barrePrevu(prelev, revenus int64) template.HTML {
	total := max(revenus, prelev)
	if total <= 0 {
		return ""
	}
	w := prelev * 1000 / total
	return template.HTML(fmt.Sprintf(`<svg class="barre-prevu" viewBox="0 0 1000 10" preserveAspectRatio="none" aria-hidden="true">`+
		`<rect class="fond-reste" width="1000" height="10" rx="5"/><rect class="prelev" width="%d" height="10" rx="5"/></svg>`, w)) // #nosec G203 -- construit ici, sans texte
}

// graphePrevu : grand écran, deux barres par mois (revenus, prélèvements).
func graphePrevu(l []moisPrevu) template.HTML {
	var haut int64 = 1
	for _, x := range l {
		haut = max(haut, x.Revenus, x.Prelev)
	}
	const h, colonne = 160, 60
	var b strings.Builder
	fmt.Fprintf(&b, `<svg class="graphe-prevu" viewBox="0 0 %d %d" role="img" aria-label="Revenus et prélèvements des prochains mois">`, colonne*len(l), h+22)
	for i, x := range l {
		hr, hp := x.Revenus*h/haut, x.Prelev*h/haut
		x0 := i*colonne + 14
		fmt.Fprintf(&b, `<rect class="rev" x="%d" y="%d" width="14" height="%d" rx="3"/>`, x0, h-hr, hr)
		fmt.Fprintf(&b, `<rect class="prelev" x="%d" y="%d" width="14" height="%d" rx="3"/>`, x0+17, h-hp, hp)
		fmt.Fprintf(&b, `<text x="%d" y="%d" text-anchor="middle">%s</text>`, i*colonne+colonne/2, h+16, template.HTMLEscapeString(moisCourts[x.Num-1]))
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String()) // #nosec G203 -- construit ici, texte échappé
}
