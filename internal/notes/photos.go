// SPDX-License-Identifier: GPL-3.0-or-later

package notes

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
	"familycocon/internal/photo"
)

// Photos dans une note (plan, recette, étiquette, code du wifi…) :
//   - vues par ceux qui voient la note (une note privée : son auteur seul) ;
//   - ajoutées et retirées par ceux qui peuvent la modifier ;
//   - réduites (1 600 px) et réécrites en JPEG, sans le lieu GPS ;
//   - gardées à côté de la base, comme les tickets du carnet
//     (<données>/pieces/<famille>/<nom>.jpg), et effacées avec la note.

// Plafonds.
const (
	PhotosParNote    = 20
	PhotosParFamille = 5000
)

// ajouterPhoto : POST d'une photo (corps brut, réduite par le navigateur).
func (m *Module) ajouterPhoto(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	n, ok := m.note(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Note introuvable.")
		return
	}
	if !modifiable(c, n) {
		m.a.Echec(w, r, http.StatusForbidden, "Seul son auteur (ou un parent) peut la modifier.")
		return
	}
	if k, err := m.a.Q.CompterPhotosNote(ctx, n.ID); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	} else if k >= PhotosParNote {
		m.a.Echec(w, r, http.StatusBadRequest, fmt.Sprintf("%d photos au plus par note.", PhotosParNote))
		return
	}
	if k, err := m.a.Q.CompterPhotosNotesFamille(ctx, c.Famille.ID); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	} else if k >= PhotosParFamille {
		m.a.Echec(w, r, http.StatusBadRequest, fmt.Sprintf("%d photos au plus dans les notes : supprimez-en d'abord.", PhotosParFamille))
		return
	}
	corps, err := io.ReadAll(r.Body) // borné par le serveur (1 Mio)
	if err != nil {
		m.a.Echec(w, r, http.StatusRequestEntityTooLarge, "Photo trop lourde.")
		return
	}
	jpeg, err := photo.Preparer(corps)
	if err != nil {
		m.a.Echec(w, r, http.StatusBadRequest, "Photo illisible (JPEG ou PNG).")
		return
	}
	nom, err := photo.Ranger(m.a.DossierPieces(), c.Famille.ID, jpeg)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	p, err := m.a.Q.AjouterPhotoNote(ctx, donnees.AjouterPhotoNoteParams{FamilleID: c.Famille.ID, NoteID: n.ID, Fichier: nom,
		Taille: int64(len(jpeg)), CreePar: sql.NullInt64{Int64: c.Membre.ID, Valid: true}, CreeLe: time.Now().Unix()})
	if err != nil {
		_ = photo.Effacer(m.a.DossierPieces(), c.Famille.ID, nom)
		m.a.ErreurInterne(w, r, err)
		return
	}
	// Photo placée dans le texte par l'éditeur : il reçoit son numéro et
	// écrit le repère ; la page ne change pas (la saisie continue).
	if r.URL.Query().Get("place") == "1" {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":%d}`, p.ID)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Photo ajoutée 📷")
	m.a.Rediriger(w, r, "/notes/"+strconv.FormatInt(n.ID, 10))
}

// photoNote : la photo et sa note, si la note est visible.
func (m *Module) photoNote(r *http.Request, c *appli.Contexte) (donnees.PhotoNote, donnees.Note, bool) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	p, err := m.a.Q.PhotoNoteParID(r.Context(), donnees.PhotoNoteParIDParams{ID: id, FamilleID: c.Famille.ID})
	if err != nil {
		return p, donnees.Note{}, false
	}
	n, err := m.a.Q.NoteParID(r.Context(), donnees.NoteParIDParams{ID: p.NoteID, FamilleID: c.Famille.ID})
	return p, n, err == nil && m.visible(r.Context(), c, n)
}

// photo : l'image (jamais gardée par le navigateur : Cache-Control no-store).
func (m *Module) photo(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	p, _, ok := m.photoNote(r, c)
	if !ok {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(photo.Chemin(m.a.DossierPieces(), p.FamilleID, p.Fichier))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.FormatInt(p.Taille, 10))
	_, _ = io.Copy(w, f)
}

// supprimerPhoto : une photo retirée de sa note.
func (m *Module) supprimerPhoto(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	p, n, ok := m.photoNote(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Photo introuvable.")
		return
	}
	if !modifiable(c, n) {
		m.a.Echec(w, r, http.StatusForbidden, "Seul son auteur (ou un parent) peut modifier cette note.")
		return
	}
	if err := m.a.Q.SupprimerPhotoNote(r.Context(), donnees.SupprimerPhotoNoteParams{ID: p.ID, FamilleID: c.Famille.ID}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.effacer(c.Famille.ID, []donnees.PhotoNote{p})
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Photo retirée")
	m.a.Rediriger(w, r, "/notes/"+strconv.FormatInt(n.ID, 10))
}

// effacer : les fichiers des photos (après leur suppression en base).
func (m *Module) effacer(famille int64, l []donnees.PhotoNote) {
	for _, p := range l {
		if err := photo.Effacer(m.a.DossierPieces(), famille, p.Fichier); err != nil {
			m.a.Journal.Warn("photo de note non effacée", "fichier", p.Fichier, "erreur", err)
		}
	}
}
