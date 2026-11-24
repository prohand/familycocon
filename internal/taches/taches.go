// SPDX-License-Identifier: GPL-3.0-or-later

// Package taches : tâches ménagères de la famille. Une tâche est confiée à
// un membre (ou à tout le monde), revient selon une règle (une fois, chaque
// jour, certains jours, chaque semaine) ; on la coche quand elle est faite,
// et l'historique dit qui a fait quoi.
//
// Droits : tout le monde crée et coche ; on ne décoche que ce qu'on a coché
// soi-même (un parent peut tout décocher) ; seuls les parents suppriment.
package taches

import (
	"context"
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
	"familycocon/internal/repetition"
	"familycocon/internal/riche"
)

// Module : le module Tâches.
type Module struct {
	a *appli.App
	// duJour : tâches à faire aujourd'hui, gardées le temps d'une requête.
	duJour appli.MemoRequete[[]ligneTache]
}

const mod = "taches"

// Brancher ajoute les routes du module.
func Brancher(a *appli.App) {
	m := &Module{a: a}
	a.Route("GET /taches", mod, appli.Membre, m.page)
	a.Route("GET /taches/contenu", mod, appli.Membre, m.contenu)
	a.Route("GET /taches/nouvelle", mod, appli.Membre, m.formulaire)
	a.Route("POST /taches", mod, appli.Membre, m.creer)
	a.Route("GET /taches/{id}/modifier", mod, appli.Membre, m.formulaire)
	a.Route("POST /taches/{id}", mod, appli.Membre, m.modifier)
	a.Route("POST /taches/{id}/faire", mod, appli.Membre, m.faire)
	a.Route("POST /taches/{id}/defaire", mod, appli.Membre, m.defaire)
	a.Route("POST /taches/{id}/supprimer", mod, appli.Parent, m.supprimer)
	a.Route("GET /taches/categories", mod, appli.Membre, m.pageCategories)
	a.Route("POST /taches/categories", mod, appli.Membre, m.categoriesPost)
	a.AjouterMinute(m.envoyerRappels)
	// Pièces jointes : vues par ceux qui voient la tâche, ajoutées par ceux
	// qui peuvent la modifier.
	a.AjouterJoints(mod, func(ctx context.Context, c *appli.Contexte, id int64) (bool, bool) {
		t, err := a.Q.TacheParID(ctx, donnees.TacheParIDParams{ID: id, FamilleID: c.Famille.ID})
		if err != nil || !m.visible(ctx, c, t) {
			return false, false
		}
		return true, modifiable(c, t)
	})

	a.AjouterResume(mod, m.resume)
	a.AjouterBloc(appli.BlocAccueil{Module: mod, Ordre: 2, Rendre: m.blocAccueil})
	a.AjouterAction(appli.Ajout{Module: mod, Emoji: "✅", Texte: "Une tâche ménagère", URL: "/taches/nouvelle"})
	a.AjouterRecherche(mod, m.chercher)
	a.AjouterCalendrier(mod, m.calendrier)
}

// calendrier : les tâches des jours affichés dans l'agenda :
//   - chaque jour, certains jours : à leurs jours ;
//   - une fois, avec une date de rappel : ce jour-là ;
//   - chaque semaine, quinzaine, mois, avec un rappel : le jour où il part
//     (premier jour de la période).
//
// Les autres (une fois sans date, semaine sans rappel…) n'ont pas de jour
// précis. L'heure du rappel suit le titre (« Poubelles · ⏰ 18:00 »).
func (m *Module) calendrier(ctx context.Context, c *appli.Contexte, debut, fin time.Time) ([]appli.ElementJour, error) {
	depuis := debutDuJour(c)
	taches, err := m.a.Q.TachesActives(ctx, donnees.TachesActivesParams{FamilleID: c.Famille.ID, MembreID: moi(c), Parent: estParent(c), Depuis: depuis})
	if err != nil {
		return nil, err
	}
	membres, err := m.a.Membres(ctx, c)
	if err != nil {
		return nil, err
	}
	parID := appli.MembresParID(membres)
	// Règle et jour de création : calculés une fois par tâche (et pas une
	// fois par jour affiché).
	type tacheDuJour struct {
		t     donnees.Tache
		regle repetition.Regle
		cree  string
	}
	var duJour []tacheDuJour
	for _, t := range taches {
		regle := repetition.Lire(t.Repetition)
		switch {
		case regle.Type == repetition.Jour || regle.Type == repetition.Jours:
		case regle.Type == repetition.Unique && t.DateRappel != "" && t.Rappel != "":
		case regle.Repete() && t.Rappel != "":
		default:
			continue
		}
		duJour = append(duJour, tacheDuJour{t: t, regle: regle, cree: repetition.CleJour(c.Heure(t.CreeLe))})
	}
	if len(duJour) == 0 {
		return nil, nil
	}
	var out []appli.ElementJour
	var faitesCles []string   // occurrences à vérifier (déjà faites ?)
	var cleDe []string        // occurrence de chaque élément de out
	var ids []int64           // tâche de chaque élément de out
	vues := map[string]bool{} // occurrences déjà demandées
	for j := debut; j.Before(fin); j = j.AddDate(0, 0, 1) {
		cle := repetition.CleJour(j)
		for _, x := range duJour {
			occ, aFaire := x.regle.Occurrence(j)
			switch x.regle.Type {
			case repetition.Jour, repetition.Jours:
				// Rien avant le jour où la tâche a été créée.
				if !aFaire || cle < x.cree {
					continue
				}
			case repetition.Unique:
				if cle != x.t.DateRappel {
					continue
				}
			default:
				// Le premier jour de la période seulement.
				if hier, _ := x.regle.Occurrence(j.AddDate(0, 0, -1)); hier == occ || cle < x.cree {
					continue
				}
			}
			e := appli.ElementJour{Jour: cle, Emoji: x.t.Emoji, Titre: x.t.Titre, URL: "/taches", Module: mod}
			if x.t.Rappel != "" {
				e.Titre += " · ⏰ " + x.t.Rappel
			}
			e.Membre = assigneLe(x.t, x.regle, j, parID)
			out = append(out, e)
			ids = append(ids, x.t.ID)
			cleDe = append(cleDe, occ)
			if !vues[occ] {
				vues[occ] = true
				faitesCles = append(faitesCles, occ)
			}
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	// Déjà faites ? (clé de l'occurrence : le jour, la semaine, « unique »…)
	faites, err := m.a.Q.TachesFaitesOccurrences(ctx, donnees.TachesFaitesOccurrencesParams{FamilleID: c.Famille.ID, Occurrences: faitesCles, Depuis: depuis})
	if err != nil {
		return nil, err
	}
	fait := map[string]bool{}
	for _, f := range faites {
		fait[strconv.FormatInt(f.TacheID, 10)+"|"+f.Occurrence] = true
	}
	for i := range out {
		out[i].Faite = fait[strconv.FormatInt(ids[i], 10)+"|"+cleDe[i]]
	}
	return out, nil
}

// emojis : mots-clés → emoji (le premier trouvé l'emporte).
var emojis = []struct{ mot, emoji string }{
	{"poubelle", "🗑️"}, {"vaisselle", "🍽️"}, {"lave-vaisselle", "🍽️"}, {"table", "🍽️"},
	{"linge", "🧺"}, {"lessive", "🧺"}, {"repass", "👕"}, {"aspirateur", "🧹"}, {"balai", "🧹"},
	{"serpill", "🧽"}, {"menage", "🧽"}, {"ménage", "🧽"}, {"salle de bain", "🛁"}, {"toilette", "🚽"},
	{"lit", "🛏️"}, {"chambre", "🧸"}, {"jouet", "🧸"}, {"range", "📦"}, {"courses", "🛒"},
	{"chien", "🐕"}, {"chat", "🐈"}, {"litière", "🐈"}, {"poisson", "🐠"}, {"plante", "🪴"},
	{"arros", "🪴"}, {"jardin", "🌳"}, {"tondre", "🌱"}, {"devoir", "📚"}, {"lecture", "📖"},
	{"lire", "📖"}, {"dent", "🪥"}, {"douche", "🚿"}, {"bain", "🛁"}, {"cuisine", "🍳"},
	{"repas", "🍝"}, {"voiture", "🚗"}, {"vitre", "🪟"}, {"poussière", "🪶"}, {"frigo", "🧊"},
	{"piano", "🎹"}, {"musique", "🎵"}, {"sport", "⚽"}, {"cartable", "🎒"},
}

// DevinerEmoji : un emoji qui va avec le titre (✅ par défaut).
func DevinerEmoji(titre string) string {
	t := strings.ToLower(titre)
	for _, e := range emojis {
		if strings.Contains(t, e.mot) {
			return e.emoji
		}
	}
	return "✅"
}

// ligneTache : une tâche et son état pour la période en cours.
type ligneTache struct {
	donnees.Tache
	Regle     repetition.Regle
	Assigne   *donnees.Membre   // à qui revient la tâche aujourd'hui
	Tour      []*donnees.Membre // tour de rôle (vide : pas de tour)
	Faite     bool
	ParQui    *donnees.Membre
	FaiteLe   string // « 18:42 »
	FaiteJour string // « 2026-11-22 »
	AFaire    bool   // due aujourd'hui (ou cette semaine, ou pas encore faite)
	// AVenir : tâche « une fois » prévue un autre jour (sa date est plus
	// tard) : dans « Les autres jours », pas dans celles du jour (accueil),
	// mais on peut déjà la cocher.
	AVenir     bool
	Modifiable bool
	Decochable bool
	Categorie  string // « 🧹 Ménage » (vide : sans catégorie)
	Rappel     string // « ⏰ 18:00 » (vide : pas de rappel)
	Cachee     bool   // réservée aux parents (la tâche ou sa catégorie)
	Joints     []appli.Joint
	Resume     string // première ligne de la description, en texte simple
}

type score struct {
	Membre donnees.Membre
	Nombre int
}

type ligneHisto struct {
	Titre, Emoji, Heure string
	ParQui              *donnees.Membre
}

type jourHisto struct {
	Jour   string
	Lignes []ligneHisto
}

type vueTaches struct {
	Pour       int64          // filtre par membre (0 : tout le monde)
	Categorie  int64          // filtre par catégorie (0 : toutes ; -1 : sans)
	Cat        string         // la même, pour les adresses (« 3 », « sans », vide)
	Voir       string         // « partagees », « privees », « enfants » ou vide
	Nombres    map[string]int // tâches de chaque filtre « Voir » (clé : sa valeur)
	Categories []Categorie
	Membres    []donnees.Membre
	AFaire     []ligneTache
	Faites     []ligneTache
	PlusTard   []ligneTache // pas aujourd'hui (certains jours)
	Scores     []score
	Histo      []jourHisto
	Formulaire *formTache
}

// tachesMax : tâches en cours par famille ; descriptionMax : caractères
// de la description.
const (
	tachesMax      = 500
	descriptionMax = 2000
)

// debutDuJour : aujourd'hui à minuit, heure de la famille (secondes Unix).
func debutDuJour(c *appli.Contexte) int64 {
	t := c.Maintenant()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, c.Lieu).Unix()
}

// modifiable : un parent modifie tout ; les autres, les tâches qu'ils ont créées.
func modifiable(c *appli.Contexte, t donnees.Tache) bool {
	return appli.PeutModifier(c, t.CreePar.Int64, t.CreePar.Valid)
}

// membresTour : les membres du tour de rôle, dans l'ordre (ceux qui ont
// quitté la famille sont sautés).
func membresTour(t donnees.Tache, parID map[int64]*donnees.Membre) []*donnees.Membre {
	if t.Tour == "" {
		return nil
	}
	var out []*donnees.Membre
	for _, x := range strings.Split(t.Tour, ",") {
		if id, err := strconv.ParseInt(x, 10, 64); err == nil && parID[id] != nil {
			out = append(out, parID[id])
		}
	}
	return out
}

// assigneLe : à qui revient la tâche le jour j (tour de rôle, ou membre
// choisi ; nil : tout le monde).
func assigneLe(t donnees.Tache, regle repetition.Regle, j time.Time, parID map[int64]*donnees.Membre) *donnees.Membre {
	if tour := membresTour(t, parID); len(tour) > 0 {
		n := regle.Numero(j) % int64(len(tour))
		if n < 0 {
			n += int64(len(tour))
		}
		return tour[n]
	}
	if t.AssigneA.Valid {
		return parID[t.AssigneA.Int64]
	}
	return nil
}

// etat : les tâches actives de la famille et leur état aujourd'hui.
func (m *Module) etat(ctx context.Context, c *appli.Contexte) ([]ligneTache, map[int64]*donnees.Membre, []donnees.Membre, error) {
	depuis := debutDuJour(c)
	taches, err := m.a.Q.TachesActives(ctx, donnees.TachesActivesParams{FamilleID: c.Famille.ID, MembreID: moi(c), Parent: estParent(c), Depuis: depuis})
	if err != nil {
		return nil, nil, nil, err
	}
	membres, err := m.a.Membres(ctx, c)
	if err != nil {
		return nil, nil, nil, err
	}
	parID := appli.MembresParID(membres)
	maintenant := c.Maintenant()
	var lignes []ligneTache
	cles := map[string]bool{}
	clesLignes := make([]string, 0, len(taches)) // clé de l'occurrence du jour, par ligne
	for _, t := range taches {
		l := ligneTache{Tache: t, Regle: repetition.Lire(t.Repetition)}
		var cle string
		cle, l.AFaire = l.Regle.Occurrence(maintenant)
		l.AVenir = l.Regle.Type == repetition.Unique && t.DateRappel > repetition.CleJour(maintenant)
		cles[cle] = true
		clesLignes = append(clesLignes, cle)
		l.Assigne, l.Tour = assigneLe(t, l.Regle, maintenant, parID), membresTour(t, parID)
		l.Modifiable = modifiable(c, t)
		l.Rappel = texteRappel(t, maintenant)
		lignes = append(lignes, l)
	}
	if len(cles) > 0 {
		var liste []string
		for k := range cles {
			liste = append(liste, k)
		}
		faites, err := m.a.Q.TachesFaitesOccurrences(ctx, donnees.TachesFaitesOccurrencesParams{FamilleID: c.Famille.ID, Occurrences: liste, Depuis: depuis})
		if err != nil {
			return nil, nil, nil, err
		}
		// Rangées par tâche et occurrence : une recherche directe par ligne.
		type cleFaite struct {
			tache      int64
			occurrence string
		}
		parCle := make(map[cleFaite]donnees.TacheFaite, len(faites))
		for _, f := range faites {
			parCle[cleFaite{f.TacheID, f.Occurrence}] = f
		}
		for i := range lignes {
			f, ok := parCle[cleFaite{lignes[i].ID, clesLignes[i]}]
			if !ok {
				continue
			}
			lignes[i].Faite = true
			lignes[i].FaiteLe = c.Heure(f.FaitLe).Format("15:04")
			lignes[i].FaiteJour = repetition.CleJour(c.Heure(f.FaitLe))
			if f.FaitPar.Valid {
				lignes[i].ParQui = parID[f.FaitPar.Int64]
				lignes[i].Decochable = c.EstParent() || f.FaitPar.Int64 == c.Membre.ID
			} else {
				lignes[i].Decochable = c.EstParent()
			}
		}
	}
	return lignes, parID, membres, nil
}

// pourMembre : la tâche concerne-t-elle ce membre (ou tout le monde) ?
// Une tâche confiée à un membre retiré de la famille revient à tout le monde ;
// une tâche « Parents » concerne chaque parent.
func pourMembre(l ligneTache, id int64, parID map[int64]*donnees.Membre) bool {
	if id == 0 {
		return true
	}
	if l.Assigne == nil && l.PourParents == 1 {
		return parID[id] != nil && parID[id].Role == appli.RoleParent
	}
	return l.Assigne == nil || l.Assigne.ID == id
}

// Filtre « Voir » : toutes, partagées, privées, ou (parents) celles que
// les enfants voient.
const (
	voirPartagees = "partagees"
	voirPrivees   = "privees"
	voirEnfants   = "enfants"
)

// lireVoir : le filtre « voir » (« enfants » : parents seulement).
func lireVoir(v string, c *appli.Contexte) string {
	switch v {
	case voirPartagees, voirPrivees:
		return v
	case voirEnfants:
		if c.EstParent() {
			return v
		}
	}
	return ""
}

// dansVoir : la tâche est-elle dans le filtre « voir » ?
func dansVoir(l ligneTache, voir string, parID map[int64]*donnees.Membre) bool {
	switch voir {
	case voirPartagees:
		return l.Privee == 0
	case voirPrivees:
		return l.Privee == 1
	case voirEnfants:
		return vueParEnfant(l.Tache, l.Cachee, parID)
	}
	return true
}

// Adresse : la liste avec ces filtres (pour les liens des pastilles).
func (v *vueTaches) Adresse(pour int64, cat, voir string) string {
	q := url.Values{}
	q.Set("pour", strconv.FormatInt(pour, 10))
	if cat != "" {
		q.Set("cat", cat)
	}
	if voir != "" {
		q.Set("voir", voir)
	}
	return "/taches?" + q.Encode()
}

// dansCategorie : la tâche est-elle dans ce filtre (0 : toutes ; -1 : sans) ?
func dansCategorie(t donnees.Tache, cat int64) bool {
	switch {
	case cat == 0:
		return true
	case cat < 0:
		return !t.CategorieID.Valid
	}
	return t.CategorieID.Valid && t.CategorieID.Int64 == cat
}

func (m *Module) vue(ctx context.Context, c *appli.Contexte, pour, cat int64, voir string) (*vueTaches, error) {
	lignes, parID, membres, err := m.etat(ctx, c)
	if err != nil {
		return nil, err
	}
	cats, err := m.categories(ctx, c)
	if err != nil {
		return nil, err
	}
	nomCat := map[int64]string{}
	catParents := map[int64]bool{}
	for i := range cats {
		cats[i].Active = cats[i].ID == cat
		nomCat[cats[i].ID] = cats[i].Etiquette()
		catParents[cats[i].ID] = cats[i].Parents
	}
	joints, err := m.a.JointsParObjet(ctx, c, mod)
	if err != nil {
		return nil, err
	}
	v := &vueTaches{Pour: pour, Categorie: cat, Cat: paramCategorie(cat), Voir: voir, Categories: cats, Membres: membres, Nombres: map[string]int{}}
	aujourdhui := repetition.CleJour(c.Maintenant())
	for _, l := range lignes {
		if !pourMembre(l, pour, parID) || !dansCategorie(l.Tache, cat) {
			continue
		}
		l.Categorie = nomCat[l.CategorieID.Int64]
		l.Joints = joints[l.ID]
		l.Resume = resumeDescription(l.Description)
		l.Cachee = l.Privee == 0 && (l.Parents == 1 || catParents[l.CategorieID.Int64])
		// Une tâche « une fois » faite un autre jour n'est plus montrée.
		if l.Faite && !l.Regle.Repete() && l.FaiteJour != aujourdhui {
			continue
		}
		// Nombres du filtre « Voir » (avec les autres filtres).
		for _, x := range []string{"", voirPartagees, voirPrivees, voirEnfants} {
			if dansVoir(l, x, parID) {
				v.Nombres[x]++
			}
		}
		if !dansVoir(l, voir, parID) {
			continue
		}
		switch {
		case l.Faite:
			v.Faites = append(v.Faites, l)
		case l.AFaire && !l.AVenir:
			v.AFaire = append(v.AFaire, l)
		default:
			v.PlusTard = append(v.PlusTard, l)
		}
	}
	// Les autres jours : les tâches « une fois » datées, de la plus proche à
	// la plus lointaine (les autres d'abord).
	sort.SliceStable(v.PlusTard, func(i, j int) bool { return v.PlusTard[i].DateRappel < v.PlusTard[j].DateRappel })
	// Historique : 7 jours ; scores : depuis lundi.
	maintenant := c.Maintenant()
	debut := time.Date(maintenant.Year(), maintenant.Month(), maintenant.Day(), 0, 0, 0, 0, c.Lieu).AddDate(0, 0, -6)
	lundi := repetition.Lundi(maintenant)
	histo, err := m.a.Q.HistoriqueTaches(ctx, donnees.HistoriqueTachesParams{FamilleID: c.Famille.ID, FaitLe: debut.Unix(), MembreID: moi(c), Parent: estParent(c)})
	if err != nil {
		return nil, err
	}
	// Scores comptés en SQL : l'historique affiché, lui, est limité à 100 lignes.
	scores, err := m.a.Q.ScoresTaches(ctx, donnees.ScoresTachesParams{FamilleID: c.Famille.ID, FaitLe: lundi.Unix()})
	if err != nil {
		return nil, err
	}
	compte := map[int64]int{}
	for _, sc := range scores {
		compte[sc.FaitPar.Int64] = int(sc.Nombre)
	}
	for _, h := range histo {
		quand := c.Heure(h.FaitLe)
		jour := jourLisible(quand, maintenant)
		if len(v.Histo) == 0 || v.Histo[len(v.Histo)-1].Jour != jour {
			v.Histo = append(v.Histo, jourHisto{Jour: jour})
		}
		lh := ligneHisto{Titre: h.Titre, Emoji: h.Emoji, Heure: quand.Format("15:04")}
		if h.FaitPar.Valid {
			lh.ParQui = parID[h.FaitPar.Int64]
		}
		v.Histo[len(v.Histo)-1].Lignes = append(v.Histo[len(v.Histo)-1].Lignes, lh)
	}
	for _, mb := range membres {
		if n := compte[mb.ID]; n > 0 {
			v.Scores = append(v.Scores, score{Membre: mb, Nombre: n})
		}
	}
	sort.SliceStable(v.Scores, func(i, j int) bool { return v.Scores[i].Nombre > v.Scores[j].Nombre })
	return v, nil
}

// jourLisible : « Aujourd'hui », « Hier », « Mardi 17 ».
func jourLisible(t, maintenant time.Time) string {
	switch repetition.CleJour(t) {
	case repetition.CleJour(maintenant):
		return "Aujourd'hui"
	case repetition.CleJour(maintenant.AddDate(0, 0, -1)):
		return "Hier"
	}
	return appli.TitreJour(t)
}

func lirePour(r *http.Request) int64 {
	n, _ := strconv.ParseInt(r.URL.Query().Get("pour"), 10, 64)
	return n
}

func (m *Module) page(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	// Le filtre « pour qui » est gardé par l'appareil (comme celui de
	// l'agenda) : on retrouve la vue choisie en revenant.
	pour := lirePour(r)
	if r.URL.Query().Has("pour") {
		m.a.PoserPreference(w, "taches_pour", strconv.FormatInt(pour, 10))
	} else if n, err := strconv.ParseInt(m.a.LirePreference(r, "taches_pour"), 10, 64); err == nil {
		pour = n
	}
	v, err := m.vue(r.Context(), c, pour, lireFiltreCategorie(r.URL.Query().Get("cat")), lireVoir(r.URL.Query().Get("voir"), c))
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Afficher(w, r, c, "taches/liste", "Tâches", mod, v)
}

func (m *Module) renvoyer(w http.ResponseWriter, r *http.Request, c *appli.Contexte, pour, cat int64, voir string) {
	if !appli.EstHtmx(r) {
		http.Redirect(w, r, "/taches", http.StatusSeeOther)
		return
	}
	v, err := m.vue(r.Context(), c, pour, cat, voir)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Fragment(w, r, c, "taches/liste", "taches", v)
}

func (m *Module) contenu(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	m.renvoyer(w, r, c, lirePour(r), lireFiltreCategorie(r.URL.Query().Get("cat")), lireVoir(r.URL.Query().Get("voir"), c))
}

// tache : la tâche {id} de la famille.
func (m *Module) tache(r *http.Request, c *appli.Contexte) (donnees.Tache, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return donnees.Tache{}, sql.ErrNoRows
	}
	t, err := m.a.Q.TacheParID(r.Context(), donnees.TacheParIDParams{ID: id, FamilleID: c.Famille.ID})
	if err == nil && !m.visible(r.Context(), c, t) {
		return donnees.Tache{}, sql.ErrNoRows
	}
	return t, err
}

// formTache : données du formulaire de création / modification.
type formTache struct {
	Tache      donnees.Tache
	Regle      repetition.Regle
	Membres    []donnees.Membre
	Categories []Categorie
	Nouvelle   bool
	DansTour   map[int64]bool // membres du tour de rôle
	Joints     appli.VueJoints
	// PeutPriver : case « Privée » (à la création, puis pour son créateur) ;
	// PeutCacher : case « Parents seulement » (parents).
	PeutPriver, PeutCacher bool
	CacheeParCategorie     bool // déjà cachée aux enfants par sa catégorie
}

func (m *Module) formulaire(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	f := &formTache{Nouvelle: r.PathValue("id") == "", DansTour: map[int64]bool{}, PeutPriver: true, PeutCacher: c.EstParent()}
	if f.Nouvelle {
		// Venue d'un filtre de la liste : la tâche y est rangée.
		if id := lireFiltreCategorie(r.URL.Query().Get("cat")); id > 0 {
			f.Tache.CategorieID = sql.NullInt64{Int64: id, Valid: true}
		}
	} else {
		t, err := m.tache(r, c)
		if err != nil {
			m.a.PageErreur(w, r, c, http.StatusNotFound, "Tâche introuvable", "Cette tâche n'existe plus.")
			return
		}
		if !modifiable(c, t) {
			m.a.PageErreur(w, r, c, http.StatusForbidden, "Réservé aux parents", "Seul un parent (ou qui l'a créée) peut modifier cette tâche.")
			return
		}
		f.Tache, f.Regle = t, repetition.Lire(t.Repetition)
		f.PeutPriver = t.CreePar.Valid && t.CreePar.Int64 == c.Membre.ID
		f.CacheeParCategorie = m.categorieParents(r.Context(), t.CategorieID)
		if f.Joints, err = m.a.Joints(r.Context(), c, mod, t.ID, true, "/taches/"+strconv.FormatInt(t.ID, 10)+"/modifier"); err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
		for _, x := range strings.Split(t.Tour, ",") {
			if id, err := strconv.ParseInt(x, 10, 64); err == nil {
				f.DansTour[id] = true
			}
		}
	}
	var err error
	if f.Membres, err = m.a.Membres(r.Context(), c); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	if f.Categories, err = m.categories(r.Context(), c); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	titre := "Nouvelle tâche"
	if !f.Nouvelle {
		titre = "Modifier la tâche"
	}
	m.a.Afficher(w, r, c, "taches/formulaire", titre, mod, f)
}

// saisieTache : une tâche lue dans le formulaire.
type saisieTache struct {
	titre   string
	assigne sql.NullInt64
	tour    string  // « 3,5,7 » : à tour de rôle
	pour    []int64 // à notifier (membre choisi, ceux du tour, ou les parents)
	// pourParents : « Parents » (l'un ou l'autre la fait).
	pourParents int64
	description string // facultative (texte mis en forme)
	regle       repetition.Regle
	// Catégorie (nulle : aucune) ; rappel : heure (« 18:00 », vide : aucun)
	// et jour d'une tâche « une fois » (vide : chaque jour jusqu'à ce
	// qu'elle soit faite).
	categorie          sql.NullInt64
	rappel, dateRappel string
	privee, parents    int64 // privée ; réservée aux parents
}

// lireFormulaire : titre, membre (ou tour de rôle) et répétition d'une tâche.
func (m *Module) lireFormulaire(r *http.Request, c *appli.Contexte) (s saisieTache, msg string) {
	if err := r.ParseForm(); err != nil {
		return s, "Formulaire invalide."
	}
	s.titre = strings.TrimSpace(r.PostForm.Get("titre"))
	if n := utf8.RuneCountInString(s.titre); n < 1 || n > 80 {
		return s, "Le titre doit faire entre 1 et 80 caractères."
	}
	s.description = strings.TrimSpace(r.PostForm.Get("description"))
	if utf8.RuneCountInString(s.description) > descriptionMax {
		return s, fmt.Sprintf("Description : %d caractères au plus.", descriptionMax)
	}
	membre := func(v string) (int64, bool) {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil || id <= 0 {
			return 0, false
		}
		_, err = m.a.Q.MembreParID(r.Context(), donnees.MembreParIDParams{ID: id, FamilleID: c.Famille.ID})
		return id, err == nil
	}
	switch v := r.PostForm.Get("assigne"); v {
	case "tour":
		// À tour de rôle : deux membres au moins, chacun une fois.
		var ids []string
		vus := map[int64]bool{}
		if len(r.PostForm["tour"]) > 20 {
			return s, "Trop de membres."
		}
		for _, x := range r.PostForm["tour"] {
			id, ok := membre(x)
			if !ok {
				return s, "Membre inconnu."
			}
			if !vus[id] {
				vus[id] = true
				ids = append(ids, strconv.FormatInt(id, 10))
				s.pour = append(s.pour, id)
			}
		}
		if len(ids) < 2 {
			return s, "À tour de rôle : cochez au moins deux personnes."
		}
		s.tour = strings.Join(ids, ",")
	case "parents":
		s.pourParents = 1
		s.pour = m.a.Parents(r.Context(), c)
	case "", "0":
	default:
		id, ok := membre(v)
		if !ok {
			return s, "Membre inconnu."
		}
		s.assigne = sql.NullInt64{Int64: id, Valid: true}
		s.pour = []int64{id}
	}
	var err error
	if s.regle, err = repetition.DepuisFormulaire(r.PostForm); err != nil {
		return s, err.Error()
	}
	if s.rappel, s.dateRappel, msg = lireRappel(r.PostForm.Get("rappel"), r.PostForm.Get("date_rappel"), s.regle); msg != "" {
		return s, msg
	}
	s.categorie = m.lireCategorie(r, c)
	if r.PostForm.Get("privee") == "1" {
		s.privee = 1
	}
	s.parents = lireParents(r, c)
	return s, ""
}

func (m *Module) creer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	s, msg := m.lireFormulaire(r, c)
	if msg != "" {
		m.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	if n, err := m.a.Q.CompterTachesActives(r.Context(), donnees.CompterTachesActivesParams{FamilleID: c.Famille.ID, Depuis: debutDuJour(c)}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	} else if n >= tachesMax {
		m.a.Echec(w, r, http.StatusBadRequest, fmt.Sprintf("%d tâches au plus : demandez à un parent d'en supprimer.", tachesMax))
		return
	}
	if s.privee == 1 {
		// Privée : elle revient à celui qui la crée (personne d'autre ne la voit).
		s.assigne, s.tour, s.pour, s.pourParents = moi(c), "", nil, 0
	}
	t, err := m.a.Q.CreerTache(r.Context(), donnees.CreerTacheParams{
		FamilleID: c.Famille.ID, Titre: s.titre, Emoji: DevinerEmoji(s.titre), AssigneA: s.assigne, Tour: s.tour,
		Repetition: s.regle.String(), CategorieID: s.categorie, Rappel: s.rappel, DateRappel: s.dateRappel,
		Privee: s.privee, Parents: s.parents, PourParents: s.pourParents, Description: s.description, CreePar: sql.NullInt64{Int64: c.Membre.ID, Valid: true}, CreeLe: time.Now().Unix(),
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	// Confiée à quelqu'un (ou à tour de rôle) : seuls eux sont notifiés.
	m.annoncer(r.Context(), c, t, fmt.Sprintf("%s a ajouté la tâche « %s » %s", c.Membre.Nom, t.Titre, t.Emoji), s.pour)
	m.a.Flash(w, "Tâche ajoutée ✓")
	m.a.Rediriger(w, r, "/taches")
}

func (m *Module) modifier(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	t, err := m.tache(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Tâche introuvable.")
		return
	}
	if !modifiable(c, t) {
		m.a.Echec(w, r, http.StatusForbidden, "Seul un parent (ou qui l'a créée) peut la modifier.")
		return
	}
	s, msg := m.lireFormulaire(r, c)
	if msg != "" {
		m.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	// Privée : seul son créateur change ce réglage ; « parents seulement » :
	// un parent. Une tâche privée revient à son créateur.
	if !(t.CreePar.Valid && t.CreePar.Int64 == c.Membre.ID) {
		s.privee = t.Privee
	}
	if !c.EstParent() {
		s.parents = t.Parents
		// Un enfant ne voit pas les catégories des parents : une tâche
		// qu'un parent y a rangée y reste.
		if m.categorieParents(r.Context(), t.CategorieID) {
			s.categorie = t.CategorieID
		}
	}
	if s.privee == 1 {
		s.assigne, s.tour, s.pourParents = t.CreePar, "", 0
	}
	regle := s.regle
	if err := m.a.Q.ModifierTache(r.Context(), donnees.ModifierTacheParams{
		Titre: s.titre, Emoji: DevinerEmoji(s.titre), AssigneA: s.assigne, Tour: s.tour, Repetition: regle.String(),
		CategorieID: s.categorie, Rappel: s.rappel, DateRappel: s.dateRappel, Privee: s.privee, Parents: s.parents,
		PourParents: s.pourParents, Description: s.description, ID: t.ID, FamilleID: c.Famille.ID,
	}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	// Une tâche « une fois » qui devient régulière : sa coche « unique » est
	// oubliée (sinon elle la ferait disparaître le jour où elle redeviendrait
	// « une fois »).
	if regle.Repete() && !repetition.Lire(t.Repetition).Repete() {
		if err := m.a.Q.AnnulerFaite(r.Context(), donnees.AnnulerFaiteParams{TacheID: t.ID, Occurrence: repetition.CleUnique}); err != nil {
			m.a.Journal.Warn("tâches : ancienne coche non effacée", "erreur", err)
		}
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Tâche enregistrée ✓")
	m.a.Rediriger(w, r, "/taches")
}

func (m *Module) faire(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	t, err := m.tache(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Tâche introuvable.")
		return
	}
	cle, aFaire := repetition.Lire(t.Repetition).Occurrence(c.Maintenant())
	if !aFaire {
		m.a.Echec(w, r, http.StatusBadRequest, "Cette tâche n'est pas prévue aujourd'hui.")
		return
	}
	n, err := m.a.Q.MarquerFaite(r.Context(), donnees.MarquerFaiteParams{
		TacheID: t.ID, Occurrence: cle, FaitPar: sql.NullInt64{Int64: c.Membre.ID, Valid: true}, FaitLe: time.Now().Unix(),
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	// Déjà cochée (double clic, deux membres en même temps) : ni annonce ni bravo.
	if n == 1 {
		m.annoncer(r.Context(), c, t, fmt.Sprintf("%s a fait « %s » %s", c.Membre.Nom, t.Titre, t.Emoji), nil)
		appli.Notifier(w, "Bravo "+c.Membre.Nom+" ! 🎉", false)
	}
	// Cochée depuis l'accueil : seule la ligne change.
	if r.PostFormValue("depuis") == "accueil" && appli.EstHtmx(r) {
		m.a.Fragment(w, r, c, "taches/liste", "accueil-tache-faite", t)
		return
	}
	m.renvoyer(w, r, c, lirePourForm(r), lireFiltreCategorie(r.PostFormValue("cat")), lireVoir(r.PostFormValue("voir"), c))
}

func lirePourForm(r *http.Request) int64 {
	n, _ := strconv.ParseInt(r.PostFormValue("pour"), 10, 64)
	return n
}

func (m *Module) defaire(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	t, err := m.tache(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Tâche introuvable.")
		return
	}
	cle, _ := repetition.Lire(t.Repetition).Occurrence(c.Maintenant())
	f, err := m.a.Q.FaiteParOccurrence(ctx, donnees.FaiteParOccurrenceParams{TacheID: t.ID, Occurrence: cle})
	if err != nil {
		m.renvoyer(w, r, c, lirePourForm(r), lireFiltreCategorie(r.PostFormValue("cat")), lireVoir(r.PostFormValue("voir"), c)) // déjà décochée
		return
	}
	if !c.EstParent() && (!f.FaitPar.Valid || f.FaitPar.Int64 != c.Membre.ID) {
		m.a.Echec(w, r, http.StatusForbidden, "Seul un parent peut décocher la tâche de quelqu'un d'autre.")
		return
	}
	if err := m.a.Q.AnnulerFaite(ctx, donnees.AnnulerFaiteParams{TacheID: t.ID, Occurrence: cle}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.renvoyer(w, r, c, lirePourForm(r), lireFiltreCategorie(r.PostFormValue("cat")), lireVoir(r.PostFormValue("voir"), c))
}

func (m *Module) supprimer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	t, err := m.tache(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Tâche introuvable.")
		return
	}
	if err := m.a.Q.RetirerTache(r.Context(), donnees.RetirerTacheParams{ID: t.ID, FamilleID: c.Famille.ID}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Tâche « "+t.Titre+" » supprimée")
	m.a.Rediriger(w, r, "/taches")
}

// aFaireAujourdhui : tâches dues et pas faites. Sur l'accueil, la tuile
// puis le bloc le demandent pour la même requête : calculé une seule fois.
func (m *Module) aFaireAujourdhui(ctx context.Context, c *appli.Contexte) ([]ligneTache, error) {
	return m.duJour.Lire(c, func() ([]ligneTache, error) {
		lignes, _, _, err := m.etat(ctx, c)
		if err != nil {
			return nil, err
		}
		var out []ligneTache
		for _, l := range lignes {
			if l.AFaire && !l.Faite && !l.AVenir {
				out = append(out, l)
			}
		}
		return out, nil
	})
}

func (m *Module) resume(ctx context.Context, c *appli.Contexte) appli.Resume {
	l, err := m.aFaireAujourdhui(ctx, c)
	if err != nil {
		return appli.Resume{}
	}
	if len(l) == 0 {
		return appli.Resume{Info: "Tout est fait 🎉"}
	}
	moi := 0
	for _, t := range l {
		if (t.Assigne != nil && t.Assigne.ID == c.Membre.ID) || (t.Assigne == nil && t.PourParents == 1 && c.EstParent()) {
			moi++
		}
	}
	return appli.Resume{Info: appli.Pluriel(len(l), "à faire", "à faire"), Badge: moi}
}

func (m *Module) blocAccueil(ctx context.Context, c *appli.Contexte) (template.HTML, error) {
	l, err := m.aFaireAujourdhui(ctx, c)
	if err != nil || len(l) == 0 {
		return "", err
	}
	if len(l) > 5 {
		l = l[:5]
	}
	return m.a.RendreBloc("taches/liste", "accueil-taches", c, l)
}

// resumeDescription : la première ligne non vide de la description, sans
// mise en forme (montrée sur une ligne, le reste en touchant).
func resumeDescription(d string) string {
	for _, l := range strings.Split(d, "\n") {
		if t := strings.TrimSpace(riche.LigneSimple(l)); t != "" {
			return t
		}
	}
	return ""
}
