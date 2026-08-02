# Authelia

## What it is and why use it

[Authelia](https://www.authelia.com/) is an authentication proxy. It protects Atria behind a single-sign-on login, so normal application routes are not accessible without an authenticated session.

Atria public article and note links are intentionally different: Atria checks that the entity has `visibility = public` and exposes only its content. Those routes must bypass Authelia.

## How to use it with Atria

When Authelia is enabled at the Angie server level, add public exceptions for Atria links and static assets before the protected catch-all location:

```nginx
include /etc/angie/authelia-auth.conf;

location ^~ /public/ {
    auth_request off;
    proxy_pass http://localhost:8089;
}

location ^~ /static/ {
    auth_request off;
    proxy_pass http://localhost:8089;
}

location / {
    proxy_pass http://localhost:8089;
    # Authelia remains enabled here.
}
```

Do **not** exempt `/data/attachments/`; public pages do not expose attachments.

Validate and reload Angie:

```sh
sudo angie -t
sudo systemctl reload angie
```

A public link should open in a private/incognito browser window, while `/read` should still redirect unauthenticated visitors to Authelia.
