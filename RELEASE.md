Run Claude Code with Opus 5.5 through your own Kiro accounts.

kclaude now looks like your normal Claude Code. On every launch it links your `CLAUDE.md`, skills, plugins, agents, commands, MCP config, keybindings and project memory from `~/.claude`, and rebuilds its settings from yours, so hooks and permissions match. Chat history stays separate. Use `KCLAUDE_SOURCE_CONFIG` for a different profile, `KCLAUDE_SHARED` to pick items, or `KCLAUDE_SHARE_PROFILE=0` for the old fully separate profile. If an agent installs kclaude for you, point it at "For coding agents installing kclaude" in the README.

Finish active work, run `kclaude stop`, then update with the installer below.

```sh
curl -fsSL https://raw.githubusercontent.com/evgenspm/kclaude/main/install.sh | sh
kclaude accounts add personal
cd your-project
kclaude
```

Requires an installed Claude Code, Python 3.9+, and an eligible Kiro API key. Archives support macOS/Linux on arm64/amd64; the installer checks SHA-256 hashes.
