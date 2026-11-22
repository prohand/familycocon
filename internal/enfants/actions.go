// SPDX-License-Identifier: GPL-3.0-or-later

package enfants

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
	"familycocon/internal/repetition"
	"familycocon/internal/taches"
)

// renvoyer : après une action, le bloc de la page (htmx) ou la page.
func (m *Module) renvoyer(w http.ResponseWriter, r *http.Request, c *appli.Contexte, enfant int64) {
	if !appli.EstHtmx(r) {
		if c.EstParent() && enfant != 0 {
			http.Redirect(w, r, fmt.Sprintf("/enfants/enfant/%d", enfant), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/enfants", http.StatusSeeOther)
		return
	}
	// La page d'où vient la requête dit quel bloc renvoyer.
	if strings.Contains(r.Header.Get("HX-Current-URL"), "/enfants/enfant/") && enfant != 0 {
		e, err := m.a.Q.MembreParID(r.Context(), donnees.MembreParIDParams{ID: enfant, FamilleID: c.Famille.ID})
		if err == nil {
			v, err := m.vueEnfant(r.Context(), c, e)
			if err != nil {
				m.a.ErreurInterne(w, r, err)
				return
			}
			m.a.Fragment(w, r, c, "enfants/espace", "espace", v)
			return
		}
	}
	m.contenu(w, r, c)
}

func (m *Module) mission(r *http.Request, c *appli.Contexte) (donnees.Mission, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return donnees.Mission{}, sql.ErrNoRows
	}
	return m.a.Q.MissionParID(r.Context(), donnees.MissionParIDParams{ID: id, FamilleID: c.Famille.ID})
}

// faire : l'enfant coche sa mission (elle attend la validation d'un parent).
func (m *Module) faire(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	mi, err := m.mission(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Mission introuvable.")
		return
	}
	if mi.EnfantID != c.Membre.ID {
		m.a.Echec(w, r, http.StatusForbidden, "Ce n'est pas ta mission 🙂")
		return
	}
	cle, due := repetition.Lire(mi.Repetition).Occurrence(c.Maintenant())
	if !due {
		m.a.Echec(w, r, http.StatusBadRequest, "Cette mission n'est pas pour aujourd'hui.")
		return
	}
	if err := m.a.Q.CocherMission(r.Context(), donnees.CocherMissionParams{MissionID: mi.ID, Occurrence: cle, Etoiles: mi.Etoiles, CocheLe: time.Now().Unix()}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, fmt.Sprintf("%s a fini « %s » %s : à valider", c.Membre.Nom, mi.Titre, mi.Emoji))
	appli.Notifier(w, "Super ! Un parent va vérifier 👀", false)
	m.renvoyer(w, r, c, mi.EnfantID)
}

// annuler : l'enfant décoche une mission pas encore validée.
func (m *Module) annuler(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	mi, err := m.mission(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Mission introuvable.")
		return
	}
	if mi.EnfantID != c.Membre.ID && !c.EstParent() {
		m.a.Echec(w, r, http.StatusForbidden, "Ce n'est pas ta mission 🙂")
		return
	}
	cle, _ := repetition.Lire(mi.Repetition).Occurrence(c.Maintenant())
	if err := m.a.Q.AnnulerCoche(r.Context(), donnees.AnnulerCocheParams{MissionID: mi.ID, Occurrence: cle}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.renvoyer(w, r, c, mi.EnfantID)
}

// decider : un parent valide (les étoiles sont gagnées) ou refuse.
func (m *Module) decider(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	f, err := m.a.Q.FaiteMissionParID(ctx, donnees.FaiteMissionParIDParams{ID: id, FamilleID: c.Famille.ID})
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Mission introuvable.")
		return
	}
	etat := map[string]string{"valider": Validee, "refuser": Refusee}[r.PathValue("decision")]
	if etat == "" {
		m.a.Echec(w, r, http.StatusNotFound, "Décision inconnue.")
		return
	}
	n, err := m.a.Q.DeciderMission(ctx, donnees.DeciderMissionParams{
		Etat: etat, DecidePar: sql.NullInt64{Int64: c.Membre.ID, Valid: true}, DecideLe: sql.NullInt64{Int64: time.Now().Unix(), Valid: true}, ID: f.ID,
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	if n > 0 {
		enfant, _ := m.a.Q.MembreParID(ctx, donnees.MembreParIDParams{ID: f.EnfantID, FamilleID: c.Famille.ID})
		if etat == Validee {
			m.a.Publier(c, mod, fmt.Sprintf("Bravo %s ! +%d ⭐ pour « %s »", enfant.Nom, f.Etoiles, f.Titre))
			appli.Notifier(w, fmt.Sprintf("+%d ⭐ pour %s", f.Etoiles, enfant.Nom), false)
		} else {
			m.a.Publier(c, mod, fmt.Sprintf("%s : « %s » est à refaire 💪", enfant.Nom, f.Titre))
			appli.Notifier(w, "Mission à refaire", false)
		}
	}
	m.renvoyer(w, r, c, f.EnfantID)
}

func (m *Module) recompense(r *http.Request, c *appli.Contexte) (donnees.Recompense, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return donnees.Recompense{}, sql.ErrNoRows
	}
	return m.a.Q.RecompenseParID(r.Context(), donnees.RecompenseParIDParams{ID: id, FamilleID: c.Famille.ID})
}

// demander : l'enfant demande une récompense ; ses étoiles sont réservées
// jusqu'à la décision d'un parent.
func (m *Module) demander(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	if c.EstParent() {
		m.a.Echec(w, r, http.StatusBadRequest, "C'est l'enfant qui demande sa récompense.")
		return
	}
	rec, err := m.recompense(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Récompense introuvable.")
		return
	}
	// Vérifié et enregistré dans une transaction : deux demandes en même
	// temps ne dépensent pas deux fois les mêmes étoiles.
	pasAssez := false
	err = m.a.Transaction(ctx, func(q *donnees.Queries) error {
		g, err := q.EtoilesGagnees(ctx, c.Membre.ID)
		if err != nil {
			return err
		}
		e, err := q.EtoilesEchangees(ctx, c.Membre.ID)
		if err != nil {
			return err
		}
		if g-e.Depensees-e.Reservees < rec.Prix {
			pasAssez = true
			return nil
		}
		_, err = q.CreerEchange(ctx, donnees.CreerEchangeParams{RecompenseID: rec.ID, EnfantID: c.Membre.ID, Prix: rec.Prix, DemandeLe: time.Now().Unix()})
		return err
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	if pasAssez {
		m.a.Echec(w, r, http.StatusBadRequest, "Pas encore assez d'étoiles ⭐")
		return
	}
	m.a.Publier(c, mod, fmt.Sprintf("%s voudrait « %s » %s (%d ⭐)", c.Membre.Nom, rec.Titre, rec.Emoji, rec.Prix))
	appli.Notifier(w, "Demande envoyée aux parents 🎁", false)
	m.renvoyer(w, r, c, c.Membre.ID)
}

// deciderEchange : un parent confirme (les étoiles sont dépensées) ou refuse
// (elles sont rendues).
func (m *Module) deciderEchange(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	e, err := m.a.Q.EchangeParID(ctx, donnees.EchangeParIDParams{ID: id, FamilleID: c.Famille.ID})
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Demande introuvable.")
		return
	}
	etat := map[string]string{"confirmer": "confirme", "refuser": "refuse"}[r.PathValue("decision")]
	if etat == "" {
		m.a.Echec(w, r, http.StatusNotFound, "Décision inconnue.")
		return
	}
	n, err := m.a.Q.DeciderEchange(ctx, donnees.DeciderEchangeParams{
		Etat: etat, DecidePar: sql.NullInt64{Int64: c.Membre.ID, Valid: true}, DecideLe: sql.NullInt64{Int64: time.Now().Unix(), Valid: true}, ID: e.ID,
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	if n > 0 {
		enfant, _ := m.a.Q.MembreParID(ctx, donnees.MembreParIDParams{ID: e.EnfantID, FamilleID: c.Famille.ID})
		if etat == "confirme" {
			m.a.Publier(c, mod, fmt.Sprintf("🎉 %s a gagné « %s » %s !", enfant.Nom, e.Titre, e.Emoji))
		} else {
			m.a.Publier(c, mod, fmt.Sprintf("%s : « %s » sera pour une autre fois", enfant.Nom, e.Titre))
		}
	}
	m.renvoyer(w, r, c, e.EnfantID)
}

// Formulaires (parents) : missions et récompenses.

type formMission struct {
	Nouvelle bool
	Mission  donnees.Mission
	Regle    repetition.Regle
	Enfants  []donnees.Membre
}

func (m *Module) enfants(r *http.Request, c *appli.Contexte) ([]donnees.Membre, error) {
	membres, err := m.a.Membres(r.Context(), c)
	var out []donnees.Membre
	for _, mb := range membres {
		if mb.Role == appli.RoleEnfant {
			out = append(out, mb)
		}
	}
	return out, err
}

func (m *Module) formMission(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	f := formMission{Nouvelle: r.PathValue("id") == ""}
	var err error
	if f.Enfants, err = m.enfants(r, c); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	if len(f.Enfants) == 0 {
		m.a.Flash(w, "Ajoutez d'abord un enfant à la famille")
		http.Redirect(w, r, "/famille", http.StatusSeeOther)
		return
	}
	if f.Nouvelle {
		f.Mission = donnees.Mission{Etoiles: 1, Categorie: "menage"}
		f.Mission.EnfantID, _ = strconv.ParseInt(r.URL.Query().Get("enfant"), 10, 64)
		f.Regle = repetition.Regle{Type: repetition.Jour}
	} else {
		if f.Mission, err = m.mission(r, c); err != nil {
			m.a.PageErreur(w, r, c, http.StatusNotFound, "Mission introuvable", "Cette mission n'existe plus.")
			return
		}
		f.Regle = repetition.Lire(f.Mission.Repetition)
	}
	m.a.Afficher(w, r, c, "enfants/mission", map[bool]string{true: "Nouvelle mission", false: "Modifier la mission"}[f.Nouvelle], mod, f)
}

// Categories de missions.
var categoriesMission = map[string]string{"menage": "🧹", "devoirs": "📚", "routine": "🪥", "autre": "⭐"}

type saisieMission struct {
	enfant       int64
	titre, emoji string
	etoiles      int64
	categorie    string
	regle        repetition.Regle
}

func (m *Module) lireMission(r *http.Request, c *appli.Contexte) (saisieMission, string) {
	var s saisieMission
	if err := r.ParseForm(); err != nil {
		return s, "Formulaire invalide."
	}
	f := r.PostForm
	s.enfant, _ = strconv.ParseInt(f.Get("enfant"), 10, 64)
	if mb, err := m.a.Q.MembreParID(r.Context(), donnees.MembreParIDParams{ID: s.enfant, FamilleID: c.Famille.ID}); err != nil || mb.Role != appli.RoleEnfant {
		return s, "Choisissez un enfant."
	}
	s.titre = strings.TrimSpace(f.Get("titre"))
	if n := utf8.RuneCountInString(s.titre); n < 1 || n > 80 {
		return s, "Le titre doit faire entre 1 et 80 caractères."
	}
	s.etoiles, _ = strconv.ParseInt(f.Get("etoiles"), 10, 64)
	if s.etoiles < 1 || s.etoiles > 5 {
		return s, "De 1 à 5 étoiles."
	}
	s.categorie = f.Get("categorie")
	defaut, ok := categoriesMission[s.categorie]
	if !ok {
		return s, "Catégorie inconnue."
	}
	s.emoji = taches.DevinerEmoji(s.titre)
	if s.emoji == "✅" {
		s.emoji = defaut
	}
	var err error
	if s.regle, err = repetition.DepuisFormulaire(f); err != nil {
		return s, err.Error()
	}
	return s, ""
}

func (m *Module) creerMission(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	s, msg := m.lireMission(r, c)
	if msg != "" {
		m.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	mi, err := m.a.Q.CreerMission(r.Context(), donnees.CreerMissionParams{
		FamilleID: c.Famille.ID, EnfantID: s.enfant, Titre: s.titre, Emoji: s.emoji, Etoiles: s.etoiles,
		Categorie: s.categorie, Repetition: s.regle.String(), CreePar: sql.NullInt64{Int64: c.Membre.ID, Valid: true}, CreeLe: time.Now().Unix(),
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, fmt.Sprintf("Nouvelle mission : « %s » %s (%d ⭐)", mi.Titre, mi.Emoji, mi.Etoiles))
	m.a.Flash(w, "Mission ajoutée ⭐")
	m.a.Rediriger(w, r, fmt.Sprintf("/enfants/enfant/%d", s.enfant))
}

func (m *Module) modifierMission(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	mi, err := m.mission(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Mission introuvable.")
		return
	}
	s, msg := m.lireMission(r, c)
	if msg != "" {
		m.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	if err := m.a.Q.ModifierMission(r.Context(), donnees.ModifierMissionParams{
		EnfantID: s.enfant, Titre: s.titre, Emoji: s.emoji, Etoiles: s.etoiles, Categorie: s.categorie,
		Repetition: s.regle.String(), ID: mi.ID, FamilleID: c.Famille.ID,
	}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Mission enregistrée ✓")
	m.a.Rediriger(w, r, fmt.Sprintf("/enfants/enfant/%d", s.enfant))
}

func (m *Module) supprimerMission(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	mi, err := m.mission(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Mission introuvable.")
		return
	}
	if err := m.a.Q.RetirerMission(r.Context(), donnees.RetirerMissionParams{ID: mi.ID, FamilleID: c.Famille.ID}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Mission supprimée (les étoiles gagnées sont gardées)")
	m.a.Rediriger(w, r, fmt.Sprintf("/enfants/enfant/%d", mi.EnfantID))
}

type formRecompense struct {
	Nouvelle   bool
	Recompense donnees.Recompense
	Emojis     []string
}

var emojisRecompense = []string{"🎁", "💶", "🍦", "🎮", "📺", "🎬", "🧸", "📚", "🎨", "⚽", "🛝", "🍕", "🌙", "🎢", "🏊", "🚲"}

func (m *Module) formRecompense(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	f := formRecompense{Nouvelle: r.PathValue("id") == "", Emojis: emojisRecompense}
	if f.Nouvelle {
		f.Recompense = donnees.Recompense{Emoji: "🎁", Prix: 10}
	} else {
		var err error
		if f.Recompense, err = m.recompense(r, c); err != nil {
			m.a.PageErreur(w, r, c, http.StatusNotFound, "Récompense introuvable", "Cette récompense n'existe plus.")
			return
		}
	}
	m.a.Afficher(w, r, c, "enfants/recompense", map[bool]string{true: "Nouvelle récompense", false: "Modifier la récompense"}[f.Nouvelle], mod, f)
}

func lireRecompense(r *http.Request) (titre, emoji string, prix int64, msg string) {
	titre = strings.TrimSpace(r.PostFormValue("titre"))
	if n := utf8.RuneCountInString(titre); n < 1 || n > 60 {
		return "", "", 0, "Le nom doit faire entre 1 et 60 caractères."
	}
	prix, err := strconv.ParseInt(r.PostFormValue("prix"), 10, 64)
	if err != nil || prix < 1 || prix > 9999 {
		return "", "", 0, "Prix : de 1 à 9999 étoiles."
	}
	emoji = r.PostFormValue("emoji")
	ok := false
	for _, e := range emojisRecompense {
		ok = ok || e == emoji
	}
	if !ok {
		emoji = "🎁"
	}
	return titre, emoji, prix, ""
}

func (m *Module) creerRecompense(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	titre, emoji, prix, msg := lireRecompense(r)
	if msg != "" {
		m.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	if _, err := m.a.Q.CreerRecompense(r.Context(), donnees.CreerRecompenseParams{FamilleID: c.Famille.ID, Titre: titre, Emoji: emoji, Prix: prix, CreeLe: time.Now().Unix()}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, fmt.Sprintf("Nouvelle récompense : « %s » %s (%d ⭐)", titre, emoji, prix))
	m.a.Flash(w, "Récompense ajoutée 🎁")
	m.a.Rediriger(w, r, "/enfants")
}

func (m *Module) modifierRecompense(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	rec, err := m.recompense(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Récompense introuvable.")
		return
	}
	titre, emoji, prix, msg := lireRecompense(r)
	if msg != "" {
		m.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	if err := m.a.Q.ModifierRecompense(r.Context(), donnees.ModifierRecompenseParams{Titre: titre, Emoji: emoji, Prix: prix, ID: rec.ID, FamilleID: c.Famille.ID}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Récompense enregistrée ✓")
	m.a.Rediriger(w, r, "/enfants")
}

func (m *Module) supprimerRecompense(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	rec, err := m.recompense(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Récompense introuvable.")
		return
	}
	if err := m.a.Q.RetirerRecompense(r.Context(), donnees.RetirerRecompenseParams{ID: rec.ID, FamilleID: c.Famille.ID}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Récompense retirée")
	m.a.Rediriger(w, r, "/enfants")
}
