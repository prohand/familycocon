// SPDX-License-Identifier: GPL-3.0-or-later

package courses

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Rayon : une section de la liste de courses.
type Rayon struct {
	Cle   string
	Nom   string
	Emoji string
}

// Rayons, dans l'ordre d'un magasin.
var Rayons = []Rayon{
	{"fruits", "Fruits et légumes", "🥕"},
	{"boulangerie", "Boulangerie", "🥖"},
	{"frais", "Frais", "🧀"},
	{"boucherie", "Viande et poisson", "🥩"},
	{"surgeles", "Surgelés", "🧊"},
	{"epicerie", "Épicerie", "🥫"},
	{"petitdej", "Petit-déjeuner", "☕"},
	{"boissons", "Boissons", "🧃"},
	{"bebe", "Bébé", "🍼"},
	{"hygiene", "Hygiène et beauté", "🧴"},
	{"maison", "Entretien et maison", "🧽"},
	{"animaux", "Animaux", "🐾"},
	{"autre", "Autre", "🛍️"},
}

// RayonParCle : le rayon (ok = faux si inconnu).
func RayonParCle(cle string) (Rayon, bool) {
	for _, r := range Rayons {
		if r.Cle == cle {
			return r, true
		}
	}
	return Rayon{}, false
}

// connu : rayon et emoji d'un produit courant.
type connu struct{ rayon, emoji string }

// dictionnaire : produits courants (clé sans accent, au singulier). Le
// rayon choisi par la famille pour un produit est retenu en base et passe
// avant ce dictionnaire.
var dictionnaire = map[string]connu{
	// Fruits et légumes
	"pomme": {"fruits", "🍎"}, "poire": {"fruits", "🍐"}, "banane": {"fruits", "🍌"}, "orange": {"fruits", "🍊"},
	"clementine": {"fruits", "🍊"}, "mandarine": {"fruits", "🍊"}, "citron": {"fruits", "🍋"}, "raisin": {"fruits", "🍇"},
	"fraise": {"fruits", "🍓"}, "framboise": {"fruits", "🍓"}, "cerise": {"fruits", "🍒"}, "peche": {"fruits", "🍑"},
	"abricot": {"fruits", "🍑"}, "melon": {"fruits", "🍈"}, "pasteque": {"fruits", "🍉"}, "ananas": {"fruits", "🍍"},
	"kiwi": {"fruits", "🥝"}, "mangue": {"fruits", "🥭"}, "avocat": {"fruits", "🥑"}, "tomate": {"fruits", "🍅"},
	"carotte": {"fruits", "🥕"}, "pomme de terre": {"fruits", "🥔"}, "patate": {"fruits", "🥔"}, "oignon": {"fruits", "🧅"},
	"ail": {"fruits", "🧄"}, "echalote": {"fruits", "🧅"}, "salade": {"fruits", "🥬"}, "laitue": {"fruits", "🥬"},
	"epinard": {"fruits", "🥬"}, "chou": {"fruits", "🥬"}, "brocoli": {"fruits", "🥦"}, "courgette": {"fruits", "🥒"},
	"concombre": {"fruits", "🥒"}, "aubergine": {"fruits", "🍆"}, "poivron": {"fruits", "🫑"}, "champignon": {"fruits", "🍄"},
	"haricot vert": {"fruits", "🫛"}, "petit pois": {"fruits", "🫛"}, "mais": {"fruits", "🌽"}, "poireau": {"fruits", "🥬"},
	"radis": {"fruits", "🥕"}, "potiron": {"fruits", "🎃"}, "courge": {"fruits", "🎃"}, "persil": {"fruits", "🌿"},
	"basilic": {"fruits", "🌿"}, "menthe": {"fruits", "🌿"}, "coriandre": {"fruits", "🌿"}, "gingembre": {"fruits", "🫚"},
	"fruit": {"fruits", "🍎"}, "legume": {"fruits", "🥕"}, "noix": {"fruits", "🌰"}, "chataigne": {"fruits", "🌰"},
	// Boulangerie
	"pain": {"boulangerie", "🥖"}, "baguette": {"boulangerie", "🥖"}, "croissant": {"boulangerie", "🥐"},
	"pain au chocolat": {"boulangerie", "🥐"}, "brioche": {"boulangerie", "🍞"}, "pain de mie": {"boulangerie", "🍞"},
	"biscotte": {"boulangerie", "🍞"}, "galette": {"boulangerie", "🫓"}, "tortilla": {"boulangerie", "🫓"},
	"gateau": {"boulangerie", "🍰"}, "tarte": {"boulangerie", "🥧"},
	// Frais
	"lait": {"frais", "🥛"}, "beurre": {"frais", "🧈"}, "oeuf": {"frais", "🥚"}, "fromage": {"frais", "🧀"},
	"gruyere": {"frais", "🧀"}, "emmental": {"frais", "🧀"}, "comte": {"frais", "🧀"}, "camembert": {"frais", "🧀"},
	"mozzarella": {"frais", "🧀"}, "parmesan": {"frais", "🧀"}, "raclette": {"frais", "🧀"}, "yaourt": {"frais", "🥛"},
	"yogourt": {"frais", "🥛"}, "creme": {"frais", "🥛"}, "creme fraiche": {"frais", "🥛"}, "fromage blanc": {"frais", "🥛"},
	"compote": {"frais", "🍏"}, "jambon": {"frais", "🥓"}, "lardon": {"frais", "🥓"}, "saucisson": {"frais", "🥓"},
	"pate feuilletee": {"frais", "🥧"}, "pate brisee": {"frais", "🥧"}, "pate a pizza": {"frais", "🍕"}, "dessert": {"frais", "🍮"},
	"houmous": {"frais", "🥙"}, "tofu": {"frais", "🧆"},
	// Viande et poisson
	"viande": {"boucherie", "🥩"}, "steak": {"boucherie", "🥩"}, "boeuf": {"boucherie", "🥩"}, "veau": {"boucherie", "🥩"},
	"porc": {"boucherie", "🥩"}, "agneau": {"boucherie", "🥩"}, "poulet": {"boucherie", "🍗"}, "dinde": {"boucherie", "🍗"},
	"saucisse": {"boucherie", "🌭"}, "merguez": {"boucherie", "🌭"}, "viande hachee": {"boucherie", "🥩"},
	"poisson": {"boucherie", "🐟"}, "saumon": {"boucherie", "🐟"}, "cabillaud": {"boucherie", "🐟"}, "thon": {"epicerie", "🐟"},
	"crevette": {"boucherie", "🦐"}, "moule": {"boucherie", "🦪"},
	// Surgelés
	"glace": {"surgeles", "🍦"}, "frite": {"surgeles", "🍟"}, "pizza": {"surgeles", "🍕"}, "surgele": {"surgeles", "🧊"},
	"glacon": {"surgeles", "🧊"}, "poisson pane": {"surgeles", "🐟"},
	// Épicerie
	"pate": {"epicerie", "🍝"}, "spaghetti": {"epicerie", "🍝"}, "riz": {"epicerie", "🍚"}, "semoule": {"epicerie", "🍚"},
	"quinoa": {"epicerie", "🍚"}, "lentille": {"epicerie", "🫘"}, "pois chiche": {"epicerie", "🫘"}, "farine": {"epicerie", "🌾"},
	"sucre": {"epicerie", "🍬"}, "sel": {"epicerie", "🧂"}, "poivre": {"epicerie", "🧂"}, "epice": {"epicerie", "🧂"},
	"huile": {"epicerie", "🫒"}, "huile d'olive": {"epicerie", "🫒"}, "vinaigre": {"epicerie", "🫙"}, "moutarde": {"epicerie", "🫙"},
	"ketchup": {"epicerie", "🍅"}, "mayonnaise": {"epicerie", "🫙"}, "sauce tomate": {"epicerie", "🍅"}, "conserve": {"epicerie", "🥫"},
	"soupe": {"epicerie", "🥣"}, "bouillon": {"epicerie", "🥣"}, "chocolat": {"epicerie", "🍫"}, "bonbon": {"epicerie", "🍬"},
	"biscuit": {"epicerie", "🍪"}, "gateau sec": {"epicerie", "🍪"}, "chips": {"epicerie", "🥔"}, "apero": {"epicerie", "🥨"},
	"olive": {"epicerie", "🫒"}, "levure": {"epicerie", "🌾"}, "chapelure": {"epicerie", "🌾"},
	// Petit-déjeuner
	"cafe": {"petitdej", "☕"}, "the": {"petitdej", "🍵"}, "tisane": {"petitdej", "🍵"}, "cereale": {"petitdej", "🥣"},
	"muesli": {"petitdej", "🥣"}, "confiture": {"petitdej", "🍯"}, "miel": {"petitdej", "🍯"}, "pate a tartiner": {"petitdej", "🍫"},
	"nutella": {"petitdej", "🍫"}, "cacao": {"petitdej", "☕"},
	// Boissons
	"eau": {"boissons", "💧"}, "eau gazeuse": {"boissons", "💧"}, "jus": {"boissons", "🧃"}, "jus d'orange": {"boissons", "🧃"},
	"soda": {"boissons", "🥤"}, "coca": {"boissons", "🥤"}, "sirop": {"boissons", "🧃"}, "vin": {"boissons", "🍷"},
	"biere": {"boissons", "🍺"}, "champagne": {"boissons", "🍾"}, "cidre": {"boissons", "🍾"},
	// Bébé
	"couche": {"bebe", "🍼"}, "lingette": {"bebe", "🍼"}, "lait bebe": {"bebe", "🍼"}, "petit pot": {"bebe", "🍼"},
	// Hygiène et beauté
	"savon": {"hygiene", "🧼"}, "gel douche": {"hygiene", "🧴"}, "shampoing": {"hygiene", "🧴"}, "shampooing": {"hygiene", "🧴"},
	"dentifrice": {"hygiene", "🪥"}, "brosse a dent": {"hygiene", "🪥"}, "deodorant": {"hygiene", "🧴"}, "coton": {"hygiene", "☁️"},
	"mouchoir": {"hygiene", "🤧"}, "papier toilette": {"hygiene", "🧻"}, "pq": {"hygiene", "🧻"}, "rasoir": {"hygiene", "🪒"},
	"creme solaire": {"hygiene", "🧴"}, "serviette hygienique": {"hygiene", "🩹"}, "tampon": {"hygiene", "🩹"}, "pansement": {"hygiene", "🩹"},
	// Entretien et maison
	"lessive": {"maison", "🧺"}, "adoucissant": {"maison", "🧺"}, "liquide vaisselle": {"maison", "🧽"}, "pastille lave-vaisselle": {"maison", "🧽"},
	"eponge": {"maison", "🧽"}, "sac poubelle": {"maison", "🗑️"}, "essuie-tout": {"maison", "🧻"}, "sopalin": {"maison", "🧻"},
	"javel": {"maison", "🧴"}, "nettoyant": {"maison", "🧴"}, "pile": {"maison", "🔋"}, "ampoule": {"maison", "💡"},
	"papier alu": {"maison", "🧻"}, "film alimentaire": {"maison", "🧻"}, "bougie": {"maison", "🕯️"},
	// Animaux
	"croquette": {"animaux", "🐾"}, "patee": {"animaux", "🐾"}, "litiere": {"animaux", "🐈"},
}

// Cle : forme normalisée d'un nom de produit (minuscules, sans accents,
// au singulier) : « Pommes de terre » → « pomme de terre ».
func Cle(nom string) string {
	mots := strings.Fields(sansAccents.Replace(strings.ToLower(strings.TrimSpace(nom))))
	for i, m := range mots {
		mots[i] = singulier(m)
	}
	return strings.Join(mots, " ")
}

// sansAccents : lettres accentuées du français (pas besoin de la
// bibliothèque Unicode complète pour ça).
var sansAccents = strings.NewReplacer(
	"à", "a", "â", "a", "ä", "a", "á", "a", "ç", "c", "é", "e", "è", "e", "ê", "e", "ë", "e",
	"î", "i", "ï", "i", "í", "i", "ô", "o", "ö", "o", "ó", "o", "ù", "u", "û", "u", "ü", "u", "ú", "u",
	"ÿ", "y", "ñ", "n", "œ", "oe", "æ", "ae", "’", "'",
)

// singulier : enlève le « s » ou le « x » final (pommes → pomme, choux → chou).
func singulier(m string) string {
	if len(m) > 3 && (strings.HasSuffix(m, "s") || strings.HasSuffix(m, "x")) &&
		!strings.HasSuffix(m, "ss") && m != "ananas" && m != "radis" && m != "riz" && m != "noix" && m != "jus" && m != "pois" && m != "mais" {
		return m[:len(m)-1]
	}
	return m
}

// Deviner : rayon et emoji d'un produit d'après le dictionnaire (le nom
// entier, puis les groupes de mots, puis chaque mot).
func Deviner(nom string) (rayon, emoji string) {
	cle := Cle(nom)
	if c, ok := dictionnaire[cle]; ok {
		return c.rayon, c.emoji
	}
	mots := strings.Fields(cle)
	// Groupes de mots, du plus long au plus court (« jus d'orange bio »).
	for n := len(mots) - 1; n >= 1; n-- {
		for i := 0; i+n <= len(mots); i++ {
			if c, ok := dictionnaire[strings.Join(mots[i:i+n], " ")]; ok {
				return c.rayon, c.emoji
			}
		}
	}
	return "autre", "🛍️"
}

// unites reconnues après un nombre (« 2 kg de … »).
var unites = map[string]bool{
	"g": true, "kg": true, "gr": true, "l": true, "cl": true, "ml": true,
	"paquet": true, "paquets": true, "boite": true, "boites": true, "boîte": true, "boîtes": true,
	"bouteille": true, "bouteilles": true, "sachet": true, "sachets": true, "pot": true, "pots": true,
	"tranche": true, "tranches": true, "pack": true, "packs": true, "botte": true, "bottes": true,
	"barquette": true, "barquettes": true, "brique": true, "briques": true, "douzaine": true, "douzaines": true,
	"rouleau": true, "rouleaux": true, "filet": true, "filets": true, "pièce": true, "pièces": true,
}

// Analyser sépare la quantité du nom : « 2 baguettes » → (« Baguettes », « 2 »),
// « 1 kg de pommes » → (« Pommes », « 1 kg »), « lait x3 » → (« Lait », « 3 »).
func Analyser(saisie string) (nom, quantite string) {
	mots := strings.Fields(saisie)
	if len(mots) == 0 {
		return "", ""
	}
	// « lait x3 » ou « lait ×3 »
	if dernier := mots[len(mots)-1]; len(mots) > 1 && (strings.HasPrefix(dernier, "x") || strings.HasPrefix(dernier, "×")) {
		reste := strings.TrimPrefix(strings.TrimPrefix(dernier, "x"), "×")
		if estNombre(reste) {
			return majuscule(strings.Join(mots[:len(mots)-1], " ")), reste
		}
	}
	if len(mots) > 1 && estNombre(mots[0]) {
		q := []string{mots[0]}
		i := 1
		if i < len(mots)-1 && unites[strings.ToLower(mots[i])] {
			q = append(q, mots[i])
			i++
		}
		// « de » ou « d' » après une quantité : « 1 kg de pommes », « 2 l d'eau ».
		if i < len(mots)-1 && strings.EqualFold(mots[i], "de") {
			i++
		} else {
			for _, pre := range []string{"d'", "d’", "D'", "D’"} {
				if strings.HasPrefix(mots[i], pre) && len(mots[i]) > len(pre) {
					mots[i] = mots[i][len(pre):]
					break
				}
			}
		}
		return majuscule(strings.Join(mots[i:], " ")), strings.Join(q, " ")
	}
	return majuscule(strings.Join(mots, " ")), ""
}

func estNombre(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsDigit(r) && r != ',' && r != '.' && r != '/' {
			return false
		}
	}
	return unicode.IsDigit([]rune(s)[0])
}

// majuscule : première lettre en majuscule.
func majuscule(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}
