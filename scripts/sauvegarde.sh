#!/bin/sh
# Sauvegarde à chaud de FamilyCocon, copiée sur l'hôte (le serveur continue
# de tourner). La copie est vérifiée (PRAGMA integrity_check) avant d'être
# écrite. À lancer par cron, par exemple chaque nuit :
#
#   15 2 * * *  /opt/familycocon/scripts/sauvegarde.sh /srv/sauvegardes/familycocon
#
# Variables : CONTENEUR (défaut : familycocon), GARDER (défaut : 30 copies).
set -eu
# La base contient des empreintes de mots de passe et des secrets : les
# fichiers créés ici ne sont lisibles que par leur propriétaire, dès l'écriture.
umask 077

CONTENEUR=${CONTENEUR:-familycocon}
GARDER=${GARDER:-30}
DOSSIER=${1:-./sauvegardes}

mkdir -p "$DOSSIER"
fichier="$DOSSIER/familycocon-$(date +%Y%m%d-%H%M%S).db"
temp="$fichier.partiel"

# La sauvegarde est écrite sur la sortie standard du binaire.
if ! docker exec "$CONTENEUR" /familycocon -sauvegarde - > "$temp"; then
  rm -f "$temp"
  echo "sauvegarde ratée" >&2
  exit 1
fi
if [ ! -s "$temp" ]; then
  rm -f "$temp"
  echo "sauvegarde vide" >&2
  exit 1
fi
mv "$temp" "$fichier"
chmod 600 "$fichier"
echo "sauvegarde : $fichier ($(du -h "$fichier" | cut -f1))"

# Rotation : seules les GARDER plus récentes sont gardées.
ls -1t "$DOSSIER"/familycocon-*.db 2>/dev/null | tail -n +"$((GARDER + 1))" | while read -r ancien; do
  rm -f -- "$ancien"
done
