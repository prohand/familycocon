// SPDX-License-Identifier: GPL-3.0-or-later

package courses

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/essai"
)

func nouvelleFamille(t *testing.T) (*appli.App, *essai.Navigateur, string) {
	t.Helper()
	a, srv := essai.NouvelleApp(t, Brancher)
	n := essai.Famille(t, srv)
	code, page := n.Get("/courses")
	if code != 200 || !strings.Contains(page, "La liste est vide") {
		t.Fatalf("page courses : %d", code)
	}
	id := essai.Trouver(t, page, `/courses/liste/(\d+)/produits`)
	return a, n, id
}

func TestAjouterCocherVider(t *testing.T) {
	_, n, id := nouvelleFamille(t)
	n.Htmx = true
	code, frag := n.Post("/courses/liste/"+id+"/produits", map[string]string{"produit": "2 baguettes"})
	if code != 200 || !strings.Contains(frag, "Baguettes") || !strings.Contains(frag, "🥖") || strings.Contains(frag, "<html") {
		t.Fatalf("ajout : %d\n%s", code, frag)
	}
	n.Post("/courses/liste/"+id+"/produits", map[string]string{"produit": "1 kg de pommes"})
	// Rayon choisi à la main : retenu pour la prochaine fois.
	n.Post("/courses/liste/"+id+"/produits", map[string]string{"produit": "Truc mystère", "rayon": "maison"})
	_, frag = n.Post("/courses/liste/"+id+"/produits", map[string]string{"produit": "truc mystère"})
	if strings.Count(frag, `data-rayon="maison"`) != 1 || strings.Contains(frag, `data-rayon="autre"`) {
		t.Fatal("rayon choisi non retenu")
	}
	pid := essai.Trouver(t, frag, `/courses/produit/(\d+)/cocher`)
	if code, frag := n.Post("/courses/produit/"+pid+"/cocher", map[string]string{"pris": "1"}); code != 200 || !strings.Contains(frag, "1 pris sur 4") {
		t.Fatalf("cocher : %d\n%s", code, frag)
	}
	// Cocher deux fois « pris » ne décoche pas.
	if _, frag := n.Post("/courses/produit/"+pid+"/cocher", map[string]string{"pris": "1"}); !strings.Contains(frag, "1 pris sur 4") {
		t.Fatal("double clic : état inversé")
	}
	// L'accueil résume.
	n.Htmx = false
	if _, page := n.Get("/"); !strings.Contains(page, "3 produits à prendre") {
		t.Fatal("résumé de la tuile absent")
	}
	n.Htmx = true
	if _, frag := n.Post("/courses/liste/"+id+"/vider", nil); !strings.Contains(frag, "0 pris sur 3") {
		t.Fatalf("vider :\n%s", frag)
	}
}

func TestModifierProduit(t *testing.T) {
	_, n, id := nouvelleFamille(t)
	n.Htmx = true
	_, frag := n.Post("/courses/liste/"+id+"/produits", map[string]string{"produit": "Saumon"})
	pid := essai.Trouver(t, frag, `/courses/produit/(\d+)/cocher`)
	if _, frag := n.Get("/courses/produit/" + pid + "/modifier"); !strings.Contains(frag, `class="edition-produit"`) || strings.Contains(frag, "maj-courses") {
		t.Fatal("formulaire de modification absent (ou rechargement automatique actif)")
	}
	if _, frag := n.Post("/courses/produit/"+pid, map[string]string{"nom": "Saumon fumé", "quantite": "4 tranches", "rayon": "frais"}); !strings.Contains(frag, "Saumon fumé") || !strings.Contains(frag, `data-rayon="frais"`) {
		t.Fatalf("modification :\n%s", frag)
	}
	if code, _ := n.Post("/courses/produit/"+pid, map[string]string{"nom": "X", "rayon": "lune"}); code != 400 {
		t.Fatalf("rayon inconnu accepté : %d", code)
	}
}

func TestEnfantNeSupprimePas(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	parent := essai.Famille(t, srv)
	_, page := parent.Get("/courses")
	id := essai.Trouver(t, page, `/courses/liste/(\d+)/produits`)
	lea := essai.Enfant(t, srv, parent, "Léa")
	lea.Get("/courses")
	lea.Htmx = true
	code, frag := lea.Post("/courses/liste/"+id+"/produits", map[string]string{"produit": "Chocolat"})
	if code != 200 || !strings.Contains(frag, "Chocolat") {
		t.Fatalf("un enfant doit pouvoir ajouter : %d", code)
	}
	pid := essai.Trouver(t, frag, `/courses/produit/(\d+)/cocher`)
	if code, _ := lea.Post("/courses/produit/"+pid+"/supprimer", nil); code != http.StatusForbidden {
		t.Fatalf("suppression par un enfant : %d", code)
	}
	if code, _ := lea.Post("/courses/liste/"+id+"/vider", nil); code != http.StatusForbidden {
		t.Fatalf("vidage par un enfant : %d", code)
	}
	if _, frag := lea.Get("/courses/produit/" + pid + "/modifier"); strings.Contains(frag, "/supprimer") {
		t.Fatal("bouton Supprimer montré à un enfant")
	}
}

func TestAutreFamilleIsolee(t *testing.T) {
	_, srv := essai.NouvelleApp(t, Brancher)
	n := essai.Famille(t, srv)
	_, page := n.Get("/courses")
	id := essai.Trouver(t, page, `/courses/liste/(\d+)/produits`)
	n.Htmx = true
	_, frag := n.Post("/courses/liste/"+id+"/produits", map[string]string{"produit": "Pain"})
	pid := essai.Trouver(t, frag, `/courses/produit/(\d+)/cocher`)
	// Un inconnu (sans session) ne peut rien faire.
	x := essai.NouveauNavigateur(t, srv)
	x.Htmx = true
	if code, _ := x.Post("/courses/produit/"+pid+"/cocher", map[string]string{"pris": "1"}); code != http.StatusUnauthorized {
		t.Fatalf("sans session : %d", code)
	}
}

func TestModuleDesactive(t *testing.T) {
	_, n, _ := nouvelleFamille(t)
	n.Get("/famille")
	n.Post("/famille/modules", map[string]string{"module_agenda": "1"})
	if code, _ := n.Get("/courses"); code != http.StatusNotFound {
		t.Fatalf("module désactivé accessible : %d", code)
	}
}

// Temps réel : Thomas a la page ouverte, Marie ajoute du pain.
func TestTempsReel(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	_, page := marie.Get("/courses")
	id := essai.Trouver(t, page, `/courses/liste/(\d+)/produits`)

	ctx, annuler := context.WithTimeout(context.Background(), 5*time.Second)
	defer annuler()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/direct", nil)
	rep, err := marie.C.Do(req) // même famille (ici, même navigateur)
	if err != nil {
		t.Fatal(err)
	}
	defer rep.Body.Close()
	if rep.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("type %q", rep.Header.Get("Content-Type"))
	}
	lecteur := bufio.NewReader(rep.Body)
	// Attendre l'abonnement avant de publier.
	for i := 0; i < 100 && a.Direct.Nombre() == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	marie.Htmx = true
	marie.Post("/courses/liste/"+id+"/produits", map[string]string{"produit": "pain"})
	for {
		ligne, err := lecteur.ReadString('\n')
		if err != nil {
			t.Fatalf("pas de message reçu : %v", err)
		}
		if strings.HasPrefix(ligne, "data: ") {
			if !strings.Contains(ligne, `"module":"courses"`) || !strings.Contains(ligne, "Marie a ajouté « Pain » 🥖") {
				t.Fatalf("message inattendu : %s", ligne)
			}
			break
		}
	}
}

// Revue : messages et plafonds.
func TestBornesCourses(t *testing.T) {
	a, n, id := nouvelleFamille(t)
	// Quantité trop longue : le message parle bien de la quantité.
	n.Htmx = true
	req := func(champs map[string]string) (int, string) { return n.Post("/courses/liste/"+id+"/produits", champs) }
	if code, _ := req(map[string]string{"produit": strings.Repeat("9", 25) + " kg de farine"}); code != http.StatusBadRequest {
		t.Fatalf("quantité de 25 chiffres acceptée : %d", code)
	}
	if !SaisieValide("2 kg de farine") || SaisieValide(strings.Repeat("a", 81)) || SaisieValide("  ") {
		t.Fatal("SaisieValide")
	}
	// Liste pleine : refus clair.
	if _, err := a.DB.Exec(`WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM n WHERE i < ?)
		INSERT INTO produits (liste_id, nom, quantite, rayon, emoji, cree_le) SELECT ?, 'p' || i, '', 'autre', '', 0 FROM n`, produitsMax, id); err != nil {
		t.Fatal(err)
	}
	if code, _ := req(map[string]string{"produit": "Pain"}); code != http.StatusBadRequest {
		t.Fatalf("liste pleine : %d", code)
	}
	// 50 listes au plus (une existe déjà).
	n.Htmx = false
	for i := 1; i < listesMax; i++ {
		if code, _ := n.Post("/courses/listes", map[string]string{"nom": "Liste"}); code != 200 {
			t.Fatalf("liste %d : %d", i+1, code)
		}
	}
	if code, _ := n.Post("/courses/listes", map[string]string{"nom": "De trop"}); code != http.StatusBadRequest {
		t.Fatalf("51e liste acceptée : %d", code)
	}
	// « Ranger » sans htmx : retour à la page, pas un bloc nu.
	if code, page := n.Post("/courses/liste/"+id+"/vider", nil); code != 200 || !strings.Contains(page, "<html") {
		t.Fatalf("vider sans htmx : %d", code)
	}
}
