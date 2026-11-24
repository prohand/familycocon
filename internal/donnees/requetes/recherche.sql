-- Recherche globale (page Chercher) : quelques resultats par module, sans
-- majuscules ni accents (fonction plier, voir internal/base/fonctions.go).

-- name: ChercherEvenements :many
SELECT id, titre, lieu, debut, repetition FROM evenements
WHERE famille_id = sqlc.arg(famille_id)
  AND (instr(plier(titre), plier(sqlc.arg(motif))) > 0 OR instr(plier(lieu), plier(sqlc.arg(motif))) > 0
       OR instr(plier(note), plier(sqlc.arg(motif))) > 0)
ORDER BY debut DESC LIMIT 8;

-- name: ChercherTaches :many
SELECT id, titre, emoji FROM taches
WHERE taches.famille_id = sqlc.arg(famille_id) AND taches.active = 1 AND (instr(plier(taches.titre), plier(sqlc.arg(motif))) > 0 OR instr(plier(taches.description), plier(sqlc.arg(motif))) > 0)
  AND (taches.cree_par = sqlc.arg(membre_id) OR (taches.privee = 0 AND (taches.assigne_a = sqlc.arg(membre_id) OR instr(',' || taches.tour || ',', ',' || sqlc.arg(membre_id) || ',') > 0 OR CAST(sqlc.arg(parent) AS INTEGER) = 1 OR NOT (taches.parents = 1 OR EXISTS (SELECT 1 FROM categories_taches k WHERE k.id = taches.categorie_id AND k.parents = 1)))))
ORDER BY id DESC LIMIT 8;

-- name: ChercherProjets :many
SELECT id, nom, emoji, termine_le FROM projets
WHERE famille_id = sqlc.arg(famille_id) AND perso_de IN (0, sqlc.arg(membre))
  AND (instr(plier(nom), plier(sqlc.arg(motif))) > 0 OR instr(plier(description), plier(sqlc.arg(motif))) > 0)
ORDER BY termine_le, id DESC LIMIT 8;

-- name: ChercherProduits :many
SELECT produits.id, produits.nom, produits.quantite, produits.emoji, produits.liste_id, listes.nom AS liste
FROM produits JOIN listes ON listes.id = produits.liste_id
WHERE listes.famille_id = sqlc.arg(famille_id) AND produits.pris = 0
  AND instr(plier(produits.nom), plier(sqlc.arg(motif))) > 0
ORDER BY produits.id DESC LIMIT 8;

-- name: ChercherRepas :many
SELECT jour, moment, nom FROM repas
WHERE famille_id = sqlc.arg(famille_id)
  AND (instr(plier(nom), plier(sqlc.arg(motif))) > 0 OR instr(plier(ingredients), plier(sqlc.arg(motif))) > 0)
ORDER BY jour DESC LIMIT 8;
