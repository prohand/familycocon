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
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Limites (garde-fous contre un fichier piégé ou démesuré).
const (
	LignesMax    = 20000    // lignes lues par véhicule
	ImagesMax    = 20000    // photos lues dans le ZIP
	csvMax       = 16 << 20 // taille d'un CSV une fois décompressé
	interneMax   = 2 << 30  // ZIP dans le ZIP (pictures.data), une fois décompressé
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
	Montant int64  // centimes
	Km      int64  // kilomètres (convertis si le compteur est en miles)
	Litres  int64  // centilitres (pleins)
	Cle     string // UniqueId de Fuelio
	Guid    string // guid de Fuelio (versions récentes)
}

// Photo : une ligne de la partie « ## Pictures » du CSV : le fichier
// (dans pictures.data), son type (plein, dépense… : un nombre propre à
// Fuelio) et l'identifiant de la ligne qu'il illustre.
type Photo struct {
	Fichier, Type, Cible string
}

// Vehicule : un véhicule de la sauvegarde.
type Vehicule struct {
	Nom    string
	Plaque string // immatriculation (« Plate »)
	Modele string // marque, modèle et année (« Ford Focus 2017 »)
	Lignes []Ligne
	Photos []Photo
	Ecart  Ecart // lignes de la sauvegarde laissées de côté (et pourquoi)
}

// Ecart : les lignes lues mais pas reprises, pour le dire à l'utilisateur.
type Ecart struct {
	PleinsSansDate   int // plein dont la date est illisible
	DepensesLues     int // lignes de « ## Costs » (toutes)
	DepensesSansDate int // dépense dont la date est illisible
	Modeles          int // modèle de dépense répétée (pas une dépense faite)
	Recettes         int // recette (revente…), pas une dépense
}

// Image : une photo trouvée dans le ZIP (lue plus tard, une à la fois).
type Image struct {
	Nom string // chemin dans le ZIP
	f   *zip.File
}

// Ouvrir : le contenu de la photo, lu par morceaux.
func (i Image) Ouvrir() (io.ReadCloser, error) { return i.f.Open() }

// Sauvegarde : ce qui a été lu dans le fichier.
type Sauvegarde struct {
	Vehicules []Vehicule
	Images    []Image
	fermer    []func() error // fichiers ouverts et provisoires
	dossier   string         // où écrire un fichier provisoire
}

// Fermer : ferme les fichiers (les images ne sont plus lisibles ensuite)
// et efface les fichiers provisoires.
func (s *Sauvegarde) Fermer() error {
	var err error
	for i := len(s.fermer) - 1; i >= 0; i-- {
		if e := s.fermer[i](); e != nil && err == nil {
			err = e
		}
	}
	s.fermer = nil
	return err
}

// Ouvrir : lit le fichier (ZIP ou CSV) posé sur le disque. Un fichier
// provisoire peut être écrit dans le même dossier (ZIP dans le ZIP) ;
// Fermer l'efface.
func Ouvrir(chemin string) (*Sauvegarde, error) {
	f, err := os.Open(chemin)
	if err != nil {
		return nil, err
	}
	s := &Sauvegarde{dossier: filepath.Dir(chemin)}
	s.fermer = append(s.fermer, f.Close)
	debut := make([]byte, 4)
	n, _ := io.ReadFull(f, debut)
	if n == 4 && string(debut) == "PK\x03\x04" {
		info, err := f.Stat()
		if err != nil {
			_ = s.Fermer()
			return nil, err
		}
		z, err := zip.NewReader(f, info.Size())
		if err != nil {
			_ = s.Fermer()
			return nil, ErrFormat
		}
		if err := s.parcourir(z, f, 0); err != nil {
			_ = s.Fermer()
			return nil, err
		}
	} else {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			_ = s.Fermer()
			return nil, err
		}
		v, err := LireCSV(io.LimitReader(f, csvMax))
		if err != nil {
			_ = s.Fermer()
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

// parcourir : les CSV de Fuelio (ceux qui ont une partie « ## Log ») et les
// photos. Un ZIP dans le ZIP (pictures.data de la sauvegarde complète, ou
// ZIP de synchro) est lu aussi, sur un seul niveau. base : le fichier qui
// porte ce ZIP (pour lire un ZIP interne rangé sans compression).
func (s *Sauvegarde) parcourir(z *zip.Reader, base io.ReaderAt, niveau int) error {
	for _, f := range z.File {
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
		case niveau == 0 && estZip(f):
			interne, err := s.ouvrirInterne(f, base)
			if err != nil {
				continue // photos illisibles : les pleins sont repris quand même
			}
			if err := s.parcourir(interne, nil, 1); err != nil {
				return err
			}
		}
		if len(s.Vehicules) > VehiculesMax {
			return ErrFormat
		}
	}
	return nil
}

// estZip : le fichier commence comme un ZIP (pictures.data, .zip).
func estZip(f *zip.File) bool {
	r, err := f.Open()
	if err != nil {
		return false
	}
	defer r.Close()
	debut := make([]byte, 4)
	n, _ := io.ReadFull(r, debut)
	return n == 4 && string(debut) == "PK\x03\x04"
}

// ouvrirInterne : un ZIP rangé dans le ZIP. Sans compression, il est lu en
// place ; sinon il est d'abord décompressé dans un fichier provisoire (il
// peut peser des centaines de Mo : jamais en mémoire).
func (s *Sauvegarde) ouvrirInterne(f *zip.File, base io.ReaderAt) (*zip.Reader, error) {
	if f.Method == zip.Store && base != nil && f.UncompressedSize64 == f.CompressedSize64 {
		debut, err := f.DataOffset()
		if err == nil {
			taille := int64(f.UncompressedSize64)
			return zip.NewReader(io.NewSectionReader(base, debut, taille), taille)
		}
	}
	if f.UncompressedSize64 > interneMax {
		return nil, ErrFormat
	}
	tmp, err := os.CreateTemp(s.dossier, "fuelio-*.tmp")
	if err != nil {
		return nil, err
	}
	s.fermer = append(s.fermer, func() error { return os.Remove(tmp.Name()) }, tmp.Close)
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	n, err := io.Copy(tmp, io.LimitReader(r, interneMax+1))
	_ = r.Close()
	if err != nil {
		return nil, err
	}
	if n > interneMax {
		return nil, ErrFormat
	}
	return zip.NewReader(tmp, n)
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
		v.Plaque = couper(vp.col(vp.lignes[0], "Plate"), 20)
		annee := vp.col(vp.lignes[0], "Year")
		if annee == "0" {
			annee = ""
		}
		v.Modele = couper(strings.Join(strings.Fields(vp.col(vp.lignes[0], "Make")+" "+vp.col(vp.lignes[0], "Model")+" "+annee), " "), 80)
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
			v.Ecart.PleinsSansDate++
			continue
		}
		x := Ligne{Plein: true, Jour: jour, Libelle: "Plein", Km: km(log.col(l, "Odo")), Cle: log.col(l, "UniqueId"), Guid: log.col(l, "guid")}
		// Full : 1 plein complet ; 0 partiel ; 2 partiel dont Fuelio estime
		// le niveau du réservoir (TankCalc) : partiel aussi pour nous (ses
		// litres comptent au plein complet suivant).
		if f := log.col(l, "Full"); f == "0" || f == "2" {
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
			break
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
			v.Ecart.DepensesLues++
			// Modèles de dépenses répétées et recettes : pas des dépenses faites.
			if vrai(cp.col(l, "isTemplate")) {
				v.Ecart.Modeles++
				continue
			}
			if vrai(cp.col(l, "isIncome")) {
				v.Ecart.Recettes++
				continue
			}
			jour, ok := lireJour(cp.col(l, "Date", "Data"))
			if !ok {
				v.Ecart.DepensesSansDate++
				continue
			}
			x := Ligne{Jour: jour, Km: km(cp.col(l, "Odo")), Note: cp.col(l, "Notes"), Cle: cp.col(l, "UniqueId"), Guid: cp.col(l, "guid")}
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
	// Les photos : fichier, type et ligne illustrée.
	if pp := parties["pictures"]; pp != nil {
		for _, l := range pp.lignes {
			ph := Photo{Fichier: pp.col(l, "Filename"), Type: pp.col(l, "Type"), Cible: pp.col(l, "target_id")}
			if ph.Fichier != "" && ph.Cible != "" && len(v.Photos) < ImagesMax {
				v.Photos = append(v.Photos, ph)
			}
		}
	}
	for i := range v.Lignes {
		v.Lignes[i].Libelle = couper(v.Lignes[i].Libelle, 80)
		v.Lignes[i].Note = couper(v.Lignes[i].Note, 500)
	}
	return v, nil
}

// vrai : « 1 » ou « true » (selon la version de Fuelio).
func vrai(s string) bool { return s == "1" || strings.EqualFold(s, "true") }

// lireJour : « 2024-03-15 18:22 », « 2024-03-15 », « 15/03/2024 » ou
// « 15.03.2024 » (format choisi dans Fuelio) → « 2024-03-15 ».
func lireJour(s string) (string, bool) {
	if len(s) < 10 {
		return "", false
	}
	for _, f := range []string{"2006-01-02", "02/01/2006", "02.01.2006", "02-01-2006", "2006/01/02"} {
		t, err := time.Parse(f, s[:10])
		if err == nil && t.Year() >= 1950 && t.Year() <= 2200 {
			return t.Format("2006-01-02"), true
		}
	}
	return "", false
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

// Lien : une photo et la ligne qu'elle illustre (place dans Vehicule.Lignes).
type Lien struct {
	Image Image
	Ligne int
}

// Rattacher : les photos du véhicule et leur ligne.
//   - Sauvegarde complète : la partie « ## Pictures » cite, pour chaque
//     photo, son fichier (dans pictures.data) et l'identifiant de sa ligne
//     (guid, ou UniqueId). Les pleins et les dépenses ont chacun leurs
//     UniqueId (1, 2, 3… des deux côtés) : le type de la photo dit lequel.
//     Fuelio ne publie pas ses numéros de type : ils sont déduits des photos
//     sans doute possible (un seul plein ou une seule dépense porte cet
//     identifiant), et un type n'est retenu que s'il désigne nettement une
//     seule sorte de ligne.
//   - Sinon : le nom du fichier (voir Rapprocher).
//
// sansLigne : photos du véhicule laissées de côté (fichier absent, ligne
// introuvable ou douteuse). Dans le doute, une photo n'est jamais rattachée.
func Rattacher(v Vehicule, images []Image) (liens []Lien, sansLigne int) {
	if len(v.Photos) == 0 {
		for _, img := range images {
			if i := Rapprocher(img.Nom, v.Lignes); i >= 0 {
				liens = append(liens, Lien{Image: img, Ligne: i})
			} else {
				sansLigne++
			}
		}
		return liens, sansLigne
	}
	parNom := make(map[string]Image, len(images))
	for _, img := range images {
		parNom[strings.ToLower(path.Base(img.Nom))] = img
	}
	candidats := func(p Photo) []int {
		var l []int
		for i, x := range v.Lignes {
			if (x.Guid != "" && strings.EqualFold(x.Guid, p.Cible)) || (x.Cle != "" && x.Cle == p.Cible) {
				l = append(l, i)
			}
		}
		return l
	}
	// Votes : pour chaque type, combien de photos sûres vont à un plein
	// [1] ou à une dépense [0].
	votes := map[string]*[2]int{}
	for _, p := range v.Photos {
		c := candidats(p)
		if len(c) == 0 {
			continue
		}
		plein := v.Lignes[c[0]].Plein
		pareil := true
		for _, i := range c[1:] {
			pareil = pareil && v.Lignes[i].Plein == plein
		}
		if !pareil {
			continue
		}
		if votes[p.Type] == nil {
			votes[p.Type] = &[2]int{}
		}
		if plein {
			votes[p.Type][1]++
		} else {
			votes[p.Type][0]++
		}
	}
	// Un type par sorte de ligne : celui qui a le plus de votes nets (au
	// moins 4 contre 1) ; les autres (trajets…) sont laissés de côté.
	meilleur := [2]string{}
	score := [2]int{}
	for t, n := range votes {
		for sorte := 0; sorte < 2; sorte++ {
			if n[sorte] > 0 && n[sorte] >= 4*n[1-sorte] && n[sorte] > score[sorte] {
				meilleur[sorte], score[sorte] = t, n[sorte]
			}
		}
	}
	for _, p := range v.Photos {
		img, ok := parNom[strings.ToLower(path.Base(strings.ReplaceAll(p.Fichier, `\`, "/")))]
		sorte := -1
		for s := 0; s < 2; s++ {
			if score[s] > 0 && meilleur[s] == p.Type {
				sorte = s
			}
		}
		var garde []int
		for _, i := range candidats(p) {
			if sorte >= 0 && v.Lignes[i].Plein == (sorte == 1) {
				garde = append(garde, i)
			}
		}
		if !ok || len(garde) != 1 {
			sansLigne++
			continue
		}
		liens = append(liens, Lien{Image: img, Ligne: garde[0]})
	}
	return liens, sansLigne
}
