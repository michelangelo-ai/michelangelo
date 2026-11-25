---
last_verified: 2026-11-18
sidebar_position: 4
---

# UI Authentication

This guide is for platform operators who have enabled `apiserver.auth.mode: k8s-rbac` (see [Authentication & Identity](./authentication.md)) and need the Web UI to keep working under it. It covers why the stock UI cannot authenticate on its own, and two ways to front it with an auth proxy — one that needs no code changes, and one that does.

**Prerequisites**: [Authentication & Identity](./authentication.md) (OIDC provider configured, `k8s-rbac` mode understood) and [Network & Ingress](./network.md) (Ingress setup for the UI and Envoy).

## Why this is needed

As [Authentication & Identity](./authentication.md#auth-modes) already warns: the Web UI does not acquire OIDC tokens on its own. Concretely, the UI's RPC transport (`@michelangelo-ai/rpc`'s `createFetchTransport`) sends a fixed set of headers on every call — `Content-Type`, `context-Ttl-Ms`, `grpc-timeout`, `Rpc-Caller`, `Rpc-Service`, `Rpc-Encoding`, plus whatever static `headers` the UI is configured with — and never an `Authorization` header derived from a login. In `dummy` mode that's fine, because the API server installs no authentication middleware. The moment `k8s-rbac` mode is on, every call through Envoy needs a valid bearer token, and the UI has no way to produce one: enabling `k8s-rbac` without doing anything else here locks the UI out entirely, including reads.

The fix is the same one used for any browser app sitting in front of an OIDC-protected API: put an auth proxy in front of it that owns the login flow and attaches the token for you. [oauth2-proxy](https://oauth2-proxy.github.io/oauth2-proxy/) is the de facto standard for this and is what this guide uses, with NGINX Ingress's `auth_request` support as the integration point (if your Ingress controller doesn't support `auth_request` — Traefik's `ForwardAuth` and HAProxy's equivalents cover the same need — adjust the annotations accordingly).

**Pick Option A unless you have a specific reason not to.** It needs no code or chart changes and has no CORS complexity to get wrong. Option B exists for deployments that must keep Envoy on a genuinely separate hostname from the UI, and it carries real, non-obvious failure modes documented below.

## Option A: Put Envoy on the UI's own origin (recommended)

If the UI and Envoy are reachable under the **same origin** — same scheme, host, and port, just different paths — none of the cross-origin problems in Option B exist: no CORS, no cookie-domain configuration, no preflight interaction with `auth_request`, because same-origin requests are never subject to CORS at all.

The chart's own `ui.apiBaseUrl` already supports this: [Network & Ingress](./network.md#envoy-proxy-configuration) notes the UI calls Envoy "on the Envoy host (**or a path on a shared host**, if your controller rewrites paths)". The chart doesn't generate a single merged Ingress for you, so you write one yourself and disable the chart's own `ui.ingress`/`envoy.ingress`:

`auth-url`/`auth-signin` and the other `nginx.ingress.kubernetes.io/*` annotations below apply to every path in the Ingress resource that carries them — there is no per-path override. oauth2-proxy's own `/oauth2` path must therefore live on a **separate** Ingress resource (same host, no auth annotations), or the auth subrequest would end up challenging oauth2-proxy's own login endpoint:

```yaml
# Unprotected: oauth2-proxy's own endpoints. Must NOT carry auth-url/auth-signin,
# or the login flow would try to authenticate against itself.
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: michelangelo-app-oauth2
spec:
  ingressClassName: nginx
  tls:
    - secretName: michelangelo-app-tls
      hosts:
        - app.your-domain.com
  rules:
    - host: app.your-domain.com
      http:
        paths:
          - path: /oauth2
            pathType: Prefix
            backend:
              service: { name: oauth2-proxy, port: { number: 4180 } }
---
# Protected: the UI and Envoy.
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: michelangelo-app
  annotations:
    nginx.ingress.kubernetes.io/auth-url: "https://app.your-domain.com/oauth2/auth"
    nginx.ingress.kubernetes.io/auth-signin: "https://app.your-domain.com/oauth2/start?rd=$escaped_request_uri"
    nginx.ingress.kubernetes.io/use-regex: "true"
    nginx.ingress.kubernetes.io/rewrite-target: /$2
spec:
  ingressClassName: nginx
  tls:
    - secretName: michelangelo-app-tls
      hosts:
        - app.your-domain.com
  rules:
    - host: app.your-domain.com
      http:
        paths:
          # /api/* -> Envoy, with the /api prefix stripped before forwarding
          - path: /api(/|$)(.*)
            pathType: ImplementationSpecific
            backend:
              service: { name: michelangelo-envoy, port: { number: 8081 } }
          # everything else -> the UI's static assets
          - path: /()(.*)
            pathType: ImplementationSpecific
            backend:
              service: { name: michelangelo-ui, port: { number: 80 } }
```

```yaml
ui:
  ingress:
    enabled: false   # replaced by the merged Ingress above
  apiBaseUrl: "https://app.your-domain.com/api"

envoy:
  ingress:
    enabled: false   # replaced by the merged Ingress above
```

Because the redirect back from the IdP (`rd=$escaped_request_uri`) targets the *same* host the request came from, there's no `--whitelist-domain` or `--cookie-domain` configuration needed on the oauth2-proxy side either, and oauth2-proxy's session cookie is scoped to `app.your-domain.com` by default — exactly the host both the UI and Envoy are now reachable on, so the cookie reaches every `auth_request` subrequest without any extra configuration. See [Deploy oauth2-proxy](#deploy-oauth2-proxy) below for the rest of its config; the one difference from Option B is Ingress exposure, called out there.

Service names above assume a release named `michelangelo`, where the chart's `michelangelo.fullname` helper collapses to just `michelangelo` (it only prepends the release name when the release name doesn't already contain it). For a release named anything else — say `prod` — `michelangelo.componentFullname` (`_helpers.tpl`) produces `<release>-michelangelo-<component>`, e.g. `prod-michelangelo-envoy`, **not** `prod-envoy`. Don't guess: run `helm template . | grep -A2 'kind: Service'` against your actual release name and values, or set `fullnameOverride: michelangelo` to keep the names in this guide exactly correct regardless of release name.

## Option B: Separate hostnames for the UI and Envoy

Use this only if you have a hard requirement to keep the UI and Envoy on distinct origins (separate TLS certs issued to different teams, DNS delegation constraints, etc.). It is meaningfully harder to get right: [Network & Ingress](./network.md#domain-names-to-update-in-helm-values) requires a distinct hostname for each of `apiserver.ingress`, `ui.ingress`, and `envoy.ingress` when you use the chart's own Ingress templates, and every cross-origin consequence below follows from that choice, not from anything specific to authentication.

### Architecture

```
Browser
  │  (1) GET app.your-domain.com/            no session cookie yet
  ▼
UI Ingress ──auth_request──► oauth2-proxy ──► redirects browser to IdP login
  │  (2) browser returns with session cookie set by oauth2-proxy
  ▼
UI pod (nginx, static assets)                normal page load from here on

Browser
  │  (3) fetch() to envoy.your-domain.com/{service}/{method}, cookie attached
  ▼
Envoy Ingress ──auth_request──► oauth2-proxy ──► session valid, returns 200
  │  (4) ingress controller forwards the original request to Envoy,
  │      with oauth2-proxy's Authorization header copied onto it
  ▼
Envoy ──► apiserver (apiserver.auth.mode: k8s-rbac verifies the token)
```

Protecting the UI Ingress is **required**, not optional: the session cookie is only ever issued through a real browser navigation (the redirect to the IdP and back), and the UI's page load is the only navigation in this flow. If only `envoy.ingress` is protected, there is no path that ever logs a user in — every API call fails from the first request, not just confusingly.

### CORS preflight: an unresolved hard problem with this option

Because the UI sends a non-simple `Content-Type` and custom headers (`Rpc-Caller`, etc.), every cross-origin call triggers a browser CORS **preflight** — an `OPTIONS` request the browser sends before the real one, which **never carries cookies**. NGINX Ingress's `auth_request` runs on every method including `OPTIONS`, so the preflight itself gets challenged by oauth2-proxy, fails (no cookie), and the real request is never sent — independent of whether the values in [Configure credential sharing](#configure-credential-sharing-option-b) are set.

This guide does not have a verified fix for this interaction. The most likely candidate is oauth2-proxy's `--skip-auth-preflight` flag, which is documented to skip authentication for `OPTIONS` requests — but `auth_request` subrequests are issued as `GET` regardless of the original method, so it is not confirmed here whether oauth2-proxy can tell the two apart in that mode. **Verify this in your own cluster before relying on it**; if it doesn't resolve the issue, [Option A](#option-a-put-envoy-on-the-uis-own-origin-recommended) sidesteps the problem entirely, since same-origin requests never trigger a CORS preflight.

### Configure credential sharing (Option B)

Making the UI send its session cookie to Envoy, and letting Envoy accept it cross-origin, needs both of these Helm values set together — a deployment with only one of them configured fails every call: either the cookie never gets sent, or it gets sent but the browser discards the response.

| Helm value | Type | Default | Description |
|---|---|---|---|
| `envoy.corsOrigins` | string (RE2 regex) | `""` (treated as `.*`) | Origins Envoy accepts via CORS. Must name your UI origin explicitly once credentials are enabled — see the validation below. |
| `envoy.corsAllowCredentials` | boolean | `false` | When `true`, renders `allow_credentials: true` into Envoy's CORS policy, so the browser accepts responses to credentialed cross-origin requests. |
| `ui.apiCredentials` | string | `""` | The `fetch()` `credentials` mode the UI uses for every API call: `"include"`, `"same-origin"`, or `"omit"`. Empty keeps the browser default (`"same-origin"`) and leaves `/config.json` unchanged. |

```yaml
envoy:
  corsOrigins: "https://app\\.your-domain\\.com"
  corsAllowCredentials: true

ui:
  apiCredentials: "include"
```

The chart refuses to render these combinations, rather than silently doing the wrong thing:

- `envoy.corsAllowCredentials` set to anything other than a real boolean. `--set-string envoy.corsAllowCredentials=false` produces the string `"false"`, which Helm/Go templates treat as truthy — the opposite of what was asked for.
- `envoy.corsAllowCredentials=true` with `corsOrigins` left unset or set to the literal wildcard `.*`. Envoy's CORS filter doesn't send a literal `Access-Control-Allow-Origin: *` — on a match it echoes back the caller's actual `Origin` header, so a wildcard paired with credentials would let **any** origin make authenticated, cookie-bearing calls.
- `ui.apiCredentials` set to anything other than empty, `"include"`, `"same-origin"`, or `"omit"`. Non-string values such as `false` or `0` are rejected too, not silently treated as unset.
- `ui.apiCredentials=include` without `envoy.corsAllowCredentials=true`. The browser would send the cookie but discard every response.

Internally: `ui.apiCredentials` is what makes the UI call `fetch()` with the matching `credentials` option — `javascript/packages/rpc/create-fetch-transport.ts` defaults to `'same-origin'` otherwise, so cookies are attached only when the request target is the same origin as the page. The chart renders the value into the UI's runtime `/config.json`, which the UI reads on startup; no application code change is needed.

### Deploy oauth2-proxy

Install the [oauth2-proxy Helm chart](https://github.com/oauth2-proxy/manifests) (or your own manifests) as its own release, registered against the same OIDC provider as `apiserver.auth.oidc`:

```yaml
config:
  clientID: <your-oidc-client-id>
  clientSecret: <your-oidc-client-secret>
  # oauth2-proxy decodes this as URL-safe base64, so standard base64's `+`/`/`
  # characters (present in most random 32-byte outputs) make it fail to start
  # with "cookie_secret must be 16, 24, or 32 bytes" — translate them first:
  cookieSecret: <openssl rand -base64 32 | tr -- '+/' '-_'>

extraArgs:
  provider: oidc
  oidc-issuer-url: https://accounts.your-idp.com      # same as apiserver.auth.oidc.issuerUrl
  cookie-samesite: lax                                 # 'strict' breaks the IdP redirect round-trip
  cookie-refresh: 50m                                  # below your IdP's id_token lifetime (commonly ~1h) — see note below
  email-domain: "*"                                    # or restrict to your org's domain
  set-authorization-header: "true"                     # injects Authorization: Bearer <id_token> on the auth_request response
  skip-auth-preflight: "true"                           # see the CORS preflight note above — unverified in auth_request mode
  reverse-proxy: "true"                                # trust X-Forwarded-* from the Ingress controller
  skip-provider-button: "true"
  upstream: "static://202"                              # auth_request only checks the 2xx/401 status; this path is never actually proxied to

  # Option B only — both are unnecessary (and the second is a no-op) under Option A:
  # cookie-domain: .your-domain.com                     # parent domain covering the UI + Envoy hosts
  # whitelist-domain: .your-domain.com                  # oauth2-proxy drops post-login redirects to any host not listed here

# Option A: no Ingress of its own — the michelangelo-app-oauth2 Ingress shown
# above already routes app.your-domain.com/oauth2 to this release's Service.
ingress:
  enabled: false

# Option B only — its own small Ingress, separate from app/envoy's hostnames.
# Delete the block above and use this one instead:
# ingress:
#   enabled: true
#   className: nginx
#   hosts:
#     - auth.your-domain.com
#   tls:
#     - secretName: auth-your-domain-tls
#       hosts:
#         - auth.your-domain.com
```

`set-authorization-header` is what makes oauth2-proxy inject `Authorization: Bearer <id_token>` on the response the Ingress controller's `auth_request` subrequest reads — that header is what the Ingress annotation below copies onto the real request. (`pass-authorization-header` is a separate flag for oauth2-proxy's *reverse-proxy* mode, where it forwards the token to a real upstream; it does nothing here, since `upstream: static://202` is never actually proxied to.) The `aud` claim in that `id_token` is the OIDC client's `client_id` — `apiserver.auth.oidc.audiences` must include your oauth2-proxy client's `clientID`, or the API server will reject every token on arrival with `token verification failed`.

**TLS on the auth subrequest**: `auth-url` is an `https://` address, and nginx validates that certificate using the Ingress controller's own default trust store (public CAs), independent of and unrelated to the `nginx.ingress.kubernetes.io/auth-tls-*` annotations — those configure *client*-certificate (mTLS) verification for a different purpose entirely. If the certificate behind `auth-url` comes from a public CA (the common case, including the `michelangelo-app-tls`/`auth-your-domain-tls` secrets above if issued via cert-manager and Let's Encrypt), nothing extra is needed. If you terminate TLS there with a private/internal CA instead, point ingress-nginx's `proxy-ssl-secret` annotation at a Secret containing that CA, or the subrequest fails closed with a TLS error. Don't disable verification to work around this — the subrequest is exactly what carries the session cookie.

**Token lifetime**: oauth2-proxy's session cookie defaults to a 168-hour lifetime, far longer than a typical ~1-hour OIDC `id_token`. Without `cookie-refresh` set *below* that token lifetime (and a refresh token — most IdPs need the `offline_access` scope requested for one to be issued), the UI will look logged in while every API call fails with `token verification failed` about an hour after login, which is confusing to diagnose. If your IdP doesn't issue refresh tokens, you'll need to re-authenticate at that same interval instead. Separately, a large OIDC session (many scopes/claims) can make oauth2-proxy split its session across multiple `Set-Cookie` headers — ingress-nginx's `auth_request` module only forwards the *first* `Set-Cookie` from the subrequest, so a refresh can leave the browser with mismatched cookie parts and an interrupted session. If you hit this, the fix is a shared session store (oauth2-proxy's `--session-store-type=redis`) rather than the default cookie-only sessions, not something this guide covers in depth.

**Non-browser clients** (the `ma` CLI, SDK scripts) should keep going through `apiserver.ingress`, not through an `auth_request`-protected Envoy Ingress — oauth2-proxy rejects bearer-only requests by default (`--skip-jwt-bearer-tokens` exists if you need to let them through the same Ingress).

### Protect the Ingress(es)

Under Option A, add these annotations to the single merged Ingress shown above. Under Option B, add them to **both** `ui.ingress` and `envoy.ingress` — but only `ui.ingress` should carry `auth-signin`: a `fetch()` call that receives a redirect to the IdP fails opaquely, where a plain `401` is something the UI can at least detect. `envoy.ingress` should get only `auth-url`.

```yaml
# ui.ingress (and the merged Ingress under Option A)
nginx.ingress.kubernetes.io/auth-url: "https://auth.your-domain.com/oauth2/auth"
nginx.ingress.kubernetes.io/auth-signin: "https://auth.your-domain.com/oauth2/start?rd=$scheme://$best_http_host$escaped_request_uri"
nginx.ingress.kubernetes.io/auth-response-headers: "Authorization"

# envoy.ingress (Option B only) — no auth-signin
nginx.ingress.kubernetes.io/auth-url: "https://auth.your-domain.com/oauth2/auth"
nginx.ingress.kubernetes.io/auth-response-headers: "Authorization"
```

`auth-response-headers: "Authorization"` is what copies the header oauth2-proxy returned on the `auth_request` subrequest onto the real request to Envoy; it's not needed on a UI Ingress that only serves static assets, but it's harmless to leave there if you're reusing the same annotation set.

## Security notes

- **`cookie-domain` widens your CSRF trust boundary.** Under Option B, setting it to a parent domain (`.your-domain.com`) makes every subdomain of it "same-site" for the purposes of `SameSite=Lax` — a compromised or user-content subdomain elsewhere on that parent domain could send credentialed requests to Envoy, and read the responses too if `corsOrigins` matches it. Prefer a dedicated parent domain used for nothing else. Option A has no equivalent exposure, since there's no cross-site cookie at all.
- **If the UI and Envoy aren't even subdomains of a shared parent domain** — genuinely different registrable domains, not just different hosts under one — `cookie-domain` can't help at all; cookies can't be scoped across domains. The only way the session cookie crosses in that case is `cookie-samesite: none` with `cookie-secure: true`, which drops the CSRF protection `SameSite=Lax` was providing. Before reaching for this, reconsider whether Option B is still the right call for your setup.
- **Escape literal dots in `corsOrigins`.** As `helm/michelangelo/values.yaml` already notes for the field generally, an unescaped `https://app.your-domain.com` also matches `https://app-your-domain.com` as a regex. This matters more once credentials are involved.
- **The bearer token also crosses two internal hops in cleartext by default.** Both the Ingress-to-Envoy hop (documented above as plain `http://` to the `michelangelo-envoy` Service) and the chart's own Envoy-to-apiserver hop default to plaintext. Setting `apiserver.tls.enabled: true` encrypts the second hop, but check what it actually buys you before relying on it for this: Envoy's config renders `trust_chain_verification: ACCEPT_UNTRUSTED` on that connection (`envoy-configmap.yaml`), which encrypts the traffic but does **not** authenticate the API server's certificate — it protects against passive sniffing on that hop, not an active on-path attacker presenting any certificate. Anyone who can observe either hop, or sit on the apiserver one, can still capture and replay the token. Terminating both hops with real, verified TLS (or routing them through a service mesh that already does) is out of scope for this guide, but don't treat `apiserver.tls.enabled: true` alone as closing this gap.

## Keep `apiserver.auth.oidc` in sync (Option B)

The API server validates the same `id_token` oauth2-proxy forwards, using its own, independently-configured OIDC settings (see [Enabling k8s-rbac mode](./authentication.md#enabling-k8s-rbac-mode)). Two fields must agree between the two configs or the token that got the browser past oauth2-proxy will still be rejected by the API server:

| oauth2-proxy (`extraArgs`) | `apiserver.auth.oidc` | Must match because |
|---|---|---|
| `oidc-issuer-url` | `issuerUrl` | Both validate the token's `iss` claim against this value |
| `config.clientID` | `audiences` | The token's `aud` claim is the client ID; the API server rejects any token whose `aud` isn't in this list |

If you set `usernameClaim: sub` on the API server (see [Choosing a usernameClaim](./authentication.md#choosing-a-usernameclaim-email-vs-sub)), that choice is independent of oauth2-proxy — oauth2-proxy only needs to pass the token through; it does not need to agree on which claim is the username.

## Troubleshooting

| Symptom | Likely cause |
|---|---|
| Browser redirect loop between the UI and the IdP | Under Option B, `cookie-domain` doesn't cover the UI's host, so oauth2-proxy never sees its own cookie on the next request |
| Every API call fails, browser console shows a CORS error | Check the Network tab first: a `401`/`302` from the Ingress (auth failure, or the preflight problem under Option B) can look identical to a CORS error. If the response really is `200` with missing CORS headers: `envoy.corsAllowCredentials` isn't set, or `ui.apiCredentials` isn't `"include"` — both values in [Configure credential sharing](#configure-credential-sharing-option-b) must be set together |
| `auth_request` returns 200 but the API call still gets `missing authorization header` | Either `auth-response-headers: "Authorization"` is missing from the protected Ingress, or oauth2-proxy is missing `set-authorization-header: "true"` — either way, no header made it onto the real request |
| API call reaches Envoy with a token, apiserver rejects it with `token verification failed` | Most likely an expired `id_token` — see the token-lifetime note under [Deploy oauth2-proxy](#deploy-oauth2-proxy) — or an `issuerUrl`/`audiences` mismatch, see [above](#keep-apiserverauthoidc-in-sync-option-b) |
| Login fails outright with a CSRF-cookie error | `cookie-samesite: strict` blocks the CSRF cookie on the cross-site navigation back from the IdP — use `lax` |
| Post-login redirect lands on the auth host's blank page instead of the app | Option B only: `whitelist-domain` is missing or doesn't cover the host in `rd=` |

## What's Next

- **Core authentication concepts**: [Authentication & Identity](./authentication.md) — OIDC configuration, `k8s-rbac` mode, RBAC grants
- **Ingress and CORS fundamentals**: [Network & Ingress](./network.md)
- **Embedding the UI elsewhere**: [Michelangelo AI React Library](../ui/michelangelo-react-library.md) — the same cross-origin considerations apply if you embed `@michelangelo-ai/core` behind your own auth proxy
