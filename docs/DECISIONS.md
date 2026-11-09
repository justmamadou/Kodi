# Décisions d'implémentation — Phase 1

## Règle de priorité

Le backlog (v1.0, 6 novembre) et le document d'architecture (v1.0, 5 novembre, « proposition ») divergent sur plusieurs points. Règle appliquée : **le backlog fait foi sur le périmètre et les critères d'acceptation ; l'architecture complète là où il ne dit rien.** Les maquettes tranchent ce qui touche à l'interface.

## Nom de l'application

L'application s'appelle **Kodi** (anciennement ChantierFlow). Le nom change partout où il se voit ou nomme une ressource : interface, manifeste PWA, emails, PDF, contrat OpenAPI, binaire (`cmd/kodi`), base et bucket de développement, métriques (`kodi_http_*`), CI. Restent volontairement à l'ancien nom :

- **les documents sources** (backlog, architecture, maquettes de `docs/ChantierFlow_HTML_Figma`), qui sont des entrées du projet et non du code ;
- **les préfixes techniques `cf_`** (rôles PostgreSQL `cf_app` et `cf_owner`, cookies `cf_session` et `cf_csrf`, caches du service worker) : invisibles, cités par les migrations déjà écrites, et les changer déconnecterait tout le monde ;
- **la base IndexedDB `chantierflow`** de la PWA : la renommer viderait la file d'envoi hors connexion et les brouillons déjà présents sur les appareils.



| Sujet | Architecture | Backlog | Implémenté |
|---|---|---|---|
| Migrations | goose | golang-migrate (CF-002) | golang-migrate, fichiers embarqués dans le binaire |
| Rôles | owner, finance, conducteur, chef_chantier, viewer | Owner, Manager, Accountant, Site Worker (CF-008) | `owner`, `manager`, `accountant`, `site_worker`, plus `viewer` (présent dans les maquettes). Libellés d'interface des maquettes : Owner / Admin, Conducteur, Finance, Chef de chantier, Lecture seule |
| Statuts de dépense | draft, submitted, approved… | Draft, Pending… (CF-016) | `draft`, `pending`, `approved`, `rejected`, `cancelled` |
| Statuts de budget | approved | Published (CF-013) | `draft`, `published`, `superseded`, `discarded` |
| Lignes de budget | un montant par catégorie | quantités, unités, montants calculés (CF-012) | `quantity × unit_price`, montant en colonne générée par la base |
| Révisions | motif dans `budget_versions` | `budget_revisions` (CF-014) | table dédiée, append-only |
| Événements de dépense | `expense_status_history` | `expense_approvals` (CF-016) | `expense_approvals`, append-only |
| Journal d'audit | `audit_events` | `audit_logs` (CF-005) | `audit_logs`, partitionné comme prévu par l'architecture |
| Mouvements de stock | journal à une ligne, toujours en vigueur | en-tête + lignes, brouillon puis posté immuable (CF-022) | `stock_movements` + `stock_movement_items`, statut `draft` / `posted` |
| Stock négatif | non interdit | une sortie ne rend pas le stock négatif (CF-022) | `CHECK (on_hand >= 0)` sur la projection |
| Tâches | un seul `assignee_id` | `task_assignments`, priorité, historique (CF-024, CF-025) | plusieurs responsables, `priority`, `task_status_history` |
| Paiements | possibles avant approbation | uniquement sur dépense approuvée (CF-018) | refus côté service et contrainte en base |
| Seuil d'auto-approbation | oui | toute soumission devient Pending (CF-016) | pas d'auto-approbation |
| Structure du code | `handler`, `service`, `store` | domain / application / infrastructure / transport (CF-001) | modules de l'architecture, avec une couche par fichier : `domain.go` (domaine), `service.go` (application), requêtes sqlc (infrastructure), `handler.go` (transport) |

## Permissions tranchées par le Product Owner (08/11/2026)

Le code est conservé ; les deux premiers points vont contre la matrice des maquettes :

1. **Un manager peut créer et publier un budget** (CF-012, CF-013 : « manager autorisé »). Les maquettes réservaient « Réviser un budget » à l'Owner.
2. **Un comptable peut saisir une dépense** (CF-015). Les maquettes lui répondaient « Non ».
3. **Déléguer une approbation est réservé à l'Owner**, même si le manager peut gérer l'équipe du chantier.

## Choix techniques notables

- **Indicateurs financiers calculés à la lecture** plutôt que dans la table d'agrégats de l'ADR-09. CF-019 demande des « agrégations indexées », et un calcul depuis les sources ne peut pas dériver : ni delta, ni réconciliation nocturne à maintenir. Mesuré : P95 de 125 ms sur le tableau de bord avec 50 000 dépenses, 60 chantiers et 20 utilisateurs simultanés (exigence : 700 ms). À revoir si `ListProjectFinancials` dépasse 200 ms en production.
- **Reste budgétaire = budget publié − dépenses approuvées** (CF-020), et non le « disponible prévisionnel » de l'architecture, qui inclut les engagements, hors backlog.
- **Pas de River ni de worker** : aucun ticket Phase 1 n'en a besoin. Les exports sont synchrones (CF-028 : « jobs asynchrones si besoin »), avec un refus explicite au-delà de 50 000 lignes plutôt qu'un fichier tronqué. La purge tourne dans l'API. Les emails d'invitation partent après le commit ; en cas d'échec, le lien reste affiché à l'administrateur.
- **Service worker écrit à la main** plutôt que Serwist, et **pas de TanStack Query** : le besoin est couvert par un petit `useResource`, et Serwist est lié au bundler. Les données hors connexion vivent dans IndexedDB (Dexie), jamais dans le cache HTTP du service worker.
- **Contrat OpenAPI maintenu à la main** plutôt que spec-first avec génération (ADR-13) : les réponses y sont décrites sommairement, et les types TypeScript sont écrits à la main. `TestOpenAPIMatchesRoutes` garantit qu'aucune route n'est oubliée, mais ne valide pas les corps de réponse. À renforcer si d'autres clients consomment l'API.
- **Conflits hors connexion gérés sur l'appareil**, sans table `sync_conflicts` : le serveur répond 409, l'application présente le choix à l'utilisateur.
- **RustFS en local et en CI** : les images MinIO ne sont plus téléchargeables sans authentification. Le code ne dépend que de l'API S3 (client `minio-go`), donc n'importe quel fournisseur S3 convient.
- **Limitation de débit en mémoire**, par instance : suffisant pour une seule instance. À partager (Redis) lors du passage à plusieurs instances.
- **Partitions d'audit créées jusqu'à fin 2028** par la migration ; ensuite, la partition par défaut prend le relais (voir le runbook).
- **Mot de passe oublié traité en arrière-plan** : `POST /auth/password-reset` répond 202 avec le même message dès la validation du champ, puis cherche le compte, crée le jeton et envoie l'email hors requête. La réponse ne dit donc rien, ni par son contenu ni par son délai, de l'existence du compte, de son email ou du plafond atteint. Jeton de 256 bits haché en SHA-256, valable 30 minutes, à usage unique ; une nouvelle demande remplace le lien précédent. Plafond de 3 demandes par compte et par heure (sérialisées par un verrou sur l'utilisateur), en plus de la limite par adresse IP des routes publiques. Le changement révoque toutes les sessions du compte, lève son verrouillage et figure au journal d'audit (`password_reset.requested`, `password.reset`). Le lien porte le jeton dans le fragment (`/nouveau-mot-de-passe#…`), que le navigateur n'envoie jamais au serveur : il n'apparaît dans aucun journal d'accès. Un email en cours d'envoi peut être perdu à l'arrêt de l'API ; l'utilisateur refait alors la demande.
- **Partitions d'audit inaccessibles à `cf_app`** : la RLS ne s'applique qu'à travers la table parente. Une nouvelle partition doit révoquer ces droits (voir le runbook).

## Limites connues

- **Inscription** : l'erreur de doublon révèle qu'un identifiant existe déjà. Seule une vérification préalable de l'email l'éviterait. La connexion, elle, ne révèle rien (CF-004).
- **Invitations par SMS** : le prestataire n'est pas choisi (décision ouverte n° 4 de l'architecture). Le lien est affiché à l'administrateur, qui peut le transmettre par SMS ou WhatsApp.
- **Mot de passe oublié sans email** : faute de prestataire SMS (même décision ouverte), un compte qui n'a qu'un téléphone ne reçoit rien. La demande reçoit la réponse habituelle et laisse une trace au journal d'audit (`password_reset.requested`, canal `none`) ; l'écran invite à contacter l'administrateur de l'entreprise. **Celui-ci n'a pour l'instant aucun moyen d'aider** : il faudrait un lien de réinitialisation généré depuis l'écran Équipe et affiché à l'administrateur, comme pour les invitations, ou le choix d'un prestataire SMS.
- **Fichiers** : type et taille vérifiés sur les octets reçus, mais pas d'analyse antivirus ni de miniatures.
- **Un fichier déposé mais jamais confirmé** reste orphelin dans le bucket. Prévoir une règle de cycle de vie côté stockage.
- **Poids de la PWA** : 206 Ko de JavaScript initial en gzip sur `/connexion`, pour une cible de 200 Ko dans l'architecture. Piste : charger Dexie à la demande sur les pages sans saisie.
- **Comptes de démonstration** : `.env.example` et `deploy/initdb` contiennent des secrets de développement, à ne jamais réutiliser en production.

## Écrans alignés sur les maquettes

Les 15 maquettes de `docs/ChantierFlow_HTML_Figma` décident de l'interface. Quand une maquette montre une fonction qui n'existe pas côté serveur, l'écran n'invente rien : il affiche le badge « Bientôt » de la maquette, désactivé.

**Affiché « Bientôt »** : transferts de stock (tuile, choix du type), abonnement, mot de passe modifiable, vérification en deux étapes, export ZIP, clôture de compte, conservation des justificatifs, préférences de notification, logo, seuil de validation, écart toléré, rappel de rapport.

**Écarts assumés avec les maquettes**

- **Tableau des permissions (Équipe)** : il montre les règles appliquées par `internal/rbac`, pas celles de la maquette. Le Conducteur peut réviser un budget et Finance peut créer une dépense, comme décidé le 08/11/2026 ; la maquette disait l'inverse. Si une règle change un jour, il faut modifier `internal/rbac` et ce tableau ensemble.
- **Colonne « Transferts » de Matériaux** remplacée par « Ajustements », qui existent réellement.
- **Export** : XLSX seulement (la maquette annonce CSV / XLSX), sauf le stock, exportable en CSV.
- **Champ « Référence »** ajouté à Nouveau chantier : CF-009 l'exige, la maquette l'omet.
- **« Commandé », « Engagé », « Engagements ouverts »** : il n'y a pas d'engagements au backlog. Ces libellés montrent les dépenses approuvées non réglées. Payé + engagé + disponible = budget.
- **Période du tableau de bord** : Semaine / Mois / Tout ne change que le montant payé. Le budget et le disponible n'ont de sens qu'en cumul.
- **Filtre de période de Rapports** retiré : `/reports/overview` calcule en cumul.
- **Photo de chantier** : emplacement seul, comme dans la maquette, sans import.

**Ajouts sans maquette**, pour qu'aucune fonction du backlog ne disparaisse : `/mot-de-passe-oublie` et `/nouveau-mot-de-passe` (la maquette de connexion n'en montre que le lien ; ils reprennent sa mise en page), `/fournisseurs`, `/alertes` (calculée à la lecture, sans notification poussée), `/profil` (reste du menu sur téléphone), `/chantiers/[id]/modifier` (informations et cycle de vie). Les phases sont dans l'onglet Tâches.

**Accueil selon le rôle** : Owner et Finance ont la maquette 03 ; Chef de chantier et Lecture seule la maquette 08 ; le Conducteur la 03 au bureau et la 08 sur téléphone.

**Nouveau chantier** enchaîne plusieurs appels (chantier, budget, lignes, publication, membres), sans transaction commune. En cas d'échec après la création, l'écran le dit et renvoie vers la page du chantier plutôt que de risquer un doublon.

## E1 — Demandes de prix (RFQ)

Une demande de prix appartient à un chantier (et donc à l'entreprise), porte des lignes de matériaux ou de prestations (quantité, unité, spécifications, date de besoin) et une date limite de réponse. Statuts : `draft` → `published` → `closed`, et `cancelled` depuis `draft` ou `published`, avec un motif. Numérotation `DP-0001` par entreprise, sans trou ni doublon même en concurrence.

- **Le fournisseur répond par un lien personnel, sans compte** : un jeton de 256 bits par fournisseur consulté, émis à la publication et stocké haché, comme les invitations. Il passe dans le fragment de l'URL (`/offre#…`) et dans le corps des requêtes, jamais dans un chemin journalisé. Le fournisseur ne voit que la demande, le lieu de livraison et ses propres offres : ni les autres fournisseurs, ni le client du chantier, ni le catalogue interne.
- **Envoi** : par email si le fournisseur en a un (champ ajouté au répertoire des fournisseurs). Sinon, faute de prestataire SMS, l'écran affiche le lien avec un bouton « Envoyer par WhatsApp » (lien `wa.me` ouvert par l'acheteur, aucune API). Les liens ne sont lisibles qu'au moment où ils sont émis ; un lien perdu se remplace (« Nouveau lien »), ce qui invalide l'ancien.
- **Contenu figé dès la publication**, par le service et par trigger : tous les fournisseurs chiffrent la même demande. On peut encore consulter un fournisseur de plus tant que la demande est ouverte.
- **Offres historisées** : chaque envoi crée une version (`rfq_offers`, `rfq_offer_lines`), jamais modifiée ni supprimée (trigger append-only, aucun droit `UPDATE` pour `cf_app`). Prix unitaires en FCFA entiers ; montant de ligne arrondi au franc, calculé par la base. Une ligne non chiffrée est simplement absente. Un double envoi (même identifiant généré par la page) n'a qu'un effet.
- **Date limite incluse**, à l'heure de Dakar : passée, la demande reste publiée mais n'accepte plus d'offre. La clôture reste un geste de l'acheteur.
- **Audit** : `rfq.created`, `rfq.updated`, `rfq.published`, `rfq.supplier_invited`, `rfq.link_renewed`, `rfq.offer_received` (sans acteur interne : c'est le fournisseur), `rfq.closed`, `rfq.cancelled`.
- **Droits, validés par le Product Owner (09/11/2026)** : créer, publier, clôturer et annuler pour l'Owner et le Conducteur (sur ses chantiers) ; consulter pour Finance et Lecture seule ; rien pour le Chef de chantier, pour ne pas exposer les prix des fournisseurs. Chaque choix se change en une ligne de `internal/rbac/rbac.go`.
- **Pas encore** : relances automatiques, pièces jointes aux offres. La sélection d'une offre et le bon de commande relèvent d'E3.

## E2 — Gestion des fournisseurs

Le répertoire des fournisseurs devient une fiche complète, propre à chaque entreprise : identité légale (raison sociale, forme juridique, NINEA, RCCM), coordonnées, adresse, activités, notes internes, plusieurs contacts dont un principal, conditions commerciales, et l'historique de ses consultations et achats.

- **Archiver plutôt que supprimer** : un fournisseur archivé quitte le répertoire et les nouvelles demandes de prix, mais reste dans l'historique, les dépenses et les demandes en cours. Il se restaure. Une saisie terrain par le nom (dépense hors ligne) réutilise la fiche existante, même archivée, sans la réactiver ni créer de doublon. Une fiche archivée n'est plus modifiable.
- **Conditions commerciales historisées** : chaque changement crée une version (`supplier_terms`), jamais modifiée ni supprimée ; la fiche affiche la dernière. Moyen et délai de paiement, acompte, remise, délai de livraison, livraison comprise ou non, minimum de commande, encours autorisé, date d'effet.
- **Historique** : demandes de prix où le fournisseur a été consulté (avec sa dernière offre) et achats enregistrés (dépenses), limités aux chantiers visibles par l'utilisateur. Les bons de commande (E3) s'y ajoutent.
- **Règles de doublon**, par entreprise seulement :
  - *bloquantes* (409 `supplier.duplicate`, même avec confirmation) : même nom une fois ignorés la casse, les accents, les espaces et la ponctuation (« Quincaillerie N'Diaye » = « quincaillerie ndiaye ») ; même NINEA ; même RCCM. Garanties aussi par des index uniques, donc même en concurrence ;
  - *probables* (409 `supplier.possible_duplicate`, levé par `confirm_not_duplicate`) : même téléphone ou même email, sur la fiche ou l'un de ses contacts ; nom très proche une fois retirées la forme juridique et les mentions « Ets », « Société »… (similarité trigramme ≥ 0,75). En modification, un doublon probable déjà accepté n'est redemandé que si le nom, le téléphone ou l'email changent.
  L'écran signale les doublons pendant la saisie, avec un lien vers la fiche existante.
- **Normalisation** : téléphones au format international (`77 123 45 67` → `+221771234567`), NINEA sans espaces en majuscules, RCCM en majuscules, emails en minuscules.
- **Accès d'un fournisseur aux données d'une autre entreprise** : impossible, faute d'autorisation explicite prévue. Chaque entreprise a ses propres fiches (deux entreprises peuvent enregistrer le même fournisseur, même NINEA compris, sans rien voir l'une de l'autre, contrôle des doublons compris), isolées par la RLS. Un fournisseur n'a pas de compte : il ne voit, par le lien d'une demande de prix, que cette demande et ses propres offres. Un annuaire partagé entre entreprises demanderait un consentement explicite, non développé.
- **Droits, validés par le Product Owner (09/11/2026)** : fiche et contacts visibles de tous (le chef de chantier appelle les fournisseurs) ; création rapide pour tous sauf la lecture seule, comme avant ; modification, archivage, contacts et conditions pour l'Owner, le Conducteur et Finance ; conditions commerciales et historique (prix) cachés au Chef de chantier, comme les demandes de prix.
- **Audit** : `supplier.created`, `supplier.updated` (avant / après), `supplier.archived`, `supplier.restored`, `supplier.contact_added`, `supplier.contact_updated`, `supplier.contact_removed`, `supplier.terms_recorded`.
- **Code** : les routes fournisseurs quittent le module `organization` pour un module `supplier`.

## E3 — Bons de commande

De l'offre retenue à la marchandise reçue sur le chantier : comparaison sur critères, sélection, bon de commande, approbation, envoi, réceptions et entrée en stock.

- **Critères de comparaison** : chaque demande de prix pondère trois critères, notés sur 100 pour la dernière offre de chaque fournisseur — *prix* (sur les lignes qu'elle chiffre, somme des meilleurs montants / somme des siens), *délai* ((plus court délai annoncé + 1) / (son délai + 1), 0 s'il n'est pas communiqué), *couverture* (part des lignes chiffrées). Poids par défaut 60 / 25 / 15, modifiables jusqu'à la sélection (total 100). Le classement est indicatif : la décision reste humaine, et une offre dont la validité est dépassée est signalée sans être écartée.
- **Sélection** : seulement une fois la consultation close (plus personne ne peut surenchérir), seulement la dernière version d'un fournisseur, avec un motif facultatif (conseillé si l'offre n'est pas la mieux notée). Elle se retire tant qu'aucune commande active n'en découle. Une seule offre par demande : pas de répartition des lignes entre fournisseurs.
- **Bon de commande** : généré en brouillon depuis l'offre retenue (articles, quantités demandées, prix offerts, conditions de paiement de l'offre, livraison sur le chantier, date souhaitée de la demande ou à défaut délai de l'offre). Numéro `BC-0001` par entreprise, sans trou ni doublon. Une offre ne donne qu'une commande active ; après annulation, on peut en générer une autre.
- **Taxes** : taux par ligne (0 par défaut, car beaucoup de fournisseurs ne facturent pas la TVA ; un bouton applique 18 %). Montants HT et taxes arrondis au franc par la base, totaux recalculés à chaque enregistrement, jamais saisis.
- **Workflow** : `draft` → `submitted` → `approved` → `sent` → `partially_received` → `received`, `submitted` → `draft` (renvoi motivé), annulation motivée tant que rien n'est reçu. Transitions et contenu figé hors brouillon garantis aussi par trigger. **Pas d'auto-approbation** : l'auteur d'une commande ne peut pas l'approuver (service et contrainte en base), comme pour les dépenses.
- **Historique** : chaque création, modification (contenu avant / après), soumission, renvoi, approbation, envoi, réception et annulation est un événement immuable de la commande, doublé d'une entrée au journal d'audit.
- **Envoi** : par email si le fournisseur en a un (texte avec lignes, totaux et conditions) ; sinon l'écran invite à le transmettre autrement. Pas encore de PDF.
- **Réceptions** : partielles ou complètes, sur commande envoyée. Quantité reçue = somme des réceptions par construction (trigger), jamais au-delà du commandé (CHECK), restant calculé par la base. Une réception renvoyée avec le même identifiant n'a qu'un effet ; un même bon de livraison ne s'enregistre qu'une fois par commande. Option « entrer en stock » : les lignes rattachées au catalogue (même unité que l'article) créent un mouvement d'entrée posté sur le chantier, comme le module de stock.
- **Droits, validés par le Product Owner (09/11/2026)** : retenir une offre et préparer, soumettre, envoyer ou annuler une commande pour l'Owner et le Conducteur ; approuver ou renvoyer pour l'Owner et Finance ; réceptionner pour l'Owner, le Conducteur et le Chef de chantier, qui voit les commandes de ses chantiers sans aucun prix.
- **Pas encore** : facture fournisseur et rapprochement commande / réception / facture, transformation en dépense, avoirs et retours, commande sans demande de prix, plusieurs fournisseurs par demande.

## Hors périmètre Phase 1

Décrits dans l'architecture ou les maquettes, mais sans ticket au backlog, donc non développés : engagements (`commitments`), transferts de stock entre chantiers, notifications et Web Push, facturation et abonnements, MFA, export complet ZIP et clôture de compte, rappels de rapport.

Ajoutés pour les maquettes : avancement déclaré, signal d'écart budget / avancement et avertissement de doublon (la décision reste humaine).

Ajouté après coup, hors backlog : **mot de passe oublié**, présent dans la maquette de connexion et indispensable à un lancement réel (voir « Choix techniques notables »).
