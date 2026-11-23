#!/bin/sh
# Restauration d'une sauvegarde de FamilyCocon.
#
#   ./scripts/restauration.sh /srv/sauvegardes/familycocon/familycocon-20261122-021500.db
#
# À lancer dans le dossier du docker-compose.yml. Étapes :
#   1. arrêt du conteneur ;
#   2. le binaire vérifie la sauvegarde (intégrité, version) et la met en
#      place ; l'ancienne base est gardée dans le volume
#      (familycocon.db.avant-restauration-…) ;
#   3. redémarrage.
set -eu

FICHIER=${1:?usage : restauration.sh FICHIER.db}
SERVICE=${SERVICE:-familycocon}
[ -f "$FICHIER" ] || { echo "fichier introuvable : $FICHIER" >&2; exit 1; }

# Copie lisible par l'utilisateur du conteneur (uid 65532).
temp=$(mktemp -d)
trap 'rm -rf "$temp"' EXIT
cp "$FICHIER" "$temp/restauration.db"
# En root : la copie appartient à l'utilisateur du conteneur, personne d'autre
# ne peut la lire. Sinon (pas le droit de changer le propriétaire) : lisible
# par tous le temps de la restauration.
if chown -R 65532:65532 "$temp" 2>/dev/null; then
  chmod 700 "$temp"
  chmod 600 "$temp/restauration.db"
else
  echo "note : lancé sans root, la copie temporaire est lisible par les autres comptes de cette machine" >&2
  chmod 755 "$temp"
  chmod 644 "$temp/restauration.db"
fi

echo "Arrêt de FamilyCocon…"
docker compose stop "$SERVICE"
echo "Restauration…"
# Fichier refusé (abîmé, version trop récente) : l'ancienne base est intacte,
# le service est relancé avant de sortir en erreur.
if ! docker compose run --rm --no-deps -v "$temp/restauration.db:/restauration.db:ro" "$SERVICE" -restaurer /restauration.db; then
  echo "Restauration refusée : l'ancienne base est gardée. Redémarrage…" >&2
  docker compose start "$SERVICE"
  exit 1
fi
echo "Redémarrage…"
docker compose start "$SERVICE"
echo "Fait. Vérifiez : curl -fsS http://127.0.0.1:8080/health"
