// SPDX-License-Identifier: GPL-3.0-or-later

package contacts

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
)

// --- Groupes (étiquettes libres) ----------------------------------------------

// Groupes : « Foot de Léo, Parents de la classe » → [Foot de Léo, Parents de la classe]
// (espaces nettoyés, sans doublon, 10 au plus de 40 caractères).
func Groupes(s string) []string {
	var out []string
	vus := map[string]bool{}
	for _, g := range strings.Split(s, ",") {
		g = strings.Join(strings.Fields(g), " ")
		if g == "" || utf8.RuneCountInString(g) > 40 || vus[strings.ToLower(g)] || len(out) == 10 {
			continue
		}
		vus[strings.ToLower(g)] = true
		out = append(out, g)
	}
	return out
}

// --- Page Contacts ------------------------------------------------------------

// Ligne : un contact dans une liste.
type Ligne struct {
	donnees.Contact
	Detail string // métier, société ou relation
	Tel    string // pour le bouton « appeler »
}

type lettre struct {
	Lettre   string
	Contacts []Ligne
}

type vuePage struct {
	Favoris   []Ligne
	Lettres   []lettre
	Groupes   []string // pour les filtres
	Filtre    string   // « », « favoris », « pros » ou un groupe
	Recherche string
	Total     int
	PeutGerer bool // parent : import / export
}

func ligne(ct donnees.Contact) Ligne {
	l := Ligne{Contact: ct, Tel: LienTel(premierTel(ct))}
	switch {
	case ct.Metier != "" && ct.Societe != "":
		l.Detail = ct.Metier + " · " + ct.Societe
	case ct.Metier != "":
		l.Detail = ct.Metier
	case ct.Societe != "":
		l.Detail = ct.Societe
	default:
		for _, r := range Relations {
			if r.Cle == ct.Relation {
				l.Detail = r.Nom
			}
		}
	}
	return l
}

func premierTel(ct donnees.Contact) string {
	for _, t := range []string{ct.Telephone, ct.TelFixe, ct.TelTravail} {
		if t != "" {
			return t
		}
	}
	return ""
}

// initialeTri : la lettre de rangement (« É » → « E », chiffre ou symbole → « # »).
func initialeTri(nom string) string {
	r, _ := utf8.DecodeRuneInString(strings.TrimSpace(nom))
	r = unicode.ToUpper(r)
	if s := sansAccent(string(r)); s != "" {
		r, _ = utf8.DecodeRuneInString(s)
	}
	if r < 'A' || r > 'Z' {
		return "#"
	}
	return string(r)
}

func sansAccent(s string) string {
	return strings.NewReplacer("É", "E", "È", "E", "Ê", "E", "Ë", "E", "À", "A", "Â", "A", "Ä", "A", "Î", "I", "Ï", "I",
		"Ô", "O", "Ö", "O", "Ù", "U", "Û", "U", "Ü", "U", "Ç", "C").Replace(s)
}

// correspond : recherche dans le nom, les téléphones, l'e-mail, la société, le métier, les groupes.
func correspond(ct donnees.Contact, q string) bool {
	if q == "" {
		return true
	}
	texte := strings.ToLower(strings.Join([]string{ct.Nom, ct.Telephone, ct.TelFixe, ct.TelTravail, ct.Email, ct.Adresse,
		ct.Societe, ct.Metier, ct.Groupes}, " "))
	return strings.Contains(texte, q) ||
		(LienTel(q) != "" && len(LienTel(q)) >= 4 && strings.Contains(LienTel(ct.Telephone+ct.TelFixe+ct.TelTravail), LienTel(q)))
}

func (m *Module) pageContacts(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	tous, err := m.a.Q.ContactsFamille(r.Context(), c.Famille.ID)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	v := vuePage{Recherche: strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q"))), Filtre: r.URL.Query().Get("filtre"),
		Total: len(tous), PeutGerer: c.EstParent()}
	vus := map[string]bool{}
	for _, ct := range tous {
		for _, g := range Groupes(ct.Groupes) {
			if !vus[strings.ToLower(g)] {
				vus[strings.ToLower(g)] = true
				v.Groupes = append(v.Groupes, g)
			}
		}
	}
	sort.Slice(v.Groupes, func(i, j int) bool { return strings.ToLower(v.Groupes[i]) < strings.ToLower(v.Groupes[j]) })
	garder := func(ct donnees.Contact) bool {
		switch v.Filtre {
		case "":
		case "favoris":
			if ct.Favori == 0 {
				return false
			}
		case "pros":
			if ct.Pro == 0 {
				return false
			}
		default:
			trouve := false
			for _, g := range Groupes(ct.Groupes) {
				trouve = trouve || strings.EqualFold(g, v.Filtre)
			}
			if !trouve {
				return false
			}
		}
		return correspond(ct, v.Recherche)
	}
	// Sans filtre ni recherche : les favoris en haut, puis tout le monde par lettre.
	sansFiltre := v.Filtre == "" && v.Recherche == ""
	var liste []donnees.Contact
	for _, ct := range tous {
		if !garder(ct) {
			continue
		}
		if sansFiltre && ct.Favori == 1 {
			v.Favoris = append(v.Favoris, ligne(ct))
		}
		liste = append(liste, ct)
	}
	sort.SliceStable(liste, func(i, j int) bool {
		a, b := initialeTri(liste[i].Nom), initialeTri(liste[j].Nom)
		if a != b {
			return a < b
		}
		return strings.ToLower(sansAccent(liste[i].Nom)) < strings.ToLower(sansAccent(liste[j].Nom))
	})
	for _, ct := range liste {
		l := initialeTri(ct.Nom)
		if len(v.Lettres) == 0 || v.Lettres[len(v.Lettres)-1].Lettre != l {
			v.Lettres = append(v.Lettres, lettre{Lettre: l})
		}
		v.Lettres[len(v.Lettres)-1].Contacts = append(v.Lettres[len(v.Lettres)-1].Contacts, ligne(ct))
	}
	m.a.Afficher(w, r, c, "contacts/contacts", "Contacts", modContacts, v)
}

func (m *Module) resumeContacts(ctx context.Context, c *appli.Contexte) appli.Resume {
	n, err := m.a.Q.CompterContacts(ctx, c.Famille.ID)
	if err != nil || n == 0 {
		return appli.Resume{Info: "Carnet vide"}
	}
	return appli.Resume{Info: appli.Pluriel(n, "contact", "contacts")}
}

// --- Adresses (contacts avec une adresse postale) -----------------------------------

type groupe struct {
	Nom, Emoji string
	Contacts   []donnees.Contact
}

func (m *Module) pageAdresses(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	tous, err := m.a.Q.ContactsFamille(r.Context(), c.Famille.ID)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	var groupes []groupe
	for _, rel := range Relations {
		g := groupe{Nom: rel.Nom, Emoji: rel.Emoji}
		for _, ct := range tous {
			if ct.Relation != rel.Cle || ct.Adresse == "" || !correspond(ct, q) {
				continue
			}
			g.Contacts = append(g.Contacts, ct)
		}
		sort.SliceStable(g.Contacts, func(i, j int) bool { return strings.ToLower(g.Contacts[i].Nom) < strings.ToLower(g.Contacts[j].Nom) })
		if len(g.Contacts) > 0 {
			groupes = append(groupes, g)
		}
	}
	m.a.Afficher(w, r, c, "contacts/adresses", "Adresses", modAdresses, map[string]any{"Groupes": groupes, "Recherche": q})
}

func (m *Module) resumeAdresses(ctx context.Context, c *appli.Contexte) appli.Resume {
	tous, err := m.a.Q.ContactsFamille(ctx, c.Famille.ID)
	if err != nil {
		return appli.Resume{}
	}
	n := 0
	for _, ct := range tous {
		if ct.Adresse != "" {
			n++
		}
	}
	if n == 0 {
		return appli.Resume{Info: "Aucune adresse"}
	}
	return appli.Resume{Info: appli.Pluriel(n, "adresse", "adresses")}
}

// --- Fiche ----------------------------------------------------------------------

func (m *Module) contact(r *http.Request, c *appli.Contexte) (donnees.Contact, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return donnees.Contact{}, false
	}
	ct, err := m.a.Q.ContactParID(r.Context(), donnees.ContactParIDParams{ID: id, FamilleID: c.Famille.ID})
	return ct, err == nil
}

// Telephone : un numéro de la fiche.
type Telephone struct {
	Nom, Numero, Lien string
}

type vueFiche struct {
	Contact     donnees.Contact
	Telephones  []Telephone
	Carte       string       // lien OpenStreetMap
	Site        template.URL // http(s) seulement
	Relation    string
	Naissance   string // « 5 mars 2018 (8 ans) »
	Groupes     []string
	Retour      string // page d'où l'on vient
	TitreRetour string
	Prestataire bool // lien vers les interventions
}

// carte : lien OpenStreetMap pour une adresse.
func carte(adresse string) string {
	return "https://www.openstreetmap.org/search?query=" + strings.ReplaceAll(url.QueryEscape(strings.ReplaceAll(adresse, "\n", ", ")), "+", "%20")
}

// SiteSur : le site, seulement s'il est en http(s).
func SiteSur(s string) template.URL {
	u, err := url.Parse(s)
	if s == "" || err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return ""
	}
	return template.URL(u.String()) // #nosec G203 -- schéma vérifié
}

// retour : la page par laquelle on revient (le premier module activé).
func retour(c *appli.Contexte, ct donnees.Contact) (string, string) {
	switch {
	case c.Actif(modContacts):
		return "/contacts", "Contacts"
	case ct.Pro == 1 && c.Actif(modPrestataires):
		return "/prestataires", "Prestataires"
	case ct.Adresse != "" && c.Actif(modAdresses):
		return "/adresses", "Adresses"
	case c.Actif(modAnniv):
		return "/anniversaires", "Anniversaires"
	}
	return "/", "Accueil"
}

func (m *Module) fiche(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	if !actif(c) {
		m.a.PageErreur(w, r, c, http.StatusNotFound, "Module désactivé", "Ce module n'est pas activé pour votre famille.")
		return
	}
	ct, ok := m.contact(r, c)
	if !ok {
		m.a.PageErreur(w, r, c, http.StatusNotFound, "Contact introuvable", "Ce contact n'existe plus.")
		return
	}
	v := vueFiche{Contact: ct, Site: SiteSur(ct.Site), Groupes: Groupes(ct.Groupes), Prestataire: ct.Pro == 1 && c.Actif(modPrestataires)}
	v.Retour, v.TitreRetour = retour(c, ct)
	for _, t := range []Telephone{{"Mobile", ct.Telephone, ""}, {"Fixe", ct.TelFixe, ""}, {"Travail", ct.TelTravail, ""}} {
		if t.Numero != "" {
			t.Lien = LienTel(t.Numero)
			v.Telephones = append(v.Telephones, t)
		}
	}
	if ct.Adresse != "" {
		v.Carte = carte(ct.Adresse)
	}
	for _, rel := range Relations {
		if rel.Cle == ct.Relation {
			v.Relation = rel.Emoji + " " + rel.Nom
		}
	}
	if n, ok := LireNaissance(ct.Naissance); ok {
		v.Naissance = fmt.Sprintf("%d %s", n.Jour, appli.MoisFR(time.Month(n.Mois)))
		if n.Annee > 0 {
			auj := minuit(c.Maintenant())
			age := auj.Year() - n.Annee
			if n.Prochain(auj).Year() == auj.Year() && !n.Prochain(auj).Equal(auj) {
				age-- // pas encore fêté cette année
			}
			v.Naissance += fmt.Sprintf(" %d (%d ans)", n.Annee, age)
		}
	}
	module := modContacts
	if !c.Actif(modContacts) {
		module = modAdresses
	}
	m.a.Afficher(w, r, c, "contacts/fiche", ct.Nom, module, v)
}

// favori : ajoute ou retire des favoris.
func (m *Module) favori(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ct, ok := m.contact(r, c)
	if !ok || !actif(c) {
		m.a.Echec(w, r, http.StatusNotFound, "Contact introuvable.")
		return
	}
	if err := m.a.Q.BasculerFavori(r.Context(), donnees.BasculerFavoriParams{ID: ct.ID, FamilleID: c.Famille.ID}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	if ct.Favori == 0 {
		m.a.Flash(w, "⭐ Ajouté aux favoris")
	} else {
		m.a.Flash(w, "Retiré des favoris")
	}
	m.a.Rediriger(w, r, fmt.Sprintf("/contacts/%d", ct.ID))
}

// --- Formulaire --------------------------------------------------------------------

type formContact struct {
	Nouveau   bool
	Contact   donnees.Contact
	Relations any
	Metiers   []string
	Date      string // AAAA-MM-JJ pour le champ date
	SansAnnee bool
	Anniv     bool // venu d'Anniversaires : la date d'abord, formulaire court
	Retour    string
}

func (m *Module) formulaire(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	if !actif(c) {
		m.a.PageErreur(w, r, c, http.StatusNotFound, "Module désactivé", "Ce module n'est pas activé pour votre famille.")
		return
	}
	q := r.URL.Query()
	f := formContact{Nouveau: r.PathValue("id") == "", Relations: Relations, Metiers: Metiers,
		Anniv: q.Get("anniv") == "1" || (!c.Actif(modContacts) && !c.Actif(modAdresses) && !c.Actif(modPrestataires))}
	if f.Nouveau {
		f.Contact = donnees.Contact{Relation: "famille"}
		if q.Get("pro") == "1" {
			f.Contact.Relation, f.Contact.Pro = "pro", 1
		}
	} else {
		ct, ok := m.contact(r, c)
		if !ok {
			m.a.PageErreur(w, r, c, http.StatusNotFound, "Contact introuvable", "Ce contact n'existe plus.")
			return
		}
		f.Contact, f.Anniv = ct, false
		if n, ok := LireNaissance(ct.Naissance); ok {
			f.SansAnnee = n.Annee == 0
			annee := n.Annee
			if annee == 0 {
				annee = 2000
			}
			f.Date = fmt.Sprintf("%04d-%02d-%02d", annee, n.Mois, n.Jour)
		}
	}
	titre := "Nouveau contact"
	switch {
	case !f.Nouveau:
		titre, f.Retour = f.Contact.Nom, fmt.Sprintf("/contacts/%d", f.Contact.ID)
	case f.Anniv:
		f.Retour = "/anniversaires"
	default:
		f.Retour, _ = retour(c, f.Contact)
	}
	module := modContacts
	if !c.Actif(modContacts) {
		module = modAdresses
	}
	m.a.Afficher(w, r, c, "contacts/formulaire", titre, module, f)
}

type saisie struct {
	nom, relation, naissance, tel, fixe, travail, email, adresse string
	societe, metier, site, groupes, notes, idees                 string
	favori, pro                                                  int64
}

func lire(r *http.Request) (saisie, string) {
	v := func(champ string) string { return strings.TrimSpace(r.PostFormValue(champ)) }
	s := saisie{
		nom: v("nom"), relation: r.PostFormValue("relation"), tel: v("telephone"), fixe: v("tel_fixe"), travail: v("tel_travail"),
		email: v("email"), adresse: v("adresse"), societe: v("societe"), metier: v("metier"), site: v("site"),
		notes: v("notes"), idees: v("idees"), groupes: strings.Join(Groupes(r.PostFormValue("groupes")), ", "),
	}
	if r.PostFormValue("favori") == "1" {
		s.favori = 1
	}
	if r.PostFormValue("pro") == "1" {
		s.pro = 1
	}
	if n := utf8.RuneCountInString(s.nom); n < 1 || n > 80 {
		return s, "Le nom doit faire entre 1 et 80 caractères."
	}
	if !relationValide(s.relation) {
		s.relation = "autre"
	}
	for _, t := range []string{s.tel, s.fixe, s.travail} {
		if utf8.RuneCountInString(t) > 30 {
			return s, "Numéro de téléphone trop long (30 caractères)."
		}
	}
	if utf8.RuneCountInString(s.email) > 120 || utf8.RuneCountInString(s.adresse) > 300 || utf8.RuneCountInString(s.societe) > 80 ||
		utf8.RuneCountInString(s.metier) > 60 || utf8.RuneCountInString(s.notes) > 2000 || utf8.RuneCountInString(s.idees) > 1000 {
		return s, "Un des champs est trop long."
	}
	if s.email != "" && (!strings.Contains(s.email, "@") || strings.ContainsAny(s.email, " <>\"")) {
		return s, "Adresse e-mail invalide."
	}
	if s.site != "" {
		if !strings.Contains(s.site, "://") {
			s.site = "https://" + s.site
		}
		if SiteSur(s.site) == "" || len(s.site) > 300 {
			return s, "Adresse du site invalide (http ou https)."
		}
	}
	if d := r.PostFormValue("naissance"); d != "" {
		t, err := time.Parse("2006-01-02", d)
		if err != nil {
			return s, "Date de naissance invalide."
		}
		if r.PostFormValue("sans_annee") == "1" {
			s.naissance = "0000" + d[4:]
		} else {
			if t.After(time.Now()) || t.Year() < 1900 {
				return s, "Date de naissance invalide."
			}
			s.naissance = d
		}
	}
	return s, ""
}

func (m *Module) creer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	if !actif(c) {
		m.a.Echec(w, r, http.StatusNotFound, "Module désactivé.")
		return
	}
	s, msg := lire(r)
	if msg != "" {
		m.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	ct, err := m.creerContact(r.Context(), c.Famille.ID, s)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Flash(w, "Contact enregistré ✓")
	if s.naissance != "" && r.PostFormValue("anniv") == "1" {
		m.a.Rediriger(w, r, "/anniversaires")
		return
	}
	m.a.Rediriger(w, r, fmt.Sprintf("/contacts/%d", ct.ID))
}

func (m *Module) creerContact(ctx context.Context, famille int64, s saisie) (donnees.Contact, error) {
	t := time.Now().Unix()
	return m.a.Q.CreerContact(ctx, donnees.CreerContactParams{
		FamilleID: famille, Nom: s.nom, Relation: s.relation, Naissance: s.naissance, Telephone: s.tel, TelFixe: s.fixe,
		TelTravail: s.travail, Email: s.email, Adresse: s.adresse, Societe: s.societe, Metier: s.metier, Site: s.site,
		Favori: s.favori, Pro: s.pro, Groupes: s.groupes, Notes: s.notes, Idees: s.idees, CreeLe: t, ModifieLe: t,
	})
}

func (m *Module) modifier(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ct, ok := m.contact(r, c)
	if !ok || !actif(c) {
		m.a.Echec(w, r, http.StatusNotFound, "Contact introuvable.")
		return
	}
	s, msg := lire(r)
	if msg != "" {
		m.a.Echec(w, r, http.StatusBadRequest, msg)
		return
	}
	if err := m.a.Q.ModifierContact(r.Context(), donnees.ModifierContactParams{
		Nom: s.nom, Relation: s.relation, Naissance: s.naissance, Telephone: s.tel, TelFixe: s.fixe, TelTravail: s.travail,
		Email: s.email, Adresse: s.adresse, Societe: s.societe, Metier: s.metier, Site: s.site, Favori: s.favori, Pro: s.pro,
		Groupes: s.groupes, Notes: s.notes, Idees: s.idees, ModifieLe: time.Now().Unix(), ID: ct.ID, FamilleID: c.Famille.ID,
	}); err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Flash(w, "Contact enregistré ✓")
	m.a.Rediriger(w, r, fmt.Sprintf("/contacts/%d", ct.ID))
}

// supprimer : le contact, et le suivi de ses interventions s'il était
// prestataire. Les modules qui le citaient gardent leur texte libre.
func (m *Module) supprimer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ct, ok := m.contact(r, c)
	if !ok || !actif(c) {
		m.a.Echec(w, r, http.StatusNotFound, "Contact introuvable.")
		return
	}
	ctx := r.Context()
	err := m.a.Transaction(ctx, func(q *donnees.Queries) error {
		if err := q.SupprimerContact(ctx, donnees.SupprimerContactParams{ID: ct.ID, FamilleID: c.Famille.ID}); err != nil {
			return err
		}
		if err := q.SupprimerSuiviObjet(ctx, donnees.SupprimerSuiviObjetParams{FamilleID: c.Famille.ID, Module: modPrestataires, ObjetID: ct.ID}); err != nil {
			return err
		}
		return q.SupprimerCarnetObjet(ctx, donnees.SupprimerCarnetObjetParams{FamilleID: c.Famille.ID, Module: modPrestataires, ObjetID: ct.ID})
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Flash(w, ct.Nom+" supprimé(e)")
	dest, _ := retour(c, ct)
	m.a.Rediriger(w, r, dest)
}
