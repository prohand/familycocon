// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// navigateur : un client HTTP avec ses cookies, comme un vrai navigateur.
type navigateur struct {
	t     *testing.T
	srv   *httptest.Server
	c     *http.Client
	jeton string // jeton CSRF lu dans la dernière page
}

func nouveauNavigateur(t *testing.T, srv *httptest.Server) *navigateur {
	jar, _ := cookiejar.New(nil)
	return &navigateur{t: t, srv: srv, c: &http.Client{Jar: jar}}
}

var reJeton = regexp.MustCompile(`name="_jeton" value="([^"]+)"`)

func (n *navigateur) lire(rep *http.Response, err error) (int, string) {
	n.t.Helper()
	if err != nil {
		n.t.Fatal(err)
	}
	defer rep.Body.Close()
	b, _ := io.ReadAll(rep.Body)
	if m := reJeton.FindSubmatch(b); m != nil {
		n.jeton = string(m[1])
	}
	return rep.StatusCode, string(b)
}

func (n *navigateur) get(chemin string) (int, string) {
	n.t.Helper()
	return n.lire(n.c.Get(n.srv.URL + chemin))
}

// post envoie un formulaire (avec le jeton CSRF de la dernière page).
func (n *navigateur) post(chemin string, champs map[string]string) (int, string) {
	n.t.Helper()
	v := url.Values{}
	for k, x := range champs {
		v.Set(k, x)
	}
	if _, ok := champs["_jeton"]; !ok && n.jeton != "" {
		v.Set("_jeton", n.jeton)
	}
	return n.lire(n.c.PostForm(n.srv.URL+chemin, v))
}

// familleTest : serveur de test avec une famille (Marie, parent) et un
// navigateur connecté.
func familleTest(t *testing.T) (*App, *httptest.Server, *navigateur) {
	t.Helper()
	a := nouvelleAppTest(t, nil)
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	n := nouveauNavigateur(t, srv)
	code, page := n.post("/inscription", map[string]string{
		"famille": "Famille Martin", "prenom": "Marie", "identifiant": "marie",
		"mdp": "un grand nid douillet", "mdp2": "un grand nid douillet",
	})
	if code != 200 || !strings.Contains(page, "Ma famille") || !strings.Contains(page, "Bienvenue dans FamilyCocon") {
		t.Fatalf("inscription : %d\n%s", code, page)
	}
	return a, srv, n
}

func TestInscriptionPuisFermee(t *testing.T) {
	_, srv, n := familleTest(t)
	code, page := n.get("/")
	if code != 200 || !strings.Contains(page, "Bonjour Marie") || !strings.Contains(page, `class="tuile courses"`) {
		t.Fatalf("accueil : %d\n%s", code, page)
	}
	// Mode « premiere » : une seconde famille est refusée.
	autre := nouveauNavigateur(t, srv)
	code, _ = autre.post("/inscription", map[string]string{
		"famille": "Intrus", "prenom": "X", "identifiant": "intrus", "mdp": "un autre mot de passe", "mdp2": "un autre mot de passe",
	})
	if code != http.StatusForbidden {
		t.Fatalf("seconde inscription : %d", code)
	}
	// Sans session : retour à la connexion.
	code, page = autre.get("/famille")
	if code != 200 || !strings.Contains(page, "Se connecter") {
		t.Fatalf("sans session : %d", code)
	}
}

func TestConnexionEtLimite(t *testing.T) {
	_, srv, _ := familleTest(t)
	n := nouveauNavigateur(t, srv)
	if code, page := n.post("/connexion", map[string]string{"identifiant": "marie", "mdp": "mauvais mot de passe"}); code != 401 || !strings.Contains(page, "incorrect") {
		t.Fatalf("mauvais mot de passe : %d", code)
	}
	if code, page := n.post("/connexion", map[string]string{"identifiant": "MARIE", "mdp": "un grand nid douillet", "suite": "//pirate.example"}); code != 200 || !strings.Contains(page, "Bonjour Marie") {
		t.Fatalf("connexion (identifiant en majuscules, suite refusée) : %d\n%s", code, page)
	}
	// 10 échecs depuis la même adresse : verrou, même avec le bon mot de passe.
	p := nouveauNavigateur(t, srv)
	for i := 0; i < 10; i++ {
		p.post("/connexion", map[string]string{"identifiant": "inconnu", "mdp": "essai numéro " + string(rune('a'+i))})
	}
	if code, page := p.post("/connexion", map[string]string{"identifiant": "marie", "mdp": "un grand nid douillet"}); code != http.StatusTooManyRequests || !strings.Contains(page, "Trop d&#39;essais") {
		t.Fatalf("verrou attendu : %d", code)
	}
}

func TestJetonCSRF(t *testing.T) {
	_, _, n := familleTest(t)
	n.get("/moi")
	if code, _ := n.post("/moi/profil", map[string]string{"prenom": "Marie", "_jeton": "faux"}); code != http.StatusForbidden {
		t.Fatalf("jeton faux accepté : %d", code)
	}
	if code, page := n.post("/moi/profil", map[string]string{"prenom": "Maria", "couleur": "3"}); code != 200 || !strings.Contains(page, "Maria") {
		t.Fatalf("jeton valide refusé : %d", code)
	}
}

func TestEnfantParCodePin(t *testing.T) {
	_, srv, n := familleTest(t)
	if code, page := n.post("/famille/enfants", map[string]string{"prenom": "Léa", "pin": "1234", "couleur": "3"}); code != 200 || !strings.Contains(page, "Léa a été ajouté") {
		t.Fatalf("ajout enfant : %d", code)
	}
	if code, _ := n.post("/famille/enfants", map[string]string{"prenom": "Hugo", "pin": "12a4"}); code != 400 {
		t.Fatalf("PIN invalide accepté : %d", code)
	}
	// Déconnexion : l'appareil reste de confiance, Léa apparaît.
	n.get("/moi")
	n.post("/deconnexion", nil)
	code, page := n.get("/connexion")
	if code != 200 || !strings.Contains(page, "Qui es-tu") || !strings.Contains(page, "Léa") {
		t.Fatalf("enfants absents de la connexion : %d\n%s", code, page)
	}
	id := regexp.MustCompile(`/connexion/enfant/(\d+)`).FindStringSubmatch(page)[1]
	if code, _ := n.post("/connexion/enfant/"+id, map[string]string{"pin": "0000"}); code != 401 {
		t.Fatalf("mauvais PIN : %d", code)
	}
	if code, page := n.post("/connexion/enfant/"+id, map[string]string{"pin": "1234"}); code != 200 || !strings.Contains(page, "Bonjour Léa") {
		t.Fatalf("connexion PIN : %d", code)
	}
	// Un enfant ne voit ni les réglages de la famille, ni le budget.
	if code, _ := n.get("/famille"); code != http.StatusForbidden {
		t.Fatalf("réglages ouverts à un enfant : %d", code)
	}
	if _, page := n.get("/"); strings.Contains(page, `class="tuile budget"`) {
		t.Fatal("tuile budget visible par un enfant")
	}
	// Un autre appareil (pas de confiance) ne voit pas les enfants et ne
	// peut pas tenter de code.
	autre := nouveauNavigateur(t, srv)
	if _, page := autre.get("/connexion"); strings.Contains(page, "Léa") {
		t.Fatal("enfants visibles sur un appareil inconnu")
	}
	if code, page := autre.post("/connexion/enfant/"+id, map[string]string{"pin": "1234"}); code != 200 || strings.Contains(page, "Bonjour Léa") {
		t.Fatalf("PIN accepté sur un appareil inconnu : %d", code)
	}
}

func TestVerrouPin(t *testing.T) {
	_, _, n := familleTest(t)
	n.post("/famille/enfants", map[string]string{"prenom": "Hugo", "pin": "4321"})
	n.get("/moi")
	n.post("/deconnexion", nil)
	_, page := n.get("/connexion")
	id := regexp.MustCompile(`/connexion/enfant/(\d+)`).FindStringSubmatch(page)[1]
	for i := 0; i < 5; i++ {
		n.post("/connexion/enfant/"+id, map[string]string{"pin": "0000"})
	}
	if code, _ := n.post("/connexion/enfant/"+id, map[string]string{"pin": "4321"}); code != http.StatusTooManyRequests {
		t.Fatalf("verrou PIN attendu : %d", code)
	}
}

func TestInvitation(t *testing.T) {
	_, srv, n := familleTest(t)
	n.get("/famille")
	// Inviter un parent redemande le mot de passe (session volée).
	if code, _ := n.post("/famille/invitations", map[string]string{"role": "parent"}); code == 200 {
		t.Fatal("invitation de parent sans mot de passe acceptée")
	}
	if code, _ := n.post("/famille/invitations", map[string]string{"role": "enfant"}); code != 200 {
		t.Fatalf("invitation d'enfant : %d", code)
	}
	code, page := n.post("/famille/invitations", map[string]string{"role": "parent", "mdp": "un grand nid douillet"})
	m := regexp.MustCompile(`/invitation/([A-Za-z0-9_-]{43})`).FindStringSubmatch(page)
	if code != 200 || m == nil {
		t.Fatalf("lien d'invitation absent : %d", code)
	}
	thomas := nouveauNavigateur(t, srv)
	if code, page := thomas.get("/invitation/" + m[1]); code != 200 || !strings.Contains(page, "Famille Martin") {
		t.Fatalf("page d'invitation : %d", code)
	}
	if code, page := thomas.post("/invitation/"+m[1], map[string]string{
		"prenom": "Thomas", "identifiant": "marie", "mdp": "le papa du nid", "mdp2": "le papa du nid",
	}); code != http.StatusConflict || !strings.Contains(page, "déjà pris") {
		t.Fatalf("identifiant en double : %d", code)
	}
	if code, page := thomas.post("/invitation/"+m[1], map[string]string{
		"prenom": "Thomas", "identifiant": "thomas", "mdp": "le papa du nid", "mdp2": "le papa du nid",
	}); code != 200 || !strings.Contains(page, "Bonjour Thomas") {
		t.Fatalf("acceptation : %d\n%s", code, page)
	}
	// Usage unique.
	if code, _ := nouveauNavigateur(t, srv).get("/invitation/" + m[1]); code != http.StatusNotFound {
		t.Fatalf("lien réutilisable : %d", code)
	}
}

func TestModulesEtRaccourcis(t *testing.T) {
	_, _, n := familleTest(t)
	// Barre par défaut : Accueil, Agenda, +, Courses, Moi.
	_, page := n.get("/")
	if !strings.Contains(page, `href="/agenda"`) {
		t.Fatal("raccourci Agenda absent")
	}
	// Désactiver l'agenda : plus de tuile, raccourci remplacé.
	n.get("/famille")
	n.post("/famille/modules", map[string]string{"module_courses": "1", "module_menus": "1"})
	_, page = n.get("/")
	if strings.Contains(page, `class="tuile agenda"`) || strings.Contains(page, `href="/agenda"`) {
		t.Fatal("agenda encore visible après désactivation")
	}
	if !strings.Contains(page, `class="tuile menus"`) {
		t.Fatal("menus absent")
	}
	// Raccourcis personnalisés ; doublons refusés.
	n.get("/moi")
	if code, _ := n.post("/moi/raccourcis", map[string]string{"r1": "accueil", "r2": "accueil", "r3": "moi", "r4": "menus"}); code != 400 {
		t.Fatalf("doublon accepté : %d", code)
	}
	if code, _ := n.post("/moi/raccourcis", map[string]string{"r1": "accueil", "r2": "agenda", "r3": "moi", "r4": "menus"}); code != 400 {
		t.Fatalf("module désactivé accepté : %d", code)
	}
	if code, page := n.post("/moi/raccourcis", map[string]string{"r1": "menus", "r2": "famille", "r3": "courses", "r4": "moi"}); code != 200 || !strings.Contains(page, `href="/famille" title="Famille"><span class="ico">🏡`) {
		t.Fatalf("raccourcis : %d", code)
	}
}

func TestDernierParent(t *testing.T) {
	a, _, n := familleTest(t)
	n.post("/famille/enfants", map[string]string{"prenom": "Léa", "pin": "1234"})
	membres, _ := a.Q.MembresDeFamille(t.Context(), 1)
	if len(membres) != 2 {
		t.Fatalf("%d membres", len(membres))
	}
	n.get("/famille")
	// Se retirer soi-même : refusé.
	if code, _ := n.post("/famille/membres/1/retirer", nil); code != 400 {
		t.Fatalf("auto-retrait : %d", code)
	}
	if code, page := n.post("/famille/membres/2/retirer", nil); code != 200 || !strings.Contains(page, "retiré") {
		t.Fatalf("retrait de l'enfant : %d", code)
	}
}
