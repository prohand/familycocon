-- Courses : listes et produits.

-- name: ListesDeFamille :many
SELECT * FROM listes WHERE famille_id = ? ORDER BY id;

-- name: ListeParID :one
SELECT * FROM listes WHERE id = ? AND famille_id = ?;

-- name: CreerListe :one
INSERT INTO listes (famille_id, nom, cree_le) VALUES (?, ?, ?) RETURNING *;

-- name: RenommerListe :exec
UPDATE listes SET nom = ? WHERE id = ? AND famille_id = ?;

-- name: SupprimerListe :exec
DELETE FROM listes WHERE id = ? AND famille_id = ?;

-- name: ProduitsDeListe :many
SELECT * FROM produits WHERE liste_id = ? ORDER BY pris, id;

-- name: ProduitParID :one
SELECT produits.* FROM produits JOIN listes ON listes.id = produits.liste_id
WHERE produits.id = ? AND listes.famille_id = ?;

-- name: AjouterProduit :one
INSERT INTO produits (liste_id, nom, quantite, rayon, emoji, ajoute_par, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: CocherProduit :exec
UPDATE produits SET pris = ?, pris_par = ?, pris_le = ? WHERE id = ?;

-- name: ModifierProduit :exec
UPDATE produits SET nom = ?, quantite = ?, rayon = ?, emoji = ? WHERE id = ?;

-- name: SupprimerProduit :exec
DELETE FROM produits WHERE id = ?;

-- name: ViderPris :execrows
DELETE FROM produits WHERE liste_id = ? AND pris = 1;

-- name: CompterAPrendre :one
SELECT count(*) FROM produits JOIN listes ON listes.id = produits.liste_id
WHERE listes.famille_id = ? AND produits.pris = 0;

-- name: ProduitConnu :one
SELECT * FROM produits_connus WHERE famille_id = ? AND cle = ?;

-- name: RetenirProduit :exec
INSERT INTO produits_connus (famille_id, cle, nom, rayon, emoji, compte, vu_le)
VALUES (?, ?, ?, ?, ?, 1, ?)
ON CONFLICT (famille_id, cle) DO UPDATE SET
  nom = excluded.nom, compte = produits_connus.compte + 1, vu_le = excluded.vu_le;

-- name: CorrigerRayon :exec
INSERT INTO produits_connus (famille_id, cle, nom, rayon, emoji, compte, vu_le)
VALUES (?, ?, ?, ?, ?, 1, ?)
ON CONFLICT (famille_id, cle) DO UPDATE SET rayon = excluded.rayon, emoji = excluded.emoji, vu_le = excluded.vu_le;

-- name: SuggestionsProduits :many
SELECT nom FROM produits_connus WHERE famille_id = ? ORDER BY compte DESC, vu_le DESC LIMIT 60;
