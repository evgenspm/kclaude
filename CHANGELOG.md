# Changelog

## [1.0.1.1] - 2026-10-07

- Wait up to three minutes for Kiro response headers instead of 30 seconds, preventing premature timeouts on conversations with many images. Cancelling the request still interrupts the wait.

## [1.0.1.0] - 2026-10-07

- Fix images returned by `Read`: send them to the model, preserve them in conversation history, and keep parallel reads in the correct order.
- Accept requests up to 64 MiB, removing an inner 4 MiB limit that blocked long conversations with images. Larger requests return HTTP 413 with a recovery hint.
- Exclude image base64 from text token estimates and allow an estimated 1,600 tokens per image when upstream usage is absent.
- Record verified native Claude installations in the separate profile, fixing the `install method is unknown` warning while preserving existing settings.

## [1.0.0.2] - 2026-10-07

- Mirror your own Claude Code setup: link `CLAUDE.md`, skills, plugins, agents, commands, output styles, MCP config, keybindings and project memory from `~/.claude`, and rebuild settings (hooks, permissions, env) from yours on every launch. History stays separate.
- Add `KCLAUDE_SHARED` and `KCLAUDE_SHARE_PROFILE=0`; `KCLAUDE_SOURCE_CONFIG` now also selects the mirrored profile.
- Add README instructions for coding agents that install kclaude on a custom setup.

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
