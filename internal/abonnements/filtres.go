// SPDX-License-Identifier: GPL-3.0-or-later

package abonnements

import (
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"familycocon/internal/budget"
	"familycocon/internal/donnees"
)

// Filtres de la liste : catégorie, pour qui, commun ou perso, fréquence.
// Ils sont dans l'adresse (?cat=…&pour=…&compte=…&freq=…) : un lien
// filtré se garde et se partage. Une rangée de filtres n'est montrée que
// s'il y a un vrai choix (au moins deux valeurs parmi les abonnements).

// filtre : les choix de l'adresse (vide ou 0 : tous).
type filtre struct {
	Cat    string // clé de catégorie du budget
	Pour   int64  // membre ; -1 : « Toute la famille » (aucun membre)
	Compte string // « commun » ou « perso »
	Freq   string // clé de fréquence
}

func lireFiltre(r *http.Request) filtre {
	q := r.URL.Query()
	f := filtre{Cat: q.Get("cat"), Freq: q.Get("freq"), Compte: q.Get("compte")}
	f.Pour, _ = strconv.ParseInt(q.Get("pour"), 10, 64)
	if f.Compte != "commun" && f.Compte != "perso" {
		f.Compte = ""
	}
	return f
}

// actif : au moins un filtre choisi.
func (f filtre) actif() bool { return f != filtre{} }

// adresse : la liste avec ces filtres.
func (f filtre) adresse() string {
	q := url.Values{}
	if f.Cat != "" {
		q.Set("cat", f.Cat)
	}
	if f.Pour != 0 {
		q.Set("pour", strconv.FormatInt(f.Pour, 10))
	}
	if f.Compte != "" {
		q.Set("compte", f.Compte)
	}
	if f.Freq != "" {
		q.Set("freq", f.Freq)
	}
	if len(q) == 0 {
		return "/abonnements"
	}
	return "/abonnements?" + q.Encode()
}

// garde : l'abonnement passe-t-il les filtres ? « sauf » : un filtre
// à ne pas regarder (pour compter les abonnements de chaque choix de
// cette rangée avec les autres filtres).
func (f filtre) garde(l Ligne, sauf string) bool {
	if sauf != "cat" && f.Cat != "" && l.Categorie != f.Cat {
		return false
	}
	if sauf != "pour" && f.Pour != 0 && pourDe(l) != f.Pour {
		return false
	}
	if sauf != "compte" && f.Compte != "" && compteDe(l) != f.Compte {
		return false
	}
	if sauf != "freq" && f.Freq != "" && l.ListeAbonnementsRow.Frequence != f.Freq {
		return false
	}
	return true
}

// pourDe : le membre de l'abonnement, ou -1 (toute la famille).
func pourDe(l Ligne) int64 {
	if l.MembreID.Valid {
		return l.MembreID.Int64
	}
	return -1
}

// compteDe : « perso » (privé, budget perso) ou « commun ».
func compteDe(l Ligne) string {
	if l.PersoDe != 0 {
		return "perso"
	}
	return "commun"
}

// Choix : une pastille d'une rangée de filtres.
type Choix struct {
	Texte  string
	URL    string
	Actif  bool
	Nombre int // abonnements en cours de ce choix (avec les autres filtres)
}

// Rangee : une rangée de filtres (« Catégorie : Toutes · Assurance · … »).
type Rangee struct {
	Nom   string
	Choix []Choix
}

// Noms courts des fréquences (les longs de budget.Frequences sont pour
// le formulaire).
var frequencesCourtes = map[string]string{
	"semaine": "Semaine", "mois": "Mois", "mois10": "10 mois (janv.→oct.)", "scolaire": "Année scolaire",
	"2mois": "2 mois", "trimestre": "Trimestre", "semestre": "Semestre", "annee": "Année",
}

// rangees : les rangées de filtres pour ces abonnements (tous, résiliés
// compris : un filtre ne disparaît pas parce qu'il ne reste rien).
func rangees(l []Ligne, f filtre, membres []donnees.Membre) []Rangee {
	var out []Rangee
	// rangee ajoute une rangée : « valeur » donne la valeur d'un abonnement,
	// « avec » le filtre où cette rangée vaut v.
	rangee := func(nom, cle string, valeurs []string, texte func(string) string, valeur func(Ligne) string,
		avec func(filtre, string) filtre, actuelle string) {
		if len(valeurs) < 2 {
			return
		}
		r := Rangee{Nom: nom}
		tous := avec(f, "")
		r.Choix = append(r.Choix, Choix{Texte: "Tous", URL: tous.adresse(), Actif: actuelle == ""})
		for _, v := range valeurs {
			n := 0
			for _, x := range l {
				if !x.Resilie && f.garde(x, cle) && valeur(x) == v {
					n++
				}
			}
			r.Choix = append(r.Choix, Choix{Texte: texte(v), URL: avec(f, v).adresse(), Actif: actuelle == v, Nombre: n})
		}
		out = append(out, r)
	}
	// distinctes : les valeurs présentes, dans l'ordre donné par « ordre ».
	distinctes := func(valeur func(Ligne) string, ordre func(a, b string) bool) []string {
		vu := map[string]bool{}
		var v []string
		for _, x := range l {
			if k := valeur(x); !vu[k] {
				vu[k] = true
				v = append(v, k)
			}
		}
		sort.SliceStable(v, func(i, j int) bool { return ordre(v[i], v[j]) })
		return v
	}

	// Catégorie (dans l'ordre de la liste du budget).
	rangCat := map[string]int{}
	for i, c := range budget.Categories {
		rangCat[c.Cle] = i
	}
	cat := func(x Ligne) string { return x.Categorie }
	rangee("Catégorie", "cat", distinctes(cat, func(a, b string) bool { return rangCat[a] < rangCat[b] }),
		func(v string) string {
			if c, ok := budget.CategorieParCle(v); ok {
				return c.Emoji + " " + c.Nom
			}
			return v
		}, cat, func(f filtre, v string) filtre { f.Cat = v; return f }, f.Cat)

	// Pour qui (dans l'ordre des membres ; « Toute la famille » en premier).
	noms := map[string]string{"-1": "👪 Toute la famille"}
	rangMembre := map[string]int{"-1": -1}
	for i, mb := range membres {
		k := strconv.FormatInt(mb.ID, 10)
		noms[k], rangMembre[k] = mb.Nom, i
	}
	pour := func(x Ligne) string { return strconv.FormatInt(pourDe(x), 10) }
	actuel := ""
	if f.Pour != 0 {
		actuel = strconv.FormatInt(f.Pour, 10)
	}
	rangee("Pour", "pour", distinctes(pour, func(a, b string) bool { return rangMembre[a] < rangMembre[b] }),
		func(v string) string {
			if n, ok := noms[v]; ok {
				return n
			}
			return "Ancien membre"
		}, pour, func(f filtre, v string) filtre { f.Pour, _ = strconv.ParseInt(v, 10, 64); return f }, actuel)

	// Budget commun ou perso.
	rangee("Budget", "compte", distinctes(compteDe, func(a, b string) bool { return a < b }),
		func(v string) string {
			if v == "perso" {
				return "🔒 Perso"
			}
			return "👪 Commun"
		}, compteDe, func(f filtre, v string) filtre { f.Compte = v; return f }, f.Compte)

	// Fréquence (dans l'ordre de budget.Frequences).
	rangFreq := map[string]int{}
	for i, fr := range budget.Frequences {
		rangFreq[fr.Cle] = i
	}
	freq := func(x Ligne) string { return x.ListeAbonnementsRow.Frequence }
	rangee("Fréquence", "freq", distinctes(freq, func(a, b string) bool { return rangFreq[a] < rangFreq[b] }),
		func(v string) string {
			if n, ok := frequencesCourtes[v]; ok {
				return n
			}
			return budget.NomFrequence(v)
		}, freq, func(f filtre, v string) filtre { f.Freq = v; return f }, f.Freq)
	return out
}

// resumeFiltres : les choix actifs (hors « Tous »), « Assurances · Mois ».
func resumeFiltres(rs []Rangee) string {
	var t []string
	for _, r := range rs {
		for i, c := range r.Choix {
			if c.Actif && i > 0 {
				t = append(t, c.Texte)
			}
		}
	}
	return strings.Join(t, " · ")
}
