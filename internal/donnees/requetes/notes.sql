-- Notes : une note privee n'est visible que par son auteur.

-- Liste : debut du texte seulement, nombre borne. motif '' : pas de
-- recherche ; sinon le texte cherche, tel quel (% et _ compris) ; filtre : '', 'partagees' ou 'privees'.
-- name: NotesVisibles :many
SELECT n.id, n.auteur_id, n.titre, CAST(substr(n.texte, 1, 300) AS TEXT) AS debut, n.couleur, n.epinglee, n.privee, n.modifie_le,
  (SELECT count(*) FROM photos_notes p WHERE p.note_id = n.id) AS photos
FROM notes n
WHERE n.famille_id = sqlc.arg(famille_id) AND (n.privee = 0 OR n.auteur_id = sqlc.arg(membre_id))
  AND (sqlc.arg(filtre) = '' OR (sqlc.arg(filtre) = 'privees' AND n.privee = 1) OR (sqlc.arg(filtre) = 'partagees' AND n.privee = 0))
  AND (sqlc.arg(motif) = '' OR instr(plier(n.titre), plier(sqlc.arg(motif))) > 0 OR instr(plier(n.texte), plier(sqlc.arg(motif))) > 0)
ORDER BY n.epinglee DESC, n.modifie_le DESC LIMIT sqlc.arg(nombre);

-- name: NoteParID :one
SELECT * FROM notes WHERE id = ? AND famille_id = ?;

-- name: CreerNote :one
INSERT INTO notes (famille_id, auteur_id, titre, texte, couleur, epinglee, privee, cree_le, modifie_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- Rien n'est ecrit si la note a change depuis son ouverture (avant : 0 =
-- pas de controle) : deux membres ne s'effacent pas l'un l'autre.
-- name: ModifierNote :execrows
UPDATE notes SET titre = sqlc.arg(titre), texte = sqlc.arg(texte), couleur = sqlc.arg(couleur), epinglee = sqlc.arg(epinglee),
  privee = sqlc.arg(privee), modifie_le = sqlc.arg(modifie_le)
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
SELECT count(*) FROM notes WHERE famille_id = sqlc.arg(famille_id) AND (privee = 0 OR auteur_id = sqlc.arg(membre_id));

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
