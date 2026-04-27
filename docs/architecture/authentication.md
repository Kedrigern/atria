# Atria - Authentication & Security

Atria employs a **Proxy Auth-Only (Forward Authentication)** architecture. It delegates all user authentication, session management, and multi-factor authentication (MFA) to a dedicated external Identity Provider (IdP) acting as a reverse proxy (e.g., **Authelia** or **Authentik**).

Atria itself does not manage passwords, session cookies, or login screens for the web interface.

## 1. Forward Authentication Flow
1. A user requests access to the Atria web interface.
2. The Reverse Proxy (Traefik, Nginx, Caddy) intercepts the request and forwards it to Authelia.
3. If the user is not authenticated, Authelia presents its own login UI.
4. Once authenticated, the proxy forwards the request to Atria, injecting HTTP headers containing the user's identity (e.g., `Remote-Email` and `Remote-Groups`).
5. Atria's internal Gin middleware reads these headers, verifies the user exists in its database, and grants access.

## 2. Security & Trusted Proxies (Spoofing Protection)
To prevent malicious actors from bypassing the proxy and sending spoofed `Remote-Email` headers directly to Atria's exposed port, the system strictly enforces IP filtering.

* **`TRUSTED_PROXIES`:** Configured via the `.env` file. Atria will ONLY trust identity headers if the request originates from these specific IP addresses or CIDR blocks (e.g., the Docker network gateway).
* If a request arrives from an untrusted IP, the identity headers are stripped and ignored, resulting in a `401 Unauthorized` response.

## 3. Role Synchronization
Atria synchronizes user roles dynamically based on the group headers provided by the IdP.
* Header: `Remote-Groups` (comma-separated list of groups).
* If the user belongs to the `admins` group in Authelia, Atria automatically upgrades their database role to `admin` upon their next request. Otherwise, they default to `user`.

## 4. Local Development Bypass
To ensure a smooth developer experience without needing to spin up an entire Authelia stack locally, Atria supports a development bypass mechanism.

* **Trigger:** Requires `ATRIA_ENV=development` in the `.env` file.
* **Behavior:** If the identity headers are missing, the middleware falls back to the user specified in the `ATRIA_USER` environment variable.
* **Security:** This bypass is strictly disabled when `ATRIA_ENV=production`.

## 5. Command Line Interface (CLI) Access
The Atria CLI operates independently of the web server's HTTP middleware.
* The CLI connects **directly to the PostgreSQL database**.
* Because it circumvents the HTTP layer entirely, the CLI is not blocked by Authelia. 
* The user context for CLI operations is determined by the `-u, --user` flag or the `ATRIA_USER` environment variable.
