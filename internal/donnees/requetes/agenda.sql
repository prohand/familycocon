-- Agenda.

-- name: EvenementsPeriode :many
SELECT * FROM evenements
WHERE famille_id = sqlc.arg(famille_id) AND debut < sqlc.arg(fin_periode)
  AND (fin >= sqlc.arg(debut_periode)
       OR (repetition <> '' AND (repetition_fin = '' OR repetition_fin >= sqlc.arg(jour_min))))
ORDER BY debut;

-- name: CompterEvenements :one
SELECT count(*) FROM evenements WHERE famille_id = ?;

-- name: EvenementParID :one
SELECT * FROM evenements WHERE id = ? AND famille_id = ?;

-- name: CreerEvenement :one
INSERT INTO evenements (famille_id, titre, lieu, note, debut, fin, journee, repetition, repetition_fin, rappel, rappel_avant, cree_par, cree_le)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING *;

-- name: ModifierEvenement :exec
UPDATE evenements SET titre = ?, lieu = ?, note = ?, debut = ?, fin = ?, journee = ?, repetition = ?, repetition_fin = ?, rappel = ?, rappel_avant = ?
WHERE id = ? AND famille_id = ?;

-- name: SupprimerEvenement :exec
DELETE FROM evenements WHERE id = ? AND famille_id = ?;

-- name: ParticipantsDeFamille :many
SELECT evenements_membres.* FROM evenements_membres
JOIN evenements ON evenements.id = evenements_membres.evenement_id
WHERE evenements.famille_id = ?;

-- Les participants des evenements affiches seulement (ids deja filtres par famille).
-- name: ParticipantsDesEvenements :many
SELECT * FROM evenements_membres WHERE evenement_id IN (sqlc.slice('evenements'));

-- Les contacts cites par les evenements affiches (ids deja filtres par famille).
-- name: ContactsDesEvenements :many
SELECT ec.evenement_id, c.id, c.nom, c.telephone
FROM evenements_contacts ec JOIN contacts c ON c.id = ec.contact_id
WHERE ec.evenement_id IN (sqlc.slice('evenements')) ORDER BY c.nom COLLATE NOCASE;

-- Liste de choix du formulaire : le strict necessaire.
-- name: ContactsChoixAgenda :many
SELECT id, nom, metier FROM contacts WHERE famille_id = ? ORDER BY nom COLLATE NOCASE;

-- name: AjouterParticipant :exec
INSERT OR IGNORE INTO evenements_membres (evenement_id, membre_id) VALUES (?, ?);

-- name: ViderParticipants :exec
DELETE FROM evenements_membres WHERE evenement_id = ?;

-- Fois annulees ou modifiees des evenements lus (elles ne sont plus a
-- leur date d'origine).
-- name: ExceptionsDesEvenements :many
SELECT evenement_id, jour FROM evenements_exceptions WHERE evenement_id IN (sqlc.slice(ids));

-- name: ExceptionsEvenement :many
SELECT jour, modifiee, nouveau_debut, nouveau_fin, nouvelle_journee, nouveau_titre, nouveau_lieu
FROM evenements_exceptions WHERE evenement_id = ? ORDER BY jour;

-- Annuler une fois (meme une fois modifiee : elle devient annulee).
-- name: AjouterException :exec
INSERT INTO evenements_exceptions (evenement_id, jour) VALUES (?, ?)
ON CONFLICT (evenement_id, jour) DO UPDATE SET modifiee = 0, nouveau_debut = 0, nouveau_fin = 0,
  nouvelle_journee = 0, nouveau_titre = '', nouveau_lieu = NULL;

-- Modifier une fois seulement (date, heures, titre, lieu).
-- name: ModifierFois :exec
INSERT INTO evenements_exceptions (evenement_id, jour, modifiee, nouveau_debut, nouveau_fin, nouvelle_journee, nouveau_titre, nouveau_lieu)
VALUES (sqlc.arg(evenement_id), sqlc.arg(jour), 1, sqlc.arg(nouveau_debut), sqlc.arg(nouveau_fin), sqlc.arg(nouvelle_journee), sqlc.arg(nouveau_titre), sqlc.arg(nouveau_lieu))
ON CONFLICT (evenement_id, jour) DO UPDATE SET modifiee = 1, nouveau_debut = excluded.nouveau_debut,
  nouveau_fin = excluded.nouveau_fin, nouvelle_journee = excluded.nouvelle_journee,
  nouveau_titre = excluded.nouveau_titre, nouveau_lieu = excluded.nouveau_lieu;

-- Fois modifiees qui touchent la periode, avec leur evenement (une fois
-- deplacee peut tomber hors des dates de l'evenement).
-- name: FoisModifieesPeriode :many
SELECT sqlc.embed(evenements), x.jour, x.nouveau_debut, x.nouveau_fin, x.nouvelle_journee, x.nouveau_titre, x.nouveau_lieu
FROM evenements_exceptions x JOIN evenements ON evenements.id = x.evenement_id
WHERE evenements.famille_id = sqlc.arg(famille_id) AND evenements.repetition <> '' AND x.modifiee = 1
  AND x.nouveau_debut < sqlc.arg(fin_periode) AND x.nouveau_fin >= sqlc.arg(debut_periode)
ORDER BY x.nouveau_debut;

-- name: RetirerException :exec
DELETE FROM evenements_exceptions WHERE evenement_id = ? AND jour = ?;

-- name: CompterExceptions :one
SELECT count(*) FROM evenements_exceptions WHERE evenement_id = ?;

-- Rappel juste avant (15 min, 1 h, la veille a 18 h).

-- Familles qui ont au moins un evenement avec un rappel juste avant.
-- name: FamillesRappelAvant :many
SELECT id, fuseau, modules FROM familles
WHERE EXISTS (SELECT 1 FROM evenements WHERE evenements.famille_id = familles.id AND evenements.rappel_avant <> '')
ORDER BY id;

-- Comme EvenementsPeriode, ceux qui ont un rappel juste avant seulement.
-- name: EvenementsRappelAvant :many
SELECT * FROM evenements
WHERE famille_id = sqlc.arg(famille_id) AND rappel_avant <> '' AND debut < sqlc.arg(fin_periode)
  AND (fin >= sqlc.arg(debut_periode)
       OR (repetition <> '' AND (repetition_fin = '' OR repetition_fin >= sqlc.arg(jour_min))))
ORDER BY debut;

-- 0 ligne : rappel deja envoye pour cette fois.
-- name: MarquerRappelAvant :execrows
INSERT INTO evenements_rappels (evenement_id, debut, envoye_le) VALUES (?, ?, ?)
ON CONFLICT (evenement_id, debut) DO NOTHING;

-- name: OublierRappelsAvant :exec
DELETE FROM evenements_rappels WHERE envoye_le < ?;

-- Lien secret de l'agenda (calendrier du telephone).

-- name: PoserIcal :exec
INSERT INTO agenda_ical (membre_id, famille_id, empreinte, cree_le) VALUES (?, ?, ?, ?)
ON CONFLICT (membre_id) DO UPDATE SET empreinte = excluded.empreinte, cree_le = excluded.cree_le, lu_le = 0;

-- name: IcalMembre :one
SELECT cree_le, lu_le FROM agenda_ical WHERE membre_id = ?;

-- Le lien d'un membre encore actif seulement.
-- name: IcalParEmpreinte :one
SELECT agenda_ical.membre_id, agenda_ical.famille_id FROM agenda_ical
JOIN membres ON membres.id = agenda_ical.membre_id AND membres.actif = 1
WHERE agenda_ical.empreinte = ?;

-- name: NoterLectureIcal :exec
UPDATE agenda_ical SET lu_le = ? WHERE membre_id = ?;

-- name: SupprimerIcal :exec
DELETE FROM agenda_ical WHERE membre_id = ?;
