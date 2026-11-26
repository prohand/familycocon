// SPDX-License-Identifier: GPL-3.0-or-later

package courses

import (
	"bufio"
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"familycocon/internal/essai"
)

// « Je pars faire les courses » : la famille est prévenue (lien de la
// liste), une seule fois toutes les 30 minutes par liste.
func TestJeParsFaireLesCourses(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	_, page := marie.Get("/courses")
	id := essai.Trouver(t, page, `/courses/liste/(\d+)/produits`)
	if !strings.Contains(page, "Je pars faire les courses") {
		t.Fatal("bouton « Je pars faire les courses » absent")
	}
	paul := essai.Enfant(t, srv, marie, "Paul")
	paul.Get("/courses")

	// Marie écoute le direct (sa page est ouverte).
	ctx, annuler := context.WithTimeout(context.Background(), 5*time.Second)
	defer annuler()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/direct", nil)
	rep, err := marie.C.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer rep.Body.Close()
	lecteur := bufio.NewReader(rep.Body)
	for i := 0; i < 100 && a.Direct.Nombre() == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}

	paul.Htmx = true
	code, _ := paul.Post("/courses/liste/"+id+"/depart", nil)
	if code != http.StatusNoContent || !strings.Contains(paul.EnTetes.Get("HX-Trigger"), "envoy") {
		t.Fatalf("départ : %d %s", code, paul.EnTetes.Get("HX-Trigger"))
	}
	for {
		ligne, err := lecteur.ReadString('\n')
		if err != nil {
			t.Fatalf("pas de message reçu : %v", err)
		}
		if strings.HasPrefix(ligne, "data: ") {
			if !strings.Contains(ligne, "Paul part faire les courses : ajoutez vite ce qui manque") {
				t.Fatalf("message inattendu : %s", ligne)
			}
			break
		}
	}
	// Deuxième appel (même par un autre membre) : refusé, message clair.
	marie.Htmx = true
	if code, _ := marie.Post("/courses/liste/"+id+"/depart", nil); code != http.StatusNoContent ||
		!strings.Contains(marie.EnTetes.Get("HX-Trigger"), "30 minutes") || !strings.Contains(marie.EnTetes.Get("HX-Trigger"), "par Paul") {
		t.Fatalf("second départ : %d %s", code, marie.EnTetes.Get("HX-Trigger"))
	}
	// Une autre liste : son propre appel.
	marie.Htmx = false
	marie.Post("/courses/listes", map[string]string{"nom": "Drive"})
	var drive string
	if err := a.DB.QueryRow(`SELECT id FROM listes WHERE nom = 'Drive'`).Scan(&drive); err != nil {
		t.Fatal(err)
	}
	marie.Htmx = true
	if marie.Post("/courses/liste/"+drive+"/depart", nil); !strings.Contains(marie.EnTetes.Get("HX-Trigger"), "envoy") {
		t.Fatalf("départ pour le Drive : %s", marie.EnTetes.Get("HX-Trigger"))
	}
	// Sans htmx : retour à la liste.
	marie.Htmx = false
	if code, page := marie.Post("/courses/liste/"+drive+"/depart", nil); code != 200 || !strings.Contains(page, "<html") {
		t.Fatalf("départ sans htmx : %d", code)
	}
}

// Garde-fou : 30 minutes par liste, puis l'appel est de nouveau possible ;
// les vieux appels sont oubliés.
func TestDepartsReserver(t *testing.T) {
	var d departs
	t0 := time.Date(2026, 11, 26, 10, 0, 0, 0, time.UTC)
	if _, ok := d.reserver(1, "Paul", t0); !ok {
		t.Fatal("premier appel refusé")
	}
	if avant, ok := d.reserver(1, "Marie", t0.Add(29*time.Minute)); ok || avant.qui != "Paul" {
		t.Fatal("second appel accepté avant 30 minutes")
	}
	if _, ok := d.reserver(2, "Marie", t0.Add(29*time.Minute)); !ok {
		t.Fatal("autre liste refusée")
	}
	if _, ok := d.reserver(1, "Marie", t0.Add(30*time.Minute)); !ok {
		t.Fatal("appel refusé après 30 minutes")
	}
	if _, ok := d.reserver(3, "Léa", t0.Add(2*time.Hour)); !ok || len(d.liste) != 1 {
		t.Fatalf("vieux appels gardés : %d", len(d.liste))
	}
}

// Rayon fini : replié, en bas, avec ✓ ; il se rouvre quand on décoche.
func TestRayonFini(t *testing.T) {
	_, n, id := nouvelleFamille(t)
	n.Htmx = true
	_, frag := n.Post("/courses/liste/"+id+"/produits", map[string]string{"produit": "2 baguettes"})
	pain := essai.Trouver(t, frag, `/courses/produit/(\d+)/cocher`)
	_, frag = n.Post("/courses/liste/"+id+"/produits", map[string]string{"produit": "Lait"})
	if strings.Index(frag, `data-rayon="boulangerie"`) > strings.Index(frag, `data-rayon="frais"`) {
		t.Fatal("ordre du magasin attendu : boulangerie avant frais")
	}
	_, frag = n.Post("/courses/produit/"+pain+"/cocher", map[string]string{"pris": "1"})
	if !strings.Contains(frag, `class="rayon fini" data-rayon="boulangerie" data-fini>`) || !strings.Contains(frag, `<span class="r-emoji">✓</span>Boulangerie<span class="compte">1/1</span>`) {
		t.Fatalf("rayon fini non replié :\n%s", frag)
	}
	if strings.Index(frag, `data-rayon="boulangerie"`) < strings.Index(frag, `data-rayon="frais"`) {
		t.Fatal("rayon fini pas en bas")
	}
	// En modification, un produit d'un rayon fini reste visible.
	if _, frag = n.Get("/courses/produit/" + pain + "/modifier"); !strings.Contains(frag, `data-fini open>`) {
		t.Fatal("rayon fini fermé pendant la modification")
	}
	_, frag = n.Post("/courses/produit/"+pain+"/cocher", map[string]string{"pris": "0"})
	if strings.Contains(frag, "data-fini") || !strings.Contains(frag, `data-rayon="boulangerie" open>`) {
		t.Fatalf("rayon décoché toujours replié :\n%s", frag)
	}
}

// Un produit change de liste (« Courses » → « Drive ») ; déjà sur l'autre
// liste : une seule ligne, quantités réunies.
func TestDeplacerProduit(t *testing.T) {
	a, srv := essai.NouvelleApp(t, Brancher)
	marie := essai.Famille(t, srv)
	_, page := marie.Get("/courses")
	id := essai.Trouver(t, page, `/courses/liste/(\d+)/produits`)
	lea := essai.Enfant(t, srv, marie, "Léa")
	lea.Get("/courses")
	lea.Htmx = true
	_, frag := lea.Post("/courses/liste/"+id+"/produits", map[string]string{"produit": "2 lait"})
	lait := essai.Trouver(t, frag, `/courses/produit/(\d+)/cocher`)
	// Une seule liste : pas de choix.
	if _, frag := lea.Get("/courses/produit/" + lait + "/modifier"); strings.Contains(frag, `name="liste"`) {
		t.Fatal("choix de la liste avec une seule liste")
	}
	marie.Post("/courses/listes", map[string]string{"nom": "Drive"})
	var drive string
	if err := a.DB.QueryRow(`SELECT id FROM listes WHERE nom = 'Drive'`).Scan(&drive); err != nil {
		t.Fatal(err)
	}
	if _, frag := lea.Get("/courses/produit/" + lait + "/modifier"); !strings.Contains(frag, `<option value="`+drive+`">Drive</option>`) {
		t.Fatalf("choix de la liste absent :\n%s", frag)
	}
	// Une liste d'une autre famille : refusée.
	var etrangere string
	if err := a.DB.QueryRow(`INSERT INTO familles (nom, cree_le) VALUES ('Autre', 0) RETURNING id`).Scan(&etrangere); err != nil {
		t.Fatal(err)
	}
	if err := a.DB.QueryRow(`INSERT INTO listes (famille_id, nom, cree_le) VALUES (?, 'Secret', 0) RETURNING id`, etrangere).Scan(&etrangere); err != nil {
		t.Fatal(err)
	}
	if code, _ := lea.Post("/courses/produit/"+lait, map[string]string{"nom": "Lait", "quantite": "2", "rayon": "frais", "liste": etrangere}); code != http.StatusBadRequest {
		t.Fatalf("liste d'une autre famille acceptée : %d", code)
	}
	// Un enfant déplace (mêmes droits que la modification).
	code, frag := lea.Post("/courses/produit/"+lait, map[string]string{"nom": "Lait", "quantite": "2", "rayon": "frais", "liste": drive})
	if code != 200 || strings.Contains(frag, "Lait") || !strings.Contains(lea.EnTetes.Get("HX-Trigger"), "Drive") {
		t.Fatalf("déplacement : %d %s\n%s", code, lea.EnTetes.Get("HX-Trigger"), frag)
	}
	if _, frag := lea.Get("/courses/liste/" + drive + "/contenu"); !strings.Contains(frag, "Lait") {
		t.Fatal("produit absent de la liste « Drive »")
	}
	// Déjà sur le Drive : quantités réunies, une seule ligne.
	_, frag = lea.Post("/courses/liste/"+id+"/produits", map[string]string{"produit": "3 lait"})
	lait2 := essai.Trouver(t, frag, `/courses/produit/(\d+)/cocher`)
	lea.Post("/courses/produit/"+lait2, map[string]string{"nom": "Lait", "quantite": "3", "rayon": "frais", "liste": drive})
	if _, frag := lea.Get("/courses/liste/" + drive + "/contenu"); strings.Count(frag, ">Lait<") != 1 || !strings.Contains(frag, "<small>5</small>") {
		t.Fatalf("fusion sur le Drive :\n%s", frag)
	}
	if _, frag := lea.Get("/courses/liste/" + id + "/contenu"); strings.Contains(frag, ">Lait<") {
		t.Fatal("produit fusionné resté sur la première liste")
	}
}
