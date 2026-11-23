// SPDX-License-Identifier: GPL-3.0-or-later

// Package abonnements : Netflix, téléphone, box, assurance… Chaque
// abonnement est une opération qui revient dans le budget (paquet budget),
// avec en plus la fin d'engagement, la page de gestion et des notes.
// On voit le coût par mois et par an, et un rappel arrive avant un
// renouvellement annuel ou la fin d'un engagement.
//
// Réservé aux parents, comme le budget.
package abonnements

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"familycocon/internal/appli"
	"familycocon/internal/budget"
	"familycocon/internal/contacts"
	"familycocon/internal/donnees"
	"familycocon/internal/repetition"
)

const mod = "abonnements"

// Emojis proposés.
var Emojis = []string{"📺", "🎵", "📱", "🌐", "☁️", "🎮", "📰", "🏋️", "🛡️", "💡", "🚗", "📦"}

// Module : les abonnements.
type Module struct {
	a *appli.App
	// liste : les abonnements lus une seule fois par requête (sur l'accueil,
	// la tuile et « À venir » demandent la même liste).
	memoListe appli.MemoRequete[[]donnees.ListeAbonnementsRow]
}

// abonnements : la liste de la famille. c : contexte de la requête, nil
// pour les rappels de fond (rien n'est alors gardé).
func (m *Module) abonnements(ctx context.Context, c *appli.Contexte, famille int64) ([]donnees.ListeAbonnementsRow, error) {
	return m.memoListe.Lire(c, func() ([]donnees.ListeAbonnementsRow, error) { return m.a.Q.ListeAbonnements(ctx, famille) })
}

// Brancher ajoute les routes du module.
func Brancher(a *appli.App) {
	m := &Module{a: a}
	a.Route("GET /abonnements", mod, appli.Parent, m.liste)
	a.Route("GET /abonnements/nouveau", mod, appli.Parent, m.formulaire)
	a.Route("POST /abonnements", mod, appli.Parent, m.enregistrer)
	a.Route("GET /abonnements/{id}", mod, appli.Parent, m.formulaire)
	a.Route("POST /abonnements/{id}", mod, appli.Parent, m.enregistrer)
	a.Route("POST /abonnements/{id}/resilier", mod, appli.Parent, m.resilier)
	a.Route("POST /abonnements/{id}/reprendre", mod, appli.Parent, m.reprendre)
	a.Route("POST /abonnements/{id}/supprimer", mod, appli.Parent, m.supprimer)

	a.AjouterResume(mod, m.resume)
	a.AjouterEcheances(mod, m.echeances)
	a.AjouterAction(appli.Ajout{Module: mod, Emoji: "📺", Texte: "Un abonnement", URL: "/abonnements/nouveau"})
}

// ---------- Liste ----------

// Ligne : un abonnement dans la liste.
type Ligne struct {
	donnees.ListeAbonnementsRow
	Frequence  string // « Chaque mois »
	ParMois    int64
	ParAn      int64
	Prochain   string // « Vendredi 5 décembre »
	Engagement string // « Engagé jusqu'au 30 juin 2027 », si l'engagement court encore
	Bientot    bool   // fin d'engagement dans moins de 60 jours
	Resilie    bool
	Lien       template.URL // page de gestion
}

type vueListe struct {
	Actifs, Resilies []Ligne
	ParMois, ParAn   int64
	Devise           string
}

func recurrence(x donnees.ListeAbonnementsRow) donnees.Recurrence {
	return donnees.Recurrence{Montant: x.Montant, Frequence: x.Frequence, Debut: x.Debut, Fin: x.Fin, Faites: x.Faites}
}

func (m *Module) lignes(ctx context.Context, c *appli.Contexte) ([]Ligne, error) {
	// Les prélèvements dus sont d'abord écrits dans le budget.
	if err := budget.GenererSiBesoin(ctx, m.a, c.Famille.ID, c.Maintenant()); err != nil {
		return nil, err
	}
	l, err := m.abonnements(ctx, c, c.Famille.ID)
	if err != nil {
		return nil, err
	}
	auj := repetition.CleJour(c.Maintenant())
	var out []Ligne
	for _, x := range l {
		li := Ligne{ListeAbonnementsRow: x, Frequence: budget.NomFrequence(x.Frequence), ParMois: budget.ParMois(x.Montant, x.Frequence),
			ParAn: budget.ParAn(x.Montant, x.Frequence),
			Lien:  contacts.SiteSur(x.Site)}
		if j, ok := budget.Prochaine(recurrence(x)); ok {
			li.Prochain = appli.DateLongue(j)
		} else {
			li.Resilie = true
		}
		if x.EngagementFin >= auj && !li.Resilie {
			fin, _ := time.Parse("2006-01-02", x.EngagementFin)
			li.Engagement = "Engagé jusqu'au " + strconv.Itoa(fin.Day()) + " " + appli.MoisFR(fin.Month()) + " " + strconv.Itoa(fin.Year())
			li.Bientot = x.EngagementFin <= repetition.CleJour(c.Maintenant().AddDate(0, 0, 60))
		}
		out = append(out, li)
	}
	return out, nil
}

func (m *Module) liste(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	l, err := m.lignes(r.Context(), c)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	v := vueListe{Devise: c.Famille.Devise}
	for _, x := range l {
		if x.Resilie {
			v.Resilies = append(v.Resilies, x)
			continue
		}
		v.Actifs = append(v.Actifs, x)
		v.ParMois += x.ParMois
		v.ParAn += x.ParAn
	}
	// Total du mois tiré du total de l'année : arrondir chaque ligne
	// (division entière) faisait perdre quelques centimes.
	v.ParMois = v.ParAn / 12
	// Les plus chers en haut.
	sort.SliceStable(v.Actifs, func(i, j int) bool { return v.Actifs[i].ParMois > v.Actifs[j].ParMois })
	m.a.Afficher(w, r, c, "abonnements/liste", "Abonnements", mod, v)
}

// ---------- Formulaire ----------

type form struct {
	Nouveau    bool
	A          donnees.Abonnement
	R          donnees.Recurrence
	Montant    string
	Prochain   string
	Resilie    bool
	Lien       template.URL
	Emojis     []string
	Frequences []budget.Frequence
	Categories []budget.Categorie
	Membres    []donnees.Membre
	Devise     string
	Paye       int64 // payé depuis le début (prélèvements écrits au budget)
	NbPaye     int64
}

func (m *Module) abonnement(r *http.Request, c *appli.Contexte) (donnees.Abonnement, donnees.Recurrence, bool) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	ab, err := m.a.Q.AbonnementParID(r.Context(), donnees.AbonnementParIDParams{ID: id, FamilleID: c.Famille.ID})
	if err != nil {
		return ab, donnees.Recurrence{}, false
	}
	rec, err := m.a.Q.RecurrenceParID(r.Context(), donnees.RecurrenceParIDParams{ID: ab.RecurrenceID, FamilleID: c.Famille.ID})
	return ab, rec, err == nil
}

func (m *Module) formulaire(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	f := form{Nouveau: r.PathValue("id") == "", Emojis: Emojis, Frequences: budget.Frequences, Devise: c.Famille.Devise,
		A: donnees.Abonnement{Emoji: "📺"}, R: donnees.Recurrence{Frequence: "mois", Categorie: "abonnements"}}
	for _, cat := range budget.Categories {
		if cat.Sens == "depense" {
			f.Categories = append(f.Categories, cat)
		}
	}
	if !f.Nouveau {
		ab, rec, ok := m.abonnement(r, c)
		if !ok {
			m.a.PageErreur(w, r, c, http.StatusNotFound, "Abonnement introuvable", "Cet abonnement n'existe plus.")
			return
		}
		f.A, f.R, f.Lien = ab, rec, contacts.SiteSur(ab.Site)
		if t, err := m.a.Q.TotalRecurrence(r.Context(), donnees.TotalRecurrenceParams{FamilleID: c.Famille.ID, RecurrenceID: sql.NullInt64{Int64: rec.ID, Valid: true}}); err == nil {
			f.Paye, f.NbPaye = t.Total, t.Nombre
		}
		f.Montant = budget.MontantSaisie(rec.Montant)
		if j, ok := budget.Prochaine(rec); ok {
			f.Prochain = repetition.CleJour(j)
		} else {
			f.Resilie = true
		}
	}
	var err error
	if f.Membres, err = m.a.Membres(r.Context(), c); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	titre := "Nouvel abonnement"
	if !f.Nouveau {
		titre = f.R.Libelle
	}
	m.a.Afficher(w, r, c, "abonnements/formulaire", titre, mod, f)
}

func emojiValide(e string) string {
	for _, x := range Emojis {
		if x == e {
			return e
		}
	}
	return "📺"
}

func (m *Module) enregistrer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	nom := strings.TrimSpace(r.PostFormValue("nom"))
	notes := strings.TrimSpace(r.PostFormValue("notes"))
	montant, errM := budget.LireMontant(r.PostFormValue("montant"))
	freq, prochain, engagement := r.PostFormValue("frequence"), r.PostFormValue("prochain"), r.PostFormValue("engagement_fin")
	_, okP := budget.LireJour(prochain, c.Maintenant())
	_, okE := budget.LireJour(engagement, c.Maintenant())
	site, siteOk := contacts.NormaliserSite(strings.TrimSpace(r.PostFormValue("site")))
	cat, catOk := budget.CategorieParCle(r.PostFormValue("categorie"))
	msg := ""
	switch {
	case utf8.RuneCountInString(nom) < 1 || utf8.RuneCountInString(nom) > 80:
		msg = "Le nom doit faire entre 1 et 80 caractères."
	case errM != nil:
		msg = errM.Error()
	case !budget.FrequenceValide(freq) || !okP:
		msg = "Indiquez la fréquence et la date du prochain prélèvement."
	case engagement != "" && !okE:
		msg = "Date de fin d'engagement invalide."
	case !siteOk:
		msg = "Adresse du site invalide (http ou https)."
	case utf8.RuneCountInString(notes) > 1000:
		msg = "Notes : 1000 caractères au plus."
	}
	if !catOk || cat.Sens != "depense" {
		cat, _ = budget.CategorieParCle("abonnements")
	}
	if msg != "" {
		m.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	var membre sql.NullInt64
	if id, _ := strconv.ParseInt(r.PostFormValue("membre"), 10, 64); id > 0 {
		if _, err := m.a.Q.MembreParID(ctx, donnees.MembreParIDParams{ID: id, FamilleID: c.Famille.ID}); err == nil {
			membre = sql.NullInt64{Int64: id, Valid: true}
		}
	}
	emoji := emojiValide(r.PostFormValue("emoji"))
	auteur := sql.NullInt64{Int64: c.Membre.ID, Valid: true}

	var err error
	if r.PathValue("id") == "" {
		if msg := budget.ValiderProchaine(prochain, nil, c.Maintenant()); msg != "" {
			m.a.Echec(w, r, http.StatusBadRequest, msg)
			return
		}
		if msg, err := budget.PlaceRecurrence(ctx, m.a, c.Famille.ID); err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		} else if msg != "" {
			m.a.Echec(w, r, http.StatusBadRequest, msg)
			return
		}
		err = m.a.Transaction(ctx, func(q *donnees.Queries) error {
			rec, err := q.CreerRecurrence(ctx, donnees.CreerRecurrenceParams{FamilleID: c.Famille.ID, Sens: "depense", Montant: montant,
				Categorie: cat.Cle, Libelle: nom, MembreID: membre, Frequence: freq, Debut: prochain, CreePar: auteur, CreeLe: time.Now().Unix()})
			if err != nil {
				return err
			}
			_, err = q.CreerAbonnement(ctx, donnees.CreerAbonnementParams{FamilleID: c.Famille.ID, RecurrenceID: rec.ID, Emoji: emoji,
				EngagementFin: engagement, Site: site, Notes: notes, CreeLe: time.Now().Unix()})
			return err
		})
	} else {
		ab, rec, ok := m.abonnement(r, c)
		if !ok {
			m.a.Echec(w, r, http.StatusNotFound, "Abonnement introuvable.")
			return
		}
		// Nouvelle date ou nouvelle fréquence : on repart de la date donnée
		// (après la dernière déjà passée : rien n'est écrit deux fois).
		debut, faites, msg := budget.Replanifier(rec, prochain, freq, c.Maintenant())
		if msg != "" {
			m.a.Echec(w, r, http.StatusBadRequest, msg)
			return
		}
		err = m.a.Transaction(ctx, func(q *donnees.Queries) error {
			// Rien n'est écrit si un prélèvement a été ajouté au budget depuis la
			// lecture (il serait écrit une seconde fois).
			n, err := q.ModifierRecurrence(ctx, donnees.ModifierRecurrenceParams{Sens: "depense", Montant: montant, Categorie: cat.Cle,
				Libelle: nom, MembreID: membre, Frequence: freq, Debut: debut, Fin: rec.Fin, Faites: faites, ID: rec.ID, FamilleID: c.Famille.ID,
				FaitesAvant: rec.Faites})
			if err != nil {
				return err
			}
			if n == 0 {
				return errModifiee
			}
			return q.ModifierAbonnement(ctx, donnees.ModifierAbonnementParams{Emoji: emoji, EngagementFin: engagement, Site: site,
				Notes: notes, ID: ab.ID, FamilleID: c.Famille.ID})
		})
		if errors.Is(err, errModifiee) {
			m.a.Echec(w, r, http.StatusConflict, budget.MessageModifiee)
			return
		}
	}
	if err == nil {
		err = budget.Generer(ctx, m.a, c.Famille.ID, c.Maintenant())
	}
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Publier(c, "budget", "")
	m.a.Flash(w, "Abonnement enregistré "+emoji)
	m.a.Rediriger(w, r, "/abonnements")
}

// resilier : plus de prélèvement après aujourd'hui ; l'abonnement reste
// dans la liste des résiliés (et ses dépenses passées dans le budget).
func (m *Module) resilier(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	_, rec, ok := m.abonnement(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Abonnement introuvable.")
		return
	}
	ctx := r.Context()
	if err := budget.Generer(ctx, m.a, c.Famille.ID, c.Maintenant()); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	if err := m.a.Q.ArreterRecurrence(ctx, donnees.ArreterRecurrenceParams{Fin: repetition.CleJour(c.Maintenant()), ID: rec.ID, FamilleID: c.Famille.ID}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Publier(c, "budget", "")
	m.a.Flash(w, "« "+rec.Libelle+" » résilié : plus de prélèvement prévu")
	m.a.Rediriger(w, r, "/abonnements")
}

// reprendre : un abonnement résilié repart (on s'est réabonné) à partir de
// la date donnée ; les prélèvements passés ne sont pas réécrits.
func (m *Module) reprendre(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	_, rec, ok := m.abonnement(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Abonnement introuvable.")
		return
	}
	if _, actif := budget.Prochaine(rec); actif {
		m.a.Echec(w, r, http.StatusBadRequest, "Cet abonnement n'est pas résilié.")
		return
	}
	prochain := r.PostFormValue("prochain")
	if msg := budget.ValiderProchaine(prochain, &rec, c.Maintenant()); msg != "" {
		m.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	ctx := r.Context()
	n, err := m.a.Q.ModifierRecurrence(ctx, donnees.ModifierRecurrenceParams{Sens: rec.Sens, Montant: rec.Montant, Categorie: rec.Categorie,
		Libelle: rec.Libelle, MembreID: rec.MembreID, Frequence: rec.Frequence, Debut: prochain, Fin: "", Faites: 0,
		ID: rec.ID, FamilleID: c.Famille.ID, FaitesAvant: rec.Faites})
	if err == nil && n == 0 {
		m.a.Echec(w, r, http.StatusConflict, budget.MessageModifiee)
		return
	}
	if err == nil {
		err = budget.Generer(ctx, m.a, c.Famille.ID, c.Maintenant())
	}
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Publier(c, "budget", "")
	m.a.Flash(w, "« "+rec.Libelle+" » repris ✓")
	m.a.Rediriger(w, r, "/abonnements")
}

// supprimer : l'abonnement et sa récurrence (les dépenses passées restent).
func (m *Module) supprimer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	_, rec, ok := m.abonnement(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Abonnement introuvable.")
		return
	}
	if err := m.a.Q.SupprimerRecurrence(r.Context(), donnees.SupprimerRecurrenceParams{ID: rec.ID, FamilleID: c.Famille.ID}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Publier(c, "budget", "")
	m.a.Flash(w, "« "+rec.Libelle+" » supprimé")
	m.a.Rediriger(w, r, "/abonnements")
}

// errModifiee : la récurrence a changé entre la lecture et l'écriture.
var errModifiee = errors.New("abonnement modifié entre-temps")

// ---------- Accueil et rappels ----------

func (m *Module) resume(ctx context.Context, c *appli.Contexte) appli.Resume {
	l, err := m.lignes(ctx, c)
	if err != nil {
		return appli.Resume{}
	}
	var n int
	var parMois int64
	for _, x := range l {
		if !x.Resilie {
			n++
			parMois += x.ParMois
		}
	}
	if n == 0 {
		return appli.Resume{Info: "Aucun abonnement"}
	}
	return appli.Resume{Info: appli.Montant(parMois, c.Famille.Devise) + " par mois"}
}

// echeances : renouvellement des abonnements trimestriels, semestriels ou annuels, et fins
// d'engagement (le bon moment pour résilier ou renégocier).
func (m *Module) echeances(ctx context.Context, f donnees.Famille, debut, fin time.Time) ([]appli.Echeance, error) {
	l, err := m.abonnements(ctx, appli.ContexteDe(ctx), f.ID)
	if err != nil {
		return nil, err
	}
	du, au := repetition.CleJour(debut), repetition.CleJour(fin)
	var out []appli.Echeance
	for _, x := range l {
		rec := recurrence(x)
		if _, actif := budget.Prochaine(rec); !actif {
			continue
		}
		// Depuis la dernière occurrence écrite : celle du jour (déjà dans le
		// budget) doit rester dans « À venir » et le rappel du matin.
		if rec.Faites > 0 {
			rec.Faites--
		}
		j, actif := budget.Prochaine(rec)
		if x.Frequence == "annee" || x.Frequence == "semestre" || x.Frequence == "trimestre" {
			// Les occurrences dans la période (souvent une seule).
			for k := 0; k < 5 && repetition.CleJour(j) < au; k++ {
				if cle := repetition.CleJour(j); cle >= du {
					out = append(out, appli.Echeance{Jour: time.Date(j.Year(), j.Month(), j.Day(), 0, 0, 0, 0, debut.Location()),
						Titre: "Renouvellement : " + x.Libelle + " (" + appli.Montant(x.Montant, f.Devise) + ")", Emoji: x.Emoji,
						URL: fmt.Sprintf("/abonnements/%d", x.ID), Module: mod})
				}
				rec.Faites++
				if j, actif = budget.Prochaine(rec); !actif {
					break
				}
			}
		}
		if x.EngagementFin >= du && x.EngagementFin < au {
			if e, err := time.ParseInLocation("2006-01-02", x.EngagementFin, debut.Location()); err == nil {
				out = append(out, appli.Echeance{Jour: e, Titre: "Fin d'engagement : " + x.Libelle, Emoji: x.Emoji,
					URL: fmt.Sprintf("/abonnements/%d", x.ID), Module: mod})
			}
		}
	}
	return out, nil
}
