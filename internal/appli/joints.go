// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"familycocon/internal/donnees"
)

// Pièces jointes (documents) : un PDF, une image, un document Office ou
// LibreOffice, un texte, un CSV ou un ZIP, attaché à une note, une tâche,
// un projet ou une ligne du carnet.
//   - 10 Mo au plus par fichier, 20 par objet, 2 Go par famille ;
//   - gardés sur le disque, à côté des photos :
//     <données>/pieces/<famille>/<nom aléatoire>.<extension> ; la base garde
//     le nom d'origine, le type et la taille ;
//   - le type est choisi par le serveur d'après l'extension (jamais celui
//     envoyé par le navigateur), et le début du fichier est vérifié (un PDF
//     commence par « %PDF »…) : ni programme, ni page web ;
//   - téléchargés seulement par ceux qui voient l'objet ; PDF et images
//     s'ouvrent dans le navigateur, le reste est téléchargé ;
//   - chaque module dit qui voit et qui modifie (AjouterJoints) ; une pièce
//     dont l'objet n'existe plus est effacée par le ménage de chaque heure.

// Bornes.
const (
	JointMax          = 10 << 20 // octets par fichier
	JointsParObjet    = 20
	JointsParFamille  = 2 << 30 // octets
	nomJointMax       = 120     // caractères du nom d'origine
	AccepteJoints     = ".pdf,.jpg,.jpeg,.png,.gif,.webp,.heic,.doc,.docx,.xls,.xlsx,.ppt,.pptx,.odt,.ods,.odp,.txt,.csv,.rtf,.zip"
	debutVerifieJoint = 512
)

// typeJoint : un type de fichier accepté.
type typeJoint struct {
	mime   string
	icone  string
	ouvrir bool              // s'ouvre dans le navigateur (sinon : téléchargé)
	debut  func([]byte) bool // le début du fichier est-il bien de ce type ?
}

// Débuts attendus.
var (
	estPDF   = func(b []byte) bool { return bytes.HasPrefix(b, []byte("%PDF-")) }
	estZIP   = func(b []byte) bool { return bytes.HasPrefix(b, []byte("PK\x03\x04")) }                       // docx, xlsx, odt… sont des ZIP
	estOLE   = func(b []byte) bool { return bytes.HasPrefix(b, []byte("\xd0\xcf\x11\xe0\xa1\xb1\x1a\xe1")) } // doc, xls, ppt
	estImage = func(mime string) func([]byte) bool {
		return func(b []byte) bool { return http.DetectContentType(b) == mime }
	}
	estHEIC  = func(b []byte) bool { return len(b) >= 12 && string(b[4:8]) == "ftyp" }
	estRTF   = func(b []byte) bool { return bytes.HasPrefix(b, []byte(`{\rtf`)) }
	estTexte = func(b []byte) bool { return strings.HasPrefix(http.DetectContentType(b), "text/plain") }
)

// typesJoints : extension → type accepté.
var typesJoints = map[string]typeJoint{
	".pdf":  {"application/pdf", "📄", true, estPDF},
	".jpg":  {"image/jpeg", "🖼️", true, estImage("image/jpeg")},
	".jpeg": {"image/jpeg", "🖼️", true, estImage("image/jpeg")},
	".png":  {"image/png", "🖼️", true, estImage("image/png")},
	".gif":  {"image/gif", "🖼️", true, estImage("image/gif")},
	".webp": {"image/webp", "🖼️", true, estImage("image/webp")},
	".heic": {"image/heic", "🖼️", false, estHEIC},
	".doc":  {"application/msword", "📝", false, estOLE},
	".docx": {"application/vnd.openxmlformats-officedocument.wordprocessingml.document", "📝", false, estZIP},
	".odt":  {"application/vnd.oasis.opendocument.text", "📝", false, estZIP},
	".rtf":  {"application/rtf", "📝", false, estRTF},
	".txt":  {"text/plain; charset=utf-8", "📝", false, estTexte},
	".xls":  {"application/vnd.ms-excel", "📊", false, estOLE},
	".xlsx": {"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "📊", false, estZIP},
	".ods":  {"application/vnd.oasis.opendocument.spreadsheet", "📊", false, estZIP},
	".csv":  {"text/csv; charset=utf-8", "📊", false, estTexte},
	".ppt":  {"application/vnd.ms-powerpoint", "📽️", false, estOLE},
	".pptx": {"application/vnd.openxmlformats-officedocument.presentationml.presentation", "📽️", false, estZIP},
	".odp":  {"application/vnd.oasis.opendocument.presentation", "📽️", false, estZIP},
	".zip":  {"application/zip", "🗜️", false, estZIP},
}

// DroitsJoints : ce membre voit-il l'objet, et peut-il y ajouter ou en
// retirer des pièces ?
type DroitsJoints func(ctx context.Context, c *Contexte, objet int64) (voir, modifier bool)

// AjouterJoints : le module accepte des pièces jointes sur ses objets.
func (a *App) AjouterJoints(module string, f DroitsJoints) { a.joints[module] = f }

// Joint : une pièce montrée.
type Joint struct {
	donnees.PiecesJointe
	Icone       string
	TailleTexte string // « 1,2 Mo »
}

// VueJoints : le bloc « 📎 Pièces jointes » d'un objet (gabarit
// « pieces-jointes »).
type VueJoints struct {
	L          []Joint
	Module     string
	Objet      int64
	Modifiable bool
	Retour     string // page où revenir après un envoi
	CSRF       string
	Accepte    string
}

func versJoint(p donnees.PiecesJointe) Joint {
	return Joint{PiecesJointe: p, Icone: typesJoints[strings.ToLower(filepath.Ext(p.Nom))].icone, TailleTexte: TailleLisible(p.Taille)}
}

// TailleLisible : « 850 Ko », « 1,2 Mo ».
func TailleLisible(n int64) string {
	switch {
	case n < 1<<10:
		return strconv.FormatInt(n, 10) + " o"
	case n < 1<<20:
		return strconv.FormatInt((n+512)>>10, 10) + " Ko"
	}
	return strings.Replace(strconv.FormatFloat(float64(n)/(1<<20), 'f', 1, 64), ".", ",", 1) + " Mo"
}

// Joints : le bloc des pièces d'un objet (modifiable : le membre peut en
// ajouter et en retirer).
func (a *App) Joints(ctx context.Context, c *Contexte, module string, objet int64, modifiable bool, retour string) (VueJoints, error) {
	v := VueJoints{Module: module, Objet: objet, Modifiable: modifiable, Retour: retour, CSRF: c.CSRF, Accepte: AccepteJoints}
	l, err := a.Q.JointsObjet(ctx, donnees.JointsObjetParams{FamilleID: c.Famille.ID, Module: module, ObjetID: objet})
	for _, p := range l {
		v.L = append(v.L, versJoint(p))
	}
	return v, err
}

// JointsParObjet : les pièces de tous les objets d'un module (une requête).
func (a *App) JointsParObjet(ctx context.Context, c *Contexte, module string) (map[int64][]Joint, error) {
	l, err := a.Q.JointsModule(ctx, donnees.JointsModuleParams{FamilleID: c.Famille.ID, Module: module})
	out := map[int64][]Joint{}
	for _, p := range l {
		out[p.ObjetID] = append(out[p.ObjetID], versJoint(p))
	}
	return out, err
}

func (a *App) routesJoints() {
	a.Route("POST /joints/{module}/{objet}", "", Membre, a.envoyerJoints)
	a.Route("GET /joints/{id}/{nom}", "", Membre, a.telechargerJoint)
	a.Route("POST /joints/{id}/supprimer", "", Membre, a.supprimerJoint)
	a.AjouterMinute(a.menageJoints)
}

// droitsJoint : voir et modifier l'objet d'un module (rien si le module
// n'a pas de pièces, est fermé, ou réservé aux parents).
func (a *App) droitsJoint(ctx context.Context, c *Contexte, module string, objet int64) (bool, bool) {
	f, ok := a.joints[module]
	// Module fermé par la famille (ou réservé aux parents) : rien. Le carnet
	// vérifie le module de son objet lui-même.
	if !ok || (module != "carnet" && !c.Voit(module)) {
		return false, false
	}
	return f(ctx, c, objet)
}

// retourSur : une adresse de ce site seulement (jamais « //autre.site »).
func retourSur(v string) string {
	if strings.HasPrefix(v, "/") && !strings.HasPrefix(v, "//") && !strings.HasPrefix(v, "/\\") && len(v) < 300 {
		return v
	}
	return "/"
}

// nomJoint : le nom d'origine, nettoyé (sans dossier ni caractère de
// contrôle, 120 caractères au plus en gardant l'extension).
func nomJoint(v string) string {
	v = filepath.Base(strings.ReplaceAll(v, "\\", "/"))
	v = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '"' || r == '/' {
			return -1
		}
		return r
	}, strings.TrimSpace(v))
	if utf8.RuneCountInString(v) > nomJointMax {
		ext := filepath.Ext(v)
		v = string([]rune(strings.TrimSuffix(v, ext))[:nomJointMax-utf8.RuneCountInString(ext)]) + ext
	}
	if v == "" || v == "." {
		return ""
	}
	return v
}

var (
	errJointType   = errors.New("type refusé")
	errJointTaille = errors.New("trop gros")
	errJointPlace  = errors.New("plus de place")
)

// garderJoint : écrit le fichier sur le disque (nom aléatoire), en
// vérifiant son type et sa taille au fil de la lecture.
func (a *App) garderJoint(famille int64, nom string, r io.Reader) (fichier string, taille int64, t typeJoint, err error) {
	ext := strings.ToLower(filepath.Ext(nom))
	t, ok := typesJoints[ext]
	if !ok {
		return "", 0, t, errJointType
	}
	br := bufio.NewReaderSize(r, debutVerifieJoint)
	debut, _ := br.Peek(debutVerifieJoint)
	if !t.debut(debut) {
		return "", 0, t, errJointType
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", 0, t, err
	}
	fichier = hex.EncodeToString(b) + ext
	dossier := filepath.Join(a.DossierPieces(), strconv.FormatInt(famille, 10))
	if err := os.MkdirAll(dossier, 0o750); err != nil {
		return "", 0, t, err
	}
	chemin := filepath.Join(dossier, fichier)
	f, err := os.OpenFile(chemin+".tmp", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return "", 0, t, err
	}
	taille, err = io.Copy(f, io.LimitReader(br, JointMax+1))
	if e := f.Close(); err == nil {
		err = e
	}
	if err == nil && taille > JointMax {
		err = errJointTaille
	}
	if err == nil && taille == 0 {
		err = errJointType
	}
	if err == nil {
		err = os.Rename(chemin+".tmp", chemin)
	}
	if err != nil {
		_ = os.Remove(chemin + ".tmp")
		return "", 0, t, err
	}
	return fichier, taille, t, nil
}

// effacerFichierJoint : le fichier d'une pièce (déjà retirée de la base).
func (a *App) effacerFichierJoint(p donnees.PiecesJointe) {
	chemin := filepath.Join(a.DossierPieces(), strconv.FormatInt(p.FamilleID, 10), filepath.Base(p.Fichier))
	if err := os.Remove(chemin); err != nil && !errors.Is(err, os.ErrNotExist) {
		a.Journal.Warn("pièce jointe non effacée", "fichier", p.Fichier, "erreur", err)
	}
}

// fichiersEnvoyes : appelle f pour chaque fichier « fichier » du
// formulaire, lus en flux (envoi par htmx : jeton dans l'en-tête, corps pas
// encore lu) ; sinon (formulaire sans JavaScript) dans r.MultipartForm.
// retour : le champ « retour » (placé avant les fichiers).
func fichiersEnvoyes(r *http.Request, f func(nom string, corps io.Reader) error) (retour string, err error) {
	if r.MultipartForm != nil {
		if v := r.MultipartForm.Value["retour"]; len(v) > 0 {
			retour = v[0]
		}
		for _, en := range r.MultipartForm.File["fichier"] {
			fh, err := en.Open()
			if err != nil {
				return retour, err
			}
			err = f(en.Filename, fh)
			_ = fh.Close()
			if err != nil {
				return retour, err
			}
		}
		return retour, nil
	}
	mr, err := r.MultipartReader()
	if err != nil {
		return "", err
	}
	for {
		p, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			return retour, nil
		}
		if err != nil {
			return retour, err
		}
		switch p.FormName() {
		case "retour":
			b, _ := io.ReadAll(io.LimitReader(p, 300))
			retour = string(b)
		case "fichier":
			if p.FileName() != "" {
				err = f(p.FileName(), p)
			}
		}
		_ = p.Close()
		if err != nil {
			return retour, err
		}
	}
}

// envoyerJoints : POST /joints/{module}/{objet} : un ou plusieurs fichiers.
func (a *App) envoyerJoints(w http.ResponseWriter, r *http.Request, c *Contexte) {
	ctx := r.Context()
	module := r.PathValue("module")
	objet, _ := strconv.ParseInt(r.PathValue("objet"), 10, 64)
	if _, modifier := a.droitsJoint(ctx, c, module, objet); !modifier {
		a.Echec(w, r, http.StatusForbidden, "Vous ne pouvez pas ajouter de pièce ici.")
		return
	}
	n := 0
	retour, err := fichiersEnvoyes(r, func(nom string, corps io.Reader) error {
		nom = nomJoint(nom)
		if nom == "" {
			return errJointType
		}
		if k, err := a.Q.CompterJointsObjet(ctx, donnees.CompterJointsObjetParams{FamilleID: c.Famille.ID, Module: module, ObjetID: objet}); err != nil {
			return err
		} else if k >= JointsParObjet {
			return errJointPlace
		}
		if k, err := a.Q.TailleJointsFamille(ctx, c.Famille.ID); err != nil {
			return err
		} else if k+JointMax > JointsParFamille {
			return errJointPlace
		}
		fichier, taille, t, err := a.garderJoint(c.Famille.ID, nom, corps)
		if err != nil {
			return err
		}
		if _, err := a.Q.AjouterJoint(ctx, donnees.AjouterJointParams{FamilleID: c.Famille.ID, Module: module, ObjetID: objet, Nom: nom,
			Type: t.mime, Taille: taille, Fichier: fichier, CreePar: sql.NullInt64{Int64: c.Membre.ID, Valid: true}, CreeLe: time.Now().Unix()}); err != nil {
			a.effacerFichierJoint(donnees.PiecesJointe{FamilleID: c.Famille.ID, Fichier: fichier})
			return err
		}
		n++
		return nil
	})
	var trop *http.MaxBytesError
	// Une partie a pu être gardée avant l'erreur : on le dit (sinon on la
	// renverrait, en double).
	deja := ""
	if err != nil && n > 0 {
		deja = Pluriel(n, "pièce jointe ajoutée", "pièces jointes ajoutées") + ", puis : "
		a.Publier(c, module, "")
	}
	switch {
	case errors.Is(err, errJointType):
		a.Echec(w, r, http.StatusBadRequest, deja+"Fichier refusé : PDF, image, document (Word, Excel, PowerPoint, LibreOffice), texte, CSV ou ZIP seulement.")
		return
	case errors.Is(err, errJointTaille), errors.As(err, &trop):
		a.Echec(w, r, http.StatusRequestEntityTooLarge, deja+fmt.Sprintf("Fichier trop gros : %d Mo au plus par fichier, 50 Mo par envoi.", JointMax>>20))
		return
	case errors.Is(err, errJointPlace):
		a.Echec(w, r, http.StatusBadRequest, deja+fmt.Sprintf("Plus de place : %d pièces au plus ici, 2 Go pour la famille.", JointsParObjet))
		return
	case err != nil:
		a.ErreurInterne(w, r, err)
		return
	case n == 0:
		a.Echec(w, r, http.StatusBadRequest, "Choisissez un fichier.")
		return
	}
	a.Journal.Info("pièces jointes ajoutées", "module", module, "objet", objet, "nombre", n, "par", c.Membre.ID)
	a.Publier(c, module, "")
	a.Flash(w, Pluriel(n, "pièce jointe ajoutée", "pièces jointes ajoutées")+" 📎")
	a.Rediriger(w, r, retourSur(retour))
}

// jointPermis : la pièce {id}, si ce membre voit son objet.
func (a *App) jointPermis(r *http.Request, c *Contexte) (donnees.PiecesJointe, bool, bool) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	p, err := a.Q.JointParID(r.Context(), donnees.JointParIDParams{ID: id, FamilleID: c.Famille.ID})
	if err != nil {
		return p, false, false
	}
	voir, modifier := a.droitsJoint(r.Context(), c, p.Module, p.ObjetID)
	return p, voir, modifier
}

// telechargerJoint : GET /joints/{id}/{nom} (le nom ne sert qu'à
// l'affichage dans le navigateur).
func (a *App) telechargerJoint(w http.ResponseWriter, r *http.Request, c *Contexte) {
	p, voir, _ := a.jointPermis(r, c)
	if !voir {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(filepath.Join(a.DossierPieces(), strconv.FormatInt(p.FamilleID, 10), filepath.Base(p.Fichier)))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	t := typesJoints[strings.ToLower(filepath.Ext(p.Fichier))]
	h := w.Header()
	h.Set("Content-Type", p.Type)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "private, no-store")
	disposition := "attachment"
	if t.ouvrir {
		disposition = "inline"
	}
	if t.mime == "application/pdf" {
		// Le lecteur PDF du navigateur ne s'ouvre pas sous la politique de
		// la page (object-src 'none') : une politique à part, sans script.
		h.Set("Content-Security-Policy", "default-src 'none'; img-src 'self' data:; style-src 'unsafe-inline'; frame-ancestors 'self'")
	}
	h.Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": p.Nom}))
	http.ServeContent(w, r, "", time.Unix(p.CreeLe, 0), f)
}

// supprimerJoint : POST /joints/{id}/supprimer.
func (a *App) supprimerJoint(w http.ResponseWriter, r *http.Request, c *Contexte) {
	p, _, modifier := a.jointPermis(r, c)
	if !modifier {
		a.Echec(w, r, http.StatusForbidden, "Vous ne pouvez pas retirer cette pièce.")
		return
	}
	if err := a.Q.SupprimerJoint(r.Context(), donnees.SupprimerJointParams{ID: p.ID, FamilleID: c.Famille.ID}); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	a.effacerFichierJoint(p)
	a.Publier(c, p.Module, "")
	a.Flash(w, "Pièce jointe retirée")
	a.Rediriger(w, r, retourSur(r.PostFormValue("retour")))
}

// menageJoints : une fois par heure, les pièces dont l'objet n'existe plus.
func (a *App) menageJoints(ctx context.Context, maintenant time.Time) {
	if maintenant.Minute() != 7 {
		return
	}
	a.EffacerJointsOrphelins(ctx)
}

// EffacerJointsOrphelins : retire les pièces dont l'objet n'existe plus.
func (a *App) EffacerJointsOrphelins(ctx context.Context) {
	l, err := a.Q.JointsOrphelins(ctx)
	if err != nil {
		a.Journal.Error("ménage des pièces jointes", "erreur", err)
		return
	}
	for _, p := range l {
		if err := a.Q.SupprimerJoint(ctx, donnees.SupprimerJointParams{ID: p.ID, FamilleID: p.FamilleID}); err != nil {
			a.Journal.Error("ménage des pièces jointes", "erreur", err)
			return
		}
		a.effacerFichierJoint(p)
	}
	if len(l) > 0 {
		a.Journal.Info("pièces jointes orphelines effacées", "nombre", len(l))
	}
}
