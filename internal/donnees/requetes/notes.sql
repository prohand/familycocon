-- Notes : une note privee n'est visible que par son auteur.

-- Liste : debut du texte seulement, nombre borne. motif '' : pas de
-- recherche ; sinon le texte cherche, tel quel (% et _ compris) ; filtre : '', 'partagees' ou 'privees'.
-- name: NotesVisibles :many
SELECT id, auteur_id, titre, CAST(substr(texte, 1, 300) AS TEXT) AS debut, couleur, epinglee, privee, modifie_le
FROM notes
WHERE famille_id = sqlc.arg(famille_id) AND (privee = 0 OR auteur_id = sqlc.arg(membre_id))
  AND (sqlc.arg(filtre) = '' OR (sqlc.arg(filtre) = 'privees' AND privee = 1) OR (sqlc.arg(filtre) = 'partagees' AND privee = 0))
  AND (sqlc.arg(motif) = '' OR instr(lower(titre), lower(sqlc.arg(motif))) > 0 OR instr(lower(texte), lower(sqlc.arg(motif))) > 0)
ORDER BY epinglee DESC, modifie_le DESC LIMIT sqlc.arg(nombre);

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

-- name: CompterNotesFamille :one
SELECT count(*) FROM notes WHERE famille_id = ?;

-- Membre retire : ses notes privees, que personne d'autre ne peut voir ni supprimer.
-- name: SupprimerNotesPriveesMembre :exec
DELETE FROM notes WHERE famille_id = ? AND auteur_id = ? AND privee = 1;
