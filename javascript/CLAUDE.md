# JavaScript Codebase

Architecture: see `packages/core/ARCHITECTURE.md`.

## Commands

All commands run from `javascript/`. See `package.json` scripts for the full list.

### CICD checks

Can be run in parallel. `yarn test` only outputs failing tests, do not add additional CLI flags.

```
yarn lint --quiet
yarn typecheck
yarn format --write
yarn test
```

## Rules

### Structure

- Kebab-case filenames, namespaced: `table-action-button.tsx` not `action-button.tsx`
- Co-locate related code alongside the component it belongs to; stay flat until you have multiple files of the same type (several hooks → `hooks/`, several utils → `utils/`)
- Start local (`components/my-component/utils/`); promote to a shared `utils/` only once multiple features need it
- `__tests__/` lives next to the code it tests

### Code quality

- Primary export first in each file; helpers follow

### TypeScript

- Create focused types mapped from generated types — include only the properties you need, not the full generated shape

### React

- Start with `useStyletron`; extract to `styled()` at 4+ CSS properties or when used in 2+ places
- Styled component names are semantic — never `Container`, `Card`, `Wrapper`

### Testing

- Query priority: `getByRole` → `getByLabelText` → `getByText`
- Mock external APIs and RPC calls; never mock internal hooks, React context, or well-tested utilities
- Prefer integration-style tests over isolated component unit tests — e.g. table behavior is tested through `table.test.tsx` rather than scattered per-sub-component tests; reserve isolated unit tests for pure utils/hooks with standalone logic
- See `.claude/calibration-notes.md` for lessons from past coverage that missed the mark

### Documentation

- No AI/agent references in documentation
