// SPDX-License-Identifier: GPL-3.0-or-later

package suivi

import (
	"testing"
	"time"
)

// Texte lu par Tesseract.js sur une vraie photo de ticket (tournée d'un
// quart de tour, 1 600 px), numéros de carte et de transaction retirés.
const ticketLu1600 = `AUCHAN ST GENIS LAVAL
$
CARTE BANCAIRE E
MM visa fe
TP Le : ‘19/26 -a>-e )
Ÿ Le 29/1072R0 55 12:42:53
HN DAC AUCHAN CARBU 69 Le
A ST GENYS LAVAI I
No AUTO : 931202 ! 1
MONTANT REEËL : il WA
62.24 EUR NU I
Pour Information - fi il WA
408.27 FRF a
DEBIT W | I
fICKET A CONSERVER |
Ticket No - Vu i
No pompe 42 We
Carburant = SPsB 0
Quantite = 27,19 L Gt
Prix unit. = 2,289 EUR We
TVA 20,69% = 10,37 EUR Ou
indications non
Controlces par V'=et3E Ve
LA STAYTONM VOUS REMERCIE RC
|, ET vous sOUHAITE Med
A BONNE ROUTE LOS`

// Le même ticket lu en plus grand (2 400 px) : la date est lisible, les
// litres et le prix au litre non.
const ticketLu2400 = `HN Le : 29/10/26 a: 12:42: 53 1
! MONTANT REEL : KM il
62.24. EUR KR Vi
Carburant = 4 19 L Ë
Quanti te —- 5 389 EUR nn il
Prix syeh so - 106 37 EUR nu fl
cantrolees par 1° “*“ ER`

func TestLireTicket(t *testing.T) {
	auj := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	// Première lecture : les nombres, pas la date.
	x := LireTicket(ticketLu1600, auj)
	if x.Litres != "27,19" || x.PrixLitre != "2,289" || x.Montant != "62,24" || !x.Complet {
		t.Fatalf("nombres : %+v", x)
	}
	if x.Carburant != "SP98" || x.Note != "Auchan St Genis Laval" {
		t.Errorf("carburant, note : %q, %q", x.Carburant, x.Note)
	}
	if x.Jour != "" {
		t.Errorf("date inventée : %q", x.Jour)
	}
	// Les deux lectures ensemble : tout.
	x = LireTicket(ticketLu1600+"\n"+ticketLu2400, auj)
	if x.Jour != "2026-10-29" || x.Litres != "27,19" || x.Montant != "62,24" {
		t.Fatalf("deux lectures : %+v", x)
	}
	// La seconde seule : la date et le montant, sans trio cohérent.
	x = LireTicket(ticketLu2400, auj)
	if x.Jour != "2026-10-29" || x.Montant != "62,24" || x.Complet {
		t.Fatalf("seconde lecture : %+v", x)
	}
}

func TestLireTicketAutres(t *testing.T) {
	auj := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	cas := []struct {
		texte                                 string
		jour, litres, montant, prix, carbu, n string
	}{
		{"TOTAL ENERGIES RENNES\n12.11.2026 08:15\nGAZOLE\nVOLUME 41,52 L\nPU 1,689 EUR/L\nTOTAL TTC 70,13 EUR", "2026-11-12", "41,52", "70,13", "1,689", "Gazole", "Total Energies Rennes"},
		// Deux sur trois : les litres se calculent.
		{"le 03/11/26\nSP95-E10\nPrix unit. 1,750\nMontant 52,50 EUR", "2026-11-03", "30,00", "52,50", "1,750", "SP95-E10", ""},
		// Date dans le futur ou trop ancienne : ignorée.
		{"31/12/2027\n01/01/2020\nQuantite 10,00 L\nPrix 2,000\nTotal 20,00", "", "10,00", "20,00", "2,000", "", ""},
		// « E10 » seul, « SANS PLOMB 98 ».
		{"CARBU E10\nQuantite 10,00 L\nPrix 2,000\nTotal 20,00", "", "10,00", "20,00", "2,000", "SP95-E10", ""},
		{"SANS PLOMB 98\nQuantite 10,00 L\nPrix 2,000\nTotal 20,00", "", "10,00", "20,00", "2,000", "SP98", ""},
	}
	for _, c := range cas {
		x := LireTicket(c.texte, auj)
		if x.Jour != c.jour || x.Litres != c.litres || x.Montant != c.montant || x.PrixLitre != c.prix || x.Carburant != c.carbu || x.Note != c.n {
			t.Errorf("%q :\n%+v", c.texte, x)
		}
	}
}

// Ticket posé sur un tissu vert : la photo entière était illisible ; le
// téléphone ne lit plus que la zone du ticket (app.js, zone). Première
// lecture (1 600 px) : la ligne en gros caractères « SP98 EXC » manque.
const ticketVert1600 = `| =
Te ;
PIERRE BEnScS Energies
126 BD DE | <E
pIERRE BENT Culte D1S OULL
TEL 04723S60000 >> 10
M eaces7enes
a TRA à
COM: FRS9306916099
FR
SIRET: 306916
Code NAF: 470.0 =264
Vers. logicier.
FCx Compact Pos Vynamic
7.19.15-sp5S-hfr1i
28/11/26 10:09:55
=> Justificatif de paiement
pompe 05
=
20,30 lit
-
1,990 €/lit
Total 40,40€
cB (D) € 46,40
TVA incluse:
Taux TTC TVA
23 20,00% 40,40 6,73
Total H.T. 33,67
Lignes articles: 1
N° de ticket : 105000044432
Code hash:
(NFS25) B525S/0587-1 -Pn-
TotalEnergies
Carte bancaire
TICKET CLIENT
REL.PIERRE BENITE
126 BOULEVARD DE L EUROPE
69310 PYERRE BENITE
30691609902564
28/11/2026 10:10:00 |
PAIEMENT
AUTORISÉ
\ nab
Montant fAnoL, 40,46 EUR`

// Seconde lecture (2 400 px, mode « une colonne », lignes vides retirées) :
// le carburant est lu.
const ticketVert2400 = `To ;
PIERRE BENTre Energies
EUROP
pIERRE BEunr E D15 ouLL
TEL 6047239600. > =10
FAX 0160679001
TVA INTRACOM:
FRS9306916099
FR
SIRET: 3069160
Code NAF: 4730 = 204
Vers. logicien:
FCx Compact Pos
7.19.15-spS-hfr1
Vynamic
28/11/26 10:09:55
=> Justificatif de paiement
SP98 EXC
pompe 05
20,30 lit
1,990 €/lit
— mm © = = = 0 me 0e 4 00 00 04 1 00 NA 00 1 00 A 0 00 0 0
Total 40,40€
€
cB (D) 40,40
TVA incluse:
Taux RG TVA
3 26,00% 40,40 6,73
Total H.T. 33,67
Lignes articles: 1
N° de ticket : 105000044432
Code hash:
(NF525) B525/058&7-1 -Pn-
TotalEnergies
Carte bancaire
TICKET CLIENT
REL.PIERRE BENITE
126 BOULEVARD DE L EUROPE
69310 PYERRE BENITE
30691609902564
28/11/2026 10:10:00
PAIEMENT
AUTORISÉ
2. mal
Montant fin 40,40 EUR`

func TestLireTicketFondVert(t *testing.T) {
	auj := time.Date(2026, 11, 28, 11, 0, 0, 0, time.UTC)
	x := LireTicket(ticketVert1600, auj)
	if x.Jour != "2026-11-28" || x.Litres != "20,30" || x.PrixLitre != "1,990" || x.Montant != "40,40" || !x.Complet {
		t.Fatalf("première lecture : %+v", x)
	}
	if x.Carburant != "" {
		t.Errorf("carburant inventé : %q", x.Carburant)
	}
	x = LireTicket(ticketVert1600+"\n"+ticketVert2400, auj)
	if x.Carburant != "SP98" || x.Litres != "20,30" || x.Montant != "40,40" {
		t.Fatalf("deux lectures : %+v", x)
	}
}

// Un mot d'une lettre (logo mal lu) : pas un nom de station.
func TestStation(t *testing.T) {
	for l, attendu := range map[string]string{"TOT R": "", "AUCHAN ST GENIS LAVAL": "Auchan St Genis Laval", "PIERRE BENITE": "Pierre Benite"} {
		if s := station(l); s != attendu {
			t.Errorf("%q : %q", l, s)
		}
	}
}
