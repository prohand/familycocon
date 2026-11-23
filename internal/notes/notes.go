// SPDX-License-Identifier: GPL-3.0-or-later

// Package notes : notes de la famille, partagées ou privées.
//
// Droits :
//   - une note partagée est vue par toute la famille ; une note privée par
//     son auteur seulement (pas même par un parent) ;
//   - on modifie ses notes ; un parent modifie aussi les notes partagées ;
//   - seuls les parents suppriment (une note partagée, ou leur note privée).
package notes

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
)

// Module : le module Notes.
type Module struct{ a *appli.App }

const mod = "notes"

// Brancher ajoute les routes du module.
func Brancher(a *appli.App) {
	m := &Module{a: a}
	a.Route("GET /notes", mod, appli.Membre, m.page)
	a.Route("GET /notes/contenu", mod, appli.Membre, m.contenu)
	a.Route("GET /notes/nouvelle", mod, appli.Membre, m.formulaire)
	a.Route("POST /notes", mod, appli.Membre, m.creer)
	a.Route("GET /notes/{id}", mod, appli.Membre, m.formulaire)
	a.Route("POST /notes/{id}", mod, appli.Membre, m.modifier)
	a.Route("POST /notes/{id}/epingler", mod, appli.Membre, m.epingler)
	a.Route("POST /notes/{id}/cocher", mod, appli.Membre, m.cocher)
	a.Route("POST /notes/{id}/supprimer", mod, appli.Membre, m.supprimer)

	a.AjouterResume(mod, m.resume)
	a.AjouterRecherche(mod, m.chercher)
	a.AjouterAction(appli.Ajout{Module: mod, Emoji: "📝", Texte: "Une note", URL: "/notes/nouvelle"})
	a.AjouterPartage(appli.Partage{Module: mod, Emoji: "📝", Texte: "En note", URL: "/notes/nouvelle"})
}

// Filtres de la liste.
const (
	toutes    = ""
	partagees = "partagees"
	privees   = "privees"
)

type carteNote struct {
	donnees.Note
	Auteur     *donnees.Membre
	Date       string
	Modifiable bool
}

type vueNotes struct {
	Filtre    string
	Recherche string
	Params    string // filtre et recherche, encodés pour une adresse (« voir=…&q=… »)
	Epinglees []carteNote
	Autres    []carteNote
	Tronquee  bool // la liste est coupée (notesListe)
}

// Bornes : notes par famille, notes montrées dans la liste, et longueur du
// début de texte montré sur chaque carte.
const (
	notesMax   = 5000
	notesListe = 300
	debutMax   = 300
)

func lireFiltre(r *http.Request) (filtre, recherche string) {
	filtre = r.URL.Query().Get("voir")
	if filtre != partagees && filtre != privees {
		filtre = toutes
	}
	recherche = strings.TrimSpace(r.URL.Query().Get("q"))
	if utf8.RuneCountInString(recherche) > 60 {
		recherche = string([]rune(recherche)[:60])
	}
	return filtre, recherche
}

// modifiable : l'auteur, ou un parent pour une note partagée.
func modifiable(c *appli.Contexte, n donnees.Note) bool {
	auteur := n.AuteurID.Valid && n.AuteurID.Int64 == c.Membre.ID
	return auteur || (c.EstParent() && n.Privee == 0)
}

// supprimable : un parent supprime les notes partagées et les siennes ;
// chacun (même un enfant) supprime ses propres notes privées, que personne
// d'autre ne voit.
func supprimable(c *appli.Contexte, n donnees.Note) bool {
	auteur := n.AuteurID.Valid && n.AuteurID.Int64 == c.Membre.ID
	if n.Privee == 1 {
		return auteur
	}
	return c.EstParent()
}

// visible : une note privée n'est vue que par son auteur.
func visible(c *appli.Contexte, n donnees.Note) bool {
	return n.Privee == 0 || (n.AuteurID.Valid && n.AuteurID.Int64 == c.Membre.ID)
}

// paramsListe : « voir=privees&q=pain+%26+beurre », pour les adresses de
// la liste (les attributs hx-* ne sont pas encodés par le gabarit).
func paramsListe(filtre, recherche string) string {
	v := url.Values{}
	if filtre != toutes {
		v.Set("voir", filtre)
	}
	if recherche != "" {
		v.Set("q", recherche)
	}
	return v.Encode()
}

func (m *Module) vue(ctx context.Context, c *appli.Contexte, filtre, recherche string) (*vueNotes, error) {
	// Le filtre, la recherche (texte cherché tel quel, % et _ compris) et la
	// borne sont appliqués en SQL ; seul le début du texte est lu.
	lignes, err := m.a.Q.NotesVisibles(ctx, donnees.NotesVisiblesParams{
		FamilleID: c.Famille.ID, MembreID: sql.NullInt64{Int64: c.Membre.ID, Valid: true},
		Filtre: filtre, Motif: recherche, Nombre: notesListe + 1})
	if err != nil {
		return nil, err
	}
	membres, err := m.a.Membres(ctx, c)
	if err != nil {
		return nil, err
	}
	parID := appli.MembresParID(membres)
	v := &vueNotes{Filtre: filtre, Recherche: recherche, Params: paramsListe(filtre, recherche)}
	if len(lignes) > notesListe {
		lignes, v.Tronquee = lignes[:notesListe], true
	}
	for _, l := range lignes {
		n := donnees.Note{ID: l.ID, FamilleID: c.Famille.ID, AuteurID: l.AuteurID, Titre: l.Titre, Texte: l.Debut,
			Couleur: l.Couleur, Epinglee: l.Epinglee, Privee: l.Privee, ModifieLe: l.ModifieLe}
		if utf8.RuneCountInString(n.Texte) >= debutMax {
			n.Texte += "…"
		}
		cn := carteNote{Note: n, Date: dateCourte(c.Heure(n.ModifieLe), c.Maintenant()), Modifiable: modifiable(c, n)}
		if n.AuteurID.Valid {
			cn.Auteur = parID[n.AuteurID.Int64]
		}
		if n.Epinglee == 1 {
			v.Epinglees = append(v.Epinglees, cn)
		} else {
			v.Autres = append(v.Autres, cn)
		}
	}
	return v, nil
}

// dateCourte : « 14:32 » aujourd'hui, « 22 nov. » sinon.
func dateCourte(t, maintenant time.Time) string {
	if t.Format("2006-01-02") == maintenant.Format("2006-01-02") {
		return t.Format("15:04")
	}
	return strconv.Itoa(t.Day()) + " " + appli.MoisFR(t.Month())
}

func (m *Module) page(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	filtre, recherche := lireFiltre(r)
	v, err := m.vue(r.Context(), c, filtre, recherche)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Afficher(w, r, c, "notes/liste", "Notes", mod, v)
}

func (m *Module) contenu(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	filtre, recherche := lireFiltre(r)
	v, err := m.vue(r.Context(), c, filtre, recherche)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Fragment(w, r, c, "notes/liste", "notes", v)
}

func (m *Module) note(r *http.Request, c *appli.Contexte) (donnees.Note, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return donnees.Note{}, false
	}
	n, err := m.a.Q.NoteParID(r.Context(), donnees.NoteParIDParams{ID: id, FamilleID: c.Famille.ID})
	return n, err == nil && visible(c, n)
}

type formNote struct {
	Nouvelle    bool
	Note        donnees.Note
	Modifiable  bool
	Supprimable bool
	Cases       []CaseNote // liste à cocher dans le texte
	// PeutPriver : la case « Privée » est proposée (toujours à la création
	// et pour une note déjà privée ; pour une note partagée, seulement à un
	// parent qui en est l'auteur).
	PeutPriver bool
}

// peutPriver : qui peut rendre privée une note déjà partagée ? Seulement
// un parent qui en est l'auteur : la note disparaît pour le reste de la
// famille, ce qui revient à la supprimer (réservé aux parents).
func peutPriver(c *appli.Contexte, n donnees.Note) bool {
	auteur := n.AuteurID.Valid && n.AuteurID.Int64 == c.Membre.ID
	return auteur && (n.Privee == 1 || c.EstParent())
}

func (m *Module) formulaire(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	f := formNote{Nouvelle: r.PathValue("id") == "", Modifiable: true, PeutPriver: true}
	if f.Nouvelle {
		f.Note = donnees.Note{Couleur: 1}
		// Venue de « Partager » : titre et texte préremplis.
		f.Note.Titre, f.Note.Texte = appli.LirePrerempli(r, 80, 10000)
		if r.URL.Query().Get("voir") == privees {
			f.Note.Privee = 1
		}
	} else {
		n, ok := m.note(r, c)
		if !ok {
			m.a.PageErreur(w, r, c, http.StatusNotFound, "Note introuvable", "Cette note n'existe plus.")
			return
		}
		f.Note, f.Modifiable, f.PeutPriver = n, modifiable(c, n), peutPriver(c, n)
		f.Supprimable = supprimable(c, n)
		f.Cases = Cases(n.Texte)
	}
	titre := "Nouvelle note"
	if !f.Nouvelle {
		titre = f.Note.Titre
		if titre == "" {
			titre = "Note"
		}
	}
	m.a.Afficher(w, r, c, "notes/note", titre, mod, f)
}

type saisie struct {
	titre, texte     string
	couleur          int64
	privee, epinglee int64
}

func lire(r *http.Request) (saisie, string) {
	s := saisie{titre: strings.TrimSpace(r.PostFormValue("titre")), texte: strings.TrimSpace(r.PostFormValue("texte"))}
	if utf8.RuneCountInString(s.titre) > 80 {
		return s, "Titre : 80 caractères au plus."
	}
	if utf8.RuneCountInString(s.texte) > 10000 {
		return s, "Note trop longue (10 000 caractères au plus)."
	}
	if s.titre == "" && s.texte == "" {
		return s, "La note est vide."
	}
	s.couleur, _ = strconv.ParseInt(r.PostFormValue("couleur"), 10, 64)
	if s.couleur < 1 || s.couleur > 6 {
		s.couleur = 1
	}
	if r.PostFormValue("privee") == "1" {
		s.privee = 1
	}
	if r.PostFormValue("epinglee") == "1" {
		s.epinglee = 1
	}
	return s, ""
}

// apercu : le titre, ou le début du texte.
func apercu(titre, texte string) string {
	if titre != "" {
		return titre
	}
	r := []rune(strings.SplitN(texte, "\n", 2)[0])
	if len(r) > 40 {
		return string(r[:40]) + "…"
	}
	return string(r)
}

func (m *Module) creer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	s, msg := lire(r)
	if msg != "" {
		m.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	if nb, err := m.a.Q.CompterNotesFamille(r.Context(), c.Famille.ID); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	} else if nb >= notesMax {
		m.a.Echec(w, r, http.StatusBadRequest, fmt.Sprintf("%d notes au plus : demandez à un parent d'en supprimer.", notesMax))
		return
	}
	t := time.Now().Unix()
	n, err := m.a.Q.CreerNote(r.Context(), donnees.CreerNoteParams{
		FamilleID: c.Famille.ID, AuteurID: sql.NullInt64{Int64: c.Membre.ID, Valid: true}, Titre: s.titre, Texte: s.texte,
		Couleur: s.couleur, Epinglee: s.epinglee, Privee: s.privee, CreeLe: t, ModifieLe: t,
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	if n.Privee == 0 {
		// Une note privée n'annonce rien (même pas son titre).
		m.a.Publier(c, mod, fmt.Sprintf("%s a écrit la note « %s » 📝", c.Membre.Nom, apercu(n.Titre, n.Texte)))
	}
	m.a.Flash(w, "Note enregistrée ✓")
	m.a.Rediriger(w, r, "/notes")
}

func (m *Module) modifier(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	n, ok := m.note(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Note introuvable.")
		return
	}
	if !modifiable(c, n) {
		m.a.Echec(w, r, http.StatusForbidden, "Seul son auteur (ou un parent) peut la modifier.")
		return
	}
	s, msg := lire(r)
	if msg != "" {
		m.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	// Rendre privée une note partagée la fait disparaître pour les autres :
	// réservé à un parent qui en est l'auteur. Et seul l'auteur change ce
	// réglage (sinon la note disparaîtrait pour lui).
	if !(n.AuteurID.Valid && n.AuteurID.Int64 == c.Membre.ID) || (s.privee == 1 && !peutPriver(c, n)) {
		s.privee = n.Privee
	}
	// version : la note telle qu'elle était à l'ouverture du formulaire. Si
	// quelqu'un l'a changée entre-temps, rien n'est écrasé : on prévient
	// (la saisie reste à l'écran).
	avant, _ := strconv.ParseInt(r.PostFormValue("version"), 10, 64)
	k, err := m.a.Q.ModifierNote(r.Context(), donnees.ModifierNoteParams{
		Titre: s.titre, Texte: s.texte, Couleur: s.couleur, Epinglee: s.epinglee, Privee: s.privee,
		ModifieLe: max(time.Now().Unix(), n.ModifieLe+1), ID: n.ID, FamilleID: c.Famille.ID, Avant: avant,
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	if k == 0 {
		m.a.Echec(w, r, http.StatusConflict, "Quelqu'un vient de modifier cette note : copiez votre texte, puis rechargez la page.")
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Note enregistrée ✓")
	m.a.Rediriger(w, r, "/notes")
}

func (m *Module) epingler(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	n, ok := m.note(r, c)
	if !ok || !modifiable(c, n) {
		m.a.Echec(w, r, http.StatusForbidden, "Note introuvable ou non modifiable.")
		return
	}
	if err := m.a.Q.EpinglerNote(r.Context(), donnees.EpinglerNoteParams{Epinglee: 1 - n.Epinglee, ID: n.ID, FamilleID: c.Famille.ID}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	if !appli.EstHtmx(r) {
		http.Redirect(w, r, "/notes", http.StatusSeeOther)
		return
	}
	filtre, recherche := lireFiltre(r)
	v, err := m.vue(r.Context(), c, filtre, recherche)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Fragment(w, r, c, "notes/liste", "notes", v)
}

func (m *Module) supprimer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	n, ok := m.note(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Note introuvable.")
		return
	}
	if !supprimable(c, n) {
		m.a.Echec(w, r, http.StatusForbidden, "Seul un parent peut supprimer une note partagée.")
		return
	}
	if err := m.a.Q.SupprimerNote(r.Context(), donnees.SupprimerNoteParams{ID: n.ID, FamilleID: c.Famille.ID}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Note supprimée")
	m.a.Rediriger(w, r, "/notes")
}

func (m *Module) resume(ctx context.Context, c *appli.Contexte) appli.Resume {
	n, err := m.a.Q.CompterNotes(ctx, donnees.CompterNotesParams{FamilleID: c.Famille.ID, MembreID: sql.NullInt64{Int64: c.Membre.ID, Valid: true}})
	if err != nil || n == 0 {
		return appli.Resume{Info: "Aucune note"}
	}
	return appli.Resume{Info: appli.Pluriel(n, "note", "notes")}
}
