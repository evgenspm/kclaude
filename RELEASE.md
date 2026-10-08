Fix image reads and conversations blocked by `request body too large`.

- Claude now receives images returned by `Read`, including images in earlier turns and parallel tool results.
- Requests up to 64 MiB reach the router. The old inner 4 MiB limit no longer rejects image-heavy conversations; larger requests return HTTP 413.
- Chat token estimates exclude image base64 and use an approximate 1,600 tokens per image when Kiro omits usage.
- Native Claude installations no longer show `install method is unknown` in the separate profile.

Run the install command again to update. Wait for active requests to finish, run `kclaude stop`, then resume your conversation with `kclaude --resume SESSION_ID`. Review visual work produced before this fix because the model may not have received its images.

Verified with Go race tests, Python installer and launcher tests, and a live Opus 5.5 test that read a random code from an image through Claude Code's `Read` tool and retained the image in a later request.
