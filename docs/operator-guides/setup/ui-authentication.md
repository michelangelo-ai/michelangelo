---
last_verified: 2026-11-02
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

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: michelangelo-app
  annotations:
    nginx.ingress.kubernetes.io/auth-url: "https://auth.your-domain.com/oauth2/auth"
    nginx.ingress.kubernetes.io/auth-signin: "https://auth.your-domain.com/oauth2/start?rd=$escaped_request_uri"
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

Because the redirect back from the IdP (`rd=$escaped_request_uri`) targets the *same* host the request came from, there's no `--whitelist-domain` or `--cookie-domain` configuration needed on the oauth2-proxy side either — see [Deploy oauth2-proxy](#deploy-oauth2-proxy) below for the rest of its config, which is identical for both options.

Service names above assume a release named `michelangelo`; adjust to `<release-name>-envoy` / `<release-name>-ui` for yours (see `michelangelo.componentFullname` in the chart's `_helpers.tpl`).

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

Because the UI sends a non-simple `Content-Type` and custom headers (`Rpc-Caller`, etc.), every cross-origin call triggers a browser CORS **preflight** — an `OPTIONS` request the browser sends before the real one, which **never carries cookies**. NGINX Ingress's `auth_request` runs on every method including `OPTIONS`, so the preflight itself gets challenged by oauth2-proxy, fails (no cookie), and the real request is never sent — independent of whether the CORS and `credentials` patches in [Required changes](#required-changes-for-option-b) are applied.

This guide does not have a verified fix for this interaction. The most likely candidate is oauth2-proxy's `--skip-auth-preflight` flag, which is documented to skip authentication for `OPTIONS` requests — but `auth_request` subrequests are issued as `GET` regardless of the original method, so it is not confirmed here whether oauth2-proxy can tell the two apart in that mode. **Verify this in your own cluster before relying on it**; if it doesn't resolve the issue, [Option A](#option-a-put-envoy-on-the-uis-own-origin-recommended) sidesteps the problem entirely, since same-origin requests never trigger a CORS preflight.

### Required changes for Option B

Verified against the current chart and UI source — as of this writing, neither of these exists, so even once the preflight problem above is solved, credentials still won't flow without both patches applied together (a deployment with only one of them fails every call: either the cookie never gets sent, or it gets sent but the browser discards the response).

**1. Envoy's CORS policy needs explicit, opt-in credential support.** `helm/michelangelo/templates/core/envoy-configmap.yaml`'s virtual-host-level `cors:` block sets `allow_origin_string_match`, `allow_methods`, `allow_headers`, `max_age`, and `expose_headers`, but never `allow_credentials`. Add a Helm value for it (`envoy.corsAllowCredentials`, default `false`) and a corresponding chart validation: **never enable it without an explicit, non-wildcard `corsOrigins`.** Envoy's CORS filter doesn't send a literal `Access-Control-Allow-Origin: *` — when `allow_origin_string_match` matches, it echoes back the caller's actual `Origin` header. Since the chart's default `corsOrigins: ""` renders as the regex `.*`, which matches every origin, pairing that default with `allow_credentials: true` would let **any** origin make authenticated, cookie-bearing calls — the opposite of what the setting is supposed to restrict. There's no `*`-literal special case here to rely on; enforce it with a `fail`.

**2. The UI's fetch transport needs an opt-in way to send cookies cross-origin.** `javascript/packages/rpc/create-fetch-transport.ts` calls `fetch()` with no `credentials` option, which defaults to `'same-origin'` — cookies are attached only when the request target is the same origin as the page. `FetchTransportOptions` (`javascript/packages/rpc/types.ts`) needs a `credentials` field threaded through to the `fetch()` call, left unset by default so nothing changes for deployments that don't need it.

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

ingress:
  enabled: true
  className: nginx
  hosts:
    - auth.your-domain.com                              # its own small Ingress; not app/envoy's hostnames
```

`set-authorization-header` is what makes oauth2-proxy inject `Authorization: Bearer <id_token>` on the response the Ingress controller's `auth_request` subrequest reads — that header is what the Ingress annotation below copies onto the real request. (`pass-authorization-header` is a separate flag for oauth2-proxy's *reverse-proxy* mode, where it forwards the token to a real upstream; it does nothing here, since `upstream: static://202` is never actually proxied to.) The `aud` claim in that `id_token` is the OIDC client's `client_id` — `apiserver.auth.oidc.audiences` must include your oauth2-proxy client's `clientID`, or the API server will reject every token on arrival with `token verification failed`.

**Token lifetime**: oauth2-proxy's session cookie defaults to a 168-hour lifetime, far longer than a typical ~1-hour OIDC `id_token`. Without `cookie-refresh` set *below* that token lifetime (and a refresh token — most IdPs need the `offline_access` scope requested for one to be issued), the UI will look logged in while every API call fails with `token verification failed` about an hour after login, which is confusing to diagnose. If your IdP doesn't issue refresh tokens, you'll need to re-authenticate at that same interval instead.

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

- **`cookie-domain` widens your CSRF trust boundary.** Under Option B, setting it to a parent domain (`.your-domain.com`) makes every subdomain of it "same-site" for the purposes of `SameSite=Lax` — a compromised or user-content subdomain elsewhere on that parent domain could send credentialed requests to Envoy, and read the responses too if `corsOrigins` matches it. Prefer a dedicated parent domain used for nothing else, and never widen `cookie-samesite` to `none` without separately addressing this. Option A has no equivalent exposure, since there's no cross-site cookie at all.
- **Escape literal dots in `corsOrigins`.** As `helm/michelangelo/values.yaml` already notes for the field generally, an unescaped `https://app.your-domain.com` also matches `https://app-your-domain.com` as a regex. This matters more once credentials are involved.

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
| Every API call fails, browser console shows a CORS error | Check the Network tab first: a `401`/`302` from the Ingress (auth failure, or the preflight problem under Option B) can look identical to a CORS error. If the response really is `200` with missing CORS headers: `allow_credentials` missing from the CORS policy, or the UI's `fetch()` isn't sending `credentials: 'include'` — both patches in [Required changes](#required-changes-for-option-b) must be applied together |
| `auth_request` returns 200 but the API call still gets `missing authorization header` | Either `auth-response-headers: "Authorization"` is missing from the protected Ingress, or oauth2-proxy is missing `set-authorization-header: "true"` — either way, no header made it onto the real request |
| API call reaches Envoy with a token, apiserver rejects it with `token verification failed` | Most likely an expired `id_token` — see the token-lifetime note under [Deploy oauth2-proxy](#deploy-oauth2-proxy) — or an `issuerUrl`/`audiences` mismatch, see [above](#keep-apiserverauthoidc-in-sync-option-b) |
| Login fails outright with a CSRF-cookie error | `cookie-samesite: strict` blocks the CSRF cookie on the cross-site navigation back from the IdP — use `lax` |
| Post-login redirect lands on the auth host's blank page instead of the app | Option B only: `whitelist-domain` is missing or doesn't cover the host in `rd=` |

## What's Next

- **Core authentication concepts**: [Authentication & Identity](./authentication.md) — OIDC configuration, `k8s-rbac` mode, RBAC grants
- **Ingress and CORS fundamentals**: [Network & Ingress](./network.md)
- **Embedding the UI elsewhere**: [Michelangelo AI React Library](../ui/michelangelo-react-library.md) — the same cross-origin considerations apply if you embed `@michelangelo-ai/core` behind your own auth proxy
