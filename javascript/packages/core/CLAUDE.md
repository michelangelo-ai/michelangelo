# CLAUDE.md - Core Package Guidelines

## Publishing to npm

### How publishing is triggered

Publishing is handled by `.github/workflows/npm-publish.yml`. It runs on every push to `main` that touches `javascript/packages/core/**`. The workflow checks whether the `version` field in `javascript/packages/core/package.json` changed in that commit. If it did, the package is built and published to npmjs.com under `@michelangelo-ai/core`. If the version is unchanged, the publish step is skipped.

To release a new version: bump the version in `javascript/packages/core/package.json`, commit, and push (or merge) to `main`.

### The `NPM_TOKEN` secret

The workflow authenticates to npm using a repository secret named `NPM_TOKEN`. This must be a **granular access token from the personal npm account of the `@michelangelo-ai` org owner** (craig.marker). A team token will not work — only the org owner's personal token carries publish rights to the org's packages.

**Creating the token:**

1. Log in to npmjs.com as craig.marker.
2. Go to Account → Access Tokens → Generate New Token → Granular Access Token.
3. Grant **read and write** publish access, scoped to the `@michelangelo-ai` organization.
4. Copy the token value.

**Adding the secret to the repository:**

1. In the GitHub repository, go to Settings → Secrets and variables → Actions.
2. Click "New repository secret".
3. Name: `NPM_TOKEN`, Value: the token copied above.
4. Save.

## Testing Guidelines

### Using Established Test Wrappers

**Use `@test/utils/wrappers/build-wrapper.tsx` for component testing**:

- **Examine available wrappers**: Check the file for current wrapper functions
- **buildWrapper()**: Compose multiple wrappers together for complex test scenarios

**Example pattern**:

```typescript
renderHook(
  () => useMyHook(),
  buildWrapper([
    // Add wrappers based on what contexts your component needs
  ])
);
```

#### RPC and External API Mocking

Use `@test/utils/wrappers/get-service-provider-wrapper.tsx` for API mocking
