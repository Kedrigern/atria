# Atria - Authentication & Security

Atria primarily uses **Forward Authentication**: it delegates user authentication and multi-factor authentication (MFA) to an external Identity Provider (IdP) acting as a reverse proxy (e.g., **Authelia** or **Authentik**). For setups without an IdP it also offers a minimal local password login (see section 4).

## 1. Forward Authentication Flow
1. A user requests access to the Atria web interface.
2. The Reverse Proxy (Traefik, Nginx, Caddy) intercepts the request and forwards it to Authelia.
3. If the user is not authenticated, Authelia presents its own login UI.
4. Once authenticated, the proxy forwards the request to Atria, injecting HTTP headers containing the user's identity (e.g., `Remote-Email` and `Remote-Groups`).
5. Atria's internal Gin middleware reads these headers, verifies the user exists in its database, and grants access.

## 2. Security & Trusted Proxies (Spoofing Protection)
To prevent malicious actors from bypassing the proxy and sending spoofed `Remote-Email` headers directly to Atria's exposed port, the system strictly enforces IP filtering.

* **`PROXY_ALLOWLIST`:** Atria ONLY trusts identity headers if the raw TCP peer address (`RemoteAddr`) exactly matches one of these IPs (e.g., the reverse proxy). If it is empty, proxy header auth is disabled.
* **`TRUSTED_PROXIES`:** Used only to resolve the real client IP from `X-Forwarded-For` (e.g., for login throttling); it never makes identity headers trusted.
* If a request arrives from an untrusted IP, the identity headers are ignored and the request must authenticate with a session cookie, otherwise it gets `401 Unauthorized`.

## 3. Role Synchronization
Atria synchronizes user roles dynamically based on the group headers provided by the IdP.
* Header: `Remote-Groups` (comma-separated list of groups).
* If the user belongs to the `admins` group in Authelia, Atria automatically upgrades their database role to `admin` upon their next request. Otherwise, they default to `user`.

## 4. Local Password Login & Sessions
* `POST /login` checks the bcrypt password hash and sets the `atria_session` cookie (`HttpOnly`, `SameSite=Strict`, `Secure` outside development).
* Cookie format: `v1.<base64url(email)>.<expiry unix>.<HMAC-SHA256>`. The HMAC covers the email, the expiry (30 days) and the user's current password hash, so changing a password invalidates all existing sessions.
* **`SESSION_SECRET` is mandatory** outside `development`/`test`: the server refuses to start without it (or with the `.env.example` placeholder). Generate one with `openssl rand -hex 32`. In development a fixed fallback secret is used.
* Failed logins are throttled per client IP (10 failures per 15 minutes). Unknown emails still run a bcrypt comparison so response timing does not reveal which accounts exist. Cross-site login posts (`Sec-Fetch-Site: cross-site`) are refused.
* CSRF: every state-changing request in the authenticated group must carry `X-CSRF-Token` or `_csrf` equal to `HMAC(SESSION_SECRET, "csrf", email)`.

## 5. Content Security
* Third-party HTML (readability output of saved articles, RSS item content) is sanitized with a bluemonday allowlist before rendering (`core.SanitizeHTML`). There is intentionally no template helper that marks arbitrary strings as safe HTML.
* Search snippets are HTML-escaped; only the highlight markers become `<mark>`.
* Every response carries `Content-Security-Policy`, `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY` and `Referrer-Policy`. The CSP still allows inline scripts because the templates use inline handlers; moving them to static files would allow dropping `'unsafe-inline'`.
* Attachments are served with the MIME type sniffed at upload time. Only raster images, PDF, plain text, audio and video are shown inline; everything else (HTML, SVG, …) is forced to download under a `sandbox` CSP.
* Request bodies are capped by `MAX_UPLOAD_MB` (default 100). Server-side fetches (articles, RSS) refuse private, loopback, link-local/metadata, CGNAT and other reserved address ranges.

## 6. Local Development Bypass
To ensure a smooth developer experience without needing to spin up an entire Authelia stack locally, Atria supports a development bypass mechanism.

* **Trigger:** Requires `ATRIA_ENV=development` in the `.env` file.
* **Behavior:** If the identity headers are missing, the middleware falls back to the user specified in the `ATRIA_USER` environment variable.
* **Security:** This bypass is strictly disabled when `ATRIA_ENV=production`.

## 7. Command Line Interface (CLI) Access
The Atria CLI operates independently of the web server's HTTP middleware.
* The CLI connects **directly to the PostgreSQL database**.
* Because it circumvents the HTTP layer entirely, the CLI is not blocked by Authelia. 
* The user context for CLI operations is determined by the `-u, --user` flag or the `ATRIA_USER` environment variable.
