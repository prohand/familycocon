# CLAUDE.md — règles de travail du projet FamilyCocon

> À relire à chaque session, puis `SPEC.md` (décisions) et `PROGRES.md` (où on en est).

## Langue et style
- Réponses en français, simples, sans longue introduction, en listes courtes.
- Code commenté en français, simple et lisible : l'utilisateur doit pouvoir le maintenir seul.
- Licence : GPL-3.0-or-later (`LICENSE`). En-tête `// SPDX-License-Identifier: GPL-3.0-or-later`
  dans chaque fichier Go ; dépendances compatibles GPL-3.0 uniquement.

## Priorité absolue : la légèreté
- Bibliothèque standard de Go d'abord. Toute dépendance est justifiée dans `SPEC.md`
  (coût RAM / taille du binaire).
- Pas de framework JS : htmx + un petit `app.js`. CSS écrit à la main (variables CSS).
- Pas de script ni de style écrit dans les pages (CSP stricte) : couleurs par classes.

## Code repris
- `mirador-stack` (`github.com/prohand/mirador-stack`, GPL-3.0) : **l'utilisateur en
  est l'auteur**. Son code peut être repris ; indiquer la provenance en commentaire
  (« repris de mirador-stack, chemin »).
- Ne jamais reprendre le nom, le logo, les couleurs ou les illustrations d'une autre appli.

## Ce que je n'ai pas le droit de faire
- Aucun accès à l'infrastructure de l'utilisateur. Je propose, il valide et déploie.
- Jamais de secret (mot de passe, clé, URL privée) dans le code, les tests, les logs ou les commits.

## Méthode
- Travailler étape par étape (voir `PROGRES.md`) ; chaque étape fonctionne seule.
- Tests avant de présenter un résultat : `make verif` puis `make test`.
- Requêtes SQL : dans `internal/donnees/requetes/*.sql`, puis `make sql` (sqlc).
  Ne jamais modifier les fichiers `*_gen.go` à la main.
- Schéma : une nouvelle migration `internal/base/migrations/NNNN_nom.sql` ; ne jamais
  modifier une migration déjà publiée.
- Mesurer la RAM et la taille de l'image à la fin de chaque grosse étape (`PROGRES.md`).
- Questions à l'utilisateur : 5 au maximum, regroupées.
