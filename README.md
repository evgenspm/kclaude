# kclaude

Use Claude Code with **Claude Opus 5.5 through your Kiro accounts**.

Add your Kiro API keys, run `kclaude` in a project, and use the Claude Code terminal, tools, and session workflow you already know. kclaude runs a local router and a Claude profile that mirrors your own `~/.claude` setup, with separate history. When an account rejects a request because of a limit or an authentication error, the router tries the next enabled account.

[Русский](README.ru.md) · [Releases](https://github.com/evgenspm/kclaude/releases) · [License](LICENSE)

## Install

You need **macOS or Linux** (Apple Silicon/ARM64 or Intel/AMD64), **Python 3.9+**, `curl`, and [Claude Code](https://code.claude.com/docs/en/setup) installed as `claude`. On Windows, use WSL2 with Claude Code installed inside WSL.

```sh
curl -fsSL https://raw.githubusercontent.com/evgenspm/kclaude/main/install.sh | sh
```

The installer downloads a release binary and checks its SHA-256 checksum. You do not need Go or Kiro CLI. It places the command in `~/.local/bin`; if that directory is outside your `PATH`, use `~/.local/bin/kclaude` or add it to your shell's path.

To inspect the installer first or pin a release:

```sh
curl -fsSLo install-kclaude.sh https://raw.githubusercontent.com/evgenspm/kclaude/v1.0.0.0/install.sh
less install-kclaude.sh
KCLAUDE_VERSION=v1.0.0.0 sh install-kclaude.sh
```

## Add your accounts

Sign in to [app.kiro.dev](https://app.kiro.dev/), open **API Keys**, and create a key. Kiro documents API keys for Pro, Pro+, Pro Max, and Power accounts. Requests consume that account's subscription credits. See [Kiro authentication](https://kiro.dev/docs/getting-started/authentication/#api-key-authentication-cli).

```sh
kclaude accounts add personal
```

Paste the `ksk_…` key into the hidden prompt. Add more accounts with different names:

```sh
kclaude accounts add work
kclaude accounts add backup
kclaude accounts list
```

You can copy a key from a local file with `--key-file /path/to/key`. For an EU account, add `--region eu-central-1`; the default is `us-east-1`. Use accounts you own or have permission to use. An AWS access key, password, or browser session token is not a Kiro API key.

## Use it

```sh
cd your-project
kclaude
```

**The default is `--dangerously-skip-permissions`.** Claude can run tools and change files without asking. Use `kclaude --safe` for normal permission prompts, or pass your own `--permission-mode`.

```sh
kclaude --safe                         # Normal permission prompts
kclaude --model sonnet                 # Sonnet 5.5
kclaude --model haiku                  # Haiku 4.5
kclaude --seat work                    # Pin one account; no failover to another
kclaude -p "Explain this repository"  # Print mode
kclaude --continue                     # Continue the last kclaude session here
```

Default model aliases:

| Claude Code selection | Kiro model | Context mapping |
| --- | --- | --- |
| `opus` | `claude-opus-5.5` | 1M |
| `sonnet` | `claude-sonnet-5.5` | 1M |
| `haiku` | `claude-haiku-4.5` | 200K |

Kiro lists Opus 5.5 and Sonnet 5.5 in its [model documentation](https://kiro.dev/docs/models/). Availability and credit costs depend on your account and Kiro's current offering. `kclaude models` shows the adapter's model aliases; it does not check your account's entitlements or remaining credits.

**Token counters:** Kiro API-key responses can omit token counts. When they do, kclaude estimates chat input with `cl100k_base`, excluding image base64 and allowing an estimated 1,600 tokens per image. It estimates output at about four characters per token, including thinking and tool arguments. These are approximations, not Claude's tokenizer or Kiro's bill. Upstream counts take priority when provided. Claude Code's dollar estimate does not represent Kiro subscription charges; check your Kiro account for credit usage. Counts recorded as zero by versions before 1.0.0.1 are not repaired retroactively.

Images returned by `Read` remain visible in later turns. The local request limit is 64 MiB; upstream image and context limits still apply. If a conversation created before 1.0.1.0 gets stuck with `request body too large`, update and restart the router between active requests, then resume that conversation. Re-run visual checks made while images were missing.

## Continue an existing Claude chat

From the same project directory:

```sh
kclaude --from-claude
```

Or copy a specific session ID from Claude's `/status`:

```sh
kclaude --from-claude-id YOUR_SESSION_UUID
```

kclaude copies the transcript and its sidecar directory, then passes `--resume` and `--fork-session` to Claude Code. Your source transcript stays intact. The two copies develop separate histories. Import does not transfer a running process or active background jobs. Project files and project-level settings remain shared because you are working in the same directory.

For a nonstandard source profile, set `KCLAUDE_SOURCE_CONFIG=/path/to/claude-profile` on the import command. Stop editing the same files in the source chat before continuing its copy.

## Your Claude setup

On every launch kclaude links your `CLAUDE.md`, `skills`, `plugins`, `agents`, `commands`, `output-styles`, `mcp.json`, `keybindings.json` and the current project's memory folder from `~/.claude` into its profile, and rebuilds its `settings.json` from yours, so hooks, permissions, plugins and env match. Only chat history and sessions stay separate. kclaude never writes to `~/.claude`, except that memory files left in an old kclaude profile move into your project memory once. `apiKeyHelper` is dropped because credentials come from the router.

| Variable | Effect |
| --- | --- |
| `KCLAUDE_SOURCE_CONFIG=/path` | Mirror a profile other than `~/.claude` |
| `KCLAUDE_SHARED=CLAUDE.md,skills` | Link only these top-level items |
| `KCLAUDE_SHARE_PROFILE=0` | Keep the kclaude profile fully separate |

Put these in your shell startup file if you need them permanently.

### For coding agents installing kclaude

Everyone's setup differs, so check it before the first run instead of assuming the defaults fit:

1. Find the user's real Claude profile: `$CLAUDE_CONFIG_DIR` if set, otherwise `~/.claude`. If it is not `~/.claude`, export `KCLAUDE_SOURCE_CONFIG` to it.
2. List the top-level items there. If the user keeps config somewhere kclaude does not link (for example extra directories referenced from hooks), tell them; hooks use absolute paths and keep working.
3. Look at `settings.json` for anything tied to Anthropic auth or a specific provider (`apiKeyHelper`, `env` entries with `ANTHROPIC_*`, Bedrock/Vertex flags). kclaude drops `apiKeyHelper` and clears `ANTHROPIC_*` variables from the environment, but `env` entries in settings still apply inside Claude. Ask before changing the user's own settings.
4. Run `kclaude -p "reply: ok"` from a project folder and confirm the reply, then check that `~/.local/share/kclaude/claude/skills` points at the user's skills.

## Manage the router

```sh
kclaude status
kclaude accounts disable work
kclaude accounts enable work
kclaude accounts remove backup
kclaude stop
```

The router starts when you launch a chat and stays running until `kclaude stop` or a restart. Account changes stop the router so that it reloads credentials on the next launch. Make those changes between active requests.

Routing uses the current account until a request receives HTTP 401, 402, 403, or 429, or a recognized throttle response. The rejected account enters a cooldown and the router tries another enabled account. It respects `Retry-After` for throttling. It does not replay an ambiguous network failure or an already-started successful stream. If all accounts are unavailable, it returns an error. Cooldowns reset when you restart the router. WebSearch uses an available account but does not retry across the pool.

## Isolation and storage

```text
Claude Code -> https://127.0.0.1:17391 -> your Kiro account -> model
```

| Location | Contents |
| --- | --- |
| `~/.local/share/kclaude/app/` | Installed releases |
| `~/.local/share/kclaude/keys/` | Your Kiro API keys, file mode `0600` |
| `~/.local/share/kclaude/accounts.json` | Account names, key paths, regions |
| `~/.local/share/kclaude/proxy.key` | Random local router token |
| `~/.local/share/kclaude/router-cert.pem`, `router-tls.key` | Certificate and private key for local TLS |
| `~/.local/share/kclaude/claude/` | kclaude profile: links to your setup, its own history |
| `~/.local/share/kclaude/router.log` | Router diagnostics |

The installer and launcher do not edit `~/.claude`, your `claude` executable, or shell startup files. kclaude sets the API URL and credentials only for the Claude process it launches. It binds the router to loopback, authenticates API requests, uses a private local TLS certificate for Claude connections, and rejects browser Origin headers. The launcher also checks a challenge before sending its token. It adds the local certificate to the Claude child's trust through `NODE_EXTRA_CA_CERTS`; it does not install a system certificate. Inference and search requests include Kiro's opt-out header. Conversations still go to Kiro to run the model, and Claude writes session data into its separate local profile.

For separate router instances, set both `KCLAUDE_HOME` and `KCLAUDE_PORT` on all commands for that instance:

```sh
KCLAUDE_HOME="$HOME/.local/share/kclaude-work" KCLAUDE_PORT=17392 kclaude accounts add work
KCLAUDE_HOME="$HOME/.local/share/kclaude-work" KCLAUDE_PORT=17392 kclaude
```

`KCLAUDE_CLAUDE_BIN` selects a different installed Claude executable. `KCLAUDE_INSTALL_DIR` and `KCLAUDE_BIN_DIR` customize installation paths. The launcher clears inherited Anthropic routing variables and HTTP proxy variables for its Claude child so that requests reach the local router; the router can still use your outbound proxy to reach Kiro.

## Update or uninstall

Run the install command again to update. Between sessions, run `kclaude stop`; the next launch starts the new router. The installer preserves your keys, accounts and chat history.

For a default installation, uninstall the command and release files with:

```sh
kclaude stop
rm "$HOME/.local/bin/kclaude"
rm -rf "$HOME/.local/share/kclaude/app"
```

Your keys and session history remain in `~/.local/share/kclaude`. Custom installation paths need the corresponding paths in these commands.

## Troubleshooting

| Problem | Check |
| --- | --- |
| `kclaude: command not found` | Run `~/.local/bin/kclaude` or put `~/.local/bin` on `PATH` |
| Claude executable missing | Install Claude Code; check `command -v claude` |
| No accounts | `kclaude accounts add personal` |
| 401/403 from Kiro | Replace the API key; verify the account, model access and region |
| All accounts cooling down | `kclaude status`; check credits in the Kiro dashboard |
| Port in use | Stop the other router or choose a different `KCLAUDE_PORT` |
| Installer refuses to replace a command | An unrelated `kclaude` already exists; choose `KCLAUDE_BIN_DIR` |
| Need router details | Inspect `~/.local/share/kclaude/router.log`; redact local paths before sharing |

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for tests, builds and release instructions. Release archives contain the open-source router and launcher. Install Claude Code separately.

kclaude is an **unofficial adapter**. Compatibility can change with Claude Code or Kiro updates; features that require Claude-hosted services are outside the adapter's scope. Initial live checks used Claude Code 2.1.293 on macOS arm64. The release includes macOS/Linux binaries for arm64/amd64; CI tests macOS and Linux host builds.

Apache-2.0. Based on [ClaudeCode Kiro](https://github.com/itututu/claudecode-kiro) by itututu and [kirocc](https://github.com/d-kuro/kirocc) by d-kuro. See [NOTICE](NOTICE). Independent of Amazon and Anthropic.
