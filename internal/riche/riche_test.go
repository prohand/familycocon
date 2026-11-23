// SPDX-License-Identifier: GPL-3.0-or-later

package riche

import (
	"strings"
	"testing"
)

func TestHTML(t *testing.T) {
	texte := "## Pneus\n" +
		"Continental **DWS06+** : _très_ ++bons++ ~~chers~~ `code`\n" +
		"https://www.reddit.com/r/mazda3/the_pothole_won_are/ et [la vidéo](https://youtube.com/watch?v=__s5)\n" +
		"- un\n- deux\n" +
		"3. trois\n4. quatre\n" +
		"> cité\n" +
		"---\n" +
		"[ ] bac\n[x] pare-soleil\n" +
		"[photo 12]\n" +
		"\\# pas un titre, 2\\*3, mon\\_fichier\n" +
		"<script>alert(1)</script> [piège](javascript:alert(1))"
	h := string(HTML(texte, Options{}))
	for _, attendu := range []string{
		"<h2>Pneus</h2>",
		"<strong>DWS06+</strong>", "<em>très</em>", "<u>bons</u>", "<s>chers</s>", "<code>code</code>",
		`<a href="https://www.reddit.com/r/mazda3/the_pothole_won_are/" target="_blank" rel="noopener noreferrer">https://www.reddit.com/r/mazda3/the_pothole_won_are/</a>`,
		`<a href="https://youtube.com/watch?v=__s5" target="_blank" rel="noopener noreferrer">la vidéo</a>`,
		"<ul><li>un</li><li>deux</li></ul>", `<ol start="3"><li>trois</li><li>quatre</li></ol>`,
		"<blockquote><div>cité</div></blockquote>", "<hr>",
		`<input type="checkbox" disabled> bac`, `<input type="checkbox" checked disabled> pare-soleil`,
		"[photo 12]", // hors d'une note : du texte
		"# pas un titre, 2*3, mon_fichier",
		"&lt;script&gt;alert(1)&lt;/script&gt;", "[piège](javascript:alert(1))",
	} {
		if !strings.Contains(h, attendu) {
			t.Errorf("sans %q :\n%s", attendu, h)
		}
	}
	if strings.Contains(h, "<script>") || strings.Contains(h, `href="javascript`) {
		t.Fatal("balise ou lien dangereux")
	}
	// Dans une note : photos et cases à cocher actives ; dans l'éditeur, les cases.
	n := string(HTML("[photo 12]\n[ ] bac", Options{Note: 5}))
	if !strings.Contains(n, `<img src="/notes/photos/12"`) || !strings.Contains(n, `hx-post="/notes/5/cocher"`) {
		t.Errorf("note :\n%s", n)
	}
	if e := string(HTML("[x] bac\n\nfin", Options{Mode: Edition})); !strings.Contains(e, `<div class="case-note" data-case data-cochee="1">bac</div>`) || !strings.Contains(e, "<div><br></div>") {
		t.Errorf("édition :\n%s", e)
	}
}

func TestSimpleEtEchapper(t *testing.T) {
	if s := Simple("## Titre\n**gras** [lien](https://a.fr)\n- puce\n[x] fait\n---"); s != "Titre\ngras lien\n• puce\n☑ fait\n—" {
		t.Errorf("Simple : %q", s)
	}
	brut := "2*3 = 6, mon_fichier [x] ~~ ++ `"
	if s := LigneSimple(Echapper(brut)); s != brut {
		t.Errorf("Echapper puis relire : %q", s)
	}
}

// Texte copié de Méli-Mélo : Markdown et balises mêlés, image jointe.
func TestDepuisCollage(t *testing.T) {
	colle := "**<u>Pneu :</u>**\nContinental dw06+ :\n\n![](attachment:680)\n\n" +
		"https://www.reddit.com/r/mazda3/comments/1izlduv/the_pothole_won_are_the_continental_dws06_still/\n" +
		"Pirelli *p0* as3plus :\n<b>Bac</b> de coffre<br>https://www.walser-shop.fr/bac-de-coffre-xtr-pour-mazda-3-hatchback-bp-11/2018-auj.\n" +
		"![photo](https://exemple.fr/a.jpg) &amp; <a href=\"https://b.fr\">lien</a>\n* puce"
	got := DepuisCollage(colle)
	want := "**++Pneu :++**\nContinental dw06+ :\n\n🖼️ (image à coller à part)\n\n" +
		"https://www.reddit.com/r/mazda3/comments/1izlduv/the_pothole_won_are_the_continental_dws06_still/\n" +
		"Pirelli _p0_ as3plus :\n**Bac** de coffre\nhttps://www.walser-shop.fr/bac-de-coffre-xtr-pour-mazda-3-hatchback-bp-11/2018-auj.\n" +
		"[🖼️ photo](https://exemple.fr/a.jpg) & [lien](https://b.fr)\n* puce"
	if got != want {
		t.Fatalf("\n%q\n%q", got, want)
	}
	h := string(HTML(got, Options{Mode: Edition}))
	for _, attendu := range []string{"<strong><u>Pneu :</u></strong>", "<em>p0</em>", `href="https://www.walser-shop.fr/bac-de-coffre-xtr-pour-mazda-3-hatchback-bp-11/2018-auj"`, "<ul><li>puce</li></ul>"} {
		if !strings.Contains(h, attendu) {
			t.Errorf("sans %q :\n%s", attendu, h)
		}
	}
}
