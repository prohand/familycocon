// SPDX-License-Identifier: GPL-3.0-or-later

package fidelite

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// Couleurs des enseignes : une carte dont le nom est reconnu (« Carrefour »,
// « Carte E.Leclerc », « Super U »…) prend la couleur de l'enseigne, avec
// une bande de sa seconde couleur. Rien que des couleurs (pas de logo, qui
// est une marque protégée) ; tout est dans l'appli, rien ne part sur
// Internet. Une carte peut garder la couleur choisie (couleur_enseigne = 0).
//
// Les couleurs sont approchées. Pour en ajouter une : une ligne dans
// enseignes (clé : lettres minuscules sans accent ; noms : la façon dont
// on l'écrit, réduite de la même manière).

type enseigne struct {
	cle   string   // classe CSS « marque-<cle> »
	noms  []string // noms reconnus (lettres et chiffres, sans accent)
	fond  string   // couleur principale
	bande string   // seconde couleur (bande en biais)
}

var enseignes = []enseigne{
	// Supermarchés
	{"carrefour", []string{"carrefour"}, "#004E9F", "#E30613"},
	{"leclerc", []string{"leclerc", "eleclerc"}, "#0066B3", "#F39200"},
	{"auchan", []string{"auchan"}, "#E2001A", "#00A651"},
	{"intermarche", []string{"intermarche"}, "#E30613", "#1D1D1B"},
	{"u", []string{"superu", "hyperu", "systemeu", "uexpress", "magasinsu", "coursesu"}, "#004C97", "#E30613"},
	{"lidl", []string{"lidl"}, "#0050AA", "#FFF000"},
	{"aldi", []string{"aldi"}, "#00205B", "#F58220"},
	{"casino", []string{"casino", "geantcasino"}, "#008C45", "#E30613"},
	{"monoprix", []string{"monoprix", "monop"}, "#1D1D1B", "#E30613"},
	{"franprix", []string{"franprix"}, "#E5007D", "#F39200"},
	{"cora", []string{"cora"}, "#E30613", "#004C97"},
	{"match", []string{"supermarchesmatch", "supermatch"}, "#E30613", "#FFD500"},
	{"spar", []string{"spar"}, "#00843D", "#E30613"},
	{"netto", []string{"netto"}, "#FFD500", "#E30613"},
	{"picard", []string{"picard"}, "#1B3F8F", "#7FC6E8"},
	{"biocoop", []string{"biocoop"}, "#78B828", "#E30613"},
	{"lavieclaire", []string{"lavieclaire"}, "#78B828", "#00843D"},
	{"naturalia", []string{"naturalia"}, "#7AB800", "#00594F"},
	{"grandfrais", []string{"grandfrais"}, "#D6001C", "#78B828"},
	// Maison, bricolage, jardin
	{"leroymerlin", []string{"leroymerlin"}, "#78BE20", "#1D1D1B"},
	{"castorama", []string{"castorama"}, "#0078D7", "#FFD500"},
	{"bricodepot", []string{"bricodepot"}, "#E30613", "#FFD500"},
	{"mrbricolage", []string{"mrbricolage", "monsieurbricolage"}, "#E30613", "#004C97"},
	{"ikea", []string{"ikea"}, "#0058A3", "#FFDB00"},
	{"conforama", []string{"conforama"}, "#E30613", "#1D1D1B"},
	{"but", []string{"but"}, "#E3001B", "#1D1D1B"},
	{"gifi", []string{"gifi"}, "#E5007E", "#FFD500"},
	{"action", []string{"action"}, "#0A3A85", "#E30613"},
	{"truffaut", []string{"truffaut"}, "#0D7D3A", "#C8D400"},
	{"jardiland", []string{"jardiland"}, "#82BC00", "#00594F"},
	{"botanic", []string{"botanic"}, "#6BA539", "#1D1D1B"},
	// Culture, électroménager
	{"fnac", []string{"fnac"}, "#E1A925", "#1D1D1B"},
	{"darty", []string{"darty"}, "#E30613", "#1D1D1B"},
	{"boulanger", []string{"boulanger"}, "#F15A22", "#1D1D1B"},
	{"cultura", []string{"cultura"}, "#E94E1B", "#1D1D1B"},
	// Sport
	{"decathlon", []string{"decathlon"}, "#0082C3", "#FFFFFF"},
	{"intersport", []string{"intersport"}, "#003B7C", "#E30613"},
	{"gosport", []string{"gosport"}, "#1D1D1B", "#E30613"},
	{"courir", []string{"courir"}, "#1D1D1B", "#E30613"},
	// Beauté, santé
	{"sephora", []string{"sephora"}, "#1D1D1B", "#FFFFFF"},
	{"nocibe", []string{"nocibe"}, "#E5007E", "#1D1D1B"},
	{"marionnaud", []string{"marionnaud"}, "#5A2A82", "#1D1D1B"},
	{"yvesrocher", []string{"yvesrocher"}, "#007A3D", "#C8D400"},
	{"pharmacie", []string{"pharmacie", "parapharmacie"}, "#00A651", "#FFFFFF"},
	// Mode
	{"kiabi", []string{"kiabi"}, "#0099D8", "#E5007E"},
	{"hm", []string{"hm", "handm"}, "#E50010", "#FFFFFF"},
	{"zara", []string{"zara"}, "#1D1D1B", "#FFFFFF"},
	{"uniqlo", []string{"uniqlo"}, "#FF0000", "#FFFFFF"},
	{"primark", []string{"primark"}, "#00A1E4", "#1D1D1B"},
	{"celio", []string{"celio"}, "#1D1D1B", "#E30613"},
	{"jules", []string{"jules"}, "#1D1D1B", "#FFFFFF"},
	{"ca", []string{"ca", "cunda"}, "#003B7C", "#E30613"},
	// Animaux
	{"maxizoo", []string{"maxizoo"}, "#E30613", "#FFD500"},
	{"animalis", []string{"animalis"}, "#00A0DF", "#F39200"},
	// Auto, carburant
	{"norauto", []string{"norauto"}, "#004B93", "#FFD500"},
	{"feuvert", []string{"feuvert"}, "#009639", "#E30613"},
	{"midas", []string{"midas"}, "#FFD100", "#1D1D1B"},
	{"speedy", []string{"speedy"}, "#E30613", "#FFD500"},
	{"totalenergies", []string{"totalenergies", "total"}, "#E2001A", "#F39200"},
	{"shell", []string{"shell"}, "#FFD500", "#DD1D21"},
	{"esso", []string{"esso"}, "#E30613", "#004C97"},
	{"bp", []string{"bp"}, "#009900", "#FFE600"},
	// Restauration, cinéma
	{"mcdonalds", []string{"mcdonalds", "mcdo"}, "#DA291C", "#FFC72C"},
	{"burgerking", []string{"burgerking"}, "#D62300", "#F5A623"},
	{"kfc", []string{"kfc"}, "#E4002B", "#1D1D1B"},
	{"starbucks", []string{"starbucks"}, "#00704A", "#1D1D1B"},
	{"subway", []string{"subway"}, "#008C15", "#FFC20E"},
	{"quick", []string{"quick"}, "#E30613", "#1D1D1B"},
	{"pathe", []string{"pathe", "gaumontpathe"}, "#FFC20E", "#1D1D1B"},
	{"ugc", []string{"ugc"}, "#E30613", "#1D1D1B"},
}

// sansAccent : les lettres accentuées courantes, ramenées à leur base.
var sansAccent = strings.NewReplacer(
	"à", "a", "â", "a", "ä", "a", "á", "a", "ã", "a",
	"ç", "c", "é", "e", "è", "e", "ê", "e", "ë", "e",
	"î", "i", "ï", "i", "í", "i", "ô", "o", "ö", "o", "ó", "o",
	"ù", "u", "û", "u", "ü", "u", "ú", "u", "ÿ", "y", "œ", "oe", "æ", "ae",
	"&", "and",
)

// mots : le nom en mots (lettres et chiffres, minuscules, sans accent).
func mots(nom string) []string {
	nom = sansAccent.Replace(strings.ToLower(nom))
	return strings.FieldsFunc(nom, func(r rune) bool { return !(r >= 'a' && r <= 'z' || unicode.IsDigit(r)) })
}

// trouverEnseigne : l'enseigne reconnue dans le nom de la carte (nil :
// aucune). Un nom reconnu doit être fait de mots entiers qui se suivent
// (« Carte Carrefour Market », « Super U », « E.Leclerc ») : « Boulangerie »
// n'est pas « Boulanger », ni « Décoration » « Cora ».
func trouverEnseigne(nom string) *enseigne {
	m := mots(nom)
	// Toutes les suites de mots, collées (« super », « superu », « u »…).
	suites := map[string]bool{}
	for i := range m {
		colle := ""
		for j := i; j < len(m) && j < i+4; j++ {
			colle += m[j]
			suites[colle] = true
		}
	}
	for i := range enseignes {
		for _, n := range enseignes[i].noms {
			if suites[n] {
				return &enseignes[i]
			}
		}
	}
	return nil
}

// Marque : la classe CSS de l'enseigne (« carrefour »), vide si le nom
// n'est pas reconnu ou si la carte garde sa couleur.
func Marque(nom string, auto int64) string {
	if auto == 0 {
		return ""
	}
	if e := trouverEnseigne(nom); e != nil {
		return e.cle
	}
	return ""
}

// texteSur : blanc ou presque noir, selon la clarté du fond.
func texteSur(hex string) string {
	v, err := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	if err != nil {
		return "#fff"
	}
	r, g, b := float64(v>>16&0xff), float64(v>>8&0xff), float64(v&0xff)
	if 0.299*r+0.587*g+0.114*b > 160 {
		return "#111"
	}
	return "#fff"
}

var (
	feuilleUneFois sync.Once
	feuille        []byte
)

// feuilleEnseignes : GET /fidelite/enseignes.css, les couleurs de chaque
// enseigne en classes (pas de style écrit dans la page : CSP).
func feuilleEnseignes(w http.ResponseWriter, r *http.Request) {
	feuilleUneFois.Do(func() {
		var b strings.Builder
		b.WriteString("/* Couleurs des enseignes (internal/fidelite/enseignes.go). */\n")
		for _, e := range enseignes {
			fmt.Fprintf(&b, ".marque-%s{--c:%s;--c2:%s;--t:%s}\n", e.cle, e.fond, e.bande, texteSur(e.fond))
		}
		feuille = []byte(b.String())
	})
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write(feuille)
}
