Run Claude Code with Opus 5.5 through your own Kiro accounts.

```sh
curl -fsSL https://raw.githubusercontent.com/evgenspm/kclaude/main/install.sh | sh
kclaude accounts add personal
cd your-project
kclaude
```

Requires an installed Claude Code, Python 3.9+, and an eligible Kiro API key. Archives support macOS/Linux on arm64/amd64; the installer checks SHA-256 hashes.

The default launch mode is `--dangerously-skip-permissions`. Use `kclaude --safe` for normal permission prompts.

This release adds account failover, separate settings/history, and `--from-claude` session copying. See the README for account setup, limitations, and storage paths. Apache-2.0; independent of Amazon and Anthropic.
