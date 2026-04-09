# types-in-types-file

## Problem

When type and interface declarations are scattered across component files, they become hard to discover, grep for, and reuse. A developer looking for the shape of a data structure shouldn't have to guess which component file defines it. Centralizing types into dedicated `types.ts` files keeps the type surface of a module predictable and browsable.

## Decision

All `type` and `interface` declarations must live in a `types.ts` file (or a file matching `*-types.ts`, or inside a `types/` directory). The rule reports any declaration found outside these locations.

When a violation is reported, decide how to fix it:

- **Small, single-use types** (one or two members/union branches, referenced once) are often better **inlined** at the call site rather than extracted into `types.ts`. A named type that exists only to annotate one parameter adds indirection without aiding discoverability.
- **Everything else** should be **moved** to a co-located `types.ts` and imported from there.

The rule detects this automatically: small, single-use types get the inline suggestion; everything else gets the move-to-types.ts suggestion.

## The Props exception

Interfaces and types whose name ends in `Props` are exempt when they are used as a function parameter type annotation or `forwardRef` generic argument in the same file. Component props are tightly coupled to the component they describe — they serve as the component's API documentation and almost never benefit from being shared.

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

## Examples

### Violation: move to types.ts

```typescript
// component.tsx — BEFORE (violation)
export interface Column {
  key: string;
  label: string;
  sortable: boolean;
}
```

```typescript
// types.ts — AFTER
export interface Column {
  key: string;
  label: string;
  sortable: boolean;
}

// component.tsx
import type { Column } from './types';
```

### Violation: inline at call site

```typescript
// component.tsx — BEFORE (violation, but type is trivial and used once)
type Direction = 'asc' | 'desc';

function sortRows(rows: Row[], dir: Direction) {
  /* ... */
}
```

```typescript
// component.tsx — AFTER (inlined)
function sortRows(rows: Row[], dir: 'asc' | 'desc') {
  /* ... */
}
```

## Escape hatch

For genuine exceptions, disable per-line:

```typescript
// eslint-disable-next-line @eslint-local-rules/types-in-types-file
type InternalState = {
  /* ... */
};
```
