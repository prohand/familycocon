// SPDX-License-Identifier: GPL-3.0-or-later

package foyer

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/budget"
	"familycocon/internal/contacts"
	"familycocon/internal/essai"
	"familycocon/internal/suivi"
)

func brancher(a *appli.App) {
	contacts.Brancher(a)
	Brancher(a, suivi.Brancher(a))
}

func TestVehicule(t *testing.T) {
	_, srv := essai.NouvelleApp(t, brancher)
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")

	if _, page := marie.Get("/vehicules"); !strings.Contains(page, "contrôle technique") {
		t.Fatal("liste vide sans texte d'aide")
	}
	marie.Get("/vehicules/nouveau")
	code, page := marie.Post("/vehicules", map[string]string{"nom": "La Clio", "genre": "voiture", "immatriculation": "ab-123-cd", "kilometrage": "85 000"})
	if code != 200 || !strings.Contains(page, "AB-123-CD") || !strings.Contains(page, "85\u202f000 km") {
		t.Fatalf("création : %d\n%s", code, page)
	}
	id := essai.Trouver(t, page, `action="/vehicules/(\d+)/km"`)

	// Échéance dans 10 jours, chaque année → alerte « proche ».
	jour := time.Now().AddDate(0, 0, 10).Format("2006-01-02")
	if code, _ := marie.Post("/suivi/vehicules/"+id+"/echeances", map[string]string{"libelle": "Assurance", "jour": jour, "repetition_mois": "12"}); code != 200 {
		t.Fatalf("échéance : %d", code)
	}
	if _, page := marie.Get("/vehicules"); !strings.Contains(page, "⏰ Assurance") {
		t.Fatalf("alerte absente :\n%s", page)
	}
	if _, page := marie.Get("/"); !strings.Contains(page, "À venir") || !strings.Contains(page, "Assurance") {
		t.Fatal("échéance absente de « À venir »")
	}
	// Objet d'un autre module : refusé.
	if code, _ := marie.Post("/suivi/animaux/"+id+"/echeances", map[string]string{"libelle": "X", "jour": jour}); code != http.StatusNotFound {
		t.Fatalf("mauvais module accepté : %d", code)
	}

	// « C'est fait » : carnet, kilométrage, et report d'un an.
	_, page = marie.Get("/vehicules/" + id)
	ech := essai.Trouver(t, page, `action="/suivi/echeances/(\d+)/faite"`)
	aujourdhui := time.Now().Format("2006-01-02")
	code, page = lea.Post("/suivi/echeances/"+ech+"/faite", map[string]string{"jour": aujourdhui, "montant": "420,50", "km": "86000"})
	if code != 200 || !strings.Contains(page, "86\u202f000 km") || !strings.Contains(page, "tous les 12 mois") {
		t.Fatalf("faite : %d\n%s", code, page)
	}
	// Un enfant ne saisit ni ne voit les montants (le budget lui est fermé).
	if strings.Contains(page, "420,50") || strings.Contains(page, `name="montant"`) {
		t.Fatal("coût visible ou saisissable par un enfant")
	}
	if _, p := marie.Get("/vehicules/" + id); strings.Contains(p, "420,50") {
		t.Fatal("coût saisi par un enfant enregistré")
	}
	if _, page := marie.Get("/vehicules"); strings.Contains(page, "⏰") {
		t.Fatal("alerte toujours là après report")
	}
	// Le compteur ne recule pas.
	if code, p := marie.Post("/vehicules/"+id+"/km", map[string]string{"km": "1000"}); code != http.StatusBadRequest || !strings.Contains(p, "déjà") {
		t.Fatalf("kilométrage plus bas annoncé comme enregistré : %d", code)
	}
	if _, page := marie.Get("/vehicules/" + id); !strings.Contains(page, "86\u202f000 km") {
		t.Fatal("le kilométrage a reculé")
	}

	// Suppression : enfant refusé, parent accepté (avec le suivi).
	if code, _ := lea.Post("/vehicules/"+id+"/supprimer", nil); code != http.StatusForbidden {
		t.Fatalf("suppression enfant : %d", code)
	}
	if code, _ := marie.Post("/vehicules/"+id+"/supprimer", nil); code != 200 {
		t.Fatalf("suppression : %d", code)
	}
	if code, _ := marie.Get("/vehicules/" + id); code != http.StatusNotFound {
		t.Fatalf("toujours là : %d", code)
	}
	if _, page := marie.Get("/"); strings.Contains(page, "Assurance") {
		t.Fatal("échéance restée après suppression")
	}
}

func TestAnimalEtPrestataire(t *testing.T) {
	_, srv := essai.NouvelleApp(t, brancher)
	marie := essai.Famille(t, srv)

	marie.Get("/animaux/nouveau")
	code, page := marie.Post("/animaux", map[string]string{"nom": "Filou", "espece": "chat", "race": "Européen",
		"naissance": time.Now().AddDate(-3, -1, 0).Format("2006-01-02"), "puce": "250269812345678"})
	if code != 200 || !strings.Contains(page, "🐈 Filou") || !strings.Contains(page, "3 ans") || !strings.Contains(page, "Carnet de santé") {
		t.Fatalf("animal : %d\n%s", code, page)
	}
	id := essai.Trouver(t, page, `action="/suivi/animaux/(\d+)/carnet"`)
	if code, _ := marie.Post("/animaux", map[string]string{"nom": "X", "naissance": "2999-01-01"}); code != http.StatusBadRequest {
		t.Fatalf("naissance future acceptée : %d", code)
	}
	// Ajout au carnet, puis un objet inconnu est refusé.
	if code, page := marie.Post("/suivi/animaux/"+id+"/carnet", map[string]string{"libelle": "Vaccin", "jour": "2026-01-15", "montant": "65"}); code != 200 || !strings.Contains(page, "Vaccin") {
		t.Fatalf("carnet : %d", code)
	}
	if code, _ := marie.Post("/suivi/animaux/999/carnet", map[string]string{"libelle": "X", "jour": "2026-01-01"}); code != http.StatusNotFound {
		t.Fatalf("objet inconnu : %d", code)
	}
	// Sans session : rien.
	x := essai.NouveauNavigateur(t, srv)
	if _, page := x.Get("/animaux/" + id); strings.Contains(page, "Filou") {
		t.Fatal("fiche visible sans session")
	}

	// Prestataire = contact « pro » du carnet.
	marie.Get("/contacts/nouveau?pro=1")
	code, page = marie.Post("/contacts", map[string]string{"nom": "Plomberie Martin", "metier": "Plombier", "pro": "1",
		"telephone": "06 12 34 56 78", "site": "plomberie.example", "adresse": "3 rue des Lilas", "relation": "pro"})
	if code != 200 {
		t.Fatalf("création du contact pro : %d", code)
	}
	idPro := essai.Trouver(t, page, `href="/prestataires/(\d+)"`)
	_, page = marie.Get("/prestataires/" + idPro)
	if !strings.Contains(page, `href="tel:0612345678"`) || !strings.Contains(page, `href="https://plomberie.example"`) ||
		!strings.Contains(page, "query=3%20rue%20des%20Lilas") || !strings.Contains(page, "Interventions") {
		t.Fatalf("fiche prestataire :\n%s", page)
	}
	if _, page := marie.Get("/prestataires"); !strings.Contains(page, "Plomberie Martin") {
		t.Fatal("prestataire absent de la liste")
	}
	if code, _ := marie.Post("/contacts", map[string]string{"nom": "X", "site": "javascript:alert(1)"}); code != http.StatusBadRequest {
		t.Fatalf("site javascript: accepté : %d", code)
	}

	// Vétérinaire choisi dans le carnet : appel direct depuis la fiche de l'animal.
	_, page = marie.Post("/contacts", map[string]string{"nom": "Dr Duval", "metier": "Vétérinaire", "pro": "1", "telephone": "02 99 00 11 22"})
	idVeto := essai.Trouver(t, page, `action="/contacts/(\d+)/favori"`)
	marie.Get("/animaux/" + id + "/modifier")
	_, page = marie.Post("/animaux/"+id, map[string]string{"nom": "Filou", "espece": "chat", "veterinaire_id": idVeto})
	if !strings.Contains(page, "Dr Duval") || !strings.Contains(page, `href="tel:0299001122"`) || !strings.Contains(page, `href="/contacts/`+idVeto+`"`) {
		t.Fatalf("vétérinaire absent de la fiche :\n%s", page)
	}
	// Contact d'une autre famille ou inconnu : ignoré.
	if _, page := marie.Post("/animaux/"+id, map[string]string{"nom": "Filou", "espece": "chat", "veterinaire_id": "9999"}); strings.Contains(page, "Dr Duval") {
		t.Fatal("contact inconnu accepté")
	}
}

// Carnet du véhicule : dépense ajoutée aussi au budget (et supprimée avec
// la ligne), pleins et consommation, coût au km, ligne corrigée,
// échéance modifiée et reportée d'un mois.
func TestCarnetAmeliore(t *testing.T) {
	_, srv := essai.NouvelleApp(t, func(a *appli.App) {
		s := suivi.Brancher(a)
		Brancher(a, s)
		budget.Brancher(a)
	})
	marie := essai.Famille(t, srv)
	_, page := marie.Post("/vehicules", map[string]string{"nom": "La Clio", "genre": "voiture", "kilometrage": "10000"})
	id := essai.Trouver(t, page, `action="/vehicules/(\d+)/km"`)
	if !strings.Contains(page, "Ajouter aussi au budget") {
		t.Fatal("case budget absente pour un parent")
	}
	carnet := "/suivi/vehicules/" + id + "/carnet"
	marie.Post(carnet, map[string]string{"libelle": "Plein", "jour": "2026-11-01", "montant": "60", "km": "10000", "litres": "40"})
	marie.Post(carnet, map[string]string{"libelle": "Plein", "jour": "2026-11-10", "montant": "45", "km": "10500", "litres": "30", "budget": "1"})
	_, page = marie.Get("/vehicules/" + id)
	if !strings.Contains(page, "6,0 L / 100 km") || !strings.Contains(page, "21,00") || !strings.Contains(page, "pour 100 km") {
		t.Fatalf("consommation ou coût au km :\n%s", page)
	}
	if _, b := marie.Get("/budget?mois=2026-11"); !strings.Contains(b, "Plein · La Clio") || !strings.Contains(b, "45,00") {
		t.Fatalf("dépense absente du budget :\n%s", b)
	}
	// Corriger la ligne : le budget suit.
	ligne := essai.Trouver(t, page, `action="/suivi/carnet/(\d+)"`)
	marie.Post("/suivi/carnet/"+ligne, map[string]string{"libelle": "Plein", "jour": "2026-11-10", "montant": "47", "km": "10500", "litres": "30"})
	if _, b := marie.Get("/budget?mois=2026-11"); !strings.Contains(b, "47,00") {
		t.Fatal("budget non corrigé avec la ligne")
	}
	// Supprimer la ligne : la dépense part aussi.
	marie.Post("/suivi/carnet/"+ligne+"/supprimer", nil)
	if _, b := marie.Get("/budget?mois=2026-11"); strings.Contains(b, "Plein · La Clio") {
		t.Fatal("dépense restée au budget")
	}
	// Échéance : modifiée puis reportée d'un mois.
	marie.Post("/suivi/vehicules/"+id+"/echeances", map[string]string{"libelle": "Vidange", "jour": "2027-01-31", "repetition_mois": "12"})
	_, page = marie.Get("/vehicules/" + id)
	ech := essai.Trouver(t, page, `action="/suivi/echeances/(\d+)/reporter"`)
	marie.Post("/suivi/echeances/"+ech, map[string]string{"libelle": "Vidange moteur", "jour": "2027-01-31", "repetition_mois": "24"})
	marie.Post("/suivi/echeances/"+ech+"/reporter", nil)
	if _, page = marie.Get("/vehicules/" + id); !strings.Contains(page, "Vidange moteur") || !strings.Contains(page, `value="2027-02-28"`) || !strings.Contains(page, "tous les 24 mois") {
		t.Fatalf("échéance modifiée et reportée :\n%s", page)
	}
}

// Garanties : la fin de garantie devient une échéance (À venir), suit la
// date d'achat quand on la change, et part avec l'achat.
func TestGaranties(t *testing.T) {
	_, srv := essai.NouvelleApp(t, func(a *appli.App) {
		s := suivi.Brancher(a)
		Brancher(a, s)
	})
	marie := essai.Famille(t, srv)
	paris, _ := time.LoadLocation("Europe/Paris")
	achat := time.Now().In(paris).AddDate(-2, 0, 5).Format("2006-01-02") // fin dans 5 jours
	code, page := marie.Post("/garanties", map[string]string{"nom": "Lave-linge", "marque": "Bosch", "achat": achat, "garantie_mois": "24", "facture": "classeur bleu"})
	if code != 200 || !strings.Contains(page, "Garanti jusqu") || !strings.Contains(page, "classeur bleu") || !strings.Contains(page, "Fin de garantie") {
		t.Fatalf("fiche :\n%s", page)
	}
	if _, page = marie.Get("/"); !strings.Contains(page, "Fin de garantie · Lave-linge") {
		t.Fatal("fin de garantie absente de « À venir »")
	}
	id := essai.Trouver(t, page, `href="/garanties/(\d+)"`)
	// Plus de garantie : l'échéance disparaît.
	marie.Post("/garanties/"+id, map[string]string{"nom": "Lave-linge", "achat": achat, "garantie_mois": "0"})
	if _, page = marie.Get("/garanties/" + id); strings.Contains(page, "Fin de garantie") {
		t.Fatal("échéance gardée sans garantie")
	}
	if _, page = marie.Get("/chercher?q=bosch"); strings.Contains(page, "Lave-linge") {
		t.Fatal("marque effacée ne devrait plus être trouvée")
	}
}

// Photo d'un ticket jointe à une ligne du carnet : gardée sur le disque,
// vue par les parents seulement, effacée avec la ligne.
func TestPiecesJointes(t *testing.T) {
	a, srv := essai.NouvelleApp(t, func(a *appli.App) {
		s := suivi.Brancher(a)
		Brancher(a, s)
	})
	marie := essai.Famille(t, srv)
	lea := essai.Enfant(t, srv, marie, "Léa")
	_, page := marie.Post("/vehicules", map[string]string{"nom": "Clio", "genre": "voiture"})
	id := essai.Trouver(t, page, `action="/vehicules/(\d+)/km"`)
	marie.Post("/suivi/vehicules/"+id+"/carnet", map[string]string{"libelle": "Vidange", "jour": "2026-11-01", "montant": "89"})
	_, page = marie.Get("/vehicules/" + id)
	ligne := essai.Trouver(t, page, `data-piece="/suivi/carnet/(\d+)/pieces"`)
	img := image.NewRGBA(image.Rect(0, 0, 800, 600))
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	envoyer := func(n *essai.Navigateur, corps []byte) int {
		req, _ := http.NewRequest("POST", srv.URL+"/suivi/carnet/"+ligne+"/pieces", bytes.NewReader(corps))
		req.Header.Set("Content-Type", "image/png")
		req.Header.Set("X-Jeton", n.Jeton)
		rep, err := n.C.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		rep.Body.Close()
		return rep.StatusCode
	}
	lea.Get("/vehicules/" + id)
	if code := envoyer(lea, b.Bytes()); code != 403 {
		t.Fatalf("photo envoyée par un enfant : %d", code)
	}
	if code := envoyer(marie, []byte("pas une image")); code != 400 {
		t.Fatalf("fichier quelconque accepté : %d", code)
	}
	if code := envoyer(marie, b.Bytes()); code != 200 {
		t.Fatalf("envoi : %d", code)
	}
	_, page = marie.Get("/vehicules/" + id)
	piece := essai.Trouver(t, page, `<img src="/suivi/pieces/(\d+)"`)
	if code, corps := marie.Get("/suivi/pieces/" + piece); code != 200 || !strings.HasPrefix(corps, "\xff\xd8") {
		t.Fatalf("photo : %d", code)
	}
	if code, _ := lea.Get("/suivi/pieces/" + piece); code != 403 {
		t.Fatalf("photo vue par un enfant : %d", code)
	}
	fichiers, _ := filepath.Glob(filepath.Join(a.Config.Donnees, "pieces", "*", "*.jpg"))
	if len(fichiers) != 1 {
		t.Fatalf("%d fichiers sur le disque", len(fichiers))
	}
	marie.Post("/suivi/carnet/"+ligne+"/supprimer", nil)
	if fichiers, _ = filepath.Glob(filepath.Join(a.Config.Donnees, "pieces", "*", "*.jpg")); len(fichiers) != 0 {
		t.Fatal("fichier gardé après la suppression de la ligne")
	}
}
