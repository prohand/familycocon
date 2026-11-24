// SPDX-License-Identifier: GPL-3.0-or-later

// Package fidelite : les cartes de fidélité de la famille. Chacun peut
// montrer la carte en caisse depuis son téléphone : le code-barres (ou le
// QR code) est dessiné par le serveur à partir du numéro.
//
// Droits : tout le monde consulte, ajoute et corrige ; seuls les parents
// suppriment.
package fidelite

import (
	"context"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"rsc.io/qr"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
)

// Module : les cartes de fidélité.
type Module struct{ a *appli.App }

const mod = "fidelite"

// Formats proposés.
var Formats = []struct{ Cle, Nom string }{
	{"auto", "Détecté tout seul"}, {"ean13", "EAN-13 (13 chiffres)"}, {"upca", "UPC-A (12 chiffres)"},
	{"ean8", "EAN-8 (8 chiffres)"}, {"code128", "Code 128"}, {"qr", "QR code"}, {"aucun", "Pas de code (numéro seul)"},
}

func formatValide(f string) bool {
	for _, x := range Formats {
		if x.Cle == f {
			return true
		}
	}
	return false
}

// Brancher ajoute les routes du module.
func Brancher(a *appli.App) {
	m := &Module{a: a}
	a.Route("GET /fidelite", mod, appli.Membre, m.page)
	// Couleurs des enseignes (aucune donnée de la famille : sans session).
	a.Mux.HandleFunc("GET /fidelite/enseignes.css", feuilleEnseignes)
	a.AjouterRecherche(mod, m.chercher)
	a.Route("GET /fidelite/nouvelle", mod, appli.Membre, m.formulaire)
	a.Route("POST /fidelite/scanner", mod, appli.Membre, m.scanner)
	a.Route("POST /fidelite", mod, appli.Membre, m.creer)
	a.Route("GET /fidelite/{id}", mod, appli.Membre, m.carte)
	a.Route("GET /fidelite/{id}/modifier", mod, appli.Membre, m.formulaire)
	a.Route("POST /fidelite/{id}", mod, appli.Membre, m.modifier)
	a.Route("POST /fidelite/{id}/supprimer", mod, appli.Parent, m.supprimer)

	a.AjouterResume(mod, m.resume)
	a.AjouterAction(appli.Ajout{Module: mod, Emoji: appli.EmojiFidelite, Texte: "Une carte de fidélité", URL: "/fidelite/nouvelle"})
}

// CartesMax : cartes par famille (garde-fou).
const CartesMax = 200

// ligneCarte : une carte dans la liste, et la fin de son numéro.
type ligneCarte struct {
	donnees.Carte
	Fin    string
	Marque string // enseigne reconnue (classe « marque-… »), vide sinon
}

// finNumero : les 4 derniers caractères (pas les 4 derniers octets : un
// numéro peut contenir des lettres accentuées, formats « QR » et « aucun »).
func finNumero(numero string) string {
	r := []rune(numero)
	if len(r) <= 4 {
		return numero
	}
	return string(r[len(r)-4:])
}

func (m *Module) page(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	cartes, err := m.a.Q.CartesFamille(r.Context(), c.Famille.ID)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	lignes := make([]ligneCarte, 0, len(cartes))
	for _, ct := range cartes {
		lignes = append(lignes, ligneCarte{Carte: ct, Fin: finNumero(ct.Numero), Marque: Marque(ct.Enseigne, ct.CouleurEnseigne)})
	}
	m.a.Afficher(w, r, c, "fidelite/cartes", "Cartes de fidélité", mod, lignes)
}

func (m *Module) trouver(r *http.Request, c *appli.Contexte) (donnees.Carte, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return donnees.Carte{}, false
	}
	ct, err := m.a.Q.CarteParID(r.Context(), donnees.CarteParIDParams{ID: id, FamilleID: c.Famille.ID})
	return ct, err == nil
}

type vueCarte struct {
	Carte    donnees.Carte
	SVG      template.HTML // code-barres
	QR       template.URL  // QR code (image PNG)
	Probleme string
	Type     string // « EAN-13 (détecté) »
	Marque   string // enseigne reconnue (classe « marque-… »), vide sinon
}

// Dessin : le code de la carte (SVG ou QR), ou la raison s'il est impossible.
func Dessin(ct donnees.Carte) vueCarte {
	v := vueCarte{Carte: ct}
	format := ct.Format
	if format == "auto" {
		format = FormatAuto(ct.Numero)
		v.Type = NomFormat(format) + " (détecté)"
	} else {
		v.Type = NomFormat(format)
	}
	switch format {
	case "aucun":
	case "qr":
		code, err := qr.Encode(ct.Numero, qr.M)
		if err != nil {
			v.Probleme = "Numéro trop long pour un QR code."
			break
		}
		code.Scale = 8
		v.QR = template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(code.PNG())) // #nosec G203 -- image produite ici
	default:
		svg, err := SVG(ct.Numero, format)
		if err != nil {
			v.Probleme = "Ce numéro ne peut pas être mis dans ce format : essayez « Code 128 »."
			break
		}
		v.SVG = svg
	}
	return v
}

// carte : la carte en grand, à montrer en caisse.
func (m *Module) carte(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ct, ok := m.trouver(r, c)
	if !ok {
		m.a.PageErreur(w, r, c, http.StatusNotFound, "Carte introuvable", "Cette carte n'existe plus.")
		return
	}
	// Les cartes les plus montrées remontent en haut de la liste.
	if err := m.a.Q.CarteUtilisee(r.Context(), donnees.CarteUtiliseeParams{UtiliseeLe: time.Now().Unix(), ID: ct.ID, FamilleID: c.Famille.ID}); err != nil {
		m.a.Journal.Error("fidélité : carte utilisée", "erreur", err)
	}
	v := Dessin(ct)
	v.Marque = Marque(ct.Enseigne, ct.CouleurEnseigne)
	m.a.Afficher(w, r, c, "fidelite/carte", ct.Enseigne, mod, v)
}

type formCarte struct {
	Nouvelle bool
	Carte    donnees.Carte
	Formats  any
	Type     string // type de code de la carte (« EAN-13 (détecté) »)
	Reconnue bool   // enseigne reconnue : ses couleurs sont proposées
}

func (m *Module) formulaire(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	f := formCarte{Nouvelle: r.PathValue("id") == "", Formats: Formats, Carte: donnees.Carte{Format: "auto", Couleur: 1, CouleurEnseigne: 1}}
	if !f.Nouvelle {
		ct, ok := m.trouver(r, c)
		if !ok {
			m.a.PageErreur(w, r, c, http.StatusNotFound, "Carte introuvable", "Cette carte n'existe plus.")
			return
		}
		f.Carte = ct
		f.Type = Dessin(ct).Type
		f.Reconnue = trouverEnseigne(ct.Enseigne) != nil
	}
	titre := "Nouvelle carte"
	if !f.Nouvelle {
		titre = f.Carte.Enseigne
	}
	// Scanner la carte : la caméra est permise sur cette page seulement.
	w.Header().Set("Permissions-Policy", "camera=(self), microphone=(), geolocation=(), payment=()")
	m.a.Afficher(w, r, c, "fidelite/formulaire", titre, mod, f)
}

type saisie struct {
	enseigne, numero, format, titulaire, notes string
	couleur, couleurEnseigne                   int64
}

func lire(r *http.Request) (saisie, string) {
	s := saisie{
		enseigne: strings.TrimSpace(r.PostFormValue("enseigne")), format: r.PostFormValue("format"),
		titulaire: strings.TrimSpace(r.PostFormValue("titulaire")), notes: strings.TrimSpace(r.PostFormValue("notes")),
	}
	// Numéro : sans les espaces (souvent tapés en groupes de 4).
	s.numero = strings.Join(strings.Fields(r.PostFormValue("numero")), "")
	if n := utf8.RuneCountInString(s.enseigne); n < 1 || n > 60 {
		return s, "Le nom de l'enseigne doit faire entre 1 et 60 caractères."
	}
	if n := utf8.RuneCountInString(s.numero); n < 1 || n > 80 {
		return s, "Le numéro doit faire entre 1 et 80 caractères."
	}
	if !formatValide(s.format) {
		s.format = "auto"
	}
	if utf8.RuneCountInString(s.titulaire) > 60 || utf8.RuneCountInString(s.notes) > 500 {
		return s, "Titulaire (60) ou notes (500) trop longs."
	}
	s.couleur, _ = strconv.ParseInt(r.PostFormValue("couleur"), 10, 64)
	if s.couleur < 1 || s.couleur > 8 {
		s.couleur = 1
	}
	if r.PostFormValue("couleur_enseigne") == "1" {
		s.couleurEnseigne = 1
	}
	// Le format choisi doit pouvoir coder le numéro.
	if v := Dessin(donnees.Carte{Numero: s.numero, Format: s.format}); v.Probleme != "" {
		return s, v.Probleme
	}
	return s, ""
}

func (m *Module) creer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	s, msg := lire(r)
	if msg != "" {
		m.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	if n, err := m.a.Q.CompterCartes(r.Context(), c.Famille.ID); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	} else if n >= CartesMax {
		m.a.Echec(w, r, http.StatusBadRequest, fmt.Sprintf("Trop de cartes (%d au plus) : supprimez-en d'abord.", CartesMax))
		return
	}
	ct, err := m.a.Q.CreerCarte(r.Context(), donnees.CreerCarteParams{
		FamilleID: c.Famille.ID, Enseigne: s.enseigne, Numero: s.numero, Format: s.format, Couleur: s.couleur,
		CouleurEnseigne: s.couleurEnseigne, Titulaire: s.titulaire, Notes: s.notes, CreeLe: time.Now().Unix(),
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Carte enregistrée ✓")
	m.a.Rediriger(w, r, fmt.Sprintf("/fidelite/%d", ct.ID))
}

func (m *Module) modifier(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ct, ok := m.trouver(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Carte introuvable.")
		return
	}
	s, msg := lire(r)
	if msg != "" {
		m.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	if err := m.a.Q.ModifierCarte(r.Context(), donnees.ModifierCarteParams{
		Enseigne: s.enseigne, Numero: s.numero, Format: s.format, Couleur: s.couleur, CouleurEnseigne: s.couleurEnseigne,
		Titulaire: s.titulaire, Notes: s.notes, ID: ct.ID, FamilleID: c.Famille.ID,
	}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Carte enregistrée ✓")
	m.a.Rediriger(w, r, fmt.Sprintf("/fidelite/%d", ct.ID))
}

func (m *Module) supprimer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ct, ok := m.trouver(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Carte introuvable.")
		return
	}
	if err := m.a.Q.SupprimerCarte(r.Context(), donnees.SupprimerCarteParams{ID: ct.ID, FamilleID: c.Famille.ID}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Publier(c, mod, "")
	m.a.Flash(w, "Carte « "+ct.Enseigne+" » supprimée")
	m.a.Rediriger(w, r, "/fidelite")
}

func (m *Module) resume(ctx context.Context, c *appli.Contexte) appli.Resume {
	n, err := m.a.Q.CompterCartes(ctx, c.Famille.ID)
	if err != nil || n == 0 {
		return appli.Resume{Info: "Aucune carte"}
	}
	return appli.Resume{Info: appli.Pluriel(n, "carte", "cartes")}
}
