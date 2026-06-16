# filename-matches-export

## What this rule enforces

A component or hook file's primary named export must match the filename stem. The filename is the canonical identifier in a component library — it's what autocomplete, "go to file", and grep resolve first.

- `.tsx` component files: filename stem in **PascalCase** (`button-group.tsx` → `ButtonGroup`)
- `use-*.ts` and `use-*.tsx` hook files: filename stem in **camelCase** (`use-studio-mutation.ts` → `useStudioMutation`)

## Flagged patterns

```tsx
// ❌ button-group.tsx — expected export: ButtonGroup
export function BtnGroup() { ... }

// ❌ provider.tsx — expected export: Provider
export function ThemeProvider() { ... }  // rename file to theme-provider.tsx

// ❌ use-scroll.ts — expected export: useScroll
export function useScrollRatio() {}  // rename file to use-scroll-ratio.ts

// ❌ use-url-query-string.ts — expected export: useUrlQueryString (acronyms lowercased)
export function useURLQueryString() {}
```

## Correct patterns

```tsx
// ✓ button-group.tsx
export function ButtonGroup() { ... }

// ✓ theme-provider.tsx
export function ThemeProvider() { ... }

// ✓ use-scroll-ratio.ts
export function useScrollRatio() {}

// ✓ use-url-query-string.ts
export function useUrlQueryString() {}
```

## Acronym convention

Hook names use **mechanical camelCase** — every word after `use` has its first letter capitalized, including acronyms:

- `use-url-query-string.ts` → `useUrlQueryString` (not `useURLQueryString`)
- `use-html-parser.ts` → `useHtmlParser` (not `useHTMLParser`)

This eliminates ambiguous word boundaries in the middle of names.

## Exemptions (auto-detected)

| Case                        | Example                              | Why exempt                          |
| --------------------------- | ------------------------------------ | ----------------------------------- |
| `index.tsx`                 | entry points                         | intentionally multi-export          |
| Files with `styled` in name | `styled-components.tsx`              | multi-component styled collections  |
| Only lowercase exports      | `helpers.tsx` exporting `formatDate` | utility files, not components       |
| Only `ALL_CAPS` exports     | `icons.tsx` exporting `ICONS`        | constant maps                       |
| Type-only exports           | `export type { Foo }`                | `exportKind === 'type'` not counted |
| Non-hook `.ts` files        | `string-utils.ts`                    | only `use-*.ts` files are checked   |

## File vs export — which to rename?

If the filename and export diverge, rename whichever is wrong:

- Export name adds meaningful context the filename lacks → rename the **file** (e.g. `provider.tsx` → `theme-provider.tsx`, `use-scroll.ts` → `use-scroll-ratio.ts`)
- Export name is an abbreviation or uses non-standard casing → rename the **export** to match the file
