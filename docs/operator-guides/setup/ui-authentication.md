---
last_verified: 2026-11-18
---

# UI Authentication Behind an Auth Proxy

This guide is for operators who put an authenticating proxy (for example `oauth2-proxy`) in front of the Michelangelo AI UI and Envoy, where the proxy identifies users with a session cookie.

**Prerequisites**: A working Ingress setup for the UI and Envoy (see [Network & Ingress](./network.md)). For API-server-side authentication and authorization, see [Authentication & Identity](./authentication.md).

## Do you need these settings?

The browser loads the UI from one URL and calls Envoy at `ui.apiBaseUrl`. Whether the proxy's session cookie reaches Envoy depends on whether those two URLs share an origin (scheme, host, and port):

- **Same origin** (for example UI at `https://ml.example.com/` and Envoy at `https://ml.example.com/api`): the browser sends the cookie by default. You do not need any of the settings below.
- **Different origins** (for example UI at `https://ml.example.com` and Envoy at `https://api.ml.example.com`): the browser sends the cookie only if the UI asks for it and Envoy's CORS response allows it. Configure all three values below.

## Helm values

| Helm value | Type | Default | Description |
|---|---|---|---|
| `envoy.corsOrigins` | string (RE2 regex) | `""` (treated as `.*`) | Origins Envoy accepts. Must name your UI origin explicitly when credentials are enabled. |
| `envoy.corsAllowCredentials` | boolean | `false` | When `true`, Envoy's CORS policy renders `allow_credentials: true`, so the browser accepts responses to credentialed cross-origin requests. |
| `ui.apiCredentials` | string | `""` | The `fetch()` `credentials` mode the UI uses for every API call. One of `"include"`, `"same-origin"`, or `"omit"`. Empty keeps the browser default (`"same-origin"`) and leaves `config.json` unchanged. |

Example for a UI and Envoy on different hosts:

```yaml
envoy:
  corsOrigins: "https://ml\\.example\\.com"
  corsAllowCredentials: true

ui:
  apiBaseUrl: "https://api.ml.example.com"
  apiCredentials: "include"
```

The chart renders `"apiCredentials": "include"` into the UI's `config.json`, and adds `allow_credentials: true` to Envoy's CORS policy.

## Chart validations

The chart refuses to render these combinations:

- `envoy.corsAllowCredentials` is not a boolean. `--set-string envoy.corsAllowCredentials=false` produces the string `"false"`, which Helm treats as true. Use `--set envoy.corsAllowCredentials=true|false`.
- `envoy.corsAllowCredentials=true` with `envoy.corsOrigins` unset or set to `".*"`. Envoy reflects the caller's `Origin` back on a match, so a wildcard plus credentials would let any site make authenticated calls.
- `ui.apiCredentials` set to anything other than empty, `"include"`, `"same-origin"`, or `"omit"`. Non-string values such as `false` or `0` are rejected too, not treated as unset.
- `ui.apiCredentials=include` without `envoy.corsAllowCredentials=true`. The browser would send the cookie but discard every response.

## Auth proxy cookie

The proxy's session cookie must also be sent to Envoy's host:

- Scope the cookie to a domain that covers both hosts (with `oauth2-proxy`, `--cookie-domain=.ml.example.com`).
- If the UI and Envoy are on different sites, not just different subdomains, the cookie needs `SameSite=None` and `Secure` (with `oauth2-proxy`, `--cookie-samesite=none` and `--cookie-secure=true`).

To verify, open the browser's developer tools on the UI. API requests to Envoy should carry the `Cookie` header, and responses should include `Access-Control-Allow-Credentials: true` and an `Access-Control-Allow-Origin` that matches the UI origin.
