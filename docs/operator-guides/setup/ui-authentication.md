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

The UI deployment carries a checksum of its `config.json` ConfigMap, so changing `ui.apiCredentials` or `ui.apiBaseUrl` with `helm upgrade` restarts the UI pods.

### Writing the `corsOrigins` regex

Envoy matches `envoy.corsOrigins` against the whole `Origin` header, so you don't need `^` or `$`. Keep the regex as narrow as your UI origins:

- Escape every literal dot. In YAML double quotes, write `"https://ml\\.example\\.com"`. An unescaped dot matches any character, so `https://ml.example.com` also matches the look-alike domain `https://ml-example.com`, which anyone can register.
- Don't put `.*` in the host part. `https://.*example\.com` also matches `https://evilexample.com`.
- To allow several origins, list them as alternatives: `"https://(ml|staging)\\.example\\.com"`.

## Chart validations

The chart refuses to render these combinations:

- `envoy.corsAllowCredentials` is not a boolean. `--set-string envoy.corsAllowCredentials=false` produces the string `"false"`, which Helm treats as true. Use `--set envoy.corsAllowCredentials=true|false`.
- `envoy.corsAllowCredentials=true` with `envoy.corsOrigins` unset, not a string, not a valid regex, or set to a regex that matches every origin, such as `".*"`, `"^.*$"`, `".+"`, `"(.*)"`, or `"https?://.*"`. Envoy reflects the caller's `Origin` back on a match, so a wildcard plus credentials would let any site make authenticated calls. The chart detects a match-everything regex by testing it against the `null` origin and a placeholder host. It can't detect look-alike patterns such as unescaped dots, so follow the rules in [Writing the `corsOrigins` regex](#writing-the-corsorigins-regex).
- `ui.apiCredentials` set to anything other than empty, `"include"`, `"same-origin"`, or `"omit"`. Non-string values such as `false` or `0` are rejected too, not treated as unset.
- `ui.apiCredentials=include` without `envoy.corsAllowCredentials=true`. The browser would send the cookie but discard every response.

## Auth proxy cookie

The proxy's session cookie must also be sent to Envoy's host:

- Scope the cookie to a domain that covers both hosts (with `oauth2-proxy`, `--cookie-domain=.ml.example.com`).
- If the UI and Envoy are on different sites, not just different subdomains, the cookie needs `SameSite=None` and `Secure` (with `oauth2-proxy`, `--cookie-samesite=none` and `--cookie-secure=true`). Use `SameSite=None` only in that case.

To verify, open the browser's developer tools on the UI. API requests to Envoy should carry the `Cookie` header, and responses should include `Access-Control-Allow-Credentials: true` and an `Access-Control-Allow-Origin` that matches the UI origin.

## CORS does not block requests

CORS controls which pages can read responses. It does not stop requests from reaching the server. A browser sends a "simple" cross-site request, such as a form POST or a `Content-Type: text/plain` POST, with cookies and without a preflight. Envoy forwards it to the API server even when the origin doesn't match `envoy.corsOrigins`. The server must therefore authenticate and validate every request on its own. The API server is expected to reject these requests because they can't carry the `rpc-caller` header that every API call requires. Don't rely on CORS alone to block them.
