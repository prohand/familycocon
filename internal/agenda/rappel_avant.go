// SPDX-License-Identifier: GPL-3.0-or-later

package agenda

import (
	"context"
	"strconv"
	"strings"
	"time"

	"familycocon/internal/donnees"
	"familycocon/internal/repetition"
)

// Rappel juste avant un événement (colonne evenements.rappel_avant) : en
// plus du rappel du matin (« Me le rappeler », à 9 h le jour même et 7 jours
// avant), une notification 15 min avant, 1 h avant ou la veille à 18 h
// (heure de la famille). Événement sur toute la journée : la veille à 18 h
// seulement.
//
// La tâche de chaque minute (AjouterMinute) envoie les rappels arrivés à
// leur heure, une seule fois par fois de l'événement (table
// evenements_rappels, clé : l'événement et le début de cette fois) : rien
// n'est renvoyé après un redémarrage. Un serveur arrêté à l'heure du rappel
// l'envoie à son retour s'il revient moins de rattrapageAvant après, et
// jamais une fois l'événement commencé. Les fois annulées n'ont pas de
// rappel ; une fois modifiée a le sien, à sa nouvelle heure.
//
// Destinataires : les mêmes que le rappel du matin (toute la famille, ceux
// qui ont gardé les notifications « Rappels »).

// Valeurs de rappel_avant.
const (
	rappel15min  = "15min"
	rappel1h     = "1h"
	rappelVeille = "veille"
)

// ChoixRappelAvant : la liste du formulaire.
var ChoixRappelAvant = []ChoixRepetition{
	{"", "Pas de rappel juste avant"}, {rappel15min, "15 min avant"},
	{rappel1h, "1 h avant"}, {rappelVeille, "La veille à 18 h"},
}

// heureVeille : l'heure du rappel « la veille » (heure de la famille).
const heureVeille = 18

// rattrapageAvant : retard au-delà duquel un rappel manqué (serveur arrêté)
// n'est plus envoyé.
const rattrapageAvant = 30 * time.Minute

// moduleRappels : les notifications « Rappels » (comme celles du matin).
const moduleRappels = "rappels"

func rappelAvantValide(v string) bool {
	for _, c := range ChoixRappelAvant {
		if c.Cle == v {
			return true
		}
	}
	return false
}

// heureRappelAvant : l'instant du rappel de cette fois (faux : aucun).
// Journée entière : toujours la veille à 18 h. time.Date garde 18 h
// d'horloge même un jour de changement d'heure ; « 1 h avant » est une vraie
// heure avant (même la nuit du changement).
func heureRappelAvant(f fois, choix string, lieu *time.Location) (time.Time, bool) {
	if choix == "" {
		return time.Time{}, false
	}
	d := f.debut.In(lieu)
	if f.evt.Journee == 1 || choix == rappelVeille {
		return time.Date(d.Year(), d.Month(), d.Day()-1, heureVeille, 0, 0, 0, lieu), true
	}
	if choix == rappel15min {
		return d.Add(-15 * time.Minute), true
	}
	return d.Add(-time.Hour), true
}

// envoiAvant : un rappel juste avant prêt à partir.
type envoiAvant struct {
	famille int64
	evt     int64
	texte   string
	url     string
}

// texteRappelAvant : « ⏰ Dans 15 min : Judo à 17 h 00 (📍 Gymnase) »,
// « ⏰ Demain : Anniversaire de Jules ».
func texteRappelAvant(f fois, maintenant time.Time) string {
	var quand string
	reste := max(f.debut.Sub(maintenant).Round(time.Minute), time.Minute)
	switch {
	case reste < time.Hour:
		quand = "Dans " + strconv.Itoa(int(reste.Minutes())) + " min"
	case reste > time.Hour && repetition.CleJour(f.debut) != repetition.CleJour(maintenant):
		quand = "Demain" // la veille à 18 h
	case reste%time.Hour == 0:
		quand = "Dans " + strconv.Itoa(int(reste.Hours())) + " h"
	default:
		quand = "Dans " + strconv.Itoa(int(reste.Hours())) + " h " + strconv.Itoa(int((reste % time.Hour).Minutes()))
	}
	t := "⏰ " + quand + " : " + f.evt.Titre
	if f.evt.Journee == 0 {
		t += " à " + heureLongue(f.debut)
	}
	if f.evt.Lieu != "" {
		t += " (📍 " + f.evt.Lieu + ")"
	}
	return t
}

// rappelsAvantDus : les rappels juste avant arrivés à leur heure, pour
// toutes les familles. Chacun est noté en base avant d'être rendu : jamais
// deux fois.
func (m *Module) rappelsAvantDus(ctx context.Context, maintenant time.Time) []envoiAvant {
	familles, err := m.a.Q.FamillesRappelAvant(ctx)
	if err != nil {
		m.a.Journal.Error("agenda : rappels juste avant", "erreur", err)
		return nil
	}
	var out []envoiAvant
	for _, fam := range familles {
		if !strings.Contains(","+fam.Modules+",", ","+mod+",") {
			continue // agenda désactivé
		}
		lieu, err := time.LoadLocation(fam.Fuseau)
		if err != nil {
			lieu = time.UTC
		}
		ici := maintenant.In(lieu)
		// Les fois qui commencent dans les 2 jours (la veille à 18 h d'un
		// événement du lendemain soir : un peu plus de 24 h avant).
		debut, fin := ici, ici.Add(48*time.Hour)
		evts, err := m.a.Q.EvenementsRappelAvant(ctx, donnees.EvenementsRappelAvantParams{FamilleID: fam.ID,
			FinPeriode: fin.Unix(), DebutPeriode: debut.Unix(), JourMin: repetition.CleJour(debut.AddDate(-1, 0, -1))})
		if err != nil {
			m.a.Journal.Error("agenda : rappels juste avant", "famille", fam.ID, "erreur", err)
			continue
		}
		avecRappel := func(e donnees.Evenement) bool { return e.RappelAvant != "" }
		lf, err := m.lesFois(ctx, fam.ID, lieu, debut, fin, evts, avecRappel)
		if err != nil {
			m.a.Journal.Error("agenda : rappels juste avant", "famille", fam.ID, "erreur", err)
			continue
		}
		for _, f := range lf {
			quand, ok := heureRappelAvant(f, f.evt.RappelAvant, lieu)
			// Pas avant l'heure, pas trop tard, jamais l'événement commencé,
			// ni pour un événement créé après l'heure du rappel.
			if !ok || ici.Before(quand) || ici.Sub(quand) > rattrapageAvant || !ici.Before(f.debut) || f.evt.CreeLe > quand.Unix() {
				continue
			}
			n, err := m.a.Q.MarquerRappelAvant(ctx, donnees.MarquerRappelAvantParams{EvenementID: f.evt.ID, Debut: f.debut.Unix(), EnvoyeLe: maintenant.Unix()})
			if err != nil {
				m.a.Journal.Error("agenda : rappel juste avant non noté", "evenement", f.evt.ID, "erreur", err)
				continue // plutôt aucun rappel que le même à chaque minute
			}
			if n == 0 {
				continue // déjà envoyé
			}
			o := Occurrence{Evt: f.evt, Debut: f.debut, Jour: f.jour}
			out = append(out, envoiAvant{famille: fam.ID, evt: f.evt.ID, texte: texteRappelAvant(f, ici), url: o.Lien()})
		}
	}
	return out
}

// envoyerRappelsAvant : chaque minute, les rappels juste avant arrivés à
// leur heure (même aux membres qui ont l'appli ouverte).
func (m *Module) envoyerRappelsAvant(ctx context.Context, maintenant time.Time) {
	for _, e := range m.rappelsAvantDus(ctx, maintenant) {
		m.a.Rappeler(ctx, e.famille, moduleRappels, e.texte, e.url, nil)
		m.a.Journal.Info("rappel d'un événement envoyé", "evenement", e.evt)
	}
	// Une fois par jour : les traces de plus de 60 jours sont oubliées.
	if maintenant.Hour() == 3 && maintenant.Minute() == 0 {
		if err := m.a.Q.OublierRappelsAvant(ctx, maintenant.AddDate(0, 0, -60).Unix()); err != nil {
			m.a.Journal.Warn("agenda : nettoyage des rappels", "erreur", err)
		}
	}
}
