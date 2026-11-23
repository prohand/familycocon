// SPDX-License-Identifier: GPL-3.0-or-later

package suivi

import (
	"context"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
	"familycocon/internal/graphique"
	txt "familycocon/internal/texte"
)

// Véhicules : le carnet en deux onglets.
//   - « Entretien » : les échéances et le carnet sans les pleins ;
//   - « Pleins » : les pleins (lignes avec des litres), des chiffres et 4
//     graphiques (consommation, prix du litre, dépenses par mois,
//     kilomètres par mois), sur 12 mois ou depuis le début.
// Les montants (prix, dépenses) ne sont montrés qu'aux parents.

// Onglets et périodes (paramètres de l'adresse de la fiche).
const (
	OngletEntretien = ""
	OngletPleins    = "pleins"
	Periode12Mois   = ""
	PeriodeTout     = "tout"
)

// PleinsParPage : pleins listés d'un coup (« Voir les pleins plus anciens »
// charge la page suivante).
const PleinsParPage = 100

// VuePleins : l'onglet « Pleins ».
type VuePleins struct {
	Periode   string
	Pleins    []donnees.Carnet // les PleinsParPage derniers
	Suite     int64            // où reprendre la liste (0 : tout est montré)
	Conso     string           // « 6,4 L / 100 km » sur la période
	PrixLitre string           // « 1,80 € » en moyenne sur la période
	Litres    string           // litres achetés sur la période
	Carburant int64            // dépensé en carburant sur la période
	ParAn     bool             // barres par année (période longue)
	// Graphiques (vides s'il n'y a rien à dessiner).
	GConso, GPrix, GDepenses, GKm template.HTML
}

// moisCourts : sous les barres.
var moisCourts = []string{"janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."}

// virgule : 6.4 → « 6,4 ».
func virgule(f float64, decimales int) string {
	return strings.Replace(strconv.FormatFloat(f, 'f', decimales, 64), ".", ",", 1)
}

// ChargerPleins : remplit l'onglet « Pleins » du bloc d'un véhicule.
func (s *Suivi) ChargerPleins(ctx context.Context, c *appli.Contexte, b *Bloc, periode string) error {
	if periode != PeriodeTout {
		periode = Periode12Mois
	}
	v := &VuePleins{Periode: periode}
	b.Pleins = v
	parent := c.EstParent()
	var err error
	if v.Pleins, v.Suite, err = s.pagePleins(ctx, c, b.Module, b.ObjetID, 0); err != nil {
		return err
	}
	auj := c.Maintenant()
	debut := time.Date(auj.Year(), auj.Month()-11, 1, 0, 0, 0, 0, c.Lieu) // 12 mois, celui-ci compris
	du := debut.Format("2006-01-02")
	if periode == PeriodeTout {
		du = "0000-00-00"
	}
	pleins, err := s.a.Q.PleinsPeriodeObjet(ctx, donnees.PleinsPeriodeObjetParams{FamilleID: c.Famille.ID, Module: b.Module, ObjetID: b.ObjetID, Du: du})
	if err != nil {
		return err
	}
	jour := func(j string) time.Time { t, _ := time.ParseInLocation("2006-01-02", j, c.Lieu); return t }
	dateCourte := func(t time.Time) string { return moisCourts[t.Month()-1] + " " + t.Format("06") }

	// Consommation à chaque plein complet : les litres mis depuis le plein
	// complet d'avant (pleins partiels compris), sur la distance parcourue.
	var conso []graphique.Point
	var litresConso, kmConso float64
	var dernierKm, cumul int64
	for _, x := range pleins {
		if x.Km <= 0 {
			continue
		}
		partiel := strings.Contains(txt.Plier(x.Libelle), "partiel")
		if dernierKm > 0 && x.Km > dernierKm {
			cumul += x.Litres
			if !partiel {
				d := float64(x.Km - dernierKm)
				l100 := float64(cumul) / 100 / d * 100
				// Un plein oublié dans le carnet donne une valeur absurde : écartée.
				if l100 >= 1 && l100 <= 60 {
					conso = append(conso, graphique.Point{Date: jour(x.Jour), Valeur: l100})
					litresConso += float64(cumul) / 100
					kmConso += d
				}
			}
		}
		if !partiel || dernierKm == 0 {
			dernierKm, cumul = x.Km, 0
		}
	}
	if kmConso > 0 {
		v.Conso = virgule(litresConso/kmConso*100, 1) + " L / 100 km"
	}
	v.GConso = graphique.Courbe("Consommation à chaque plein (L / 100 km)", conso, func(f float64) string { return virgule(f, 1) }, dateCourte)

	// Prix du litre et litres achetés.
	var prix []graphique.Point
	var litres, montant, litresPayes int64
	for _, x := range pleins {
		litres += x.Litres
		if x.Montant > 0 {
			montant += x.Montant
			litresPayes += x.Litres
			prix = append(prix, graphique.Point{Date: jour(x.Jour), Valeur: float64(x.Montant) / float64(x.Litres)}) // €/L
		}
	}
	if litres > 0 {
		v.Litres = appli.Milliers(int64(math.Round(float64(litres)/100))) + " L"
	}
	symbole := appli.Symbole(c.Famille.Devise)
	if parent {
		v.Carburant = montant
		if litresPayes > 0 {
			v.PrixLitre = virgule(float64(montant)/float64(litresPayes), 3) + " " + symbole
		}
		v.GPrix = graphique.Courbe("Prix du litre", prix, func(f float64) string { return virgule(f, 2) + " " + symbole }, dateCourte)
	}

	// Mois de la période (ou années, au-delà de 3 ans).
	mois, err := s.a.Q.DepensesMoisObjet(ctx, donnees.DepensesMoisObjetParams{FamilleID: c.Famille.ID, Module: b.Module, ObjetID: b.ObjetID, Du: du})
	if err != nil {
		return err
	}
	kms, err := s.a.Q.KmMoisObjet(ctx, donnees.KmMoisObjetParams{FamilleID: c.Famille.ID, Module: b.Module, ObjetID: b.ObjetID})
	if err != nil {
		return err
	}
	premier := debut
	if periode == PeriodeTout {
		premier = time.Date(auj.Year(), auj.Month(), 1, 0, 0, 0, 0, c.Lieu)
		for _, m := range []string{premierMois(mois), premierMoisKm(kms)} {
			if t, err := time.ParseInLocation("2006-01", m, c.Lieu); err == nil && t.Before(premier) {
				premier = t
			}
		}
	}
	var cles []string // « 2025-01 »… jusqu'au mois en cours
	for t := premier; !t.After(auj); t = t.AddDate(0, 1, 0) {
		cles = append(cles, t.Format("2006-01"))
	}
	v.ParAn = len(cles) > 36
	groupe := func(m string) string { // mois → colonne
		if v.ParAn {
			return m[:4]
		}
		return m
	}
	etiquette := func(g string) (court, long string) {
		if v.ParAn {
			return g, g
		}
		t, _ := time.ParseInLocation("2006-01", g, c.Lieu)
		return moisCourts[t.Month()-1], appli.MoisFR(t.Month()) + " " + strconv.Itoa(t.Year())
	}
	var colonnes []string
	vu := map[string]bool{}
	for _, m := range cles {
		if g := groupe(m); !vu[g] {
			vu[g] = true
			colonnes = append(colonnes, g)
		}
	}

	// Dépenses par mois : carburant et entretien empilés (parents).
	if parent {
		carb, entr := map[string]float64{}, map[string]float64{}
		for _, m := range mois {
			carb[groupe(m.Mois)] += float64(m.Carburant) / 100
			entr[groupe(m.Mois)] += float64(m.Entretien) / 100
		}
		var barres []graphique.Barre
		for _, g := range colonnes {
			court, long := etiquette(g)
			barres = append(barres, graphique.Barre{Etiquette: court, Valeurs: []float64{carb[g], entr[g]},
				Detail: fmt.Sprintf("%s : carburant %s, entretien %s", long, euros(carb[g], symbole), euros(entr[g], symbole))})
		}
		v.GDepenses = graphique.Barres("Dépenses par mois : carburant et entretien", barres, func(f float64) string { return euros(f, symbole) })
	}

	// Kilomètres par mois : le compteur le plus haut du mois, moins le plus
	// haut noté avant (un mois sans relevé compte 0, le suivant rattrape).
	parcourus := map[string]float64{}
	var avant int64
	for _, k := range kms {
		if avant > 0 && k.Km > avant {
			parcourus[groupe(k.Mois)] += float64(k.Km - avant)
		}
		avant = max(avant, k.Km)
	}
	var barresKm []graphique.Barre
	for _, g := range colonnes {
		court, long := etiquette(g)
		barresKm = append(barresKm, graphique.Barre{Etiquette: court, Valeurs: []float64{parcourus[g]},
			Detail: long + " : " + appli.Milliers(int64(parcourus[g])) + " km"})
	}
	v.GKm = graphique.Barres("Kilomètres parcourus par mois", barresKm, func(f float64) string { return appli.Milliers(int64(f)) })
	return nil
}

// euros : « 120 € » (sans centimes, pour les graphiques).
func euros(f float64, symbole string) string {
	return appli.Milliers(int64(math.Round(f))) + " " + symbole
}

func premierMois(l []donnees.DepensesMoisObjetRow) string {
	if len(l) == 0 {
		return ""
	}
	return l[0].Mois
}

func premierMoisKm(l []donnees.KmMoisObjetRow) string {
	if len(l) == 0 {
		return ""
	}
	return l[0].Mois
}

// pagePleins : une page de la liste des pleins (du plus récent au plus
// ancien) et où reprendre ensuite (0 : c'était la dernière page).
func (s *Suivi) pagePleins(ctx context.Context, c *appli.Contexte, module string, objet, depuis int64) ([]donnees.Carnet, int64, error) {
	l, err := s.a.Q.PleinsListeObjet(ctx, donnees.PleinsListeObjetParams{FamilleID: c.Famille.ID, Module: module, ObjetID: objet,
		Decalage: depuis, Nombre: PleinsParPage + 1}) // un de plus : y a-t-il une suite ?
	if err != nil || len(l) <= PleinsParPage {
		return l, 0, err
	}
	return l[:PleinsParPage], depuis + PleinsParPage, nil
}

// suitePleins : les pleins plus anciens (bouton en bas de la liste), ajoutés
// à la suite par htmx, avec le bouton de la page d'après s'il en reste.
func (s *Suivi) suitePleins(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	objet, _ := strconv.ParseInt(r.PathValue("objet"), 10, 64)
	o, ok := s.cible(r, c, r.PathValue("module"), objet)
	depuis, err := strconv.ParseInt(r.URL.Query().Get("depuis"), 10, 64)
	if !ok || !o.AvecKm || err != nil || depuis < 0 {
		s.a.Echec(w, r, http.StatusNotFound, "Introuvable.")
		return
	}
	b := &Bloc{Module: o.Module, ObjetID: objet, AvecKm: true, Onglets: true, Devise: c.Famille.Devise}
	v := &VuePleins{}
	b.Pleins = v
	if v.Pleins, v.Suite, err = s.pagePleins(ctx, c, o.Module, objet, depuis); err != nil {
		s.a.ErreurInterne(w, r, err)
		return
	}
	if c.EstParent() {
		if b.Pieces, err = s.piecesParLigne(ctx, c, o.Module, objet); err != nil {
			s.a.ErreurInterne(w, r, err)
			return
		}
	}
	s.a.Fragment(w, r, c, "foyer/fiche", "suite-pleins", map[string]any{"S": b, "J": c.CSRF, "P": c.EstParent()})
}
