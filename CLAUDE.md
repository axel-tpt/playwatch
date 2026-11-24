# CLAUDE.md

> Nom du projet : **playwatch**.

## Vision

TUI pour Ansible **dans le terminal, inspirée de k9s** : navigation centrée sur les *ressources* (hôtes, groupes, playbooks, rôles, runs…), barre de commande `:`, filtre `/`, drill-down avec `Enter` / retour avec `Esc`, actions contextuelles au clavier.

Fonctionne sur un projet Ansible local (pas de serveur, pas d'AWX). Ansible doit être installé et dans le `$PATH`.

### Ce qui nous différencie de l'existant

Projets comparables : `kocierik/lazyansible`, `daviddwlee84/lazyansible` (Go, style lazydocker), `ansible-navigator` (officiel, curses), `ansible-community/awx-tui` (AWX uniquement).

- **Modèle k9s** (vues par ressource + pile de navigation), pas des panneaux fixes façon lazydocker.
- **Ansible est la source de vérité** : on ne parse jamais nous-mêmes les inventaires ou la config. Tout passe par les commandes Ansible en JSON → inventaires dynamiques, plugins et variables résolues gérés gratuitement.
- **Événements structurés** via un callback plugin embarqué : jamais de parsing de la sortie texte d'`ansible-playbook`.
- **Vue `describe` d'un hôte** : variables effectives, facts, derniers runs.

### Non-objectifs (pour l'instant)

- Pas d'UI web, pas de mode serveur, pas de multi-utilisateur.
- Pas d'éditeur YAML intégré (on délègue à `$EDITOR`).
- Pas de réimplémentation d'une logique Ansible (parsing d'inventaire, précédence des variables, etc.).

## Stack

- **Go** (version récente stable), binaire unique.
- **Bubble Tea v2**, **Bubbles v2**, **Lip Gloss v2** (Charm). ⚠️ La v2 a changé les API et les chemins d'import : toujours vérifier la doc officielle v2, ne pas recopier d'exemples v1.
- `cobra` pour les sous-commandes CLI (mode non interactif).
- Aucune dépendance Python côté utilisateur en dehors d'Ansible lui-même (pas d'`ansible-runner`).

## Architecture

```
cmd/playwatch/               main, flags, sous-commandes cobra
internal/ansible/      intégration Ansible, SANS aucune dépendance UI
  exec.go              exécution des commandes ansible-* (context, timeout, env)
  inventory.go         ansible-inventory --list / --host
  runner.go            lancement ansible-playbook / ansible ad-hoc + flux d'événements
  callback/            plugin Python embarqué (go:embed) + écriture en dossier temp
  events.go            types Go des événements (contrat JSON)
  playbook.go          ansible-playbook --list-hosts --list-tasks (aperçu affiché tel quel)
  doc.go               ansible-doc -l -j / -j <module>
  facts.go             fact cache jsonfile ou `-m setup` à la demande
internal/resource/     registre de ressources (une par fichier : hosts, groups, playbooks, roles, runs, modules)
internal/ui/           app racine, pile de vues, cmdbar, filtre, table générique, styles
internal/store/        persistance des runs sous $XDG_STATE_HOME/playwatch/runs (0700/0600) :
                       <id>.json (méta, écrite atomiquement), <id>.jsonl (événements), <id>.log (sortie brute)
```

**Règle stricte** : `internal/ansible` et `internal/store` n'importent jamais `internal/ui` ni Bubble Tea. Ils doivent être testables sans terminal et réutilisables par les sous-commandes CLI.

### Sources de données (toujours en JSON)

| Besoin | Commande |
|---|---|
| Inventaire complet | `ansible-inventory -i <inv> --list` |
| Variables d'un hôte | `ansible-inventory -i <inv> --host <host>` |
| Liste des modules | `ansible-doc -l -j` |
| Doc d'un module | `ansible-doc -j <module>` |
| Facts | fact cache jsonfile si configuré, sinon `ansible <host> -m setup` à la demande |
| Config effective | `ansible-config dump --format json` (si dispo) |

### Callback plugin embarqué (cœur du projet)

- Plugin Python minimal dans `internal/ansible/callback/`, embarqué avec `go:embed`, écrit au lancement dans un dossier temporaire.
- Activé via l'environnement du sous-processus :
  - `ANSIBLE_CALLBACK_PLUGINS=<tmpdir>`
  - `ANSIBLE_CALLBACKS_ENABLED=playwatch_events`
  - `PLAYWATCH_EVENTS_FD=3` (le pipe est passé via `cmd.ExtraFiles`)
- Le plugin écrit **une ligne JSON par événement** sur ce fd. stdout/stderr restent disponibles pour l'affichage brut.
- Hooks implémentés : `v2_playbook_on_start`, `v2_playbook_on_play_start`, `v2_playbook_on_task_start`, `v2_playbook_on_handler_task_start` (émis comme `playbook_on_task_start` avec `is_handler`), `v2_runner_on_start` (statut « en cours »), `v2_runner_on_ok`, `v2_runner_on_failed`, `v2_runner_on_skipped`, `v2_runner_on_unreachable`, `v2_runner_item_on_ok|failed|skipped` (boucles), `v2_playbook_on_stats`.
- Contrat d'un événement (à garder stable et versionné) :

```json
{"v":1,"type":"runner_on_ok","ts":"…","play":"…","task":"…","task_uuid":"…","host":"…","changed":false,"result":{}}
```

- Champs optionnels selon le type : `playbook` (playbook_on_start), `play_uuid`, `task_action`, `is_handler`, `ignore_errors` (runner_on_failed), `stats` (playbook_on_stats : `{host: {ok, changed, failures, unreachable, skipped, rescued, ignored}}`). Les clés internes `_ansible_*` sont retirées de `result`.
- Le plugin rend le fd non héritable (`os.set_inheritable`) : un processus lancé par Ansible ne doit pas garder le pipe ouvert.
- `ansible-playbook` tourne dans son propre groupe de processus : l'annulation envoie SIGINT au groupe (arrêt propre d'Ansible), puis SIGKILL après un délai.
- Limitation connue : `ANSIBLE_CALLBACK_PLUGINS` / `ANSIBLE_CALLBACKS_ENABLED` écrasent les valeurs d'`ansible.cfg` (à fusionner via `ansible-config dump`).

- Les types Go correspondants vivent dans `events.go`. Toute évolution du contrat = bump de `v` + mise à jour des deux côtés + tests.
- Ne jamais logger de secrets : respecter `no_log` (le plugin doit masquer `result` quand `_ansible_no_log` est vrai).

### Modèle UI (façon k9s)

- **Modèle racine** = pile de `View` + barre de commande + barre de statut.
- Interface `View` (`internal/ui/view.go`) : `Init`, `Update(msg) tea.Cmd` (vues mutables, pointeurs), `View() string`, `SetSize`, plus `Title()` et `KeyMap()` pour l'aide contextuelle. Interfaces optionnelles : `Filterable` (filtre `/`), `Interrupter` (ctrl+c géré par la vue, ex. un run).
- Les messages asynchrones implémentent `Targeted` : le modèle racine les livre à leur vue même si elle n'est pas au sommet, et les jette si la vue a disparu. `KeepAlive` garde une vue vivante après `Esc` (un run continue en arrière-plan, on y revient via `:runs`).
- `internal/ui` ne connaît aucune ressource : `internal/resource` construit des `ui.Resource` (colonnes, `Load`, `Enter`, `Actions`) et `cmd/playwatch` les enregistre.
- `Enter` empile une vue (ex. hôte → describe), `Esc` dépile.
- `:<ressource>` remplace la vue courante par la vue liste de cette ressource (avec autocomplétion).
- `/` = filtre générique sur toute vue tableau. `?` = aide.
- **Registre de ressources** : chaque ressource déclare ses colonnes, son chargement, ses actions (`r` run, `l` limit, `d` describe, `e` éditer…). Ajouter une ressource ne doit pas toucher au modèle racine.

### Règles Bubble Tea

- **Ne jamais bloquer dans `Update`** : toute I/O passe par une `tea.Cmd`.
- Streaming : goroutine qui lit le pipe d'événements → channel → `tea.Cmd` qui lit un message puis se relance après traitement :

```go
func waitEvent(ch <-chan ansible.Event) tea.Cmd {
    return func() tea.Msg { return <-ch }
}
```

- Annulation via `context.Context` (quitter ou `ctrl+c` sur un run doit tuer proprement le processus ansible).
- Un run n'attend jamais l'UI : une goroutine (`RunView.pump`) vide le flux d'événements, les enregistre dans le store et les met en attente ; l'UI les récupère par lots (`wait`). Quitter playwatch interrompt les runs en cours puis attend leur enregistrement.
- Sortie brute d'un run (`o`) : `TextView` en mode live (`NewLiveTextView`) : rechargement toutes les 500 ms tant que le run est actif (arrêt quand `RunView.ctx` est annulé), collée en bas sauf si l'utilisateur a remonté (`G` reprend le suivi). Les runs passés sont lus depuis le `.log`, sans suivi.
- Couleurs : l'UI lance `ansible-playbook` avec `PlaybookOptions.Color` (`ANSIBLE_FORCE_COLOR=1`) pour que la sortie brute soit colorée ; le `.log` contient donc des codes ANSI. `c` (dans la sortie brute) les retire à l'affichage uniquement. La CLI (`run --raw`) garde la couleur désactivée par défaut.
- Limitation connue : après un `kill -9` d'playwatch, `ansible-playbook` (dans son propre groupe de processus) continue jusqu'à sa prochaine écriture sur le pipe ; le run apparaît `aborted` dans l'historique (PID d'playwatch mort).
- `$EDITOR` via le mécanisme d'exécution externe de Bubble Tea (suspend/reprise du terminal).
- Styles centralisés dans `internal/ui/styles.go`. Statuts : ok = vert, changed = jaune, failed/unreachable = rouge, skipped = gris.

## Commandes

```bash
go build ./cmd/playwatch          # build
go run ./cmd/playwatch -i testdata/project/inventory.ini
go test ./...               # tests unitaires (-race disponible, gcc installé)
go test -tags integration ./...   # tests d'intégration (Ansible requis)
go vet ./... && golangci-lint run
```

CI/CD (GitHub Actions + GoReleaser, `.goreleaser.yaml`) :
- `.github/workflows/ci.yml` (push sur `main`, PR) : vet, tests `-race`, golangci-lint, tests d'intégration (ansible-core via pip), build snapshot des paquets (artefact).
- `.github/workflows/release.yml` (tag `v*`) : release GitHub avec binaires linux/darwin × amd64/arm64 (tar.gz), paquets deb/rpm/apk, checksums.
- Version injectée via `-ldflags "-X main.version=…"` (`playwatch --version`), `dev` par défaut.
- Release : `git tag v0.1.0 && git push origin v0.1.0`. Test local : `goreleaser release --snapshot --clean`.

## Software factory

Boucle d'agents Claude Code pour livrer une fonctionnalité de bout en bout. La session qui lance `/factory` devient l'orchestrateur : elle lance les agents et elle seule écrit `factory/board.json`.

```
feature → spec-writer (opus) → builder (sonnet) ⇄ security (opus) · ux · ui · code (sonnet) → approver (opus) → merge main | needs-human
```

- `.claude/skills/factory/SKILL.md` : la boucle et les commandes (`/factory <feature>`, `/factory next`, `/factory approve <id>`, `/factory rework <id> <note>`). Pour changer la factory, c'est ce fichier qu'on modifie.
- `.claude/agents/` : un fichier par agent (`spec-writer`, `builder`, `{security,ux,ui,code}-reviewer`, `approver`).
- `factory/backlog.md` : file des fonctionnalités, `factory/dashboard.html` : tableau visuel (`cd factory && python3 -m http.server 8000`).
- Par job : `factory/jobs/<id>/` (`feature.md`, `spec.md`, `build.md`, `round-<n>/review-*.md`, `decision.md`), branche `factory/<id>`, worktree `.worktrees/<id>/` (le checkout principal reste sur `main`).
- Règles : les agents lisent des fichiers, jamais la conversation, et n'écrivent que leur propre fichier. Une review commence par `VERDICT: PASS|CHANGES`, `decision.md` par `APPROVE|ESCALATE`. Un round n'est terminé que quand les 4 reviews existent ; 2 relances automatiques du builder au maximum. Merge local uniquement, jamais de push.
- `board.json`, `factory/jobs/` et `.worktrees/` sont ignorés par git (état d'exécution).

## Tests

- `internal/ansible` : tests unitaires sur des **fixtures JSON** (sorties réelles d'`ansible-inventory`, flux d'événements enregistrés) dans `testdata/`.
- `testdata/project/playbook/` : `site.yml` (tous les types d'événements), `tags.yml` (tags, `always`/`never`, play taggé), `slow.yml` (annulation), `vault.yml`.
- Tests d'intégration marqués par build tag `integration`, qui nécessitent Ansible installé, contre un projet exemple `testdata/project/` ciblant **uniquement localhost** (`connection: local`, tâches `debug`, aucune modification système).
- UI : tester les modèles via leurs messages (`Update`) plutôt que le rendu ; snapshots de `View()` seulement pour les composants stables.

## Conventions

- Code, identifiants et commentaires en **anglais** ; ce fichier et les échanges peuvent rester en français.
- Erreurs enveloppées avec `fmt.Errorf("…: %w", err)` ; jamais de `panic` hors `main`.
- Les erreurs d'Ansible (stderr) sont remontées telles quelles à l'utilisateur dans la barre de statut ou une vue dédiée.
- Ansible peut réussir (exit 0) en n'écrivant que des warnings sur stderr (ex. inventaire illisible → seul `localhost` implicite) : stderr est donc toujours remonté, même en cas de succès.
- Pas de dépendance nouvelle sans raison claire ; préférer la stdlib.
- Petits commits ciblés, une fonctionnalité de la roadmap à la fois.
