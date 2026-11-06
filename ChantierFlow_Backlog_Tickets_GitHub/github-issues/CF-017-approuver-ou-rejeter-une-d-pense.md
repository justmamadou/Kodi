---
title: "CF-017 — Approuver ou rejeter une dépense"
labels: "phase-1,p0,backend"
---

## Epic
CF-EPIC-05 — Dépenses et approbations

## User story
En tant qu'approbateur, je veux décider avec traçabilité.

## Priorité
**P0** — Bloquant pour le lancement

## Tâches techniques
- [ ] Implémenter approve/reject transactionnels
- [ ] interdire auto-approbation
- [ ] exiger motif de rejet
- [ ] créer inbox d'approbation.

## Critères d'acceptation
- [ ] Seule Pending est décidée
- [ ] créateur ne peut approuver
- [ ] rejet motivé
- [ ] acteur/date/décision historisés atomiquement.

## Definition of Done
- [ ] Code revu et fusionné
- [ ] Tests automatisés passants
- [ ] Permissions et isolation tenant vérifiées
- [ ] Documentation/OpenAPI mises à jour
