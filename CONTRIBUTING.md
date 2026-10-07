# Contributing

Use Go 1.26.1 or later with `GOEXPERIMENT=jsonv2`, plus Python 3.9 or later. Python uses only the standard library.

```sh
GOEXPERIMENT=jsonv2 go test -race ./...
GOEXPERIMENT=jsonv2 go vet ./...
GOEXPERIMENT=jsonv2 go build -o /tmp/kclaude-test-router ./cmd/kclaude-router
KCLAUDE_TEST_ROUTER=/tmp/kclaude-test-router python3 -m unittest discover -s tests -v
sh -n install.sh scripts/build-release.sh
```

Tests use synthetic keys and mocked upstream HTTP servers. The process integration test starts a real router on an ephemeral loopback port, checks authentication and model aliases, launches a fake Claude executable, and stops the router. Installer tests mock downloads and check checksum failure, archive traversal, preservation of existing commands, and repeat installation. Without `KCLAUDE_TEST_ROUTER`, Python skips the process integration test.

Inherited Go packages cover request conversion, streaming, tools, model mapping, token counting and protocol errors. Router tests cover account failover, pinned accounts, cooldowns, and rejected versus started responses. Tests must not consume Kiro credits or require a real account. The tokenizer may download its public vocabulary on first use.

## Build a release

```sh
sh scripts/build-release.sh
```

This cross-compiles macOS/Linux binaries for arm64/amd64 with CGO disabled. Archives and `SHA256SUMS` go into `dist/`. To build one platform, use `KCLAUDE_TARGETS=darwin/arm64`.

The archive contains `kclaude`, `kclaude-router`, `VERSION`, `LICENSE`, and `NOTICE`. The launcher resolves the router alongside itself. A source checkout can run against a local build by setting `KCLAUDE_ROUTER` and invoking `python3 cli/kclaude.py`.

## Publish

Update `VERSION`, `CHANGELOG.md`, and `RELEASE.md`. Versions use four components, for example `1.0.0.0`. Push a matching `v1.0.0.0` tag. The release workflow runs tests, builds the four archives, uploads checksums, and publishes the GitHub release. To retry a release, select its existing tag in the workflow dispatch UI. Do not republish different code under an existing version.

Keep release and CI workflows on standard GitHub-hosted runners. Neither workflow uses real credentials, paid APIs, or uploaded Actions build artifacts.

## Scope

Keep this repository independent of personal Claude settings, account keys, chat logs, and project handoffs. The distribution uses a fresh source snapshot, with attribution in `NOTICE`. Do not copy a development user's state directory into tests or release assets. Preserve the upstream license and add a notice when modifying derived files.
