# Changelog

## [1.0.0.1] - 2026-10-07

- Fix zero token usage when Kiro sends metadata containing only a stop reason. Use local input/output estimates when upstream counts are absent, including thinking and tool arguments.
- Keep cached tokens separate from uncached input when upstream token counts are available.
- Document that Claude Code token and dollar figures are estimates, not Kiro billing.

## [1.0.0.0] - 2026-10-07

- Run installed Claude Code through your own Kiro API keys, with Opus 5.5 as the default.
- Add, replace, disable and remove accounts; switch accounts when a request is rejected.
- Keep a separate Claude profile and copy existing sessions into a new fork.
- Install ready-made binaries on macOS and Linux, on arm64 and amd64.
- Choose normal permission prompts with `--safe`; the default skips tool permission prompts.
