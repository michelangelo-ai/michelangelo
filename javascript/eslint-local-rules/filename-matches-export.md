# filename-matches-export

## What this rule enforces

A `.tsx` component file's primary named export must match the filename stem in PascalCase. The filename is the canonical identifier in a component library — it's what autocomplete, "go to file", and grep resolve first.

## Flagged patterns

```tsx
// ❌ button-group.tsx — expected export: ButtonGroup
export function BtnGroup() { ... }

// ❌ form-control.tsx — expected export: FormControl
export const FormCtrl = () => ...;

// ❌ provider.tsx — expected export: Provider
export function ThemeProvider() { ... }  // rename file to theme-provider.tsx
```

## Correct patterns

```tsx
// ✓ button-group.tsx
export function ButtonGroup() { ... }

// ✓ form-control.tsx
export const FormControl = () => ...;

// ✓ theme-provider.tsx
export function ThemeProvider() { ... }
```

## Exemptions (auto-detected)

| Case                        | Example                              | Why exempt                          |
| --------------------------- | ------------------------------------ | ----------------------------------- |
| `index.tsx`                 | entry points                         | intentionally multi-export          |
| Files with `styled` in name | `styled-components.tsx`              | multi-component styled collections  |
| Only lowercase exports      | `helpers.tsx` exporting `formatDate` | utility files, not components       |
| Only `ALL_CAPS` exports     | `icons.tsx` exporting `ICONS`        | constant maps                       |
| Type-only exports           | `export type { Foo }`                | `exportKind === 'type'` not counted |

**Note on `.ts` files**: this rule only applies to `.tsx` files. Type definition files (`types.ts`) are `.ts` and exempt by default. Hook files (`use-*.ts`) are similarly `.ts` and exempt — the `types` stem exclusion was removed as redundant.

**Future**: hook files that are `.tsx` (e.g. `use-studio-mutation.tsx` → `useStudioMutation`) are not yet covered. Camelcase stem conversion is tracked for a follow-up.

## File vs export — which to rename?

If the filename and export diverge, rename whichever is wrong:

- Export name adds meaningful context the filename lacks → rename the **file** (e.g. `provider.tsx` → `theme-provider.tsx`)
- Export name is an abbreviation or shorthand → rename the **export** to match the file
