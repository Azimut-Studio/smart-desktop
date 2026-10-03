# Smart Desktop

Application Go native en français pour **Windows 10 (1703 ou plus récent)/11 x64**, permettant de sauvegarder et restaurer les positions des icônes du bureau. Sans WebView, sans élévation et sans télémétrie. La version minimale de Windows correspond à la prise en charge DPI Per-Monitor V2.

## Compiler

Installer **Go 1.25 ou plus récent**, puis, depuis la racine du dépôt :

```powershell
go mod download
powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\build.ps1
```

Le script génère une icône originale, les ressources Windows et le manifeste DPI, puis produit `dist\SmartDesktop.exe`. Le compilateur C n'est pas nécessaire. Les dépendances de compilation sont déclarées et versionnées dans le module Go.

Dans VS Code, la tâche **Build Smart Desktop** exécute ce même script. Préférer ce script à `go build` seul : les ressources et le manifeste font partie du comportement DPI attendu.

## Utilisation

1. Lancer `SmartDesktop.exe`.
2. Cliquer sur **Sauvegarder**.
3. Retrouver une disposition dans la liste ou le menu **Sauvegardes**, avec date locale, résolutions, DPI, origine et nombre d'icônes.
4. Sélectionner une disposition, puis **Restaurer**. Le menu propose les 50 dispositions les plus récentes ; la liste contient l'historique complet.

Une restauration crée d'abord une **sauvegarde de sécurité** de l'état actuel. Les icônes sont reconnues par leur identité Shell, pas seulement leur libellé ni leur index. Les icônes disparues ou renommées ne sont pas recréées ; les nouvelles ne sont pas déplacées. Un bilan vérifie les positions obtenues et signale les résultats partiels.

La configuration actuelle doit correspondre à la sauvegarde : identités des écrans, géométrie du bureau, écran principal, orientation, DPI et résolutions des cibles physiques, y compris en duplication. L'application **ne change jamais la résolution**. En cas de refus, rétablir la configuration d'affichage initiale puis réessayer.

Pour une restitution exacte, désactiver **Réorganiser automatiquement les icônes** et **Aligner les icônes sur la grille** dans le menu **Affichage** du bureau Windows. Si l'une de ces options est active, l'application refuse de déplacer les icônes et explique pourquoi, sans modifier ces réglages.

Le bureau est lu via les interfaces COM documentées du Shell Windows (`IFolderView` / `IFolderView2`). `desktop.ini` sert à personnaliser les dossiers et n'est pas utilisé pour les positions.

## Paramètres et arrière-plan

Modifier les cases et valeurs, puis cliquer sur **Appliquer les paramètres**.

| Paramètre | Valeur initiale |
| --- | --- |
| Icône dans la barre système (tray) | Désactivée |
| Démarrage à l'ouverture de session Windows | Désactivé |
| Continuer en arrière-plan après fermeture de la fenêtre | Désactivé |
| Sauvegarde automatique de l'ancien état d'affichage | Désactivée |
| Intervalle de capture | 5 secondes, réglable de 1 à 3600 |
| Nombre de sauvegardes automatiques conservées | 100, réglable de 1 à 10000 |

Par défaut, fermer la fenêtre quitte l'application. **Quitter** arrête toujours l'agent, indépendamment du réglage d'arrière-plan.

Le tray permet d'ouvrir la fenêtre, sauvegarder, restaurer une disposition et quitter. L'arrière-plan fonctionne aussi **sans tray** : relancer l'exécutable ouvre la fenêtre du processus déjà actif. Une seule instance s'exécute par session et utilisateur.

Le démarrage automatique utilise la valeur `SmartDesktop` de :

```text
HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run
```

Elle contient le chemin absolu de l'exécutable suivi de `--background`. Placer l'exécutable dans son emplacement définitif avant d'activer cette option ; après un déplacement, appliquer à nouveau les paramètres depuis le nouvel emplacement.

```powershell
.\dist\SmartDesktop.exe --background
```

Il s'agit d'un **agent de session utilisateur**, pas d'un service Windows SCM. Il n'est pas actif avant l'ouverture de session et ne manipule pas le bureau d'autres utilisateurs.

## Limite importante de l'automatisation

Windows envoie notamment `WM_DISPLAYCHANGE` **après** un changement d'affichage. Il n'existe pas ici de notification universelle garantissant une capture juste avant chaque changement.

Quand l'option automatique est activée, Smart Desktop :

- renouvelle périodiquement un checkpoint de la disposition observée ;
- contrôle la configuration avant et après la capture ;
- archive le dernier checkpoint de l'ancienne configuration lorsqu'un changement est détecté ;
- attend au moins deux secondes de stabilité observée avant de remplacer le checkpoint par celui de la nouvelle configuration ;
- regroupe les notifications d'une transition et ne restaure jamais automatiquement.

L'archive conserve **la date de capture d'origine** et indique séparément **la date de détection**. Elle peut être plus ancienne que le changement et ne reflète pas nécessairement le dernier déplacement d'une icône. Explorer peut aussi réorganiser les icônes avant la notification.

La première archive nécessite un checkpoint préalable. Si le bureau est indisponible ou la session verrouillée, l'observation échoue explicitement et ne remplace pas le checkpoint par une capture vide. Avec un intervalle élevé, la reprise et l'observation d'une stabilisation peuvent être retardées jusqu'à la prochaine échéance.

## Données locales

```text
%LOCALAPPDATA%\SmartDesktop\
  settings.json
  checkpoint.json
  status.json
  backups\<identifiant>.json
  logs\agent.log
  logs\agent.log.1
```

Les JSON sont versionnés, validés et remplacés atomiquement. Les dates sont stockées en UTC et affichées en heure locale. Les sauvegardes contiennent les identités des icônes, donc potentiellement des chemins locaux : les traiter comme des données personnelles.

Toutes les sauvegardes **manuelles et de sécurité** sont conservées jusqu'à suppression explicite avec confirmation. La limite s'applique uniquement aux sauvegardes automatiques et après une nouvelle archive réussie. Aucun effacement automatique de l'historique n'est réalisé lors d'une simple capture périodique.

Les diagnostics sont accessibles dans la fenêtre et les journaux locaux ; si le tray est activé, une notification signale les erreurs de l'agent. Les journaux tournent à environ 1 Mio avec une archive. Sans tray ni fenêtre, relancer l'application pour lire le dernier diagnostic.

Un fichier invalide est signalé, pas remplacé silencieusement. En cas de paramètres corrompus, fermer l'application et corriger ou déplacer `settings.json` avant de relancer. Le déplacement permet de conserver le fichier pour diagnostic.

Pour arrêter et désactiver l'agent : relancer l'application, décocher le démarrage Windows, appliquer, puis cliquer **Quitter**. Les données ne sont pas supprimées.

## Tests

```powershell
go vet .\...
go test .\internal\... -count=1
```

Tests d'intégration sur un bureau Windows interactif, **sans déplacement d'icône** et avec une fenêtre native de test masquée :

```powershell
$env:SMART_DESKTOP_INTEGRATION = '1'
go test .\internal\win32 .\internal\ui -run 'TestDesktopReadOnlyIntegration|TestNativeWindowSmoke' -v -count=1 -timeout=30s
Remove-Item Env:\SMART_DESKTOP_INTEGRATION
```

Un test distinct déplace temporairement **une seule icône**, vérifie le déplacement puis la remise en place, et conserve une sauvegarde de sécurité locale. Ne l'exécuter qu'avec l'accord de l'utilisateur du bureau. Il est ignoré si l'arrangement automatique ou l'alignement sur grille est actif ; il ne change pas ces réglages :

```powershell
$env:SMART_DESKTOP_ALLOW_MOVE = '1'
go test .\internal\win32 -run TestDesktopRestoreConsentedIntegration -v -count=1 -timeout=30s
Remove-Item Env:\SMART_DESKTOP_ALLOW_MOVE
```

Les tests unitaires couvrent les schemas, identités, tris, paramètres, écritures, fichiers corrompus, rétention, compatibilité multi-écrans, sauvegardes de sécurité, restaurations partielles et transitions simulées d'affichage.

À vérifier aussi sur les machines cibles : Windows 10 et 11, plusieurs écrans et DPI mixtes, écran à origine négative, duplication, déconnexion/reconnexion, reprise de session, redémarrage d'Explorer, tray et démarrage réel. Les tests automatisés ne changent ni la résolution ni les inscriptions au démarrage.

### Validation de cette première version

La compilation Windows x64, `go vet`, les tests unitaires, la lecture réelle du Shell, la création de la fenêtre native et un essai du binaire compilé ont été réalisés. L'essai du binaire a vérifié les ressources DPI, l'agent masqué et réactif, l'ouverture de la fenêtre par une seconde instance et l'arrêt propre.

L'essai de déplacement/restauration réel a été ignoré parce que les options d'arrangement ou d'alignement du bureau étaient actives. Aucun réglage du bureau n'a été changé. Les restaurations complètes et partielles sont vérifiées par doubles dans les tests unitaires, mais restent à confirmer sur un bureau réel configuré pour le positionnement libre.

Le détecteur `go test -race` a aussi été tenté : ThreadSanitizer a échoué à allouer sa mémoire avant l'exécution des tests dans l'environnement de validation. L'analyse de races n'est donc pas validée. Les transitions réelles multi-écrans et les inscriptions au démarrage restent des vérifications manuelles.

## Périmètre

Pas de service système, installateur, signature numérique, cloud, import/export DesktopOK, changement de résolution ou restauration automatique. Windows ARM64 et 32 bits ne sont pas ciblés.

Références :

- [IFolderView](https://learn.microsoft.com/en-us/windows/win32/api/shobjidl_core/nn-shobjidl_core-ifolderview)
- [Positions des icônes via le Shell](https://devblogs.microsoft.com/oldnewthing/20130318-00/?p=4933)
- [WM_DISPLAYCHANGE](https://learn.microsoft.com/en-us/windows/win32/gdi/wm-displaychange)
- [QueryDisplayConfig](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-querydisplayconfig)
