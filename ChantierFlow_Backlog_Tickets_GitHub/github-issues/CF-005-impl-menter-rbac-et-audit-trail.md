---
title: "CF-005 — Implémenter RBAC et audit trail"
labels: "phase-1,p0,backend"
---

## Epic
CF-EPIC-01 — Fondations, sécurité et multi-tenancy

## User story
En tant qu'administrateur, je veux contrôler les permissions et tracer les actions.

## Priorité
**P0** — Bloquant pour le lancement

## Tâches techniques
- [ ] Définir permissions
- [ ] ajouter middleware d'autorisation
- [ ] créer audit_logs append-only
- [ ] auditer validations financières et changements de rôles.

## Critères d'acceptation
- [ ] Les permissions sont vérifiées côté serveur
- [ ] les refus retournent 403
- [ ] les opérations sensibles sont historisées.

## Definition of Done
- [ ] Code revu et fusionné
- [ ] Tests automatisés passants
- [ ] Permissions et isolation tenant vérifiées
- [ ] Documentation/OpenAPI mises à jour
