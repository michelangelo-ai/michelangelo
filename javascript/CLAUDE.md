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
- `__tests__/` lives next to the code it tests

### Code quality

- Follow existing patterns first — look before you invent
- Minimum complexity for the current task — don't design for hypothetical requirements
- Don't abstract speculatively — three similar lines beat a forced abstraction
- Primary export first in each file; helpers follow

### React

- Start with `useStyletron`; extract to `styled()` at 4+ CSS properties or when used in 2+ places
- Styled component names are semantic — never `Container`, `Card`, `Wrapper`
- Handler names express intent: `toggleMenu` not `handleOnClick`

### Testing

- Query priority: `getByRole` → `getByLabelText` → `getByText` → `getByTestId`
- `getByTestId` only for elements with no semantic role, or mock components — everything else is a bug
- Mock external APIs and RPC calls; never mock internal hooks, React context, or well-tested utilities

### Documentation

- Comments explain _why_ — prefer renaming identifiers if code needs clarification
- Skip JSDoc for simple functions whose TypeScript signature is self-explanatory
