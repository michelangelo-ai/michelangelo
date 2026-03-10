---
name: File and Directory structuring
description: "Trigger when implementing any new feature, hook, or component — decisions about where to put new files and how to import them must follow these rules. Also trigger when adding imports or spotting barrel exports (index.ts), which are banned in this codebase."
user-invocable: false
---

# File & Import Organization

## Import Strategy

- **Direct imports only** — no `index.ts` barrel exports; import directly from source files
- **Co-locate related code** — types, context, hooks, tests, and utils live alongside the component they belong to
- **Flat over nested** — prefer flat structures when file count is manageable (~10 files)

## Directory Structure

- `components/` — feature building blocks
- `__tests__/` — place in the closest directory to the tested code
- `constants.ts`, `types.ts` — single files at the feature root
- `styled-components.ts` — reusable styled components for the feature

## Code Placement Strategy

1. **Start local** — put code in `components/my-component/utils/`
2. **Promote when needed** — move to a shared `utils/` when multiple parts need it
3. **Namespace filenames** — `table/components/table-action-button.tsx`, not `action-button.tsx`

## Hooks/Utils

- Single file: `use-hook-name.ts` directly in the component directory
- Multiple files: extract to a `hooks/` or `utils/` subdirectory

## Naming

- **Files**: kebab-case, `.tsx` for JSX, `.ts` otherwise
- **Components**: PascalCase in code, kebab-case filename
- **Variables**: camelCase
- **Constants**: UPPER_SNAKE_CASE

## Anti-Patterns

- ❌ `index.ts` barrel exports — import directly from source
- ❌ Generic filenames without namespace (`action-button.tsx` vs `table-action-button.tsx`)
