-- Notes : une note privee n'est visible que par son auteur.

-- name: NotesVisibles :many
SELECT * FROM notes
WHERE famille_id = sqlc.arg(famille_id) AND (privee = 0 OR auteur_id = sqlc.arg(membre_id))
  AND (titre LIKE sqlc.arg(motif) OR texte LIKE sqlc.arg(motif))
ORDER BY epinglee DESC, modifie_le DESC;

-- name: NoteParID :one
SELECT * FROM notes WHERE id = ? AND famille_id = ?;

-- name: CreerNote :one
INSERT INTO notes (famille_id, auteur_id, titre, texte, couleur, epinglee, privee, cree_le, modifie_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: ModifierNote :exec
UPDATE notes SET titre = ?, texte = ?, couleur = ?, epinglee = ?, privee = ?, modifie_le = ?
WHERE id = ? AND famille_id = ?;

-- name: EpinglerNote :exec
UPDATE notes SET epinglee = ? WHERE id = ? AND famille_id = ?;

-- name: SupprimerNote :exec
DELETE FROM notes WHERE id = ? AND famille_id = ?;

-- name: CompterNotes :one
SELECT count(*) FROM notes WHERE famille_id = sqlc.arg(famille_id) AND (privee = 0 OR auteur_id = sqlc.arg(membre_id));
