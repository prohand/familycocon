// SPDX-License-Identifier: GPL-3.0-or-later

package suivi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
	"familycocon/internal/graphique"
	txt "familycocon/internal/texte"
)

// Véhicules : le carnet en trois onglets.
//   - « Entretien » : les échéances et le carnet sans les pleins ;
//   - « Pleins » : la liste des pleins (lignes avec des litres), compacte,
//     avec la consommation de chaque plein ;
//   - « Chiffres » : consommation, prix, dépenses et 4 graphiques
//     (consommation, prix du litre, dépenses par mois, kilomètres par mois)
//     sur la période choisie (3, 6, 12 mois, une année, tout, ou des dates).
// Un plein s'ajoute d'un geste, dans une fenêtre ouverte par-dessus la
// fiche (bouton « Ajouter un plein ») : voir AjoutPlein.
// Les montants (prix, dépenses) ne sont montrés qu'aux parents.

// Onglets et périodes (paramètres de l'adresse de la fiche).
const (
	OngletEntretien = ""
	OngletPleins    = "pleins"
	OngletChiffres  = "chiffres"
	Periode12Mois   = ""
	Periode3Mois    = "3m"
	Periode6Mois    = "6m"
	PeriodeTout     = "tout"
	PeriodeDates    = "dates"
)

// PleinsParPage : pleins listés d'un coup (« Voir les pleins plus anciens »
// charge la page suivante).
const PleinsParPage = 100

// VuePleins : l'onglet « Pleins » (la liste).
type VuePleins struct {
	Pleins     []donnees.Carnet // les PleinsParPage derniers
	Suite      int64            // où reprendre la liste (0 : tout est montré)
	Conso      map[int64]string // consommation de chaque plein complet (« 6,1 »)
	Carburants []string         // les types proposés (formulaire « Corriger »)
}

// ChoixPeriode : un bouton de période.
type ChoixPeriode struct {
	Cle, Texte string
	Actif      bool
}

// VueChiffres : l'onglet « Chiffres ».
type VueChiffres struct {
	Periode    string         // clé de la période choisie
	Choix      []ChoixPeriode // 3 mois, 6 mois, 12 mois, années, tout
	Du, Au     string         // la période (AAAA-MM-JJ), pour le choix des dates
	Titre      string         // « sur 12 mois », « en 2025 », « du 01/03/2025 au 30/06/2025 »
	Conso      string         // « 6,4 L / 100 km » sur la période
	PrixLitre  string         // « 1,80 € » en moyenne sur la période
	Litres     string         // litres achetés sur la période
	Carburant  int64          // dépensé en carburant sur la période
	Km         string         // kilomètres parcourus sur la période
	Comparer   string         // « Par rapport à 2024 : carburant −4 %, dépenses +3 % »
	ParAn      bool           // barres par année (période longue)
	AucunPlein bool
	// Graphiques (vides s'il n'y a rien à dessiner).
	GConso, GPrix, GDepenses, GKm template.HTML
}

// AjoutPlein : la fenêtre « Ajouter un plein » (et ce qu'elle propose).
type AjoutPlein struct {
	Carburants       []string
	DernierCarburant string   // proposé pour le suivant
	Stations         []string // les plus fréquentes parmi les derniers pleins
}

// moisCourts : sous les barres.
var moisCourts = []string{"janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."}

// virgule : 6.4 → « 6,4 ».
func virgule(f float64, decimales int) string {
	return strings.Replace(strconv.FormatFloat(f, 'f', decimales, 64), ".", ",", 1)
}

// ChargerAjout : ce que propose la fenêtre « Ajouter un plein ».
func (s *Suivi) ChargerAjout(ctx context.Context, c *appli.Contexte, b *Bloc) error {
	a := &AjoutPlein{Carburants: Carburants}
	b.Ajout = a
	var err error
	a.DernierCarburant, err = s.a.Q.DernierCarburant(ctx, donnees.DernierCarburantParams{FamilleID: c.Famille.ID, Module: b.Module, ObjetID: b.ObjetID})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	notes, err := s.a.Q.NotesPleinsRecents(ctx, donnees.NotesPleinsRecentsParams{FamilleID: c.Famille.ID, Module: b.Module, ObjetID: b.ObjetID})
	if err != nil {
		return err
	}
	a.Stations = stationsFrequentes(notes, 4)
	return nil
}

// stationsFrequentes : les notes courtes (une station) les plus fréquentes,
// la plus récente d'abord à égalité.
func stationsFrequentes(notes []string, n int) []string {
	compte := map[string]int{}
	var ordre []string
	for _, x := range notes {
		x = strings.TrimSpace(x)
		if x == "" || utf8.RuneCountInString(x) > 40 {
			continue
		}
		if compte[x] == 0 {
			ordre = append(ordre, x)
		}
		compte[x]++
	}
	sort.SliceStable(ordre, func(i, j int) bool { return compte[ordre[i]] > compte[ordre[j]] })
	if len(ordre) > n {
		ordre = ordre[:n]
	}
	return ordre
}

// ChargerPleins : remplit l'onglet « Pleins » (la liste).
func (s *Suivi) ChargerPleins(ctx context.Context, c *appli.Contexte, b *Bloc) error {
	v := &VuePleins{Carburants: Carburants}
	b.Pleins = v
	var err error
	if v.Pleins, v.Suite, err = s.pagePleins(ctx, c, b.Module, b.ObjetID, 0); err != nil {
		return err
	}
	v.Conso, err = s.consoParPlein(ctx, c, b.Module, b.ObjetID)
	return err
}

// consoParPlein : la consommation notée à chaque plein complet (voir conso).
func (s *Suivi) consoParPlein(ctx context.Context, c *appli.Contexte, module string, objet int64) (map[int64]string, error) {
	pleins, err := s.a.Q.PleinsPeriodeObjet(ctx, donnees.PleinsPeriodeObjetParams{FamilleID: c.Famille.ID, Module: module, ObjetID: objet,
		Du: "0000-00-00", Au: "9999-99-99"})
	if err != nil {
		return nil, err
	}
	m := map[int64]string{}
	for _, p := range consommations(pleins) {
		m[p.id] = virgule(p.l100, 1)
	}
	return m, nil
}

// consoPlein : la consommation d'un plein complet.
type consoPlein struct {
	id, km   int64
	jour     string
	l100     float64
	litres   float64 // litres mis depuis le plein complet d'avant
	distance float64
}

// consommations : à chaque plein complet, les litres mis depuis le plein
// complet d'avant (pleins partiels compris), sur la distance parcourue.
// Les pleins sont dans l'ordre (du plus ancien au plus récent).
func consommations(pleins []donnees.PleinsPeriodeObjetRow) []consoPlein {
	var l []consoPlein
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
					l = append(l, consoPlein{id: x.ID, km: x.Km, jour: x.Jour, l100: l100, litres: float64(cumul) / 100, distance: d})
				}
			}
		}
		if !partiel || dernierKm == 0 {
			dernierKm, cumul = x.Km, 0
		}
	}
	return l
}

// periode : les dates de la période choisie (debut, fin compris), sa clé
// (corrigée si elle est inconnue) et son titre.
func periode(cle, du, au string, auj time.Time, premier string) (debut, fin time.Time, cleOK, titre string) {
	lieu := auj.Location()
	mois := func(n int) time.Time { return time.Date(auj.Year(), auj.Month()-time.Month(n-1), 1, 0, 0, 0, 0, lieu) }
	fin = time.Date(auj.Year(), auj.Month(), auj.Day(), 0, 0, 0, 0, lieu)
	switch {
	case cle == Periode3Mois:
		return mois(3), fin, cle, "sur 3 mois"
	case cle == Periode6Mois:
		return mois(6), fin, cle, "sur 6 mois"
	case cle == PeriodeTout:
		if t, err := time.ParseInLocation("2006-01-02", premier, lieu); err == nil && t.Before(fin) {
			return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, lieu), fin, cle, "depuis le début"
		}
		return mois(12), fin, cle, "depuis le début"
	case cle == PeriodeDates:
		d, err1 := time.ParseInLocation("2006-01-02", du, lieu)
		f, err2 := time.ParseInLocation("2006-01-02", au, lieu)
		if err1 == nil && err2 == nil && d.Year() >= 1950 && f.Year() <= 2200 {
			if f.Before(d) {
				d, f = f, d
			}
			if f.After(fin) {
				f = fin
			}
			if d.After(f) {
				d = f
			}
			return d, f, cle, "du " + d.Format("02/01/2006") + " au " + f.Format("02/01/2006")
		}
	case len(cle) == 4:
		if a, err := strconv.Atoi(cle); err == nil && a >= 1950 && a <= auj.Year() {
			d := time.Date(a, 1, 1, 0, 0, 0, 0, lieu)
			f := time.Date(a, 12, 31, 0, 0, 0, 0, lieu)
			if f.After(fin) {
				f = fin
			}
			return d, f, cle, "en " + cle
		}
	}
	return mois(12), fin, Periode12Mois, "sur 12 mois"
}

// avant : la période de même longueur juste avant (l'année d'avant pour
// une année) et son nom ; ok faux s'il n'y en a pas (« tout »).
func avant(cle string, debut, fin time.Time) (d, f time.Time, nom string, ok bool) {
	switch {
	case cle == PeriodeTout:
		return d, f, "", false
	case len(cle) == 4:
		a := debut.Year() - 1
		// Année en cours : jusqu'au même jour l'année d'avant.
		return debut.AddDate(-1, 0, 0), fin.AddDate(-1, 0, 0), strconv.Itoa(a) + (map[bool]string{true: " (même période)"})[fin.Month() != 12 || fin.Day() != 31], true
	case cle == PeriodeDates:
		jours := int(fin.Sub(debut).Hours()/24+0.5) + 1
		return debut.AddDate(0, 0, -jours), debut.AddDate(0, 0, -1), "la période d'avant", true
	}
	n := map[string]int{Periode3Mois: 3, Periode6Mois: 6, Periode12Mois: 12}[cle]
	return debut.AddDate(0, -n, 0), debut.AddDate(0, 0, -1), fmt.Sprintf("aux %d mois d'avant", n), true
}

// ecart : « −4 % », « +12 % » (rien si pas de base).
func ecart(nouveau, ancien float64) string {
	if ancien <= 0 {
		return ""
	}
	p := math.Round((nouveau - ancien) / ancien * 100)
	if p == 0 {
		return "="
	}
	if p > 0 {
		return "+" + strconv.Itoa(int(p)) + " %"
	}
	return "−" + strconv.Itoa(int(-p)) + " %"
}

// ChargerChiffres : remplit l'onglet « Chiffres » sur la période choisie.
func (s *Suivi) ChargerChiffres(ctx context.Context, c *appli.Contexte, b *Bloc, cle, du, au string) error {
	v := &VueChiffres{}
	b.Chiffres = v
	parent := c.EstParent()
	auj := c.Maintenant()
	premier, err := s.a.Q.PremierJourObjet(ctx, donnees.PremierJourObjetParams{FamilleID: c.Famille.ID, Module: b.Module, ObjetID: b.ObjetID})
	if err != nil {
		return err
	}
	debut, fin, cle, titre := periode(cle, du, au, auj, premier)
	v.Periode, v.Titre = cle, titre
	v.Du, v.Au = debut.Format("2006-01-02"), fin.Format("2006-01-02")
	v.Choix = []ChoixPeriode{{Cle: Periode3Mois, Texte: "3 mois"}, {Cle: Periode6Mois, Texte: "6 mois"}, {Cle: Periode12Mois, Texte: "12 mois"}}
	if len(premier) >= 4 {
		a0, _ := strconv.Atoi(premier[:4])
		for a := max(a0, auj.Year()-30); a <= auj.Year(); a++ {
			v.Choix = append(v.Choix, ChoixPeriode{Cle: strconv.Itoa(a), Texte: strconv.Itoa(a)})
		}
	}
	v.Choix = append(v.Choix, ChoixPeriode{Cle: PeriodeTout, Texte: "Tout"})
	for i := range v.Choix {
		v.Choix[i].Actif = v.Choix[i].Cle == cle
	}

	pleins, err := s.a.Q.PleinsPeriodeObjet(ctx, donnees.PleinsPeriodeObjetParams{FamilleID: c.Famille.ID, Module: b.Module, ObjetID: b.ObjetID,
		Du: "0000-00-00", Au: v.Au})
	if err != nil {
		return err
	}
	jour := func(j string) time.Time { t, _ := time.ParseInLocation("2006-01-02", j, c.Lieu); return t }
	dateCourte := func(t time.Time) string { return moisCourts[t.Month()-1] + " " + t.Format("06") }
	dans := func(j, d, f string) bool { return j >= d && j <= f }

	// Consommation à chaque plein complet de la période (le plein complet
	// d'avant peut être plus ancien).
	var conso []graphique.Point
	var litresConso, kmConso float64
	for _, p := range consommations(pleins) {
		if dans(p.jour, v.Du, v.Au) {
			conso = append(conso, graphique.Point{Date: jour(p.jour), Valeur: p.l100})
			litresConso += p.litres
			kmConso += p.distance
		}
	}
	if kmConso > 0 {
		v.Conso = virgule(litresConso/kmConso*100, 1) + " L / 100 km"
	}
	v.GConso = graphique.Courbe("Consommation à chaque plein (L / 100 km)", conso, func(f float64) string { return virgule(f, 1) }, dateCourte)

	// Prix du litre et litres achetés.
	totaux := func(d, f string) (litres, montant, litresPayes int64) {
		for _, x := range pleins {
			if !dans(x.Jour, d, f) {
				continue
			}
			litres += x.Litres
			if x.Montant > 0 {
				montant += x.Montant
				litresPayes += x.Litres
			}
		}
		return
	}
	var prix []graphique.Point
	for _, x := range pleins {
		if dans(x.Jour, v.Du, v.Au) && x.Montant > 0 {
			prix = append(prix, graphique.Point{Date: jour(x.Jour), Valeur: float64(x.Montant) / float64(x.Litres)}) // €/L
		}
	}
	litres, montant, litresPayes := totaux(v.Du, v.Au)
	v.AucunPlein = litres == 0
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

	// Par rapport à la période d'avant : litres et dépensé en carburant.
	// Seulement si le carnet couvre toute la période d'avant (sinon l'écart
	// ne veut rien dire : « +900 % » le premier mois).
	if d, f, nom, ok := avant(cle, debut, fin); ok && litres > 0 && premier != "" && premier <= d.Format("2006-01-02") {
		l0, m0, _ := totaux(d.Format("2006-01-02"), f.Format("2006-01-02"))
		var l []string
		if e := ecart(float64(litres), float64(l0)); e != "" {
			l = append(l, "carburant "+e)
		}
		if parent {
			if e := ecart(float64(montant), float64(m0)); e != "" {
				l = append(l, "dépensé "+e)
			}
		}
		if len(l) > 0 {
			if len(cle) == 4 {
				nom = "à " + nom
			}
			v.Comparer = "Par rapport " + nom + " : " + strings.Join(l, ", ")
		}
	}

	// Mois de la période (ou années, au-delà de 3 ans).
	mois, err := s.a.Q.DepensesMoisObjet(ctx, donnees.DepensesMoisObjetParams{FamilleID: c.Famille.ID, Module: b.Module, ObjetID: b.ObjetID, Du: v.Du, Au: v.Au})
	if err != nil {
		return err
	}
	kms, err := s.a.Q.KmMoisObjet(ctx, donnees.KmMoisObjetParams{FamilleID: c.Famille.ID, Module: b.Module, ObjetID: b.ObjetID})
	if err != nil {
		return err
	}
	var cles []string // « 2025-01 »… jusqu'au dernier mois de la période
	for t := time.Date(debut.Year(), debut.Month(), 1, 0, 0, 0, 0, c.Lieu); !t.After(fin); t = t.AddDate(0, 1, 0) {
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
	var precedent int64
	total := 0.0
	for _, k := range kms {
		if precedent > 0 && k.Km > precedent && vu[groupe(k.Mois)] {
			parcourus[groupe(k.Mois)] += float64(k.Km - precedent)
			total += float64(k.Km - precedent)
		}
		precedent = max(precedent, k.Km)
	}
	if total > 0 {
		v.Km = appli.Milliers(int64(total)) + " km"
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
	b := &Bloc{Module: o.Module, ObjetID: objet, AvecKm: true, Onglets: true, Onglet: OngletPleins, Devise: c.Famille.Devise}
	v := &VuePleins{Carburants: Carburants}
	b.Pleins = v
	if v.Pleins, v.Suite, err = s.pagePleins(ctx, c, o.Module, objet, depuis); err == nil {
		v.Conso, err = s.consoParPlein(ctx, c, o.Module, objet)
	}
	if err != nil {
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
