-- Notes : une note privee n'est visible que par son auteur ; une note
-- reservee aux parents (elle-meme ou sa categorie) est cachee aux enfants,
-- sauf a son auteur. parent : 1 pour un parent.

-- Liste : debut du texte seulement, nombre borne. motif '' : pas de
-- recherche ; sinon le texte cherche, tel quel (% et _ compris) ; filtre : '', 'partagees' ou 'privees'.
-- Filtre : '' toutes ou 'privees' ; categorie : 0 toutes, -1 sans categorie, sinon son numero.
-- name: NotesVisibles :many
SELECT n.id, n.auteur_id, n.titre, CAST(substr(n.texte, 1, 300) AS TEXT) AS debut, n.couleur, n.epinglee, n.privee, n.modifie_le,
  n.rappel_le, n.rappel_envoye, n.categorie_id, (SELECT count(*) FROM photos_notes p WHERE p.note_id = n.id) AS photos,
  CAST((n.parents = 1 OR EXISTS (SELECT 1 FROM categories_notes k WHERE k.id = n.categorie_id AND k.parents = 1)) AS INTEGER) AS cachee
FROM notes n
WHERE n.famille_id = sqlc.arg(famille_id) AND (n.auteur_id = sqlc.arg(membre_id) OR (n.privee = 0 AND (CAST(sqlc.arg(parent) AS INTEGER) = 1 OR NOT (n.parents = 1 OR EXISTS (SELECT 1 FROM categories_notes k WHERE k.id = n.categorie_id AND k.parents = 1)))))
  AND (sqlc.arg(filtre) = '' OR (sqlc.arg(filtre) = 'privees' AND n.privee = 1))
  AND (sqlc.arg(categorie) = 0 OR (sqlc.arg(categorie) = -1 AND n.categorie_id IS NULL) OR n.categorie_id = sqlc.arg(categorie))
  AND (sqlc.arg(motif) = '' OR instr(plier(n.titre), plier(sqlc.arg(motif))) > 0 OR instr(plier(n.texte), plier(sqlc.arg(motif))) > 0)
ORDER BY n.epinglee DESC, n.modifie_le DESC LIMIT sqlc.arg(nombre);

-- name: NoteParID :one
SELECT * FROM notes WHERE id = ? AND famille_id = ?;

-- name: CreerNote :one
INSERT INTO notes (famille_id, auteur_id, titre, texte, couleur, epinglee, privee, parents, cree_le, modifie_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- Rien n'est ecrit si la note a change depuis son ouverture (avant : 0 =
-- pas de controle) : deux membres ne s'effacent pas l'un l'autre.
-- name: ModifierNote :execrows
UPDATE notes SET titre = sqlc.arg(titre), texte = sqlc.arg(texte), couleur = sqlc.arg(couleur), epinglee = sqlc.arg(epinglee),
  privee = sqlc.arg(privee), parents = sqlc.arg(parents), modifie_le = sqlc.arg(modifie_le)
WHERE id = sqlc.arg(id) AND famille_id = sqlc.arg(famille_id) AND (sqlc.arg(avant) = 0 OR modifie_le = sqlc.arg(avant));

-- Case d'une liste cochee : seulement le texte change.
-- name: CocherNote :execrows
UPDATE notes SET texte = sqlc.arg(texte), modifie_le = sqlc.arg(modifie_le)
WHERE id = sqlc.arg(id) AND famille_id = sqlc.arg(famille_id) AND texte = sqlc.arg(ancien);

-- name: EpinglerNote :exec
UPDATE notes SET epinglee = ? WHERE id = ? AND famille_id = ?;

-- name: SupprimerNote :exec
DELETE FROM notes WHERE id = ? AND famille_id = ?;

-- name: CompterNotes :one
SELECT count(*) FROM notes n WHERE n.famille_id = sqlc.arg(famille_id) AND (n.auteur_id = sqlc.arg(membre_id) OR (n.privee = 0 AND (CAST(sqlc.arg(parent) AS INTEGER) = 1 OR NOT (n.parents = 1 OR EXISTS (SELECT 1 FROM categories_notes k WHERE k.id = n.categorie_id AND k.parents = 1)))));

-- name: CompterNotesFamille :one
SELECT count(*) FROM notes WHERE famille_id = ?;

-- Membre retire : ses notes privees, que personne d'autre ne peut voir ni supprimer.
-- name: SupprimerNotesPriveesMembre :exec
DELETE FROM notes WHERE famille_id = ? AND auteur_id = ? AND privee = 1;

-- Photos des notes.

-- name: AjouterPhotoNote :one
INSERT INTO photos_notes (famille_id, note_id, fichier, taille, cree_par, cree_le) VALUES (?, ?, ?, ?, ?, ?) RETURNING *;

-- name: PhotoNoteParID :one
SELECT * FROM photos_notes WHERE id = ? AND famille_id = ?;

-- name: PhotosNote :many
SELECT * FROM photos_notes WHERE note_id = ? AND famille_id = ? ORDER BY id;

-- name: CompterPhotosNote :one
SELECT count(*) FROM photos_notes WHERE note_id = ?;

-- name: CompterPhotosNotesFamille :one
SELECT count(*) FROM photos_notes WHERE famille_id = ?;

-- name: SupprimerPhotoNote :exec
DELETE FROM photos_notes WHERE id = ? AND famille_id = ?;

-- Photos des notes privees d'un membre retire (effacees avec elles).
-- name: PhotosNotesPriveesMembre :many
SELECT p.* FROM photos_notes p JOIN notes n ON n.id = p.note_id
WHERE n.famille_id = ? AND n.auteur_id = ? AND n.privee = 1;

-- Listes : cases cochees en bas (affichage) ou a leur place.
-- name: ChangerCocheesEnBas :exec
UPDATE notes SET cochees_en_bas = sqlc.arg(en_bas) WHERE id = sqlc.arg(id) AND famille_id = sqlc.arg(famille_id);

-- Rappel d'une note (0 : aucun) ; un nouveau rappel repart a zero.
-- name: ChangerRappelNote :exec
UPDATE notes SET rappel_le = sqlc.arg(rappel_le), rappel_envoye = 0 WHERE id = sqlc.arg(id) AND famille_id = sqlc.arg(famille_id);

-- Rappels arrives a echeance (toutes les familles).
-- name: NotesARappeler :many
SELECT n.id, n.famille_id, n.auteur_id, n.titre, n.texte, n.privee, n.rappel_le, CAST((n.parents = 1 OR EXISTS (SELECT 1 FROM categories_notes k WHERE k.id = n.categorie_id AND k.parents = 1)) AS INTEGER) AS cachee FROM notes n
WHERE rappel_le > 0 AND rappel_envoye = 0 AND rappel_le <= sqlc.arg(maintenant) ORDER BY rappel_le LIMIT 200;

-- name: RappelNoteEnvoye :exec
UPDATE notes SET rappel_envoye = 1 WHERE id = ?;

-- Categories des notes que ce membre voit (celles des parents : parents seulement),
-- avec le nombre de notes qu'il y voit.
-- name: CategoriesNotes :many
SELECT c.id, c.nom, c.emoji, c.parents,
  (SELECT count(*) FROM notes n WHERE n.categorie_id = c.id AND (n.auteur_id = sqlc.arg(membre_id) OR (n.privee = 0 AND (CAST(sqlc.arg(parent) AS INTEGER) = 1 OR NOT (n.parents = 1 OR EXISTS (SELECT 1 FROM categories_notes k WHERE k.id = n.categorie_id AND k.parents = 1)))))) AS nombre
FROM categories_notes c WHERE c.famille_id = sqlc.arg(famille_id) AND (CAST(sqlc.arg(parent) AS INTEGER) = 1 OR c.parents = 0)
ORDER BY c.nom COLLATE NOCASE;

-- name: CompterCategoriesNotes :one
SELECT count(*) FROM categories_notes WHERE famille_id = ?;

-- Categorie utilisable par ce membre (parent : 1 pour un parent).
-- name: CategorieNoteExiste :one
SELECT count(*) FROM categories_notes
WHERE id = sqlc.arg(id) AND famille_id = sqlc.arg(famille_id) AND (parents = 0 OR CAST(sqlc.arg(parent) AS INTEGER) = 1);

-- name: CategorieNoteParents :one
SELECT parents FROM categories_notes WHERE id = ?;

-- name: CreerCategorieNote :one
INSERT INTO categories_notes (famille_id, nom, emoji, parents, cree_le) VALUES (?, ?, ?, ?, ?) RETURNING *;

-- name: ModifierCategorieNote :exec
UPDATE categories_notes SET nom = sqlc.arg(nom), emoji = sqlc.arg(emoji)
WHERE id = sqlc.arg(id) AND famille_id = sqlc.arg(famille_id) AND (parents = 0 OR CAST(sqlc.arg(parent) AS INTEGER) = 1);

-- Reserver une categorie aux parents (ou la rendre a tous) : un parent seulement.
-- name: ChangerParentsCategorie :exec
UPDATE categories_notes SET parents = ? WHERE id = ? AND famille_id = ?;

-- Les notes de la categorie restent, sans categorie (ON DELETE SET NULL).
-- name: SupprimerCategorieNote :exec
DELETE FROM categories_notes
WHERE id = sqlc.arg(id) AND famille_id = sqlc.arg(famille_id) AND (parents = 0 OR CAST(sqlc.arg(parent) AS INTEGER) = 1);

-- name: ChangerCategorieNote :exec
UPDATE notes SET categorie_id = sqlc.arg(categorie_id) WHERE id = sqlc.arg(id) AND famille_id = sqlc.arg(famille_id);
