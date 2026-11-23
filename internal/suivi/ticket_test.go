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
	if x.Note != "SP98 · Auchan St Genis Laval" {
		t.Errorf("note : %q", x.Note)
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
		texte                          string
		jour, litres, montant, prix, n string
	}{
		{"TOTAL ENERGIES RENNES\n12.11.2026 08:15\nGAZOLE\nVOLUME 41,52 L\nPU 1,689 EUR/L\nTOTAL TTC 70,13 EUR", "2026-11-12", "41,52", "70,13", "1,689", "Gazole · Total Energies Rennes"},
		// Deux sur trois : les litres se calculent.
		{"le 03/11/26\nSP95-E10\nPrix unit. 1,750\nMontant 52,50 EUR", "2026-11-03", "30,00", "52,50", "1,750", "SP95-E10"},
		// Date dans le futur ou trop ancienne : ignorée.
		{"31/12/2027\n01/01/2020\nQuantite 10,00 L\nPrix 2,000\nTotal 20,00", "", "10,00", "20,00", "2,000", ""},
	}
	for _, c := range cas {
		x := LireTicket(c.texte, auj)
		if x.Jour != c.jour || x.Litres != c.litres || x.Montant != c.montant || x.PrixLitre != c.prix || x.Note != c.n {
			t.Errorf("%q :\n%+v", c.texte, x)
		}
	}
}
