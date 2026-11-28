// SPDX-License-Identifier: GPL-3.0-or-later

package suivi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"familycocon/internal/appli"
	"familycocon/internal/photo"
	txt "familycocon/internal/texte"
)

// Scan d'un ticket de carburant : le téléphone lit le texte de la photo
// (Tesseract.js, dans le navigateur, voir app.js) et l'envoie ici ; on y
// cherche la date, les litres, le prix payé et le prix au litre, le
// carburant et la station. Le texte lu est souvent abîmé (tickets
// thermiques) : on garde les trois nombres qui vont ensemble
// (litres × prix au litre = prix payé, au centime près).

// Ticket : ce qui a été trouvé sur le ticket (vide : pas trouvé).
type Ticket struct {
	Jour      string `json:"jour"`      // AAAA-MM-JJ
	Litres    string `json:"litres"`    // « 27,19 »
	Montant   string `json:"montant"`   // « 62,24 »
	PrixLitre string `json:"prixLitre"` // « 2,289 »
	Carburant string `json:"carburant"` // « SP98 » (un de Carburants)
	Note      string `json:"note"`      // la station : « Auchan St Genis Laval »
	Complet   bool   `json:"complet"`   // litres, prix et montant vont ensemble
}

// nombreTicket : un nombre à virgule lu sur le ticket et sa ligne.
type nombreTicket struct {
	v         float64
	decimales int
	ligne     string // ligne en majuscules, sans accents
}

var (
	reNombre = regexp.MustCompile(`(\d{1,4}) ?[.,] ?(\d{1,3})\b`)
	reDate   = regexp.MustCompile(`\b(\d{1,2}) ?[/.-] ?(\d{1,2}) ?[/.-] ?(\d{4}|\d{2})\b`)
	// SP95, SP98 (lus parfois « SPS8 », « SPsB »), E10, E85, gazole…
	reCarbu = regexp.MustCompile(`\b(SP ?[9S][58SB](?:[ -]?E ?10)?|E ?10|E ?85|GAZOLE|GASOIL|DIESEL|B ?7|GPL|SANS ?PLOMB ?9[58]|SUPER ?ETHANOL|ADBLUE)\b`)
)

// LireTicket : les informations du texte lu sur un ticket.
func LireTicket(texte string, aujourdhui time.Time) Ticket {
	var t Ticket
	var lignes []string
	for _, l := range strings.Split(texte, "\n") {
		l = strings.ToUpper(txt.Plier(strings.TrimSpace(l)))
		if l != "" {
			lignes = append(lignes, l)
		}
	}

	// Les nombres à virgule, avec leur ligne. Les lignes de TVA, de montant
	// hors taxe ou en francs (« pour information ») sont écartées.
	// Les mots d'une ligne sont cherchés aussi sur la ligne d'avant
	// (« MONTANT REEL : » puis « 62.24 EUR »).
	var nombres []nombreTicket
	for i, l := range lignes {
		if contientMot(l, "TVA", "HT", "FRF", "TAUX") || strings.Contains(l, "%") {
			continue
		}
		contexte := l
		if i > 0 {
			contexte = lignes[i-1] + " " + l
		}
		for _, m := range reNombre.FindAllStringSubmatch(l, -1) {
			v, err := strconv.ParseFloat(m[1]+"."+m[2], 64)
			if err == nil && v > 0 {
				nombres = append(nombres, nombreTicket{v: v, decimales: len(m[2]), ligne: contexte})
			}
		}
	}

	// Les trois nombres qui vont ensemble : litres × prix au litre = prix
	// payé. Les mots de la ligne départagent (« Quantité », « Prix unit. »,
	// « Montant »…).
	meilleur, score := [3]float64{}, -1
	for _, l := range nombres {
		if l.v < 1 || l.v > 200 {
			continue
		}
		for _, p := range nombres {
			if p.v < 0.3 || p.v > 5 || p.decimales < 2 {
				continue
			}
			for _, m := range nombres {
				if m.v < 1 || m.v > 1000 || m.decimales != 2 {
					continue
				}
				if math.Abs(l.v*p.v-m.v) > math.Max(0.02, m.v*0.003) {
					continue
				}
				s := 0
				if contientMot(l.ligne, "QUANT", "VOLUME", "VOL", "LITRES", "LITRE", "QTE", "L") {
					s += 2
				}
				if contientMot(p.ligne, "PRIX", "UNIT", "P.U", "PU", "/L", "EUR/L") || p.decimales == 3 {
					s += 2
				}
				if contientMot(m.ligne, "MONTANT", "TOTAL", "PAYER", "PAYE", "REEL", "EUR") {
					s += 2
				}
				if s > score {
					meilleur, score = [3]float64{l.v, p.v, m.v}, s
				}
			}
		}
	}
	if score >= 0 {
		t.Litres, t.PrixLitre, t.Montant = virgule(meilleur[0], 2), virgule(meilleur[1], 3), virgule(meilleur[2], 2)
		t.Complet = true
	} else {
		// Pas de trio cohérent : chaque nombre d'après les mots de sa ligne.
		var litres, prix, montant float64
		for _, n := range nombres {
			switch {
			case litres == 0 && n.v >= 1 && n.v <= 200 && contientMot(n.ligne, "QUANT", "VOLUME", "LITRES", "QTE"):
				litres = n.v
			case prix == 0 && n.v >= 0.3 && n.v <= 5 && contientMot(n.ligne, "PRIX", "UNIT", "PU"):
				prix = n.v
			case montant == 0 && n.v >= 1 && n.v <= 1000 && n.decimales == 2 && contientMot(n.ligne, "MONTANT", "TOTAL", "PAYER"):
				montant = n.v
			}
		}
		// Deux sur trois : le troisième se calcule.
		switch {
		case litres == 0 && prix > 0 && montant > 0:
			litres = montant / prix
		case montant == 0 && prix > 0 && litres > 0:
			montant = litres * prix
		}
		if litres > 0 {
			t.Litres = virgule(litres, 2)
		}
		if montant > 0 {
			t.Montant = virgule(montant, 2)
		}
		if prix > 0 {
			t.PrixLitre = virgule(prix, 3)
		}
	}

	// La date : la première qui existe, ni dans le futur ni de plus d'un an.
	for _, l := range lignes {
		for _, m := range reDate.FindAllStringSubmatch(l, -1) {
			j, _ := strconv.Atoi(m[1])
			mo, _ := strconv.Atoi(m[2])
			a, _ := strconv.Atoi(m[3])
			if a < 100 {
				a += 2000
			}
			d := time.Date(a, time.Month(mo), j, 0, 0, 0, 0, aujourdhui.Location())
			if d.Day() != j || int(d.Month()) != mo || d.After(aujourdhui) || d.Before(aujourdhui.AddDate(-1, 0, 0)) {
				continue
			}
			t.Jour = d.Format("2006-01-02")
			break
		}
		if t.Jour != "" {
			break
		}
	}

	// Le carburant, et la station (première ligne faite seulement de mots).
	for _, l := range lignes {
		if m := reCarbu.FindString(l); m != "" {
			if t.Carburant = carburantValide(nomCarburant(m)); t.Carburant != "" {
				break
			}
		}
	}
	for i, l := range lignes {
		if i > 3 {
			break
		}
		if s := station(l); s != "" {
			t.Note = s
			break
		}
	}
	return t
}

// contientMot : la ligne contient l'un des mots (entier, pas au milieu d'un autre).
func contientMot(ligne string, mots ...string) bool {
	for _, m := range mots {
		for i := 0; ; {
			k := strings.Index(ligne[i:], m)
			if k < 0 {
				break
			}
			k += i
			avant, _ := utf8.DecodeLastRuneInString(ligne[:k])
			apres, _ := utf8.DecodeRuneInString(ligne[k+len(m):])
			if (k == 0 || !unicode.IsLetter(avant)) && (k+len(m) == len(ligne) || !unicode.IsLetter(apres)) {
				return true
			}
			i = k + 1
		}
	}
	return false
}

// nomCarburant : « SPSB » → « SP98 », « GASOIL » → « Gazole »…
func nomCarburant(m string) string {
	m = strings.ReplaceAll(m, " ", "")
	switch {
	case strings.HasPrefix(m, "SP") && len(m) >= 4:
		n := "SP9"
		if m[3] == '8' || m[3] == 'B' {
			n += "8"
		} else {
			n += "5"
		}
		if strings.Contains(m[4:], "10") {
			n += "-E10"
		}
		return n
	case strings.HasPrefix(m, "SANSPLOMB"):
		return "SP" + m[len(m)-2:]
	case m == "E10":
		return "SP95-E10"
	case m == "GAZOLE" || m == "GASOIL" || m == "DIESEL" || m == "B7":
		return "Gazole"
	case m == "SUPERETHANOL" || m == "E85":
		return "E85"
	case m == "ADBLUE":
		return "AdBlue"
	}
	return m
}

// station : une ligne faite de 2 à 6 mots en lettres, d'au moins 2
// lettres chacun (« AUCHAN ST GENIS LAVAL » → « Auchan St Genis Laval »),
// sinon « ». Un mot d'une lettre vient souvent d'un logo mal lu (« TOT R »).
func station(l string) string {
	mots := strings.Fields(l)
	if len(mots) < 2 || len(mots) > 6 || utf8.RuneCountInString(l) > 40 {
		return ""
	}
	for _, m := range mots {
		if utf8.RuneCountInString(m) < 2 {
			return ""
		}
		for _, r := range m {
			if !unicode.IsLetter(r) && r != '-' && r != '\'' {
				return ""
			}
		}
	}
	if contientMot(l, "CARTE", "TICKET", "BANCAIRE", "VISA", "MASTERCARD", "CB", "CLIENT", "MERCI", "BIENVENUE") {
		return ""
	}
	for i, m := range mots {
		r := []rune(strings.ToLower(m))
		r[0] = unicode.ToUpper(r[0])
		mots[i] = string(r)
	}
	return strings.Join(mots, " ")
}

// lireTicketHTTP : POST du texte lu par le téléphone ; réponse en JSON.
func (s *Suivi) lireTicketHTTP(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	var corps struct {
		Texte string `json:"texte"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&corps); err != nil || corps.Texte == "" {
		http.Error(w, `{"erreur":"texte absent"}`, http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(LireTicket(corps.Texte, c.Maintenant()))
}

// ticketMax : taille de la photo du ticket envoyée avec le plein (base64,
// réduite par le téléphone à 1 200 px ; le formulaire reste sous 1 Mio).
const ticketMax = 900 << 10

// joindreTicket : la photo du ticket scanné (champ « ticket » du formulaire
// d'un plein, en base64) jointe à la ligne. ok = faux si elle n'a pas pu
// l'être (le plein, lui, est enregistré).
func (s *Suivi) joindreTicket(ctx context.Context, c *appli.Contexte, ligne int64, b64 string) bool {
	if len(b64) > ticketMax {
		return false
	}
	brut, err := base64.StdEncoding.DecodeString(b64)
	if err == nil {
		var jpeg []byte
		if jpeg, err = photo.Preparer(brut); err == nil {
			err = AjouterPiece(ctx, s.a, s.a.Q, c.Famille.ID, ligne, c.Membre.ID, jpeg)
		}
	}
	if err != nil {
		s.a.Journal.Warn("ticket scanné non joint", "ligne", ligne, "erreur", err)
		return false
	}
	return true
}
