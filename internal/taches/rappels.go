// SPDX-License-Identifier: GPL-3.0-or-later

package taches

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
	"familycocon/internal/repetition"
)

// Rappel d'une tâche : une heure (« 18:00 », heure de la famille). Chaque
// jour où la tâche revient, si elle n'est pas encore faite, une
// notification part vers celui qui doit la faire (le membre choisi ou celui
// dont c'est le tour ; « tout le monde » : toute la famille, ou les parents
// pour une tâche cachée aux enfants ; privée : son créateur).
//
//   - chaque jour, certains jours : ces jours-là ;
//   - chaque semaine, toutes les 2 semaines, chaque mois : le premier jour
//     de la période (lundi, 1er du mois) ;
//   - une fois : le jour choisi (date_rappel), ou sinon chaque jour jusqu'à
//     ce qu'elle soit faite.
//
// Une seule fois par tâche et par jour (table taches_rappels). Un serveur
// arrêté à l'heure du rappel l'envoie à son retour, s'il revient moins de
// fenetreRappel après.

const (
	formatHeure   = "15:04"
	formatDate    = "2006-01-02"
	fenetreRappel = 15 * time.Minute
)

// lireRappel : l'heure et la date du formulaire (vides : pas de rappel).
// La date ne sert qu'à une tâche « une fois ».
func lireRappel(heure, date string, regle repetition.Regle) (string, string, string) {
	heure, date = strings.TrimSpace(heure), strings.TrimSpace(date)
	if heure == "" {
		return "", "", ""
	}
	if utf8.RuneCountInString(heure) > 5 {
		return "", "", "Heure du rappel illisible."
	}
	h, err := time.Parse(formatHeure, heure)
	if err != nil {
		return "", "", "Heure du rappel illisible (ex. : 18:00)."
	}
	heure = h.Format(formatHeure)
	if regle.Repete() || date == "" {
		return heure, "", ""
	}
	d, err := time.Parse(formatDate, date)
	if err != nil {
		return "", "", "Date du rappel illisible."
	}
	return heure, d.Format(formatDate), ""
}

// texteRappel : « ⏰ 18:00 », « ⏰ 28/11 à 18:00 » (montré sur la ligne).
func texteRappel(t donnees.Tache) string {
	if t.Rappel == "" {
		return ""
	}
	if d, err := time.Parse(formatDate, t.DateRappel); err == nil {
		return "⏰ " + d.Format("02/01") + " à " + t.Rappel
	}
	return "⏰ " + t.Rappel
}

// rappelDu : faut-il rappeler la tâche à cet instant (heure de la famille) ?
// Renvoie la clé de l'occurrence (pour savoir si elle est faite).
func rappelDu(t donnees.TachesAvecRappelRow, maintenant time.Time) (string, bool) {
	h, err := time.Parse(formatHeure, t.Rappel)
	if err != nil {
		return "", false
	}
	cible := time.Date(maintenant.Year(), maintenant.Month(), maintenant.Day(), h.Hour(), h.Minute(), 0, 0, maintenant.Location())
	// Pas avant l'heure, pas trop tard, pas pour une tâche créée après.
	if maintenant.Before(cible) || maintenant.Sub(cible) > fenetreRappel || t.CreeLe > cible.Unix() {
		return "", false
	}
	regle := repetition.Lire(t.Repetition)
	cle, aFaire := regle.Occurrence(maintenant)
	if !aFaire {
		return "", false
	}
	switch regle.Type {
	case repetition.Unique:
		if t.DateRappel != "" && t.DateRappel != repetition.CleJour(maintenant) {
			return "", false
		}
	case repetition.Jour, repetition.Jours:
	default:
		// Semaine, quinzaine, mois : le premier jour de la période.
		if hier, _ := regle.Occurrence(maintenant.AddDate(0, 0, -1)); hier == cle {
			return "", false
		}
	}
	return cle, true
}

// envoyerRappels : chaque minute, les rappels arrivés à leur heure.
func (m *Module) envoyerRappels(ctx context.Context, maintenant time.Time) {
	l, err := m.a.Q.TachesAvecRappel(ctx)
	if err != nil {
		m.a.Journal.Error("rappels des tâches", "erreur", err)
		return
	}
	lieux := map[string]*time.Location{}
	for _, t := range l {
		lieu, ok := lieux[t.Fuseau]
		if !ok {
			if lieu, err = time.LoadLocation(t.Fuseau); err != nil {
				lieu = time.UTC
			}
			lieux[t.Fuseau] = lieu
		}
		ici := maintenant.In(lieu)
		cle, du := rappelDu(t, ici)
		if !du {
			continue
		}
		// Déjà faite ?
		if _, err := m.a.Q.FaiteParOccurrence(ctx, donnees.FaiteParOccurrenceParams{TacheID: t.ID, Occurrence: cle}); err == nil {
			continue
		}
		// Une seule fois par jour (marqué d'abord : jamais deux fois).
		if n, err := m.a.Q.MarquerRappelTache(ctx, donnees.MarquerRappelTacheParams{TacheID: t.ID, Jour: repetition.CleJour(ici), EnvoyeLe: maintenant.Unix()}); err != nil || n == 0 {
			if err != nil {
				m.a.Journal.Error("rappel d'une tâche", "tache", t.ID, "erreur", err)
			}
			continue
		}
		membres, err := m.a.Q.MembresDeFamille(ctx, t.FamilleID)
		if err != nil {
			m.a.Journal.Error("rappel d'une tâche", "tache", t.ID, "erreur", err)
			continue
		}
		tache := donnees.Tache{ID: t.ID, FamilleID: t.FamilleID, AssigneA: t.AssigneA, Tour: t.Tour, Repetition: t.Repetition, CreePar: t.CreePar}
		var pour []int64 // vide : toute la famille
		qui := assigneLe(tache, repetition.Lire(t.Repetition), ici, appli.MembresParID(membres))
		switch {
		case t.Privee == 1:
			// Privée : son créateur seul.
			if !t.CreePar.Valid {
				continue
			}
			pour = []int64{t.CreePar.Int64}
		case qui != nil:
			pour = []int64{qui.ID}
		case t.Cachee == 1:
			// Pour « tout le monde » mais cachée aux enfants : les parents (et
			// son créateur).
			for _, mb := range membres {
				if mb.Role == appli.RoleParent || (t.CreePar.Valid && mb.ID == t.CreePar.Int64) {
					pour = append(pour, mb.ID)
				}
			}
			if len(pour) == 0 {
				continue // surtout pas « toute la famille » (pour vide)
			}
		}
		m.a.Rappeler(ctx, t.FamilleID, mod, "⏰ "+t.Emoji+" "+t.Titre, "/taches", pour)
		m.a.Journal.Info("rappel d'une tâche envoyé", "tache", t.ID)
	}
	// Une fois par jour : les traces de plus de 60 jours sont oubliées.
	if maintenant.Hour() == 3 && maintenant.Minute() == 0 {
		if err := m.a.Q.OublierRappelsTaches(ctx, maintenant.AddDate(0, 0, -60).Unix()); err != nil {
			m.a.Journal.Warn("rappels des tâches : nettoyage", "erreur", err)
		}
	}
}
