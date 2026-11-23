-- Agenda dans le calendrier du telephone : un lien secret par membre (seule
-- son empreinte SHA-256 est gardee). Recreer le lien rend l'ancien inutile.
CREATE TABLE agenda_ical (
  membre_id  INTEGER PRIMARY KEY REFERENCES membres(id) ON DELETE CASCADE,
  famille_id INTEGER NOT NULL REFERENCES familles(id) ON DELETE CASCADE,
  empreinte  BLOB    NOT NULL UNIQUE,
  cree_le    INTEGER NOT NULL,
  lu_le      INTEGER NOT NULL DEFAULT 0
);
