// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// Tests des corrections de la revue du 23/11/2026 (lot socle).

// Une page privée n'est jamais gardée par le navigateur ; un fichier
// statique versionné l'est toujours.
func TestPagesPriveesSansCache(t *testing.T) {
	a, srv, n := familleTest(t)
	rep, err := n.c.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	rep.Body.Close()
	if cc := rep.Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("accueil : Cache-Control %q, no-store attendu", cc)
	}
	rep, err = n.c.Get(srv.URL + a.Statique.URL("app.css"))
	if err != nil {
		t.Fatal(err)
	}
	rep.Body.Close()
	if cc := rep.Header.Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("fichier statique : Cache-Control %q", cc)
	}
}

// Le mot de passe actuel ne se devine pas depuis une session volée :
// 5 erreurs, puis refus même avec le bon.
func TestChangementMotDePasseLimite(t *testing.T) {
	_, _, n := familleTest(t)
	n.get("/moi")
	for i := 0; i < seuilConfirmation; i++ {
		if code, _ := n.post("/moi/mdp", map[string]string{"actuel": "faux numéro " + strconv.Itoa(i), "nouveau": "une autre phrase longue", "nouveau2": "une autre phrase longue"}); code != http.StatusBadRequest {
			t.Fatalf("essai %d : %d", i, code)
		}
		n.get("/moi")
	}
	if code, _ := n.post("/moi/mdp", map[string]string{"actuel": "un grand nid douillet", "nouveau": "une autre phrase longue", "nouveau2": "une autre phrase longue"}); code != http.StatusTooManyRequests {
		t.Fatalf("verrou attendu, obtenu %d", code)
	}
}

// Inscriptions ouvertes : quelques-unes par adresse, pas une rafale.
func TestInscriptionLimitee(t *testing.T) {
	a := nouvelleAppTest(t, map[string]string{"FC_INSCRIPTIONS": "ouverte"})
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	inscrire := func(i int) int {
		code, _ := nouveauNavigateur(t, srv).post("/inscription", map[string]string{
			"famille": "Famille", "prenom": "Zoé", "identifiant": "zoe" + strconv.Itoa(i),
			"mdp": "un grand nid douillet", "mdp2": "un grand nid douillet",
		})
		return code
	}
	for i := 0; i < seuilInscriptions; i++ {
		if code := inscrire(i); code != 200 {
			t.Fatalf("inscription %d : %d", i, code)
		}
	}
	if code := inscrire(99); code != http.StatusTooManyRequests {
		t.Fatalf("limite attendue, obtenu %d", code)
	}
}

// Après une session expirée, on revient sur la page affichée, pas sur un
// fragment demandé par htmx.
func TestSuiteDemandee(t *testing.T) {
	page := httptest.NewRequest("GET", "/agenda?vue=mois", nil)
	if s := suiteDemandee(page); s != "/agenda?vue=mois" {
		t.Fatalf("page : %q", s)
	}
	lien := httptest.NewRequest("GET", "/courses", nil)
	lien.Header.Set("HX-Request", "true")
	lien.Header.Set("HX-Boosted", "true")
	lien.Header.Set("HX-Current-URL", "http://exemple.test/")
	if s := suiteDemandee(lien); s != "/courses" {
		t.Fatalf("lien : %q", s)
	}
	fragment := httptest.NewRequest("GET", "/agenda/contenu?vue=mois", nil)
	fragment.Header.Set("HX-Request", "true")
	fragment.Header.Set("HX-Current-URL", "http://exemple.test/agenda?vue=mois")
	if s := suiteDemandee(fragment); s != "/agenda?vue=mois" {
		t.Fatalf("fragment : %q", s)
	}
	fragment.Header.Del("HX-Current-URL")
	if s := suiteDemandee(fragment); s != "/" {
		t.Fatalf("fragment sans page : %q", s)
	}
}

// Le cache du service worker change de nom avec les fichiers, et la police
// citée par le CSS porte sa version.
func TestStatiqueVersions(t *testing.T) {
	a := nouvelleAppTest(t, nil)
	sw := string(a.Statique.fichiers["sw.js"].contenu)
	if strings.Contains(sw, `"`+versionSW+`"`) || !strings.Contains(sw, `const CACHE = "familycocon-`) {
		t.Fatal("nom du cache du service worker non versionné")
	}
	css := string(a.Statique.fichiers["app.css"].contenu)
	if !strings.Contains(css, policeCSS+"?v="+a.Statique.fichiers[policeCSS].hash) {
		t.Fatal("police sans version dans app.css")
	}
}

// Supprimer une passkey redemande le mot de passe.
func TestSupprimerPasskeyMotDePasse(t *testing.T) {
	a, _, n := familleTest(t)
	ajouterPasskeyTest(t, n, "un grand nid douillet")
	marie := membreID(t, a, "marie")
	cles, err := a.Q.PasskeysMembre(context.Background(), marie)
	if err != nil || len(cles) != 1 {
		t.Fatalf("passkeys : %d %v", len(cles), err)
	}
	chemin := "/moi/securite/passkeys/" + strconv.FormatInt(cles[0].ID, 10) + "/supprimer"
	n.get("/moi/securite")
	if code, _ := n.post(chemin, map[string]string{"mdp": "pas le bon mot de passe"}); code != http.StatusBadRequest {
		t.Fatalf("mauvais mot de passe : %d", code)
	}
	if nb, _ := a.Q.CompterPasskeys(context.Background(), marie); nb != 1 {
		t.Fatal("passkey supprimée sans le mot de passe")
	}
	n.get("/moi/securite")
	if code, _ := n.post(chemin, map[string]string{"mdp": "un grand nid douillet"}); code != 200 {
		t.Fatalf("suppression : %d", code)
	}
	if nb, _ := a.Q.CompterPasskeys(context.Background(), marie); nb != 0 {
		t.Fatal("passkey toujours là")
	}
}
