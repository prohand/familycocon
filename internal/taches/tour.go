// SPDX-License-Identifier: GPL-3.0-or-later

package taches

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
	"familycocon/internal/repetition"
)

// Tour de rôle et « ⏭️ Passer au suivant ».
//
// L'occurrence n° n (Regle.Numero) revient au membre n° n modulo leur
// nombre. Passer son tour ajoute une ligne (table taches_tours_passes) au
// numéro de l'occurrence du jour : à partir de ce numéro, tout est décalé
// d'un cran. La tâche du jour revient donc au suivant, et la suite
// continue à partir de lui. Les jours d'avant gardent leur membre (agenda).

// decalages : par tâche, les numéros d'occurrence où le tour a été passé
// (dans l'ordre croissant).
type decalages map[int64][]int64

// avant : le décalage de l'occurrence numero (passes de numéro <= au sien).
func (d decalages) avant(tache, numero int64) int64 {
	var n int64
	for _, x := range d[tache] {
		if x <= numero {
			n++
		}
	}
	return n
}

// decalagesFamille : les tours passés des tâches en cours de la famille.
func (m *Module) decalagesFamille(ctx context.Context, famille int64) (decalages, error) {
	l, err := m.a.Q.ToursPassesFamille(ctx, famille)
	if err != nil {
		return nil, err
	}
	d := decalages{}
	for _, x := range l {
		d[x.TacheID] = append(d[x.TacheID], x.Numero)
	}
	return d, nil
}

// rang : la place dans le tour (0 à n-1) de l'occurrence numero.
func rang(numero, decalage int64, n int) int {
	k := (numero + decalage) % int64(n)
	if k < 0 {
		k += int64(n)
	}
	return int(k)
}

// passer : « ⏭️ Passer au suivant » : celui dont c'est le tour (ou un
// parent) donne la tâche du jour au membre suivant du tour.
func (m *Module) passer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	t, err := m.tache(r, c)
	if err != nil {
		m.a.Echec(w, r, http.StatusNotFound, "Tâche introuvable.")
		return
	}
	membres, err := m.a.Membres(ctx, c)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	tour := membresTour(t, appli.MembresParID(membres))
	if len(tour) < 2 {
		m.a.Echec(w, r, http.StatusBadRequest, "Cette tâche n'est pas à tour de rôle.")
		return
	}
	maintenant := c.Maintenant()
	regle := repetition.Lire(t.Repetition)
	cle, aFaire := regle.Occurrence(maintenant)
	if !aFaire || (regle.Type == repetition.Unique && t.DateRappel > repetition.CleJour(maintenant)) {
		m.a.Echec(w, r, http.StatusBadRequest, "Cette tâche n'est pas prévue aujourd'hui.")
		return
	}
	if _, err := m.a.Q.FaiteParOccurrence(ctx, donnees.FaiteParOccurrenceParams{TacheID: t.ID, Occurrence: cle}); err == nil {
		m.a.Echec(w, r, http.StatusBadRequest, "Cette tâche est déjà faite.")
		return
	}
	passes, err := m.a.Q.ToursPassesTache(ctx, t.ID)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	numero := regle.Numero(maintenant)
	dec := decalages{t.ID: passes}.avant(t.ID, numero)
	actuel := tour[rang(numero, dec, len(tour))]
	suivant := tour[rang(numero, dec+1, len(tour))]
	// La page montrait un autre membre : le tour vient d'être passé (double
	// envoi, ou un autre l'a passé entre-temps).
	if de, err := strconv.ParseInt(r.PostFormValue("de"), 10, 64); err == nil && de != actuel.ID {
		m.a.Echec(w, r, http.StatusConflict, "Le tour vient déjà d'être passé à "+actuel.Nom+".")
		return
	}
	if !c.EstParent() && actuel.ID != c.Membre.ID {
		m.a.Echec(w, r, http.StatusForbidden, "Seul celui dont c'est le tour (ou un parent) peut le passer.")
		return
	}
	n, err := m.a.Q.PasserTour(ctx, donnees.PasserTourParams{
		Numero: numero, DeMembre: sql.NullInt64{Int64: actuel.ID, Valid: true}, AMembre: sql.NullInt64{Int64: suivant.ID, Valid: true},
		PassePar: sql.NullInt64{Int64: c.Membre.ID, Valid: true}, PasseLe: time.Now().Unix(),
		TacheID: t.ID, FamilleID: c.Famille.ID, Deja: int64(len(passes)),
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	if n == 0 {
		// Passé par un autre au même moment : rien n'est fait deux fois.
		m.a.Echec(w, r, http.StatusConflict, "Le tour vient déjà d'être passé.")
		return
	}
	// Annonce : à celui qui la reçoit (et à celui dont c'était le tour, si
	// c'est un parent qui l'a passé).
	texte := fmt.Sprintf("%s passe son tour : « %s » %s, c'est pour %s", actuel.Nom, t.Titre, t.Emoji, suivant.Nom)
	if actuel.ID != c.Membre.ID {
		texte = fmt.Sprintf("%s a passé le tour de %s : « %s » %s, c'est pour %s", c.Membre.Nom, actuel.Nom, t.Titre, t.Emoji, suivant.Nom)
	}
	m.annoncer(ctx, c, t, texte, []int64{suivant.ID, actuel.ID})
	appli.Notifier(w, "C'est au tour de "+suivant.Nom+" ⏭️", false)
	m.renvoyer(w, r, c, lirePourForm(r), lireFiltreCategorie(r.PostFormValue("cat")), lireVoir(r.PostFormValue("voir"), c))
}

// histoPasses : les tours passés récents, en lignes d'historique.
func (m *Module) histoPasses(ctx context.Context, c *appli.Contexte, depuis int64, parID map[int64]*donnees.Membre) ([]ligneHisto, []int64, error) {
	l, err := m.a.Q.HistoriqueToursPasses(ctx, donnees.HistoriqueToursPassesParams{FamilleID: c.Famille.ID, Depuis: depuis, MembreID: moi(c), Parent: estParent(c)})
	if err != nil {
		return nil, nil, err
	}
	var lignes []ligneHisto
	var quand []int64
	for _, p := range l {
		lh := ligneHisto{Titre: p.Titre, Emoji: p.Emoji, Heure: c.Heure(p.PasseLe).Format("15:04"), Passe: true}
		if p.PassePar.Valid {
			lh.ParQui = parID[p.PassePar.Int64]
		}
		if p.DeMembre.Valid {
			lh.De = parID[p.DeMembre.Int64]
		}
		if p.AMembre.Valid {
			lh.A = parID[p.AMembre.Int64]
		}
		lignes = append(lignes, lh)
		quand = append(quand, p.PasseLe)
	}
	return lignes, quand, nil
}
