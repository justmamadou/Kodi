---
title: "CF-016 — Soumettre une dépense"
labels: "phase-1,p0,backend"
---

## Epic
CF-EPIC-05 — Dépenses et approbations

## User story
En tant que créateur, je veux soumettre une dépense à validation.

## Priorité
**P0** — Bloquant pour le lancement

## Tâches techniques
- [ ] Créer événement expense_approvals
- [ ] implémenter transition transactionnelle
- [ ] ajouter idempotence
- [ ] créer action UI Soumettre.

## Critères d'acceptation
- [ ] Seule une Draft est soumise
- [ ] statut devient Pending
- [ ] événement écrit dans la même transaction
- [ ] double soumission sans doublon.

## Definition of Done
- [ ] Code revu et fusionné
- [ ] Tests automatisés passants
- [ ] Permissions et isolation tenant vérifiées
- [ ] Documentation/OpenAPI mises à jour
