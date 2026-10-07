# Security

Do not include API keys, proxy tokens, or chat transcripts in an issue. Report a vulnerability through [GitHub private vulnerability reporting](https://github.com/evgenspm/kclaude/security/advisories/new).

The default launch mode skips Claude tool permission prompts. Use `--safe` when you want Claude to ask before running tools. This setting does not grant access to Kiro accounts; you supply your own keys.

The router binds to loopback and requires a random local token. It generates a private local TLS certificate, which Claude trusts through a child-process environment variable. The launcher also checks a nonce/HMAC proof before sending the token over the same connection. Files containing credentials use mode `0600` within a private state directory. A process running as your own OS user can still read your keys and history. Do not expose the router through a tunnel or reverse proxy.

Revoke a leaked Kiro key in the Kiro dashboard, then replace it with `kclaude accounts add NAME`. Removing an account locally does not revoke the remote key. Request content goes to Kiro, and provider policies still apply.
