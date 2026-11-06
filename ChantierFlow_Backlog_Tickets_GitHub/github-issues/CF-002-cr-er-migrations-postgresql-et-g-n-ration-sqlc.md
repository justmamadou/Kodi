---
title: "CF-002 — Créer migrations PostgreSQL et génération sqlc"
labels: "phase-1,p0,backend"
---

## Epic
CF-EPIC-01 — Fondations, sécurité et multi-tenancy

## User story
En tant que développeur, je veux versionner le schéma SQL.

## Priorité
**P0** — Bloquant pour le lancement

## Tâches techniques
- [ ] Créer migrations golang-migrate
- [ ] configurer pgx/v5 et sqlc
- [ ] ajouter contraintes et index
- [ ] tester migrations sur PostgreSQL.

## Critères d'acceptation
- [ ] Migrations up/down fonctionnent
- [ ] sqlc generate passe
- [ ] les tables tenant-owned portent organization_id
- [ ] les FK composites protègent les relations pertinentes.

## Definition of Done
- [ ] Code revu et fusionné
- [ ] Tests automatisés passants
- [ ] Permissions et isolation tenant vérifiées
- [ ] Documentation/OpenAPI mises à jour
