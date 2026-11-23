// SPDX-License-Identifier: GPL-3.0-or-later

package contacts

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"familycocon/internal/donnees"
	"familycocon/internal/essai"
)

// Revue du 23/11/2026 : liens, favoris, import vCard en flux.
func TestRevueLiens(t *testing.T) {
	if l := string(LienMail("a@b.fr?cc=x@y.fr&body=coucou")); strings.Contains(l, "?") || strings.Contains(l, "&") || !strings.HasPrefix(l, "mailto:a@b.fr") {
		t.Errorf("mailto non échappé : %q", l)
	}
	if LienMail("pas une adresse") != "" {
		t.Error("mailto pour une adresse invalide")
	}
	if s, ok := NormaliserSite("exemple.fr"); !ok || s != "https://exemple.fr" {
		t.Errorf("NormaliserSite : %q %v", s, ok)
	}
	if _, ok := NormaliserSite("javascript:alert(1)"); ok {
		t.Error("site javascript accepté")
	}
	if PremierTel("", "02", "03") != "02" {
		t.Error("PremierTel")
	}
	for qp, attendu := range map[string]string{"H=C3=A9l=c3=a8ne": "Hélène", "a=4Gb": "a=4Gb", "fin=": "fin=", "=3D": "=", "sans": "sans"} {
		if v := decoderQP(qp); v != attendu {
			t.Errorf("decoderQP(%q) = %q", qp, v)
		}
	}
}

func TestRevueVCardFlux(t *testing.T) {
	// Photo sur une seule ligne de 300 Kio (plus longue que le tampon de
	// lecture), puis une note en quoted-printable sur plusieurs lignes.
	vcf := "BEGIN:VCARD\nFN:Avec photo\nPHOTO;ENCODING=b:" + strings.Repeat("QUJD", 75000) + "\nTEL:06 00 00 00 01\nEND:VCARD\n" +
		"BEGIN:VCARD\nFN:Suivant\nNOTE;ENCODING=QUOTED-PRINTABLE:Bonjour =\n=C3=A0 tous\nEND:VCARD\n"
	cartes, err := lireVCards(context.Background(), strings.NewReader(vcf))
	if err != nil || len(cartes) != 2 || cartes[0].tel != "06 00 00 00 01" || cartes[1].notes != "Bonjour à tous" {
		t.Fatalf("%v : %+v", err, cartes)
	}
	// Trop de cartes : la lecture s'arrête tout de suite.
	var b strings.Builder
	for i := range CartesMax + 1 {
		fmt.Fprintf(&b, "BEGIN:VCARD\nFN:Contact %d\nEND:VCARD\n", i)
	}
	if _, err := lireVCards(context.Background(), strings.NewReader(b.String())); !errors.Is(err, errTropDeCartes) {
		t.Fatalf("plafond de cartes : %v", err)
	}
}

// envoyerFichierHtmx : comme htmx, le jeton CSRF dans l'en-tête : le corps
// est alors lu en flux, sans être gardé en mémoire.
func envoyerFichierHtmx(t *testing.T, n *essai.Navigateur, url, contenu string) (int, string) {
	t.Helper()
	var corps bytes.Buffer
	mw := multipart.NewWriter(&corps)
	_ = mw.WriteField("autre", "champ avant le fichier")
	fw, _ := mw.CreateFormFile("fichier", "contacts.vcf")
	_, _ = io.WriteString(fw, contenu)
	_ = mw.Close()
	req, _ := http.NewRequest("POST", url, &corps)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Jeton", n.Jeton)
	rep, err := n.C.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer rep.Body.Close()
	b, _ := io.ReadAll(rep.Body)
	return rep.StatusCode, string(b)
}

func TestRevueCarnet(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	ctx := context.Background()

	// Import envoyé par htmx (lecture en flux), puis doublons.
	marie.Get("/contacts")
	code, page := envoyerFichierHtmx(t, marie, srv.URL+"/contacts/import", vcfAndroid)
	if code != 200 || !strings.Contains(page, "4 contacts importés") {
		t.Fatalf("import en flux : %d\n%s", code, page)
	}
	if _, page := envoyerFichierHtmx(t, marie, srv.URL+"/contacts/import", vcfAndroid); !strings.Contains(page, "4 déjà présent") {
		t.Fatalf("doublons :\n%s", page)
	}
	if code, _ := envoyerFichierHtmx(t, marie, srv.URL+"/contacts/import", "pas un vcard"); code != http.StatusBadRequest {
		t.Fatalf("fichier invalide : %d", code)
	}
	// Résumé des adresses : compté en SQL.
	if _, page := marie.Get("/"); !strings.Contains(page, "1 adresse") {
		t.Fatal("résumé des adresses")
	}

	// Favori : l'état voulu est envoyé, un double envoi ne le défait pas.
	tous, _ := a.Q.ContactsFamille(ctx, 1)
	var id int64
	for _, ct := range tous {
		if ct.Favori == 0 {
			id = ct.ID
			break
		}
	}
	chemin := fmt.Sprintf("/contacts/%d/favori", id)
	marie.Post(chemin, map[string]string{"favori": "1"})
	marie.Post(chemin, map[string]string{"favori": "1"})
	if ct, _ := a.Q.ContactParID(ctx, donnees.ContactParIDParams{ID: id, FamilleID: 1}); ct.Favori != 1 {
		t.Fatal("double envoi : favori retiré")
	}
	_, page = marie.Post(chemin, map[string]string{"favori": "0"})
	if ct, _ := a.Q.ContactParID(ctx, donnees.ContactParIDParams{ID: id, FamilleID: 1}); ct.Favori != 0 || !strings.Contains(page, `name="favori" value="1"`) {
		t.Fatal("favori non retiré")
	}

	// Fiche : lien e-mail échappé.
	marie.Get("/contacts/nouveau")
	_, page = marie.Post("/contacts", map[string]string{"nom": "Piège", "email": "a@b.fr?cc=x@y.fr"})
	if strings.Contains(page, "?cc=") && strings.Contains(page, `href="mailto:a@b.fr?cc=`) {
		t.Fatalf("mailto non échappé :\n%s", page)
	}
	if !strings.Contains(page, `href="mailto:a@b.fr%3Fcc=x@y.fr"`) {
		t.Fatalf("lien e-mail absent :\n%s", page)
	}

	// Carnet plein : création refusée, import arrêté au plafond.
	t0 := int64(1)
	n, _ := a.Q.CompterContacts(ctx, 1)
	for i := n; i < ContactsMax; i++ {
		if _, err := a.Q.CreerContact(ctx, donnees.CreerContactParams{FamilleID: 1, Nom: fmt.Sprintf("Bourrage %d", i), Relation: "autre", CreeLe: t0, ModifieLe: t0}); err != nil {
			t.Fatal(err)
		}
	}
	if code, _ := marie.Post("/contacts", map[string]string{"nom": "De trop"}); code != http.StatusBadRequest {
		t.Fatalf("plafond de contacts : %d", code)
	}
	_, page = envoyerFichierHtmx(t, marie, srv.URL+"/contacts/import", "BEGIN:VCARD\nFN:Encore un\nEND:VCARD\n")
	if !strings.Contains(page, "carnet plein") {
		t.Fatalf("import dans un carnet plein :\n%s", page)
	}
	if n, _ := a.Q.CompterContacts(ctx, 1); n != ContactsMax {
		t.Fatalf("%d contacts", n)
	}
}
