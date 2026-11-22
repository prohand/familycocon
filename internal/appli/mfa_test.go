// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"familycocon/internal/mfa"
	"familycocon/internal/mfa/fauxcle"
)

// La configuration de test : http://localhost:8080 → domaine « localhost ».
const (
	domaineTest = "localhost"
	origineTest = "http://localhost:8080"
)

func (n *navigateur) json(methode, chemin string, corps any) (int, map[string]string) {
	n.t.Helper()
	var b io.Reader
	if corps != nil {
		j, _ := json.Marshal(corps)
		b = bytes.NewReader(j)
	}
	req, _ := http.NewRequest(methode, n.srv.URL+chemin, b)
	req.Header.Set("Content-Type", "application/json")
	if n.jeton != "" {
		req.Header.Set("X-Jeton", n.jeton)
	}
	rep, err := n.c.Do(req)
	if err != nil {
		n.t.Fatal(err)
	}
	defer rep.Body.Close()
	brut, _ := io.ReadAll(rep.Body)
	var v map[string]string
	_ = json.Unmarshal(brut, &v)
	return rep.StatusCode, v
}

func (n *navigateur) jetonHtmx(page string) {
	if m := regexp.MustCompile(`"X-Jeton": "([^"]+)"`).FindStringSubmatch(page); m != nil {
		n.jeton = m[1]
	}
}

// connexionMotDePasse : renvoie la page obtenue après le mot de passe.
func connexionMotDePasse(t *testing.T, n *navigateur, ident, mdp string) string {
	t.Helper()
	_, page := n.post("/connexion", map[string]string{"identifiant": ident, "mdp": mdp})
	return page
}

func TestDoubleAuthentificationTOTP(t *testing.T) {
	_, srv, marie := familleTest(t)
	_, page := marie.get("/moi/securite")
	secret := regexp.MustCompile(`class="cle-totp">([A-Z2-7]+)<`).FindStringSubmatch(page)
	if secret == nil || !strings.Contains(page, "data:image/png;base64,") {
		t.Fatal("QR code ou clé absents")
	}
	if code, _ := marie.post("/moi/securite/totp/activer", map[string]string{"code": "000000"}); code != 400 {
		t.Fatalf("mauvais code accepté : %d", code)
	}
	maintenant := time.Now()
	code, _ := mfa.CodeTOTP(secret[1], maintenant)
	_, page = marie.post("/moi/securite/totp/activer", map[string]string{"code": code})
	codes := regexp.MustCompile(`<code>([A-Z2-9]{4}-[A-Z2-9]{4}-[A-Z2-9]{4})</code>`).FindAllStringSubmatch(page, -1)
	if len(codes) != 10 || !strings.Contains(page, "Activée") {
		t.Fatalf("activation : %d codes", len(codes))
	}

	// Nouvelle connexion : le mot de passe ne suffit plus.
	n := nouveauNavigateur(t, srv)
	if page := connexionMotDePasse(t, n, "marie", "un grand nid douillet"); !strings.Contains(page, "Vérification") || strings.Contains(page, "Bonjour Marie") {
		t.Fatal("pas de second facteur demandé")
	}
	if c, _ := n.get("/"); c != 200 {
		t.Fatal(c)
	} else if _, p := n.get("/"); strings.Contains(p, "Bonjour Marie") {
		t.Fatal("session ouverte sans second facteur")
	}
	if c, _ := n.post("/connexion/verification", map[string]string{"code": "123456"}); c != 401 {
		t.Fatalf("faux code : %d", c)
	}
	// Le code déjà utilisé pour l'activation est refusé (anti-rejeu) ;
	// un code de l'intervalle suivant passe.
	if c, _ := n.post("/connexion/verification", map[string]string{"code": code}); c != 401 {
		t.Fatalf("code rejoué accepté : %d", c)
	}
	suivant, _ := mfa.CodeTOTP(secret[1], maintenant.Add(30*time.Second))
	if c, p := n.post("/connexion/verification", map[string]string{"code": suivant}); c != 200 || !strings.Contains(p, "Bonjour Marie") {
		t.Fatalf("bon code refusé : %d", c)
	}

	// Code de secours : une seule fois.
	for i, attendu := range []int{200, 401} {
		x := nouveauNavigateur(t, srv)
		connexionMotDePasse(t, x, "marie", "un grand nid douillet")
		c, p := x.post("/connexion/verification", map[string]string{"code": strings.ToLower(codes[0][1])})
		if c != attendu {
			t.Fatalf("code de secours, essai %d : %d", i+1, c)
		}
		if attendu == 200 && !strings.Contains(p, "Bonjour Marie") {
			t.Fatal("code de secours sans session")
		}
	}

	// Désactivation : mot de passe exigé.
	marie.get("/moi/securite")
	if c, _ := marie.post("/moi/securite/totp/desactiver", map[string]string{"mdp": "faux"}); c != 400 {
		t.Fatalf("désactivation sans mot de passe : %d", c)
	}
	marie.post("/moi/securite/totp/desactiver", map[string]string{"mdp": "un grand nid douillet"})
	x := nouveauNavigateur(t, srv)
	if p := connexionMotDePasse(t, x, "marie", "un grand nid douillet"); !strings.Contains(p, "Bonjour Marie") {
		t.Fatal("toujours un second facteur après désactivation")
	}
}

func b64t(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// ajouterPasskeyTest enregistre une fausse passkey pour le navigateur connecté.
func ajouterPasskeyTest(t *testing.T, n *navigateur) *fauxcle.Cle {
	t.Helper()
	_, page := n.get("/moi/securite")
	n.jetonHtmx(page)
	c, opts := n.json("GET", "/moi/securite/passkeys/defi", nil)
	if c != 200 || opts["challenge"] == "" {
		t.Fatalf("options d'inscription : %d %v", c, opts)
	}
	cle := fauxcle.Nouvelle(-7, domaineTest, origineTest)
	client, att := cle.Creer(opts["challenge"])
	if c, v := n.json("POST", "/moi/securite/passkeys", map[string]string{"clientDataJSON": client, "attestationObject": att, "nom": "Téléphone"}); c != http.StatusNoContent {
		t.Fatalf("enregistrement : %d %v", c, v)
	}
	return cle
}

func TestPasskeys(t *testing.T) {
	_, srv, marie := familleTest(t)
	cle := ajouterPasskeyTest(t, marie)
	if _, page := marie.get("/moi/securite"); !strings.Contains(page, "Téléphone") {
		t.Fatal("passkey absente de la liste")
	}

	// Connexion sans mot de passe.
	n := nouveauNavigateur(t, srv)
	c, opts := n.json("GET", "/connexion/passkey/defi", nil)
	if c != 200 {
		t.Fatal(c)
	}
	cle.Compteur++
	client, aut, sig := cle.Signer(opts["challenge"])
	if c, v := n.json("POST", "/connexion/passkey", map[string]string{"id": b64t(cle.Identifiant), "clientDataJSON": client, "authenticatorData": aut, "signature": sig}); c != 200 || v["suite"] != "/" {
		t.Fatalf("connexion par passkey : %d %v", c, v)
	}
	if _, page := n.get("/"); !strings.Contains(page, "Bonjour Marie") {
		t.Fatal("pas de session après la passkey")
	}
	// Défi à usage unique : rejouer la même réponse est refusé.
	if c, _ := n.json("POST", "/connexion/passkey", map[string]string{"id": b64t(cle.Identifiant), "clientDataJSON": client, "authenticatorData": aut, "signature": sig}); c == 200 {
		t.Fatal("réponse rejouée acceptée")
	}
	// Sans vérification de la personne (simple toucher) : refusé sans mot de passe.
	x := nouveauNavigateur(t, srv)
	_, opts = x.json("GET", "/connexion/passkey/defi", nil)
	cle.SansVerification, cle.Compteur = true, cle.Compteur+1
	client, aut, sig = cle.Signer(opts["challenge"])
	if c, _ := x.json("POST", "/connexion/passkey", map[string]string{"id": b64t(cle.Identifiant), "clientDataJSON": client, "authenticatorData": aut, "signature": sig}); c != 401 {
		t.Fatalf("passkey sans vérification acceptée seule : %d", c)
	}

	// Second facteur : après le mot de passe, la passkey suffit (simple toucher permis).
	y := nouveauNavigateur(t, srv)
	if p := connexionMotDePasse(t, y, "marie", "un grand nid douillet"); !strings.Contains(p, "Utiliser ma passkey") {
		t.Fatal("passkey non proposée en second facteur")
	}
	_, opts = y.json("GET", "/connexion/verification/defi", nil)
	cle.Compteur++
	client, aut, sig = cle.Signer(opts["challenge"])
	if c, v := y.json("POST", "/connexion/verification/passkey", map[string]string{"id": b64t(cle.Identifiant), "clientDataJSON": client, "authenticatorData": aut, "signature": sig}); c != 200 {
		t.Fatalf("second facteur par passkey : %d %v", c, v)
	}
	if _, page := y.get("/"); !strings.Contains(page, "Bonjour Marie") {
		t.Fatal("pas de session")
	}
}

func TestReinitialisationParUnParent(t *testing.T) {
	_, srv, marie := familleTest(t)
	marie.get("/famille")
	_, page := marie.post("/famille/invitations", map[string]string{"role": "parent"})
	lien := regexp.MustCompile(`/invitation/[A-Za-z0-9_-]{43}`).FindString(page)
	thomas := nouveauNavigateur(t, srv)
	thomas.post(lien, map[string]string{"prenom": "Thomas", "identifiant": "thomas", "mdp": "le papa du nid", "mdp2": "le papa du nid"})
	ajouterPasskeyTest(t, thomas)
	x := nouveauNavigateur(t, srv)
	if p := connexionMotDePasse(t, x, "thomas", "le papa du nid"); !strings.Contains(p, "Vérification") {
		t.Fatal("second facteur attendu")
	}
	// Thomas a perdu son téléphone : Marie réinitialise.
	_, page = marie.get("/famille")
	id := regexp.MustCompile(`/famille/membres/(\d+)/securite`).FindStringSubmatch(page)
	if id == nil {
		t.Fatal("bouton de réinitialisation absent")
	}
	marie.post("/famille/membres/"+id[1]+"/securite", nil)
	// Thomas est déconnecté partout, puis se reconnecte avec son seul mot de passe.
	if _, p := thomas.get("/"); strings.Contains(p, "Bonjour Thomas") {
		t.Fatal("sessions de Thomas encore ouvertes")
	}
	y := nouveauNavigateur(t, srv)
	if p := connexionMotDePasse(t, y, "thomas", "le papa du nid"); !strings.Contains(p, "Bonjour Thomas") {
		t.Fatal("réinitialisation sans effet")
	}
}
