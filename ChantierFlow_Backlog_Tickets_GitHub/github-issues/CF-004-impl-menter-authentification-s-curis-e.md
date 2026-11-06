---
title: "CF-004 — Implémenter authentification sécurisée"
labels: "phase-1,p0,backend"
---

## Epic
CF-EPIC-01 — Fondations, sécurité et multi-tenancy

## User story
En tant qu'utilisateur, je veux me connecter de façon sécurisée.

## Priorité
**P0** — Bloquant pour le lancement

## Tâches techniques
- [ ] Implémenter login/logout et sessions
- [ ] hasher les mots de passe Argon2id
- [ ] gérer expiration/révocation
- [ ] rate limiter les endpoints sensibles.

## Critères d'acceptation
- [ ] Mots de passe jamais stockés en clair
- [ ] sessions expirables et révocables
- [ ] réponses d'erreur ne révélant pas l'existence d'un email.

## Definition of Done
- [ ] Code revu et fusionné
- [ ] Tests automatisés passants
- [ ] Permissions et isolation tenant vérifiées
- [ ] Documentation/OpenAPI mises à jour
