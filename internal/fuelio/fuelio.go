// SPDX-License-Identifier: GPL-3.0-or-later

// Package fuelio : lire une sauvegarde de l'appli Fuelio (carnet de
// pleins et de dépenses d'un véhicule) pour la reprendre dans le carnet
// d'entretien. Bibliothèque standard seulement.
//
// Fichiers acceptés :
//   - la sauvegarde complète « .fuelio » (un ZIP) : un ou plusieurs
//     fichiers CSV, et les photos (tickets, factures) si elles y sont ;
//   - le ZIP de synchronisation « vehicle-N-sync.csv.zip » ;
//   - le CSV seul « vehicle-N-sync.csv ».
//
// Le CSV de Fuelio est fait de parties qui commencent par « ## Nom »
// (## Vehicle, ## Log, ## CostCategories, ## Costs…), chacune avec sa
// propre ligne de titres. Les colonnes sont retrouvées par leur titre, pas
// par leur place (elles changent selon les versions de Fuelio).
package fuelio

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"math"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Limites (garde-fous contre un fichier piégé ou démesuré).
const (
	LignesMax    = 20000    // lignes lues par véhicule
	ImagesMax    = 3000     // photos lues dans le ZIP
	ImageMax     = 16 << 20 // taille d'une photo une fois décompressée
	csvMax       = 16 << 20 // taille d'un CSV une fois décompressé
	VehiculesMax = 50
)

// ErrFormat : le fichier n'est pas une sauvegarde Fuelio lisible.
var ErrFormat = errors.New("fichier Fuelio illisible")

// Ligne : un plein ou une dépense, prêt pour le carnet.
type Ligne struct {
	Plein   bool   // plein (sinon dépense : entretien, péage…)
	Jour    string // AAAA-MM-JJ
	Libelle string
	Note    string
	Montant int64 // centimes
	Km      int64 // kilomètres (convertis si le compteur est en miles)
	Litres  int64 // centilitres (pleins)
	Cle     string
}

// Vehicule : un véhicule de la sauvegarde.
type Vehicule struct {
	Nom    string
	Lignes []Ligne
}

// Image : une photo trouvée dans le ZIP (lue plus tard, une à la fois).
type Image struct {
	Nom string // chemin dans le ZIP
	f   *zip.File
}

// Lire : le contenu de la photo (borné à ImageMax).
func (i Image) Lire() ([]byte, error) {
	if i.f.UncompressedSize64 > ImageMax {
		return nil, ErrFormat
	}
	r, err := i.f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	b, err := io.ReadAll(io.LimitReader(r, ImageMax+1))
	if err == nil && len(b) > ImageMax {
		err = ErrFormat
	}
	return b, err
}

// Sauvegarde : ce qui a été lu dans le fichier.
type Sauvegarde struct {
	Vehicules []Vehicule
	Images    []Image
	zip       *zip.ReadCloser
}

// Fermer : ferme le ZIP (les images ne sont plus lisibles ensuite).
func (s *Sauvegarde) Fermer() error {
	if s.zip != nil {
		return s.zip.Close()
	}
	return nil
}

// Ouvrir : lit le fichier (ZIP ou CSV) posé sur le disque.
func Ouvrir(chemin string) (*Sauvegarde, error) {
	f, err := os.Open(chemin)
	if err != nil {
		return nil, err
	}
	debut := make([]byte, 4)
	n, _ := io.ReadFull(f, debut)
	_ = f.Close()
	s := &Sauvegarde{}
	if n == 4 && string(debut) == "PK\x03\x04" {
		z, err := zip.OpenReader(chemin)
		if err != nil {
			return nil, ErrFormat
		}
		s.zip = z
		if err := s.lireZip(); err != nil {
			_ = z.Close()
			return nil, err
		}
	} else {
		f, err := os.Open(chemin)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		v, err := LireCSV(io.LimitReader(f, csvMax))
		if err != nil {
			return nil, err
		}
		s.Vehicules = []Vehicule{v}
	}
	if len(s.Vehicules) == 0 {
		_ = s.Fermer()
		return nil, ErrFormat
	}
	return s, nil
}

// lireZip : les CSV de Fuelio (ceux qui ont une partie « ## Log ») et les
// photos. Un ZIP dans le ZIP (sauvegarde de synchronisation rangée dans la
// sauvegarde complète) est lu aussi, sur un seul niveau.
func (s *Sauvegarde) lireZip() error {
	for _, f := range s.zip.File {
		nom := strings.ToLower(f.Name)
		switch {
		case strings.HasSuffix(nom, "/"):
		case estImage(nom):
			if len(s.Images) < ImagesMax {
				s.Images = append(s.Images, Image{Nom: f.Name, f: f})
			}
		case strings.HasSuffix(nom, ".csv"):
			b, err := lireEntree(f, csvMax)
			if err != nil || !bytes.Contains(b, []byte("## Log")) {
				continue
			}
			if v, err := LireCSV(bytes.NewReader(b)); err == nil {
				s.Vehicules = append(s.Vehicules, v)
			}
		case strings.HasSuffix(nom, ".zip"):
			b, err := lireEntree(f, csvMax)
			if err != nil {
				continue
			}
			z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
			if err != nil {
				continue
			}
			for _, g := range z.File {
				if !strings.HasSuffix(strings.ToLower(g.Name), ".csv") {
					continue
				}
				c, err := lireEntree(g, csvMax)
				if err != nil || !bytes.Contains(c, []byte("## Log")) {
					continue
				}
				if v, err := LireCSV(bytes.NewReader(c)); err == nil {
					s.Vehicules = append(s.Vehicules, v)
				}
			}
		}
		if len(s.Vehicules) > VehiculesMax {
			return ErrFormat
		}
	}
	return nil
}

func estImage(nom string) bool {
	switch path.Ext(nom) {
	case ".jpg", ".jpeg", ".png":
		return true
	}
	return false
}

// lireEntree : un fichier du ZIP, borné (un ZIP « bombe » est refusé).
func lireEntree(f *zip.File, max int64) ([]byte, error) {
	if f.UncompressedSize64 > uint64(max) {
		return nil, ErrFormat
	}
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	b, err := io.ReadAll(io.LimitReader(r, max+1))
	if err == nil && int64(len(b)) > max {
		err = ErrFormat
	}
	return b, err
}

// partie : une partie du CSV (titres et lignes).
type partie struct {
	titres []string
	lignes [][]string
}

// col : la valeur de la colonne nommée (voir index) ; "" si absente.
func (p *partie) col(ligne []string, noms ...string) string {
	i := p.index(noms...)
	if i < 0 || i >= len(ligne) {
		return ""
	}
	return strings.TrimSpace(ligne[i])
}

func (p *partie) index(noms ...string) int {
	// Titre exact d'abord (« Cost » n'est pas « CostTitle »), puis le début
	// du titre (« Notes (optional) »).
	for _, n := range noms {
		for i, t := range p.titres {
			if strings.EqualFold(strings.TrimSpace(t), n) {
				return i
			}
		}
	}
	for _, n := range noms {
		for i, t := range p.titres {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(t)), strings.ToLower(n)) {
				return i
			}
		}
	}
	return -1
}

// titre : le titre complet de la colonne (pour l'unité : « Odo (mi) »).
func (p *partie) titre(noms ...string) string {
	if i := p.index(noms...); i >= 0 {
		return strings.ToLower(p.titres[i])
	}
	return ""
}

// LireCSV : un véhicule depuis le CSV de Fuelio.
func LireCSV(r io.Reader) (Vehicule, error) {
	br := bufio.NewReader(r)
	if debut, _ := br.Peek(3); string(debut) == "\ufeff" {
		_, _ = br.Discard(3) // marque UTF-8 en tête de fichier
	}
	cr := csv.NewReader(br)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	cr.ReuseRecord = false
	parties := map[string]*partie{}
	var p *partie
	total := 0
	for {
		l, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Vehicule{}, ErrFormat
		}
		if len(l) == 0 {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(l[0]), "##") {
			nom := strings.ToLower(strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(l[0]), "#")))
			p = &partie{}
			parties[nom] = p
			continue
		}
		if p == nil {
			continue
		}
		if p.titres == nil {
			p.titres = l
			continue
		}
		if total++; total > 4*LignesMax {
			return Vehicule{}, ErrFormat
		}
		p.lignes = append(p.lignes, l)
	}
	log := parties["log"]
	if log == nil || log.titres == nil {
		return Vehicule{}, ErrFormat
	}
	var v Vehicule
	enMiles, gallon := false, 0.0
	if vp := parties["vehicle"]; vp != nil && len(vp.lignes) > 0 {
		v.Nom = vp.col(vp.lignes[0], "Name")
		// DistUnit : 0 km, 1 miles. FuelUnit : 0 litres, 1 gallon US, 2 gallon UK.
		enMiles = vp.col(vp.lignes[0], "DistUnit") == "1"
		switch vp.col(vp.lignes[0], "FuelUnit") {
		case "1":
			gallon = 3.78541
		case "2":
			gallon = 4.54609
		}
	}
	// L'unité écrite dans les titres l'emporte : « Odo (mi) », « Fuel (gal) ».
	if t := log.titre("Odo"); strings.Contains(t, "(mi") {
		enMiles = true
	} else if strings.Contains(t, "(km") {
		enMiles = false
	}
	if t := log.titre("Fuel"); strings.Contains(t, "gal") && gallon == 0 {
		gallon = 3.78541
	} else if strings.Contains(t, "lit") {
		gallon = 0
	}
	km := func(s string) int64 {
		f, ok := nombre(s)
		if !ok || f <= 0 {
			return 0
		}
		if enMiles {
			f *= 1.609344
		}
		return min(int64(math.Round(f)), 5_000_000)
	}

	for _, l := range log.lignes {
		jour, ok := lireJour(log.col(l, "Data", "Date"))
		if !ok {
			continue
		}
		x := Ligne{Plein: true, Jour: jour, Libelle: "Plein", Km: km(log.col(l, "Odo")), Cle: log.col(l, "UniqueId")}
		if log.col(l, "Full") == "0" {
			x.Libelle = "Plein partiel"
		}
		if f, ok := nombre(log.col(l, "Fuel")); ok && f > 0 {
			if gallon > 0 {
				f *= gallon
			}
			x.Litres = min(int64(math.Round(f*100)), 1_000_000)
		}
		if f, ok := nombre(log.col(l, "Price")); ok && f > 0 {
			x.Montant = centimes(f)
		}
		x.Note = joindre(log.col(l, "City"), log.col(l, "Notes"))
		v.Lignes = append(v.Lignes, x)
		if len(v.Lignes) >= LignesMax {
			return v, nil
		}
	}

	// Les dépenses : leur titre, sinon le nom de leur catégorie.
	categories := map[string]string{}
	if cp := parties["costcategories"]; cp != nil {
		for _, l := range cp.lignes {
			categories[cp.col(l, "CostTypeID")] = cp.col(l, "Name")
		}
	}
	if cp := parties["costs"]; cp != nil {
		for _, l := range cp.lignes {
			// Modèles de dépenses répétées et recettes : pas des dépenses faites.
			if cp.col(l, "isTemplate") == "1" || cp.col(l, "isIncome") == "1" {
				continue
			}
			jour, ok := lireJour(cp.col(l, "Date"))
			if !ok {
				continue
			}
			x := Ligne{Jour: jour, Km: km(cp.col(l, "Odo")), Note: cp.col(l, "Notes"), Cle: cp.col(l, "UniqueId")}
			x.Libelle = cp.col(l, "CostTitle")
			if x.Libelle == "" {
				x.Libelle = categories[cp.col(l, "CostTypeID")]
			}
			if x.Libelle == "" {
				x.Libelle = "Dépense"
			}
			if f, ok := nombre(cp.col(l, "Cost")); ok && f > 0 {
				x.Montant = centimes(f)
			}
			v.Lignes = append(v.Lignes, x)
			if len(v.Lignes) >= LignesMax {
				break
			}
		}
	}
	for i := range v.Lignes {
		v.Lignes[i].Libelle = couper(v.Lignes[i].Libelle, 80)
		v.Lignes[i].Note = couper(v.Lignes[i].Note, 500)
	}
	return v, nil
}

// lireJour : « 2024-03-15 18:22 » ou « 2024-03-15 » → « 2024-03-15 ».
func lireJour(s string) (string, bool) {
	if len(s) < 10 {
		return "", false
	}
	t, err := time.Parse("2006-01-02", s[:10])
	if err != nil || t.Year() < 1950 || t.Year() > 2200 {
		return "", false
	}
	return s[:10], true
}

// nombre : « 45.67 » ou « 45,67 ».
func nombre(s string) (float64, bool) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	if s == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

// centimes : un prix en euros (plafonné à 1 million d'euros).
func centimes(f float64) int64 { return min(int64(math.Round(f*100)), 100_000_000) }

func joindre(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + " · " + b
}

func couper(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// Rapprocher : la ligne à laquelle une photo appartient, ou -1. Le nom de
// la photo doit citer l'identifiant d'une seule ligne (UniqueId), ou, à
// défaut, une date (20240315, 2024-03-15, ou un horodatage) que porte une
// seule ligne. Dans le doute, la photo n'est pas rattachée.
func Rapprocher(nom string, lignes []Ligne) int {
	mots := morceaux.FindAllString(nom, -1)
	trouve := -1
	for _, m := range mots {
		if len(m) < 3 {
			continue // trop court pour être un identifiant sûr
		}
		for i, l := range lignes {
			if l.Cle != "" && l.Cle == m {
				if trouve >= 0 && trouve != i {
					return -1
				}
				trouve = i
			}
		}
	}
	if trouve >= 0 {
		return trouve
	}
	for _, j := range joursCites(nom) {
		for i, l := range lignes {
			if l.Jour == j {
				if trouve >= 0 && trouve != i {
					return -1
				}
				trouve = i
			}
		}
	}
	return trouve
}

var (
	morceaux = regexp.MustCompile(`[0-9A-Za-z]+`)
	chiffres = regexp.MustCompile(`[0-9]+`)
	jourTire = regexp.MustCompile(`(19|20)[0-9]{2}-[01][0-9]-[0-3][0-9]`)
)

// joursCites : les dates écrites dans un nom de fichier.
func joursCites(nom string) []string {
	var l []string
	for _, m := range jourTire.FindAllString(nom, -1) {
		if j, ok := lireJour(m); ok {
			l = append(l, j)
		}
	}
	for _, m := range chiffres.FindAllString(nom, -1) {
		var t time.Time
		switch len(m) {
		case 8, 14: // 20240315 ou 20240315182200
			x, err := time.Parse("20060102", m[:8])
			if err != nil {
				continue
			}
			t = x
		case 10, 13: // horodatage en secondes ou millisecondes
			n, _ := strconv.ParseInt(m, 10, 64)
			if len(m) == 13 {
				n /= 1000
			}
			t = time.Unix(n, 0)
		default:
			continue
		}
		if j, ok := lireJour(t.Format("2006-01-02")); ok {
			l = append(l, j)
		}
	}
	return l
}
