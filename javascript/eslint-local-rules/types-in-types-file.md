Flags type and interface declarations outside of `types.ts` files.

## Why this rule exists

Scattering types across component files makes them hard to discover. Centralizing types into dedicated `types.ts` files keeps the type surface of a module predictable and browseable.

## The Props exception

Interfaces and types whose name ends in `Props` are exempt when they are used as a function parameter type annotation or `forwardRef` generic argument in the same file. Component props are tightly coupled to the component they describe — they serve as the component's API documentation and rarely benefit from being shared.

```typescript
// Allowed: FooProps is used as a parameter type in the same file
interface FooProps {
  label: string;
  onClick: () => void;
}

function Foo({ label, onClick }: FooProps) {
  return <button onClick={onClick}>{label}</button>;
}
```

## How to fix violations

### Move to types.ts

Most types belong in a co-located `types.ts` and should be imported from there:

```typescript
// component.tsx — BEFORE (violation)
export interface Column {
  key: string;
  label: string;
  sortable: boolean;
}

// types.ts — AFTER
export interface Column {
  key: string;
  label: string;
  sortable: boolean;
}

// component.tsx
import type { Column } from './types';
```

### Inline at the call site

Small, single-use types (one or two members/union branches, referenced once) can often be inlined at the call site. If the type name adds semantic clarity, keeping it named is fine — move it to `types.ts` instead. The rule suggests inlining when it detects a small, single-use type:

```typescript
// component.tsx — BEFORE (violation, but type is trivial and used once)
type Direction = 'asc' | 'desc';

function sortRows(rows: Row[], dir: Direction) {
  /* ... */
}

// component.tsx — AFTER (inlined)
function sortRows(rows: Row[], dir: 'asc' | 'desc') {
  /* ... */
}
```
