// SPDX-License-Identifier: GPL-3.0-or-later

// Package enfants : l'espace enfants. Un parent donne des missions à chaque
// enfant (ménage, devoirs, routines), qui valent de 1 à 5 étoiles et peuvent
// revenir (chaque jour, certains jours, chaque semaine). L'enfant coche ; un
// parent valide (les étoiles sont gagnées) ou refuse (l'enfant peut
// recommencer). Les étoiles s'échangent contre des récompenses créées par
// les parents ; un parent confirme l'échange.
package enfants

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
	"familycocon/internal/repetition"
)

// Module : l'espace enfants.
type Module struct{ a *appli.App }

const mod = "enfants"

// Brancher ajoute les routes du module.
func Brancher(a *appli.App) {
	m := &Module{a: a}
	a.Route("GET /enfants", mod, appli.Membre, m.page)
	a.Route("GET /enfants/contenu", mod, appli.Membre, m.contenu)
	a.Route("GET /enfants/enfant/{id}", mod, appli.Parent, m.pageEnfant)
	a.Route("GET /enfants/enfant/{id}/contenu", mod, appli.Parent, m.contenuEnfant)

	a.Route("GET /enfants/missions/nouvelle", mod, appli.Parent, m.formMission)
	a.Route("POST /enfants/missions", mod, appli.Parent, m.creerMission)
	a.Route("GET /enfants/missions/{id}", mod, appli.Parent, m.formMission)
	a.Route("POST /enfants/missions/{id}", mod, appli.Parent, m.modifierMission)
	a.Route("POST /enfants/missions/{id}/supprimer", mod, appli.Parent, m.supprimerMission)
	a.Route("POST /enfants/missions/{id}/faire", mod, appli.Membre, m.faire)
	a.Route("POST /enfants/missions/{id}/annuler", mod, appli.Membre, m.annuler)
	a.Route("POST /enfants/validations/{id}/{decision}", mod, appli.Parent, m.decider)

	a.Route("GET /enfants/recompenses/nouvelle", mod, appli.Parent, m.formRecompense)
	a.Route("POST /enfants/recompenses", mod, appli.Parent, m.creerRecompense)
	a.Route("GET /enfants/recompenses/{id}", mod, appli.Parent, m.formRecompense)
	a.Route("POST /enfants/recompenses/{id}", mod, appli.Parent, m.modifierRecompense)
	a.Route("POST /enfants/recompenses/{id}/supprimer", mod, appli.Parent, m.supprimerRecompense)
	a.Route("POST /enfants/recompenses/{id}/demander", mod, appli.Membre, m.demander)
	a.Route("POST /enfants/echanges/{id}/{decision}", mod, appli.Parent, m.deciderEchange)

	a.AjouterResume(mod, m.resume)
	a.AjouterAction(appli.Ajout{Module: mod, Emoji: "⭐", Texte: "Une mission pour un enfant", URL: "/enfants/missions/nouvelle"})
}

// États d'une mission pour la période en cours.
const (
	AFaire        = "afaire"
	Attente       = "attente"
	Validee       = "validee"
	Refusee       = "refusee"
	PasAujourdhui = "pasaujourdhui"
)

// CarteMission : une mission et son état aujourd'hui.
type CarteMission struct {
	donnees.Mission
	Regle        repetition.Regle
	Etat         string
	FaiteID      int64 // pour valider directement depuis l'espace de l'enfant
	JusteValidee bool  // validée il y a moins de 2 minutes : petite fête
}

// Etoiles : « ⭐⭐⭐ ».
func (c CarteMission) Etoiles() string { return strings.Repeat("⭐", int(c.Mission.Etoiles)) }

// missionsDuJour : missions actives de la famille et leur état.
func (m *Module) missionsDuJour(ctx context.Context, c *appli.Contexte) ([]CarteMission, error) {
	missions, err := m.a.Q.MissionsActives(ctx, c.Famille.ID)
	if err != nil {
		return nil, err
	}
	maintenant := c.Maintenant()
	var cartes []CarteMission
	cles := map[string]bool{}
	for _, mi := range missions {
		ca := CarteMission{Mission: mi, Regle: repetition.Lire(mi.Repetition), Etat: PasAujourdhui}
		cle, due := ca.Regle.Occurrence(maintenant)
		if due {
			ca.Etat = AFaire
		}
		cles[cle] = true
		cartes = append(cartes, ca)
	}
	if len(cartes) == 0 {
		return nil, nil
	}
	var liste []string
	for k := range cles {
		liste = append(liste, k)
	}
	faites, err := m.a.Q.MissionsFaitesOccurrences(ctx, donnees.MissionsFaitesOccurrencesParams{FamilleID: c.Famille.ID, Occurrences: liste})
	if err != nil {
		return nil, err
	}
	// Rangées par mission et occurrence : une recherche directe par carte.
	type cleFaite struct {
		mission    int64
		occurrence string
	}
	parCle := make(map[cleFaite]donnees.MissionFaite, len(faites))
	for _, f := range faites {
		parCle[cleFaite{f.MissionID, f.Occurrence}] = f
	}
	for i := range cartes {
		cle, _ := cartes[i].Regle.Occurrence(maintenant)
		f, ok := parCle[cleFaite{cartes[i].ID, cle}]
		if !ok {
			continue
		}
		// Une mission « une fois » validée un autre jour ne revient pas : elle
		// est archivée (les étoiles restent), et ne sera plus relue à chaque page.
		if !cartes[i].Regle.Repete() && f.Etat == Validee && f.DecideLe.Valid &&
			repetition.CleJour(c.Heure(f.DecideLe.Int64)) != repetition.CleJour(maintenant) {
			cartes[i].Etat = "finie"
			if err := m.a.Q.RetirerMission(ctx, donnees.RetirerMissionParams{ID: cartes[i].ID, FamilleID: c.Famille.ID}); err != nil {
				m.a.Journal.Error("enfants : archivage d'une mission finie", "erreur", err)
			}
			continue
		}
		cartes[i].Etat = f.Etat
		cartes[i].FaiteID = f.ID
		cartes[i].JusteValidee = f.Etat == Validee && f.DecideLe.Valid && time.Now().Unix()-f.DecideLe.Int64 < 120
	}
	return cartes, nil
}

// Solde : étoiles d'un enfant.
type Solde struct {
	Gagnees    int64
	Depensees  int64
	Reservees  int64 // demandes d'échange en attente
	Disponible int64
}

func (m *Module) solde(ctx context.Context, enfant int64) (Solde, error) {
	g, err := m.a.Q.EtoilesGagnees(ctx, enfant)
	if err != nil {
		return Solde{}, err
	}
	e, err := m.a.Q.EtoilesEchangees(ctx, enfant)
	if err != nil {
		return Solde{}, err
	}
	return Solde{Gagnees: g, Depensees: e.Depensees, Reservees: e.Reservees, Disponible: g - e.Depensees - e.Reservees}, nil
}

// carteRecompense : une récompense vue par un enfant.
type carteRecompense struct {
	donnees.Recompense
	Possible  bool
	Manque    int64
	EnAttente bool
}

type semaine struct {
	Titre    string
	Etoiles  int64
	Missions int
}

// vueEnfant : l'espace d'un enfant (vu par lui ou par un parent).
type vueEnfant struct {
	Enfant      donnees.Membre
	Solde       Solde
	Missions    []CarteMission
	Ailleurs    []CarteMission // pas aujourd'hui
	Recompenses []carteRecompense
	Semaines    []semaine
	MaxSemaine  int64
	Echanges    []donnees.EchangesEnfantRow
	Recents     []donnees.MissionsValideesDepuisRow
	ParParent   bool // un parent regarde
	Faites      int
	Total       int
}

func (m *Module) vueEnfant(ctx context.Context, c *appli.Contexte, enfant donnees.Membre) (*vueEnfant, error) {
	v := &vueEnfant{Enfant: enfant, ParParent: c.EstParent()}
	var err error
	if v.Solde, err = m.solde(ctx, enfant.ID); err != nil {
		return nil, err
	}
	cartes, err := m.missionsDuJour(ctx, c)
	if err != nil {
		return nil, err
	}
	for _, ca := range cartes {
		if ca.EnfantID != enfant.ID || ca.Etat == "finie" {
			continue
		}
		if ca.Etat == PasAujourdhui {
			v.Ailleurs = append(v.Ailleurs, ca)
			continue
		}
		v.Missions = append(v.Missions, ca)
		v.Total++
		if ca.Etat == Validee || ca.Etat == Attente {
			v.Faites++
		}
	}
	if v.Echanges, err = m.a.Q.EchangesEnfant(ctx, enfant.ID); err != nil {
		return nil, err
	}
	enAttente := map[int64]bool{}
	for _, e := range v.Echanges {
		if e.Etat == "attente" {
			enAttente[e.RecompenseID] = true
		}
	}
	recs, err := m.a.Q.RecompensesActives(ctx, c.Famille.ID)
	if err != nil {
		return nil, err
	}
	for _, r := range recs {
		cr := carteRecompense{Recompense: r, Possible: v.Solde.Disponible >= r.Prix, EnAttente: enAttente[r.ID]}
		if !cr.Possible {
			cr.Manque = r.Prix - v.Solde.Disponible
		}
		v.Recompenses = append(v.Recompenses, cr)
	}
	// Progrès : les 4 dernières semaines (la plus récente d'abord).
	lundi := repetition.Lundi(c.Maintenant())
	debut := lundi.AddDate(0, 0, -21)
	valides, err := m.a.Q.MissionsValideesDepuis(ctx, donnees.MissionsValideesDepuisParams{EnfantID: enfant.ID, DecideLe: sql.NullInt64{Int64: debut.Unix(), Valid: true}})
	if err != nil {
		return nil, err
	}
	for k := 0; k < 4; k++ {
		d := lundi.AddDate(0, 0, -7*k)
		s := semaine{Titre: fmt.Sprintf("Semaine du %d %s", d.Day(), appli.MoisFR(d.Month()))}
		if k == 0 {
			s.Titre = "Cette semaine"
		}
		for _, mv := range valides {
			t := c.Heure(mv.DecideLe.Int64)
			if !t.Before(d) && t.Before(d.AddDate(0, 0, 7)) {
				s.Etoiles += mv.Etoiles
				s.Missions++
			}
		}
		v.MaxSemaine = max(v.MaxSemaine, s.Etoiles)
		v.Semaines = append(v.Semaines, s)
	}
	if len(valides) > 8 {
		valides = valides[:8]
	}
	v.Recents = valides
	return v, nil
}

// lignesAValider : missions cochées et demandes de récompense, pour les parents.
type ligneAValider struct {
	ID      int64
	Emoji   string
	Titre   string
	Etoiles int64
	Enfant  *donnees.Membre
	Echange bool
}

type resumeEnfant struct {
	Enfant donnees.Membre
	Solde  Solde
	Faites int
	Total  int
}

type vueParent struct {
	AValider    []ligneAValider
	Enfants     []resumeEnfant
	Recompenses []donnees.Recompense
}

func (m *Module) vueParent(ctx context.Context, c *appli.Contexte) (*vueParent, error) {
	v := &vueParent{}
	membres, err := m.a.Membres(ctx, c)
	if err != nil {
		return nil, err
	}
	parID := map[int64]*donnees.Membre{}
	for i := range membres {
		parID[membres[i].ID] = &membres[i]
	}
	missions, err := m.a.Q.MissionsEnAttente(ctx, c.Famille.ID)
	if err != nil {
		return nil, err
	}
	for _, mi := range missions {
		v.AValider = append(v.AValider, ligneAValider{ID: mi.ID, Emoji: mi.Emoji, Titre: mi.Titre, Etoiles: mi.Etoiles, Enfant: parID[mi.EnfantID]})
	}
	echanges, err := m.a.Q.EchangesEnAttente(ctx, c.Famille.ID)
	if err != nil {
		return nil, err
	}
	for _, e := range echanges {
		v.AValider = append(v.AValider, ligneAValider{ID: e.ID, Emoji: e.Emoji, Titre: e.Titre, Etoiles: e.Prix, Enfant: parID[e.EnfantID], Echange: true})
	}
	cartes, err := m.missionsDuJour(ctx, c)
	if err != nil {
		return nil, err
	}
	for _, mb := range membres {
		if mb.Role != appli.RoleEnfant {
			continue
		}
		r := resumeEnfant{Enfant: mb}
		if r.Solde, err = m.solde(ctx, mb.ID); err != nil {
			return nil, err
		}
		for _, ca := range cartes {
			if ca.EnfantID == mb.ID && ca.Etat != PasAujourdhui && ca.Etat != "finie" {
				r.Total++
				// Comme sur la page de l'enfant : faites = validées ou à valider.
				if ca.Etat == Validee || ca.Etat == Attente {
					r.Faites++
				}
			}
		}
		v.Enfants = append(v.Enfants, r)
	}
	v.Recompenses, err = m.a.Q.RecompensesActives(ctx, c.Famille.ID)
	return v, err
}

func (m *Module) page(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	if c.EstParent() {
		v, err := m.vueParent(ctx, c)
		if err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
		m.a.Afficher(w, r, c, "enfants/parent", "Espace enfants", mod, v)
		return
	}
	v, err := m.vueEnfant(ctx, c, c.Membre)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Afficher(w, r, c, "enfants/espace", "Mon espace", mod, v)
}

func (m *Module) contenu(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	if c.EstParent() {
		v, err := m.vueParent(ctx, c)
		if err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
		m.a.Fragment(w, r, c, "enfants/parent", "parent", v)
		return
	}
	v, err := m.vueEnfant(ctx, c, c.Membre)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Fragment(w, r, c, "enfants/espace", "espace", v)
}

// enfantCible : l'enfant {id} de la famille.
func (m *Module) enfantCible(r *http.Request, c *appli.Contexte) (donnees.Membre, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return donnees.Membre{}, false
	}
	mb, err := m.a.Q.MembreParID(r.Context(), donnees.MembreParIDParams{ID: id, FamilleID: c.Famille.ID})
	return mb, err == nil && mb.Role == appli.RoleEnfant
}

func (m *Module) pageEnfant(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	e, ok := m.enfantCible(r, c)
	if !ok {
		m.a.PageErreur(w, r, c, http.StatusNotFound, "Enfant introuvable", "Cet enfant n'existe pas.")
		return
	}
	v, err := m.vueEnfant(r.Context(), c, e)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Afficher(w, r, c, "enfants/espace", "Espace de "+e.Nom, mod, v)
}

func (m *Module) contenuEnfant(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	e, ok := m.enfantCible(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Enfant introuvable.")
		return
	}
	v, err := m.vueEnfant(r.Context(), c, e)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Fragment(w, r, c, "enfants/espace", "espace", v)
}

func (m *Module) resume(ctx context.Context, c *appli.Contexte) appli.Resume {
	if c.EstParent() {
		mi, err1 := m.a.Q.MissionsEnAttente(ctx, c.Famille.ID)
		ec, err2 := m.a.Q.EchangesEnAttente(ctx, c.Famille.ID)
		if err1 != nil || err2 != nil {
			return appli.Resume{}
		}
		if n := len(mi) + len(ec); n > 0 {
			return appli.Resume{Info: appli.Pluriel(n, "à valider", "à valider"), Badge: n}
		}
		return appli.Resume{Info: "Rien à valider"}
	}
	s, err := m.solde(ctx, c.Membre.ID)
	if err != nil {
		return appli.Resume{}
	}
	cartes, _ := m.missionsDuJour(ctx, c)
	reste := 0
	for _, ca := range cartes {
		if ca.EnfantID == c.Membre.ID && (ca.Etat == AFaire || ca.Etat == Refusee) {
			reste++
		}
	}
	return appli.Resume{Info: fmt.Sprintf("⭐ %d", s.Disponible) + " " + map[bool]string{true: "étoile", false: "étoiles"}[s.Disponible <= 1], Badge: reste}
}
