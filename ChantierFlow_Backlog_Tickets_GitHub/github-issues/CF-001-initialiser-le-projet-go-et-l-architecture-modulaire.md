---
title: "CF-001 — Initialiser le projet Go et l'architecture modulaire"
labels: "phase-1,p0,backend"
---

## Epic
CF-EPIC-01 — Fondations, sécurité et multi-tenancy

## User story
En tant que développeur, je veux un socle Go maintenable.

## Priorité
**P0** — Bloquant pour le lancement

## Tâches techniques
- [ ] Créer module Go et structure domain/application/infrastructure/transport
- [ ] intégrer Chi et configuration validée
- [ ] ajouter Dockerfile et environnement local
- [ ] configurer lint, tests et CI.

## Critères d'acceptation
- [ ] Le projet compile
- [ ] la configuration est validée au démarrage
- [ ] /healthz répond
- [ ] la CI exécute lint, tests et build.

## Definition of Done
- [ ] Code revu et fusionné
- [ ] Tests automatisés passants
- [ ] Permissions et isolation tenant vérifiées
- [ ] Documentation/OpenAPI mises à jour
