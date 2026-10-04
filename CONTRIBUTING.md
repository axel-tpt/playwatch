# Contributing to playwatch

Thanks for your interest in playwatch! Bug reports, ideas and pull requests are all welcome.

## Before you start

- **Bugs**: open an issue with your playwatch version (`playwatch --version`), your Ansible version (`ansible --version`), your OS, and the steps to reproduce.
- **Features**: open an issue first to discuss it, especially for anything larger than a small fix. The [roadmap](README.md#roadmap) shows what is planned.
- **Security issues**: do not open a public issue. See [SECURITY.md](SECURITY.md).

## Development setup

You need Go (the version in `go.mod`) and, for integration tests, `ansible-core` in your `$PATH`.

```bash
go build ./cmd/playwatch                    # build
go run ./cmd/playwatch -i testdata/project/inventory.ini
go test ./...                               # unit tests
go test -tags integration ./...             # integration tests (need Ansible)
go vet ./... && golangci-lint run           # checks
```

The sample project in `testdata/project/` only targets localhost aliases and only runs `debug` tasks, so it is safe to run on your machine.

## Design principles

playwatch follows a few rules. Pull requests that break them will be asked to change.

- **Ansible is the source of truth.** Never parse inventories, `ansible.cfg` or variables yourself, and never reimplement Ansible logic (variable precedence, inventory plugins…). Use the JSON output of Ansible's own commands (`ansible-inventory --list`, `ansible-doc -j`, …).
- **No text scraping.** Run results come from the embedded callback plugin as structured JSON events, never from `ansible-playbook`'s text output.
- **No new runtime dependency** besides Ansible itself (no `ansible-runner`, no extra Python packages). Prefer the Go standard library; justify any new Go module.
- **Never log secrets.** Respect `no_log`, and never write a vault password to disk or to the run history.

## Architecture

```
cmd/playwatch/       entry point, cobra subcommands
internal/ansible/    Ansible integration (commands, events, callback plugin)
internal/store/      run history on disk
internal/resource/   resources shown in the UI (hosts, groups, playbooks, runs…)
internal/ui/         Bubble Tea app: view stack, command bar, filter, tables
```

- `internal/ansible` and `internal/store` **must not import** `internal/ui` or Bubble Tea. They must stay testable without a terminal and usable from the CLI subcommands.
- `internal/ui` knows no resource. A new resource lives in its own file in `internal/resource` and must not require changes to the root model.
- Never block in `Update`: all I/O goes through a `tea.Cmd`.
- playwatch uses **Bubble Tea, Bubbles and Lip Gloss v2**. Their APIs and import paths differ from v1; check the v2 documentation rather than copying v1 examples.

### Event contract

The callback plugin (`internal/ansible/callback/playwatch_events.py`) and the Go types (`internal/ansible/events.go`) share a versioned JSON contract. Any change to it must:

1. bump the version (`EVENTS_VERSION` in the plugin and `EventsVersion` in `events.go`),
2. update both sides in the same pull request,
3. come with tests.

## Tests

- Unit tests in `internal/ansible` run against JSON fixtures in `testdata/` (real `ansible-inventory` output, recorded event streams). Add a fixture rather than calling Ansible.
- Integration tests use the `integration` build tag and the sample project. They must only target localhost and must not change the system.
- UI models are tested through their messages (`Update`) rather than their rendering.

Please add or update tests with every change. CI runs vet, `go test -race`, golangci-lint and the integration tests on every pull request.

## Pull requests

- Keep pull requests small and focused on one thing.
- Use [Conventional Commits](https://www.conventionalcommits.org/) prefixes (`feat:`, `fix:`, `docs:`, `test:`, `refactor:`, `chore:`…). The release changelog is generated from them.
- Code, identifiers and comments are in English.
- Wrap errors with `fmt.Errorf("…: %w", err)`; never `panic` outside `main`.
- Update the README when you change user-facing behaviour.

## License

By contributing, you agree that your contributions are licensed under the [Apache License 2.0](LICENSE), like the rest of the project.
