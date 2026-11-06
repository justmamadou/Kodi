---
title: "CF-018 — Enregistrer les paiements partiels"
labels: "phase-1,p1,backend"
---

## Epic
CF-EPIC-05 — Dépenses et approbations

## User story
En tant que comptable, je veux suivre les paiements d'une dépense approuvée.

## Priorité
**P1** — À livrer dans la Phase 1 après les fondations

## Tâches techniques
- [ ] Créer payments
- [ ] verrouiller dépense lors du cumul
- [ ] calculer solde
- [ ] construire interface paiements.

## Critères d'acceptation
- [ ] Paiements uniquement sur dépense approuvée
- [ ] cumul ne dépasse jamais montant
- [ ] solde restant exact
- [ ] paiements historisés.

## Definition of Done
- [ ] Code revu et fusionné
- [ ] Tests automatisés passants
- [ ] Permissions et isolation tenant vérifiées
- [ ] Documentation/OpenAPI mises à jour
