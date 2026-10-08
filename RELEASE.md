Fix premature upstream timeouts when a conversation contains many images.

Kiro can take more than 30 seconds after an upload to send response headers. The router now waits up to three minutes. Cancelling a request still stops the wait immediately; retry behavior is unchanged.

Run the install command again to update, then run `kclaude stop` between active requests. The next launch starts the updated router. Stopping the router can interrupt current responses; terminal tabs and saved conversations remain available.

Verified with a server that delays headers for 31 seconds, a cancellation check, and a live request with 19 images that returned successfully after the previous deadline.
