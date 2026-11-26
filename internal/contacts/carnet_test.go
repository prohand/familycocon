// SPDX-License-Identifier: GPL-3.0-or-later

package contacts

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"familycocon/internal/donnees"
	"familycocon/internal/essai"
)

func TestCarnet(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")

	marie.Get("/contacts/nouveau")
	ajouter := func(champs map[string]string) string {
		t.Helper()
		code, page := marie.Post("/contacts", champs)
		if code != 200 {
			t.Fatalf("création %v : %d", champs["nom"], code)
		}
		return essai.Trouver(t, page, `action="/contacts/(\d+)/favori"`)
	}
	mamie := ajouter(map[string]string{"nom": "Mamie Jeanne", "relation": "famille", "telephone": "06 11 22 33 44",
		"tel_fixe": "02 99 88 77 66", "adresse": "12 rue des Roses\n35000 Rennes", "favori": "1"})
	ajouter(map[string]string{"nom": "Élodie (maman de Tom)", "relation": "ecole", "telephone": "07 00 00 00 01", "groupes": "Foot de Léo, Parents de la classe, foot de léo"})
	ajouter(map[string]string{"nom": "Dr Martin", "relation": "pro", "pro": "1", "metier": "Pédiatre", "societe": "Cabinet des Lilas"})

	_, page := lea.Get("/contacts")
	if !strings.Contains(page, `class="favoris"`) || !strings.Contains(page, `href="tel:0611223344"`) || !strings.Contains(page, "Pédiatre · Cabinet des Lilas") {
		t.Fatalf("liste :\n%s", page)
	}
	// « Élodie » est rangée à E, groupes dédoublonnés.
	if !strings.Contains(page, `class="sous-titre lettre">E<`) || strings.Count(page, "🏷️ Foot de Léo") != 1 {
		t.Fatal("lettre ou groupes")
	}
	// Filtres et recherche (par numéro aussi).
	if _, page := lea.Get("/contacts?filtre=" + url.QueryEscape("Parents de la classe")); !strings.Contains(page, "Élodie") || strings.Contains(page, "Mamie") {
		t.Fatal("filtre par groupe")
	}
	if _, page := lea.Get("/contacts?filtre=pros"); !strings.Contains(page, "Dr Martin") || strings.Contains(page, "Élodie") {
		t.Fatal("filtre pros")
	}
	if _, page := lea.Get("/contacts?q=0299887766"); !strings.Contains(page, "Mamie Jeanne") || strings.Contains(page, "Dr Martin") {
		t.Fatal("recherche par numéro")
	}
	// Fiche : deux numéros, plan, groupes.
	_, page = lea.Get("/contacts/" + mamie)
	if !strings.Contains(page, "Fixe") || !strings.Contains(page, "openstreetmap.org") {
		t.Fatalf("fiche :\n%s", page)
	}
	// Adresses : seulement les contacts qui ont une adresse.
	if _, page := lea.Get("/adresses"); !strings.Contains(page, "Mamie Jeanne") || strings.Contains(page, "Dr Martin") {
		t.Fatal("vue Adresses")
	}
	// Favori : retiré par Léa (tout le monde peut), suppression : parents.
	lea.Post("/contacts/"+mamie+"/favori", nil)
	if _, page := lea.Get("/contacts"); strings.Contains(page, `class="favoris"`) {
		t.Fatal("favori non retiré")
	}
	if code, _ := lea.Post("/contacts/"+mamie+"/supprimer", nil); code != http.StatusForbidden {
		t.Fatalf("suppression par un enfant : %d", code)
	}
	// Import / export : parents seulement.
	if code, _ := lea.Get("/contacts/export.vcf"); code != http.StatusForbidden {
		t.Fatalf("export par un enfant : %d", code)
	}
}

// envoyerFichier : formulaire multipart (import vCard).
func envoyerFichier(t *testing.T, n *essai.Navigateur, url, contenu string) (int, string) {
	t.Helper()
	var corps bytes.Buffer
	mw := multipart.NewWriter(&corps)
	_ = mw.WriteField("_jeton", n.Jeton)
	fw, _ := mw.CreateFormFile("fichier", "contacts.vcf")
	_, _ = io.WriteString(fw, contenu)
	_ = mw.Close()
	req, _ := http.NewRequest("POST", url, &corps)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rep, err := n.C.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer rep.Body.Close()
	b, _ := io.ReadAll(rep.Body)
	return rep.StatusCode, string(b)
}

const vcfAndroid = "BEGIN:VCARD\r\nVERSION:2.1\r\nN;CHARSET=UTF-8;ENCODING=QUOTED-PRINTABLE:Lef=C3=A8vre;H=C3=A9l=C3=A8ne;;;\r\n" +
	"FN;CHARSET=UTF-8;ENCODING=QUOTED-PRINTABLE:H=C3=A9l=C3=A8ne Lef=C3=A8vre\r\nTEL;CELL:06 98 76 54 32\r\nTEL;HOME:02 11 22 33 44\r\n" +
	"EMAIL;HOME:helene@exemple.fr\r\nBDAY:1985-07-14\r\nCATEGORIES:starred,Voisins\r\nPHOTO;ENCODING=BASE64;JPEG:/9j/4AAQSkZJRgABAQ\r\n" +
	" AAAQABAAD\r\n\r\nEND:VCARD\r\n" +
	"BEGIN:VCARD\nVERSION:3.0\nFN:Garage du Centre\nORG:Garage du Centre;Atelier\nTITLE:Garagiste\nTEL;TYPE=WORK,VOICE:+33 2 99 00 00 00\n" +
	"ADR;TYPE=WORK:;;1 avenue de la Gare;Vitré;;35500;France\nURL:https://garage.example\nNOTE:Ouvert le samedi\\, le matin\nEND:VCARD\n" +
	"BEGIN:VCARD\nVERSION:4.0\nFN:Parrain\nBDAY:--0315\nEND:VCARD\n" +
	"BEGIN:VCARD\nVERSION:3.0\nFN:Tata Zoé\nBDAY:1604-12-25\nX-SOCIALPROFILE:ignoré\nEND:VCARD\n"

func TestVCard(t *testing.T) {
	cartes, err := lireVCards(context.Background(), strings.NewReader(vcfAndroid))
	if err != nil || len(cartes) != 4 {
		t.Fatalf("%d cartes, %v", len(cartes), err)
	}
	h := cartes[0]
	if h.nom != "Hélène Lefèvre" || h.tel != "06 98 76 54 32" || h.fixe != "02 11 22 33 44" || h.naissance != "1985-07-14" ||
		h.favori != 1 || h.groupes != "Voisins" || h.email != "helene@exemple.fr" {
		t.Errorf("Android : %+v", h)
	}
	g := cartes[1]
	if g.societe != "Garage du Centre" || g.metier != "Garagiste" || g.travail != "+33 2 99 00 00 00" ||
		g.adresse != "1 avenue de la Gare\n35500 Vitré\nFrance" || g.site != "https://garage.example" || g.notes != "Ouvert le samedi, le matin" {
		t.Errorf("vCard 3.0 : %+v", g)
	}
	if cartes[2].naissance != "0000-03-15" || cartes[3].naissance != "0000-12-25" {
		t.Errorf("anniversaires sans année : %q %q", cartes[2].naissance, cartes[3].naissance)
	}

	// Aller-retour : export puis import d'un contact complet.
	_, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	marie.Get("/contacts/nouveau")
	marie.Post("/contacts", map[string]string{"nom": "Paul, l'ami; de Léo", "telephone": "06 00 00 00 02", "naissance": "2016-02-29",
		"adresse": "3 rue X\n75000 Paris", "groupes": "Foot", "notes": strings.Repeat("très long texte ", 10)})
	// Le lien télécharge le fichier (pas de navigation htmx qui mettrait le
	// texte du vCard dans la page).
	if _, p := marie.Get("/contacts"); !strings.Contains(p, `href="/contacts/export.vcf" download hx-boost="false"`) {
		t.Fatal("lien d'export vCard sans download / hx-boost=false")
	}
	code, vcf := marie.Get("/contacts/export.vcf")
	if code != 200 || !strings.Contains(vcf, `FN:Paul\, l'ami\; de Léo`) || !strings.Contains(vcf, "BDAY:2016-02-29") {
		t.Fatalf("export : %d\n%s", code, vcf)
	}
	for _, l := range strings.Split(vcf, "\r\n") {
		if len(l) > 75 {
			t.Fatalf("ligne non pliée (%d octets) : %q", len(l), l)
		}
	}
	relu, _ := lireVCards(context.Background(), strings.NewReader(vcf))
	if len(relu) != 1 || relu[0].nom != "Paul, l'ami; de Léo" || relu[0].adresse != "3 rue X\n75000 Paris" || !strings.HasPrefix(relu[0].notes, "très long texte très") {
		t.Fatalf("aller-retour : %+v", relu)
	}

	// Import par la page : 4 ajoutés, puis 0 la deuxième fois (doublons).
	marie.Get("/contacts")
	code, page := envoyerFichier(t, marie, srv.URL+"/contacts/import", vcfAndroid)
	if code != 200 || !strings.Contains(page, "4 contacts importés") {
		t.Fatalf("import : %d\n%s", code, page)
	}
	if _, page := envoyerFichier(t, marie, srv.URL+"/contacts/import", vcfAndroid); !strings.Contains(page, "4 déjà présent") {
		t.Fatalf("doublons :\n%s", page)
	}
	if _, page := marie.Get("/anniversaires"); !strings.Contains(page, "Hélène Lefèvre") || !strings.Contains(page, "Parrain") {
		t.Fatal("anniversaires importés absents")
	}
	if code, _ := envoyerFichier(t, marie, srv.URL+"/contacts/import", "pas un vcard"); code != http.StatusBadRequest {
		t.Fatalf("fichier invalide : %d", code)
	}
}

// Fichier piégé : 200 000 lignes de suite (avant : plusieurs secondes,
// croissance quadratique) ; un retour chariot seul n'injecte rien à l'export.
func TestVCardPiege(t *testing.T) {
	var b strings.Builder
	b.WriteString("BEGIN:VCARD\nFN:X\nNOTE:x\n")
	for range 200000 {
		b.WriteString(" a\n")
	}
	b.WriteString("END:VCARD\n")
	debut := time.Now()
	cartes, err := lireVCards(context.Background(), strings.NewReader(b.String()))
	if err != nil || len(cartes) != 1 || time.Since(debut) > 2*time.Second {
		t.Fatalf("%v, %d cartes, %v", err, len(cartes), time.Since(debut))
	}
	v := VCard(donnees.Contact{Nom: "Mamie\rTEL:+33 6 66 66 66 66\rEND:VCARD"})
	fins, tels := 0, 0
	for _, l := range strings.Split(v, "\r\n") {
		if l == "END:VCARD" {
			fins++
		}
		if strings.HasPrefix(l, "TEL") {
			tels++
		}
	}
	if fins != 1 || tels != 0 || strings.Contains(v, "\rTEL") {
		t.Fatalf("injection :\n%q", v)
	}
}
