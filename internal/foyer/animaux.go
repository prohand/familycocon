// SPDX-License-Identifier: GPL-3.0-or-later

package foyer

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"familycocon/internal/appli"
	"familycocon/internal/donnees"
	"familycocon/internal/suivi"
)

const modAnimaux = "animaux"

// Especes proposées.
var Especes = []struct{ Cle, Nom, Emoji string }{
	{"chien", "Chien", "🐕"}, {"chat", "Chat", "🐈"}, {"lapin", "Lapin", "🐇"}, {"rongeur", "Rongeur", "🐹"},
	{"oiseau", "Oiseau", "🐦"}, {"poisson", "Poisson", "🐠"}, {"reptile", "Reptile", "🦎"}, {"cheval", "Cheval", "🐴"},
	{"autre", "Autre", "🐾"},
}

func emojiEspece(e string) string {
	for _, x := range Especes {
		if x.Cle == e {
			return x.Emoji
		}
	}
	return "🐾"
}

func (m *Module) brancherAnimaux() {
	a := m.a
	a.Route("GET /animaux", modAnimaux, appli.Membre, m.listeAnimaux)
	a.Route("GET /animaux/nouveau", modAnimaux, appli.Membre, m.formAnimal)
	a.Route("POST /animaux", modAnimaux, appli.Membre, m.enregistrerAnimal)
	a.Route("GET /animaux/{id}", modAnimaux, appli.Membre, m.ficheAnimal)
	a.AjouterRecherche(modAnimaux, m.chercherAnimaux)
	a.Route("GET /animaux/{id}/modifier", modAnimaux, appli.Membre, m.formAnimal)
	a.Route("POST /animaux/{id}", modAnimaux, appli.Membre, m.enregistrerAnimal)
	a.Route("POST /animaux/{id}/supprimer", modAnimaux, appli.Parent, m.supprimerAnimal)

	m.s.Enregistrer(suivi.Objet{
		Module: modAnimaux, Emoji: "🐾",
		Modeles: []suivi.Modele{{Libelle: "Vaccin", Mois: 12}, {Libelle: "Vermifuge", Mois: 3}, {Libelle: "Antiparasitaire", Mois: 1}, {Libelle: "Visite chez le vétérinaire", Mois: 12},
			{Libelle: "Toilettage", Mois: 0}, {Libelle: "Croquettes", Mois: 1}, {Libelle: "Assurance", Mois: 12}},
		Nom: func(ctx context.Context, famille, id int64) (string, bool) {
			x, err := a.Q.AnimalParID(ctx, donnees.AnimalParIDParams{ID: id, FamilleID: famille})
			return x.Nom, err == nil
		},
		Noms: func(ctx context.Context, famille int64) (map[int64]string, error) {
			l, err := a.Q.NomsAnimaux(ctx, famille)
			noms := make(map[int64]string, len(l))
			for _, x := range l {
				noms[x.ID] = x.Nom
			}
			return noms, err
		},
		URL: func(id int64) string { return fmt.Sprintf("/animaux/%d", id) },
	})
	a.AjouterResume(modAnimaux, func(ctx context.Context, c *appli.Contexte) appli.Resume {
		l, err := a.Q.AnimauxFamille(ctx, c.Famille.ID)
		if err != nil || len(l) == 0 {
			return appli.Resume{Info: "Aucun animal"}
		}
		alertes := m.alertes(ctx, c, modAnimaux)
		for _, x := range l {
			if al := alertes[x.ID]; al != "" {
				return appli.Resume{Info: al, Badge: 1}
			}
		}
		noms := l[0].Nom
		if len(l) > 1 {
			noms += fmt.Sprintf(" et %d autre(s)", len(l)-1)
		}
		return appli.Resume{Info: noms}
	})
	a.AjouterAction(appli.Ajout{Module: modAnimaux, Emoji: "🐾", Texte: "Un animal", URL: "/animaux/nouveau"})
}

func (m *Module) animal(r *http.Request, c *appli.Contexte) (donnees.Animal, bool) {
	x, err := m.a.Q.AnimalParID(r.Context(), donnees.AnimalParIDParams{ID: idDe(r), FamilleID: c.Famille.ID})
	return x, err == nil
}

func (m *Module) listeAnimaux(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	l, err := m.a.Q.AnimauxFamille(r.Context(), c.Famille.ID)
	if err != nil {
		m.a.ErreurInterne(w, r, err)
		return
	}
	v := vueListe{Titre: "Animaux", Emoji: "🐾", Module: modAnimaux, Nouveau: "/animaux/nouveau", TexteNouveau: "＋ Animal",
		Vide: "Ajoutez vos compagnons : vaccins, vermifuge et visites chez le vétérinaire seront rappelés."}
	alertes := m.alertes(r.Context(), c, modAnimaux)
	for _, x := range l {
		detail := x.Race
		if a := age(x.Naissance, c.Maintenant()); a != "" {
			if detail != "" {
				detail += " · "
			}
			detail += a
		}
		v.Elements = append(v.Elements, Element{URL: fmt.Sprintf("/animaux/%d", x.ID), Emoji: emojiEspece(x.Espece), Nom: x.Nom,
			Detail: detail, Alerte: alertes[x.ID]})
	}
	m.a.Afficher(w, r, c, "foyer/liste", "Animaux", modAnimaux, v)
}

func (m *Module) ficheAnimal(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	x, ok := m.animal(r, c)
	if !ok {
		m.a.PageErreur(w, r, c, http.StatusNotFound, "Animal introuvable", "Cet animal n'existe plus.")
		return
	}
	v := vueFiche{Retour: "/animaux", TitreRetour: "Animaux", Emoji: emojiEspece(x.Espece), Nom: x.Nom, Sous: x.Race,
		TitreSuivi: "Soins à venir", TitreCarnet: "Carnet de santé",
		Modifier: fmt.Sprintf("/animaux/%d/modifier", x.ID), Supprimer: fmt.Sprintf("/animaux/%d/supprimer", x.ID)}
	if a := age(x.Naissance, c.Maintenant()); a != "" {
		v.Infos = append(v.Infos, "🎂 "+a)
	}
	if x.Puce != "" {
		v.Infos = append(v.Infos, "🔖 Puce / tatouage : "+x.Puce)
	}
	if ct, ok := m.citer(r.Context(), c, "🩺 Vétérinaire", x.VeterinaireID); ok {
		v.Contacts = append(v.Contacts, ct)
	}
	if x.Veterinaire != "" {
		v.Infos = append(v.Infos, "🩺 "+x.Veterinaire)
	}
	if x.Notes != "" {
		v.Infos = append(v.Infos, "📝 "+x.Notes)
	}
	m.afficherFiche(w, r, c, modAnimaux, x.ID, v)
}

type formAnimal struct {
	Nouveau  bool
	A        donnees.Animal
	Especes  any
	Contacts []donnees.ContactsChoixRow
}

func (m *Module) formAnimal(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	f := formAnimal{Nouveau: r.PathValue("id") == "", Especes: Especes, A: donnees.Animal{Espece: "chien"}}
	if !f.Nouveau {
		x, ok := m.animal(r, c)
		if !ok {
			m.a.PageErreur(w, r, c, http.StatusNotFound, "Animal introuvable", "Cet animal n'existe plus.")
			return
		}
		f.A = x
	}
	f.Contacts = m.contactsChoix(r.Context(), c)
	m.a.Afficher(w, r, c, "foyer/animal", "Animal", modAnimaux, f)
}

func (m *Module) enregistrerAnimal(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	ctx := r.Context()
	nom, err1 := texte(r, "nom", 40)
	race, err2 := texte(r, "race", 60)
	puce, err3 := texte(r, "puce", 40)
	veto, err4 := texte(r, "veterinaire", 200)
	notes, err5 := texte(r, "notes", 1000)
	if err := firstErr(err1, err2, err3, err4, err5); err != nil || nom == "" {
		m.a.Echec(w, r, http.StatusBadRequest, "Nom (40 caractères) ou un autre champ invalide.")
		return
	}
	espece := r.PostFormValue("espece")
	if emojiEspece(espece) == "🐾" {
		espece = "autre"
	}
	naissance := r.PostFormValue("naissance")
	if t, err := time.Parse("2006-01-02", naissance); naissance != "" && (err != nil || t.After(time.Now()) || t.Year() < 1900) {
		m.a.Echec(w, r, http.StatusBadRequest, "Date de naissance invalide.")
		return
	}
	id := idDe(r)
	if r.PathValue("id") == "" {
		if n, err := m.a.Q.CompterAnimaux(ctx, c.Famille.ID); err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		} else if n >= ObjetsMax {
			m.a.Echec(w, r, http.StatusBadRequest, fmt.Sprintf("Trop d'animaux (%d au plus).", ObjetsMax))
			return
		}
		x, err := m.a.Q.CreerAnimal(ctx, donnees.CreerAnimalParams{FamilleID: c.Famille.ID, Nom: nom, Espece: espece, Race: race,
			Naissance: naissance, Puce: puce, Veterinaire: veto, VeterinaireID: m.lireContact(r, c, "veterinaire_id"), Notes: notes, CreeLe: time.Now().Unix()})
		if err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
		id = x.ID
	} else {
		if _, ok := m.animal(r, c); !ok {
			m.a.Echec(w, r, http.StatusNotFound, "Animal introuvable.")
			return
		}
		if err := m.a.Q.ModifierAnimal(ctx, donnees.ModifierAnimalParams{Nom: nom, Espece: espece, Race: race, Naissance: naissance,
			Puce: puce, Veterinaire: veto, VeterinaireID: m.lireContact(r, c, "veterinaire_id"), Notes: notes, ID: id, FamilleID: c.Famille.ID}); err != nil {
			m.a.ErreurInterne(w, r, err)
			return
		}
	}
	m.a.Publier(c, modAnimaux, "")
	m.a.Flash(w, "Fiche enregistrée 🐾")
	m.a.Rediriger(w, r, fmt.Sprintf("/animaux/%d", id))
}

func (m *Module) supprimerAnimal(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	x, ok := m.animal(r, c)
	if !ok {
		m.a.Echec(w, r, http.StatusNotFound, "Animal introuvable.")
		return
	}
	m.supprimerObjet(w, r, c, modAnimaux, x.ID, x.Nom, "/animaux", func(ctx context.Context, q *donnees.Queries) error {
		return q.SupprimerAnimal(ctx, donnees.SupprimerAnimalParams{ID: x.ID, FamilleID: c.Famille.ID})
	})
}
