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
	"familycocon/internal/budget"
	"familycocon/internal/donnees"
	"familycocon/internal/repetition"
)

const mod = "abonnements"

// Emojis proposés.
var Emojis = []string{"📺", "🎵", "📱", "🌐", "☁️", "🎮", "📰", "🏋️", "🛡️", "💡", "🚗", "📦"}

// Module : les abonnements.
type Module struct{ a *appli.App }

// Brancher ajoute les routes du module.
func Brancher(a *appli.App) {
	m := &Module{a: a}
	a.Route("GET /abonnements", mod, appli.Parent, m.liste)
	a.Route("GET /abonnements/nouveau", mod, appli.Parent, m.formulaire)
	a.Route("POST /abonnements", mod, appli.Parent, m.enregistrer)
	a.Route("GET /abonnements/{id}", mod, appli.Parent, m.formulaire)
	a.Route("POST /abonnements/{id}", mod, appli.Parent, m.enregistrer)
	a.Route("POST /abonnements/{id}/resilier", mod, appli.Parent, m.resilier)
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
	if err := budget.Generer(ctx, m.a, c.Famille.ID, c.Maintenant()); err != nil {
		return nil, err
	}
	l, err := m.a.Q.ListeAbonnements(ctx, c.Famille.ID)
	if err != nil {
		return nil, err
	}
	auj := repetition.CleJour(c.Maintenant())
	var out []Ligne
	for _, x := range l {
		li := Ligne{ListeAbonnementsRow: x, Frequence: budget.NomFrequence(x.Frequence), ParMois: budget.ParMois(x.Montant, x.Frequence),
			Lien: SiteSur(x.Site)}
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
	}
	v.ParAn = v.ParMois * 12
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
		f.A, f.R, f.Lien = ab, rec, SiteSur(ab.Site)
		f.Montant = fmt.Sprintf("%d,%02d", rec.Montant/100, rec.Montant%100)
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

// siteValide : vide, ou une adresse http(s) (« exemple.fr » devient https://exemple.fr).
func siteValide(s string) (string, bool) {
	if s == "" {
		return "", true
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || len(s) > 300 {
		return "", false
	}
	return u.String(), true
}

func (m *Module) enregistrer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	nom := strings.TrimSpace(r.PostFormValue("nom"))
	notes := strings.TrimSpace(r.PostFormValue("notes"))
	montant, errM := budget.LireMontant(r.PostFormValue("montant"))
	freq, prochain, engagement := r.PostFormValue("frequence"), r.PostFormValue("prochain"), r.PostFormValue("engagement_fin")
	_, errP := time.Parse("2006-01-02", prochain)
	_, errE := time.Parse("2006-01-02", engagement)
	site, siteOk := siteValide(strings.TrimSpace(r.PostFormValue("site")))
	cat, catOk := budget.CategorieParCle(r.PostFormValue("categorie"))
	msg := ""
	switch {
	case utf8.RuneCountInString(nom) < 1 || utf8.RuneCountInString(nom) > 80:
		msg = "Le nom doit faire entre 1 et 80 caractères."
	case errM != nil:
		msg = errM.Error()
	case !budget.FrequenceValide(freq) || errP != nil:
		msg = "Indiquez la fréquence et la date du prochain prélèvement."
	case engagement != "" && errE != nil:
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
		// Nouvelle date ou nouvelle fréquence : on repart de la date donnée.
		debut, faites := rec.Debut, rec.Faites
		if j, ok := budget.Prochaine(rec); !ok || repetition.CleJour(j) != prochain || freq != rec.Frequence {
			debut, faites = prochain, 0
		}
		err = m.a.Transaction(ctx, func(q *donnees.Queries) error {
			if err := q.ModifierRecurrence(ctx, donnees.ModifierRecurrenceParams{Sens: "depense", Montant: montant, Categorie: cat.Cle,
				Libelle: nom, MembreID: membre, Frequence: freq, Debut: debut, Fin: "", Faites: faites, ID: rec.ID, FamilleID: c.Famille.ID}); err != nil {
				return err
			}
			return q.ModifierAbonnement(ctx, donnees.ModifierAbonnementParams{Emoji: emoji, EngagementFin: engagement, Site: site,
				Notes: notes, ID: ab.ID, FamilleID: c.Famille.ID})
		})
	}
	if err == nil {
		err = budget.Generer(ctx, m.a, c.Famille.ID, c.Maintenant())
	}
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
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
	m.a.Flash(w, "« "+rec.Libelle+" » résilié : plus de prélèvement prévu")
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
	m.a.Flash(w, "« "+rec.Libelle+" » supprimé")
	m.a.Rediriger(w, r, "/abonnements")
}

// SiteSur : le lien de gestion, seulement en http(s) (vérifié à
// l'enregistrement, revérifié ici).
func SiteSur(s string) template.URL {
	if v, ok := siteValide(s); ok && v != "" {
		return template.URL(v) // #nosec G203 -- schéma vérifié
	}
	return ""
}

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

// echeances : renouvellement des abonnements non mensuels, et fins
// d'engagement (le bon moment pour résilier ou renégocier).
func (m *Module) echeances(ctx context.Context, f donnees.Famille, debut, fin time.Time) ([]appli.Echeance, error) {
	l, err := m.a.Q.ListeAbonnements(ctx, f.ID)
	if err != nil {
		return nil, err
	}
	du, au := repetition.CleJour(debut), repetition.CleJour(fin)
	var out []appli.Echeance
	for _, x := range l {
		rec := recurrence(x)
		j, actif := budget.Prochaine(rec)
		if !actif {
			continue
		}
		if x.Frequence == "annee" || x.Frequence == "trimestre" {
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
