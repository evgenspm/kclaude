Run Claude Code with Opus 5.5 through your own Kiro accounts.

Fixes token counters stuck near zero: stop-reason metadata no longer overwrites token estimates with zeros. Thinking and tool arguments are included. Exact upstream counts take priority, and cached input is reported separately.

Kiro API-key responses may omit token counts, so Claude Code's counters are approximate and its dollar estimate is not Kiro billing. Existing zero counts in saved history remain unchanged. Finish active work, run `kclaude stop`, then update with the installer below; the next launch starts the updated router.

```sh
curl -fsSL https://raw.githubusercontent.com/evgenspm/kclaude/main/install.sh | sh
kclaude accounts add personal
cd your-project
kclaude
```

Requires an installed Claude Code, Python 3.9+, and an eligible Kiro API key. Archives support macOS/Linux on arm64/amd64; the installer checks SHA-256 hashes.

The default launch mode is `--dangerously-skip-permissions`. Use `kclaude --safe` for normal permission prompts.

Includes account failover, separate settings/history, and `--from-claude` session copying. See the README for account setup, limitations, and storage paths. Apache-2.0; independent of Amazon and Anthropic.
