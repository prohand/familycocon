// SPDX-License-Identifier: GPL-3.0-or-later

package contacts

// Import et export au format vCard (.vcf), celui des carnets d'adresses des
// téléphones et des messageries. Écrit ici, sans dépendance :
//   - export : vCard 3.0 (lu partout) ;
//   - import : vCard 2.1 (Android, accents en « quoted-printable »), 3.0
//     et 4.0 ; photos et champs inconnus ignorés ; un contact déjà présent
//     (même nom et même téléphone) n'est pas ajouté deux fois.

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"familycocon/internal/riche"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
)

// --- Export ---------------------------------------------------------------------

// echapper : \ , ; et retours à la ligne, comme le veut la norme.
func echapper(s string) string {
	return strings.NewReplacer(`\`, `\\`, ",", `\,`, ";", `\;`, "\r\n", `\n`, "\n", `\n`, "\r", `\n`).Replace(s)
}

// plier : lignes de 75 octets au plus, la suite commence par une espace
// (sans couper un caractère en deux).
func plier(b *bytes.Buffer, ligne string) {
	limite := 75 // les lignes de suite commencent par une espace : 74 octets de texte
	for len(ligne) > limite {
		n := limite
		for n > 0 && !utf8.RuneStart(ligne[n]) {
			n--
		}
		b.WriteString(ligne[:n] + "\r\n ")
		ligne, limite = ligne[n:], 74
	}
	b.WriteString(ligne + "\r\n")
}

// VCard : un contact au format vCard 3.0.
func VCard(ct donnees.Contact) string {
	var b bytes.Buffer
	l := func(s string) { plier(&b, s) }
	l("BEGIN:VCARD")
	l("VERSION:3.0")
	l("FN:" + echapper(ct.Nom))
	l("N:" + echapper(ct.Nom) + ";;;;")
	for _, t := range []struct{ type_, num string }{{"CELL", ct.Telephone}, {"HOME", ct.TelFixe}, {"WORK", ct.TelTravail}} {
		if t.num != "" {
			l("TEL;TYPE=" + t.type_ + ":" + echapper(t.num))
		}
	}
	if ct.Email != "" {
		l("EMAIL;TYPE=INTERNET:" + echapper(ct.Email))
	}
	if ct.Adresse != "" {
		l("ADR;TYPE=HOME:;;" + echapper(ct.Adresse) + ";;;;")
	}
	if n, ok := LireNaissance(ct.Naissance); ok {
		if n.Annee > 0 {
			l(fmt.Sprintf("BDAY:%04d-%02d-%02d", n.Annee, n.Mois, n.Jour))
		} else {
			l(fmt.Sprintf("BDAY:--%02d-%02d", n.Mois, n.Jour))
		}
	}
	if ct.Societe != "" {
		l("ORG:" + echapper(ct.Societe))
	}
	if ct.Metier != "" {
		l("TITLE:" + echapper(ct.Metier))
	}
	if ct.Site != "" {
		l("URL:" + ct.Site)
	}
	if ct.Notes != "" {
		l("NOTE:" + echapper(riche.Simple(ct.Notes)))
	}
	if g := Groupes(ct.Groupes); len(g) > 0 {
		for i := range g {
			g[i] = echapper(g[i])
		}
		l("CATEGORIES:" + strings.Join(g, ","))
	}
	l("END:VCARD")
	return b.String()
}

func (m *Module) exporter(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	tous, err := m.a.Q.ContactsFamille(r.Context(), c.Famille.ID)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/vcard; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="contacts-familycocon-%s.vcf"`, c.Maintenant().Format("2006-01-02")))
	w.Header().Set("Cache-Control", "no-store")
	// Carte par carte : tout le carnet n'est pas assemblé en mémoire.
	for _, ct := range tous {
		if _, err := io.WriteString(w, VCard(ct)); err != nil {
			return
		}
	}
}

// --- Import ---------------------------------------------------------------------

// propriete : une ligne « NOM;PARAM=VAL:valeur ».
type propriete struct {
	nom    string
	params []string // en majuscules : « TYPE=CELL », « CELL », « ENCODING=QUOTED-PRINTABLE »…
	valeur string
}

func (p propriete) a(param string) bool {
	for _, x := range p.params {
		for _, v := range strings.Split(strings.TrimPrefix(x, "TYPE="), ",") {
			if v == param || x == param {
				return true
			}
		}
	}
	return false
}

// lignes : rend une à une les lignes dépliées (suite commençant par une
// espace ou une tabulation ; en quoted-printable, « = » en fin de ligne).
// Lecture en flux : le fichier n'est jamais gardé en entier. Une ligne
// logique fait 256 Kio au plus ; le contenu des photos est sauté ; 500 000
// lignes au plus. Une ligne physique plus longue que le tampon (photo sur
// une seule ligne) arrive en plusieurs morceaux.
func lignes(ctx context.Context, r io.Reader, rendre func(l string) error) error {
	const (
		ligneMax  = 256 << 10
		lignesMax = 500000
	)
	br := bufio.NewReaderSize(r, 64<<10)
	var cur []byte
	en, qp, ignorer, finEgal, milieu := false, false, false, false, false
	// ecrire ajoute un morceau à la ligne logique ; fin : le morceau termine
	// la ligne physique (c'est là que se lit le « = » du quoted-printable).
	ecrire := func(t []byte, fin bool) {
		if qp {
			finEgal = fin && bytes.HasSuffix(t, []byte("="))
			if finEgal {
				t = t[:len(t)-1]
			}
		}
		if !ignorer && len(cur)+len(t) <= ligneMax {
			cur = append(cur, t...)
		}
	}
	n := 0
	for {
		l, suite, err := br.ReadLine()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		fin := !suite
		if fin {
			l = bytes.TrimRight(l, "\r")
		}
		switch {
		case milieu: // suite d'une ligne physique plus longue que le tampon
			ecrire(l, fin)
		case en && qp && finEgal:
			ecrire(l, fin)
		case en && len(l) > 0 && (l[0] == ' ' || l[0] == '\t'):
			ecrire(l[1:], fin)
		default:
			if en {
				if err := rendre(string(cur)); err != nil {
					return err
				}
				cur = cur[:0]
			}
			en, finEgal = true, false
			tete := bytes.ToUpper(l[:max(0, bytes.IndexByte(l, ':'))])
			qp = bytes.Contains(tete, []byte("QUOTED-PRINTABLE"))
			ignorer = false
			ecrire(l, fin)
			// Photos (souvent des centaines de Kio) : l'en-tête suffit.
			ignorer = bytes.HasPrefix(tete, []byte("PHOTO")) || bytes.Contains(tete, []byte(".PHOTO"))
		}
		if !milieu {
			if n++; n > lignesMax {
				return errors.New("fichier trop long")
			}
			if n%10000 == 0 && ctx.Err() != nil {
				return ctx.Err()
			}
		}
		milieu = suite
	}
	if en {
		return rendre(string(cur))
	}
	return nil
}

func lirePropriete(l string) (propriete, bool) {
	i := strings.Index(l, ":")
	if i <= 0 {
		return propriete{}, false
	}
	tete := strings.Split(l[:i], ";")
	p := propriete{nom: strings.ToUpper(tete[0]), valeur: l[i+1:]}
	if j := strings.LastIndex(p.nom, "."); j >= 0 { // « item1.TEL »
		p.nom = p.nom[j+1:]
	}
	for _, x := range tete[1:] {
		p.params = append(p.params, strings.ToUpper(x))
	}
	if p.a("ENCODING=QUOTED-PRINTABLE") || p.a("QUOTED-PRINTABLE") {
		p.valeur = decoderQP(p.valeur)
	}
	if !utf8.ValidString(p.valeur) { // vieux fichiers en Latin-1
		r := []rune{}
		for _, c := range []byte(p.valeur) {
			r = append(r, rune(c))
		}
		p.valeur = string(r)
	}
	return p, true
}

// decoderQP : « =C3=A9 » → « é ». Écrit à la main (fmt.Sscanf, appelé pour
// chaque « =XX », coûtait l'essentiel du temps d'un gros import).
func decoderQP(s string) string {
	if !strings.Contains(s, "=") {
		return s
	}
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '=' && i+2 < len(s) {
			h, ok1 := hexa(s[i+1])
			l, ok2 := hexa(s[i+2])
			if ok1 && ok2 {
				b = append(b, h<<4|l)
				i += 2
				continue
			}
		}
		b = append(b, s[i])
	}
	return string(b)
}

// hexa : la valeur d'un chiffre hexadécimal.
func hexa(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	}
	return 0, false
}

// composants : « a;b\;c;d » → [a, b;c, d] (séparateur sep, échappements retirés).
func composants(s string, sep rune) []string {
	var out []string
	var cur strings.Builder
	echap := false
	for _, r := range s {
		switch {
		case echap:
			switch r {
			case 'n', 'N':
				cur.WriteRune('\n')
			default:
				cur.WriteRune(r)
			}
			echap = false
		case r == '\\':
			echap = true
		case r == sep:
			out = append(out, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	return append(out, cur.String())
}

func texteVCard(s string) string { return strings.TrimSpace(composants(s, 0)[0]) }

// naissanceVCard : 2018-03-05, 20180305, --03-05, --0305, 1604-03-05
// (année inconnue chez Apple), avec ou sans heure → « 2018-03-05 » ou « 0000-03-05 ».
func naissanceVCard(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, 'T'); i > 0 {
		s = s[:i]
	}
	s = strings.ReplaceAll(s, "-", "")
	var a, m, j int
	switch len(s) {
	case 8:
		if _, err := fmt.Sscanf(s, "%4d%2d%2d", &a, &m, &j); err != nil {
			return ""
		}
	case 4: // --MMJJ
		if _, err := fmt.Sscanf(s, "%2d%2d", &m, &j); err != nil {
			return ""
		}
	default:
		return ""
	}
	if a == 1604 { // Apple : année inconnue
		a = 0
	}
	if a > time.Now().Year() {
		return ""
	}
	n := fmt.Sprintf("%04d-%02d-%02d", a, m, j)
	if _, ok := LireNaissance(n); !ok {
		return ""
	}
	return n
}

// CartesMax : contacts lus dans un seul fichier.
const CartesMax = 3000

// errTropDeCartes : plus de CartesMax contacts dans le fichier.
var errTropDeCartes = errors.New("trop de contacts dans le fichier")

// lireVCards : les contacts d'un fichier .vcf, lu en flux (carte par
// carte) ; la lecture s'arrête dès que CartesMax est dépassé.
func lireVCards(ctx context.Context, r io.Reader) ([]saisie, error) {
	var out []saisie
	var s *saisie
	var prenomNom string
	var notes []string
	err := lignes(ctx, r, func(l string) error {
		p, ok := lirePropriete(l)
		if !ok {
			return nil
		}
		switch p.nom {
		case "BEGIN":
			if strings.EqualFold(p.valeur, "VCARD") {
				s, prenomNom, notes = &saisie{relation: "autre"}, "", nil
			}
		case "END":
			if s != nil && strings.EqualFold(p.valeur, "VCARD") {
				if s.nom == "" {
					s.nom = prenomNom
				}
				// Coupées tout de suite : 3000 cartes ne gardent pas 3000 longues notes.
				s.notes = borner(strings.Join(notes, "\n"), 2000)
				if s.nom != "" {
					if len(out) == CartesMax {
						return errTropDeCartes
					}
					out = append(out, *s)
				}
				s = nil
			}
		}
		if s == nil {
			return nil
		}
		switch p.nom {
		case "FN":
			s.nom = texteVCard(p.valeur)
		case "N":
			c := composants(p.valeur, ';') // nom;prénom;autres;civilité;suffixe
			if len(c) >= 2 {
				prenomNom = strings.TrimSpace(strings.TrimSpace(c[1]) + " " + strings.TrimSpace(c[0]))
			}
		case "TEL":
			num := texteVCard(strings.TrimPrefix(p.valeur, "tel:"))
			switch {
			case p.a("WORK") && s.travail == "":
				s.travail = num
			case p.a("HOME") && !p.a("CELL") && s.fixe == "":
				s.fixe = num
			case s.tel == "":
				s.tel = num
			case s.fixe == "":
				s.fixe = num
			case s.travail == "":
				s.travail = num
			}
		case "EMAIL":
			if s.email == "" {
				s.email = texteVCard(p.valeur)
			}
		case "ADR":
			if s.adresse == "" {
				c := composants(p.valeur, ';') // bp;complément;rue;ville;région;code postal;pays
				for len(c) < 7 {
					c = append(c, "")
				}
				var parts []string
				for _, x := range []string{c[1], c[2], strings.TrimSpace(c[5] + " " + c[3]), c[4], c[6]} {
					if x = strings.TrimSpace(x); x != "" {
						parts = append(parts, x)
					}
				}
				s.adresse = strings.Join(parts, "\n")
			}
		case "BDAY":
			s.naissance = naissanceVCard(p.valeur)
		case "ORG":
			s.societe = strings.TrimSpace(composants(p.valeur, ';')[0])
		case "TITLE", "ROLE":
			if s.metier == "" {
				s.metier = texteVCard(p.valeur)
			}
		case "URL":
			if v := texteVCard(p.valeur); SiteSur(v) != "" && s.site == "" {
				s.site = v
			}
		case "NOTE":
			if v := texteVCard(p.valeur); v != "" {
				notes = append(notes, v)
			}
		case "CATEGORIES":
			var g []string
			for _, x := range composants(p.valeur, ',') {
				if x = strings.TrimSpace(x); x != "" && !strings.EqualFold(x, "myContacts") && !strings.EqualFold(x, "starred") {
					g = append(g, x)
				}
				if strings.EqualFold(strings.TrimSpace(x), "starred") {
					s.favori = 1
				}
			}
			s.groupes = strings.Join(Groupes(strings.Join(append(Groupes(s.groupes), g...), ",")), ", ")
		}
		return nil
	})
	return out, err
}

// borner : un champ importé coupé à n caractères.
func borner(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// fichierEnvoye : le fichier du formulaire d'import, lu en flux.
//   - envoi par htmx (jeton CSRF dans l'en-tête) : le corps n'a pas encore
//     été lu, on avance jusqu'au champ « fichier » sans rien garder en mémoire ;
//   - formulaire sans JavaScript : le jeton était dans le corps, que la
//     bibliothèque standard a donc déjà lu en entier (r.MultipartForm).
func fichierEnvoye(r *http.Request) (io.ReadCloser, error) {
	if r.MultipartForm != nil {
		f, _, err := r.FormFile("fichier")
		return f, err
	}
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, err
	}
	for {
		p, err := mr.NextPart()
		if err != nil {
			return nil, err // io.EOF : pas de champ « fichier »
		}
		if p.FormName() == "fichier" {
			return p, nil
		}
		_ = p.Close()
	}
}

// ContactsMax : contacts par famille (garde-fou ; un carnet de téléphone
// bien rempli en compte quelques centaines).
const ContactsMax = 5000

func (m *Module) importer(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	trop := func(err error) bool {
		var e *http.MaxBytesError
		if errors.As(err, &e) {
			m.a.Echec(w, r, http.StatusRequestEntityTooLarge, "Fichier trop gros (8 Mo au plus).")
			return true
		}
		return false
	}
	f, err := fichierEnvoye(r)
	if err != nil {
		if !trop(err) {
			m.a.Echec(w, r, http.StatusBadRequest, "Choisissez un fichier .vcf.")
		}
		return
	}
	defer f.Close()
	// Le fichier est d'abord lu en entier (sans transaction : l'envoi peut
	// être lent et la base n'a qu'une connexion), puis tout est écrit d'un coup.
	cartes, err := lireVCards(r.Context(), f)
	switch {
	case errors.Is(err, errTropDeCartes):
		m.a.Echec(w, r, http.StatusBadRequest, fmt.Sprintf("Trop de contacts d'un coup (%d au plus).", CartesMax))
		return
	case trop(err):
		return
	case err != nil || len(cartes) == 0:
		m.a.Echec(w, r, http.StatusBadRequest, "Aucun contact trouvé dans ce fichier (format vCard .vcf attendu).")
		return
	}
	ctx := r.Context()
	existants, err := m.a.Q.ClesContacts(ctx, c.Famille.ID)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	cle := func(nom, tel string) string { return strings.ToLower(strings.TrimSpace(nom)) + "|" + LienTel(tel) }
	deja := map[string]bool{}
	for _, ct := range existants {
		deja[cle(ct.Nom, PremierTel(ct.Telephone, ct.TelFixe, ct.TelTravail))] = true
	}
	ajoutes, doublons, refuses := 0, 0, 0
	err = m.a.Transaction(ctx, func(q *donnees.Queries) error {
		t := time.Now().Unix()
		for _, s := range cartes {
			k := cle(s.nom, PremierTel(s.tel, s.fixe, s.travail))
			if deja[k] {
				doublons++
				continue
			}
			// Carnet plein : le reste du fichier n'est pas ajouté.
			if len(existants)+ajoutes >= ContactsMax {
				refuses++
				continue
			}
			deja[k] = true
			if !EmailValide(s.email) {
				s.email = ""
			}
			if _, err := q.CreerContact(ctx, donnees.CreerContactParams{
				FamilleID: c.Famille.ID, Nom: borner(s.nom, 80), Relation: s.relation, Naissance: s.naissance,
				Telephone: borner(s.tel, 30), TelFixe: borner(s.fixe, 30), TelTravail: borner(s.travail, 30),
				Email: borner(s.email, 120), Adresse: borner(s.adresse, 300), Societe: borner(s.societe, 80),
				Metier: borner(s.metier, 60), Site: borner(s.site, 300), Favori: s.favori, Groupes: s.groupes,
				Notes: borner(s.notes, 2000), CreeLe: t, ModifieLe: t,
			}); err != nil {
				return err
			}
			ajoutes++
		}
		return nil
	})
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	m.a.Journal.Info("contacts importés", "famille", c.Famille.ID, "ajoutes", ajoutes, "doublons", doublons, "refuses", refuses)
	msg := appli.Pluriel(ajoutes, "contact importé", "contacts importés")
	if doublons > 0 {
		msg += fmt.Sprintf(" (%d déjà présent(s))", doublons)
	}
	if refuses > 0 {
		msg += fmt.Sprintf(" ; carnet plein (%d contacts au plus) : %d non ajouté(s)", ContactsMax, refuses)
	}
	if ajoutes > 0 {
		m.a.Publier(c, modContacts, "")
	}
	m.a.Flash(w, msg+" ✓")
	m.a.Rediriger(w, r, "/contacts")
}
