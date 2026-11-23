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
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
	"familycocon/internal/repetition"
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

	a.AjouterResume(mod, m.resume)
	a.AjouterBloc(appli.BlocAccueil{Module: mod, Ordre: 2, Rendre: m.blocAccueil})
	a.AjouterAction(appli.Ajout{Module: mod, Emoji: "✅", Texte: "Une tâche ménagère", URL: "/taches/nouvelle"})
	a.AjouterRecherche(mod, m.chercher)
	a.AjouterCalendrier(mod, m.calendrier)
}

// calendrier : les tâches des jours affichés dans l'agenda. Seules les tâches
// rattachées à un jour (chaque jour, certains jours) y sont : « une fois »
// et « chaque semaine » n'ont pas de jour précis.
func (m *Module) calendrier(ctx context.Context, c *appli.Contexte, debut, fin time.Time) ([]appli.ElementJour, error) {
	depuis := debutDuJour(c)
	taches, err := m.a.Q.TachesActives(ctx, donnees.TachesActivesParams{FamilleID: c.Famille.ID, Depuis: depuis})
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
		if regle := repetition.Lire(t.Repetition); regle.Type == repetition.Jour || regle.Type == repetition.Jours {
			duJour = append(duJour, tacheDuJour{t: t, regle: regle, cree: repetition.CleJour(c.Heure(t.CreeLe))})
		}
	}
	if len(duJour) == 0 {
		return nil, nil
	}
	var out []appli.ElementJour
	var ids []int64 // tâche de chaque élément de out
	var cles []string
	for j := debut; j.Before(fin); j = j.AddDate(0, 0, 1) {
		cle := repetition.CleJour(j)
		cles = append(cles, cle)
		for _, x := range duJour {
			// Rien avant le jour où la tâche a été créée.
			if _, aFaire := x.regle.Occurrence(j); !aFaire || cle < x.cree {
				continue
			}
			e := appli.ElementJour{Jour: cle, Emoji: x.t.Emoji, Titre: x.t.Titre, URL: "/taches", Module: mod}
			e.Membre = assigneLe(x.t, x.regle, j, parID)
			out = append(out, e)
			ids = append(ids, x.t.ID)
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	// Déjà faites ? (la clé d'une occurrence quotidienne est le jour).
	faites, err := m.a.Q.TachesFaitesOccurrences(ctx, donnees.TachesFaitesOccurrencesParams{FamilleID: c.Famille.ID, Occurrences: cles, Depuis: depuis})
	if err != nil {
		return nil, err
	}
	fait := map[string]bool{}
	for _, f := range faites {
		fait[strconv.FormatInt(f.TacheID, 10)+"|"+f.Occurrence] = true
	}
	for i := range out {
		out[i].Faite = fait[strconv.FormatInt(ids[i], 10)+"|"+out[i].Jour]
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
	Regle      repetition.Regle
	Assigne    *donnees.Membre   // à qui revient la tâche aujourd'hui
	Tour       []*donnees.Membre // tour de rôle (vide : pas de tour)
	Faite      bool
	ParQui     *donnees.Membre
	FaiteLe    string // « 18:42 »
	FaiteJour  string // « 2026-11-22 »
	AFaire     bool   // due aujourd'hui (ou cette semaine, ou pas encore faite)
	Modifiable bool
	Decochable bool
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
	Pour       int64 // filtre par membre (0 : tout le monde)
	Membres    []donnees.Membre
	AFaire     []ligneTache
	Faites     []ligneTache
	PlusTard   []ligneTache // pas aujourd'hui (certains jours)
	Scores     []score
	Histo      []jourHisto
	Formulaire *formTache
}

// tachesMax : tâches en cours par famille.
const tachesMax = 500

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
	taches, err := m.a.Q.TachesActives(ctx, donnees.TachesActivesParams{FamilleID: c.Famille.ID, Depuis: depuis})
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
		cles[cle] = true
		clesLignes = append(clesLignes, cle)
		l.Assigne, l.Tour = assigneLe(t, l.Regle, maintenant, parID), membresTour(t, parID)
		l.Modifiable = modifiable(c, t)
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
// Une tâche confiée à un membre retiré de la famille revient à tout le monde.
func pourMembre(l ligneTache, id int64) bool {
	return id == 0 || l.Assigne == nil || l.Assigne.ID == id
}

func (m *Module) vue(ctx context.Context, c *appli.Contexte, pour int64) (*vueTaches, error) {
	lignes, parID, membres, err := m.etat(ctx, c)
	if err != nil {
		return nil, err
	}
	v := &vueTaches{Pour: pour, Membres: membres}
	aujourdhui := repetition.CleJour(c.Maintenant())
	for _, l := range lignes {
		if !pourMembre(l, pour) {
			continue
		}
		switch {
		case l.Faite:
			// Une tâche « une fois » faite un autre jour n'est plus montrée.
			if l.Regle.Repete() || l.FaiteJour == aujourdhui {
				v.Faites = append(v.Faites, l)
			}
		case l.AFaire:
			v.AFaire = append(v.AFaire, l)
		default:
			v.PlusTard = append(v.PlusTard, l)
		}
	}
	// Historique : 7 jours ; scores : depuis lundi.
	maintenant := c.Maintenant()
	debut := time.Date(maintenant.Year(), maintenant.Month(), maintenant.Day(), 0, 0, 0, 0, c.Lieu).AddDate(0, 0, -6)
	lundi := repetition.Lundi(maintenant)
	histo, err := m.a.Q.HistoriqueTaches(ctx, donnees.HistoriqueTachesParams{FamilleID: c.Famille.ID, FaitLe: debut.Unix()})
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
	v, err := m.vue(r.Context(), c, lirePour(r))
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Afficher(w, r, c, "taches/liste", "Tâches", mod, v)
}

func (m *Module) renvoyer(w http.ResponseWriter, r *http.Request, c *appli.Contexte, pour int64) {
	if !appli.EstHtmx(r) {
		http.Redirect(w, r, "/taches", http.StatusSeeOther)
		return
	}
	v, err := m.vue(r.Context(), c, pour)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Fragment(w, r, c, "taches/liste", "taches", v)
}

func (m *Module) contenu(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	m.renvoyer(w, r, c, lirePour(r))
}

// tache : la tâche {id} de la famille.
func (m *Module) tache(r *http.Request, c *appli.Contexte) (donnees.Tache, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return donnees.Tache{}, sql.ErrNoRows
	}
	return m.a.Q.TacheParID(r.Context(), donnees.TacheParIDParams{ID: id, FamilleID: c.Famille.ID})
}

// formTache : données du formulaire de création / modification.
type formTache struct {
	Tache    donnees.Tache
	Regle    repetition.Regle
	Membres  []donnees.Membre
	Nouvelle bool
	DansTour map[int64]bool // membres du tour de rôle
}

func (m *Module) formulaire(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	f := &formTache{Nouvelle: r.PathValue("id") == "", DansTour: map[int64]bool{}}
	if !f.Nouvelle {
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
	pour    []int64 // à notifier (membre choisi ou ceux du tour)
	regle   repetition.Regle
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
	t, err := m.a.Q.CreerTache(r.Context(), donnees.CreerTacheParams{
		FamilleID: c.Famille.ID, Titre: s.titre, Emoji: DevinerEmoji(s.titre), AssigneA: s.assigne, Tour: s.tour,
		Repetition: s.regle.String(), CreePar: sql.NullInt64{Int64: c.Membre.ID, Valid: true}, CreeLe: time.Now().Unix(),
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	// Confiée à quelqu'un (ou à tour de rôle) : seuls eux sont notifiés.
	m.a.PublierPour(c, mod, fmt.Sprintf("%s a ajouté la tâche « %s » %s", c.Membre.Nom, t.Titre, t.Emoji), s.pour)
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
	regle := s.regle
	if err := m.a.Q.ModifierTache(r.Context(), donnees.ModifierTacheParams{
		Titre: s.titre, Emoji: DevinerEmoji(s.titre), AssigneA: s.assigne, Tour: s.tour, Repetition: regle.String(), ID: t.ID, FamilleID: c.Famille.ID,
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
		m.a.Publier(c, mod, fmt.Sprintf("%s a fait « %s » %s", c.Membre.Nom, t.Titre, t.Emoji))
		appli.Notifier(w, "Bravo "+c.Membre.Nom+" ! 🎉", false)
	}
	// Cochée depuis l'accueil : seule la ligne change.
	if r.PostFormValue("depuis") == "accueil" && appli.EstHtmx(r) {
		m.a.Fragment(w, r, c, "taches/liste", "accueil-tache-faite", t)
		return
	}
	m.renvoyer(w, r, c, lirePourForm(r))
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
		m.renvoyer(w, r, c, lirePourForm(r)) // déjà décochée
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
	m.renvoyer(w, r, c, lirePourForm(r))
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
			if l.AFaire && !l.Faite {
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
		if t.Assigne != nil && t.Assigne.ID == c.Membre.ID {
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
