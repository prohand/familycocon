// SPDX-License-Identifier: GPL-3.0-or-later

package suivi

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
	"familycocon/internal/photo"
)

// Pièces jointes du carnet : la photo d'un ticket ou d'une facture, sur une
// ligne (plein, réparation, vaccin…). Réservées aux parents (elles montrent
// des prix, et le budget est fermé aux enfants).
//   - gardées sur le disque, à côté de la base :
//     <données>/pieces/<famille>/<nom aléatoire>.jpg ; la base n'a que le nom ;
//   - réduites (1 600 px) et réécrites en JPEG avant d'être gardées (les
//     informations cachées du téléphone, comme le lieu, disparaissent) ;
//   - supprimées avec leur ligne, ou avec l'objet (véhicule…).

// Plafonds.
const (
	PiecesParLigne   = 10
	PiecesParFamille = 5000
)

// Pieces : le dossier des pièces jointes (dans le dossier des données).
func Pieces(a *appli.App) string { return filepath.Join(a.Config.Donnees, "pieces") }

// cheminPiece : le fichier d'une pièce. Le nom vient de la base (créé
// ici, aléatoire) : aucun chemin fourni par un navigateur.
func cheminPiece(a *appli.App, p donnees.Piece) string {
	return filepath.Join(Pieces(a), strconv.FormatInt(p.FamilleID, 10), filepath.Base(p.Fichier))
}

var errTropDePieces = errors.New("trop de pièces jointes")

// AjouterPiece : garde la photo (déjà préparée par photo.Preparer) et
// l'attache à la ligne. q : requêtes (transaction de l'appelant ou a.Q).
func AjouterPiece(ctx context.Context, a *appli.App, q *donnees.Queries, famille, ligne int64, auteur int64, jpeg []byte) error {
	if n, err := q.CompterPiecesLigne(ctx, ligne); err != nil {
		return err
	} else if n >= PiecesParLigne {
		return errTropDePieces
	}
	if n, err := q.CompterPiecesFamille(ctx, famille); err != nil {
		return err
	} else if n >= PiecesParFamille {
		return errTropDePieces
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	nom := hex.EncodeToString(b) + ".jpg"
	dossier := filepath.Join(Pieces(a), strconv.FormatInt(famille, 10))
	if err := os.MkdirAll(dossier, 0o750); err != nil {
		return err
	}
	// Écrite sous un nom provisoire puis renommée : jamais de fichier coupé.
	chemin := filepath.Join(dossier, nom)
	if err := os.WriteFile(chemin+".tmp", jpeg, 0o640); err != nil {
		return err
	}
	if err := os.Rename(chemin+".tmp", chemin); err != nil {
		return err
	}
	_, err := q.AjouterPiece(ctx, donnees.AjouterPieceParams{FamilleID: famille, CarnetID: ligne, Fichier: nom, Taille: int64(len(jpeg)),
		CreePar: sql.NullInt64{Int64: auteur, Valid: auteur > 0}, CreeLe: time.Now().Unix()})
	if err != nil {
		_ = os.Remove(chemin)
	}
	return err
}

// effacerFichiers : les fichiers des pièces (après leur suppression en base).
func (s *Suivi) effacerFichiers(l []donnees.Piece) {
	for _, p := range l {
		if err := os.Remove(cheminPiece(s.a, p)); err != nil && !errors.Is(err, os.ErrNotExist) {
			s.a.Journal.Warn("pièce jointe non effacée", "fichier", p.Fichier, "erreur", err)
		}
	}
}

// ajouterPiece : POST d'une photo (corps brut, réduite par le navigateur).
func (s *Suivi) ajouterPiece(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	l, err := s.a.Q.LigneCarnetParID(ctx, donnees.LigneCarnetParIDParams{ID: id, FamilleID: c.Famille.ID})
	o, ok := s.objets[l.Module]
	if err != nil || !ok || !c.Actif(l.Module) {
		s.a.Echec(w, r, http.StatusNotFound, "Ligne introuvable.")
		return
	}
	corps, err := io.ReadAll(r.Body) // borné par le serveur (1 Mio)
	if err != nil {
		s.a.Echec(w, r, http.StatusRequestEntityTooLarge, "Photo trop lourde.")
		return
	}
	jpeg, err := photo.Preparer(corps)
	if err != nil {
		s.a.Echec(w, r, http.StatusBadRequest, "Photo illisible (JPEG ou PNG).")
		return
	}
	err = AjouterPiece(ctx, s.a, s.a.Q, c.Famille.ID, l.ID, c.Membre.ID, jpeg)
	if errors.Is(err, errTropDePieces) {
		s.a.Echec(w, r, http.StatusBadRequest, fmt.Sprintf("%d photos au plus par ligne.", PiecesParLigne))
		return
	}
	if err != nil {
		s.a.ErreurInterne(w, r, err)
		return
	}
	s.publier(c, o)
	s.a.Flash(w, "Photo ajoutée 📎")
	s.a.Rediriger(w, r, o.URL(l.ObjetID))
}

// piece : la photo (parents seulement), jamais gardée par le navigateur.
func (s *Suivi) piece(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	p, err := s.a.Q.PieceParID(r.Context(), donnees.PieceParIDParams{ID: id, FamilleID: c.Famille.ID})
	if err != nil {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(cheminPiece(s.a, p))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.FormatInt(p.Taille, 10))
	_, _ = io.Copy(w, f)
}

// supprimerPiece : une photo retirée de sa ligne.
func (s *Suivi) supprimerPiece(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	p, err := s.a.Q.PieceParID(ctx, donnees.PieceParIDParams{ID: id, FamilleID: c.Famille.ID})
	if err != nil {
		s.a.Echec(w, r, http.StatusNotFound, "Photo introuvable.")
		return
	}
	l, err := s.a.Q.LigneCarnetParID(ctx, donnees.LigneCarnetParIDParams{ID: p.CarnetID, FamilleID: c.Famille.ID})
	o, ok := s.objets[l.Module]
	if err != nil || !ok {
		s.a.Echec(w, r, http.StatusNotFound, "Photo introuvable.")
		return
	}
	if err := s.a.Q.SupprimerPiece(ctx, donnees.SupprimerPieceParams{ID: p.ID, FamilleID: c.Famille.ID}); err != nil {
		s.a.ErreurInterne(w, r, err)
		return
	}
	s.effacerFichiers([]donnees.Piece{p})
	s.publier(c, o)
	s.a.Flash(w, "Photo retirée")
	s.a.Rediriger(w, r, o.URL(l.ObjetID))
}
