# Import Zones in `packages/core`

## The Principle

`packages/core` is structured as a layered dependency graph. Each layer may only import from layers below it — never from layers above. We call these **import zones**.

```
types/          ← foundation: no imports from other core layers
utils/          ← pure functions: imports types/ only
providers/      ← React context: imports types/, utils/
hooks/          ← React hooks: imports types/, utils/, providers/
interpolation/  ← dynamic resolution: imports types/, utils/, providers/, hooks/
components/     ← React components: imports all layers below
router/         ← top-level assembly: imports all layers
```

The rule is simple: arrows only point downward.

## Why It Matters

**Not because violations have caused bugs.** As of this writing, the codebase has two upward imports and both are harmless — the project builds cleanly, all tests pass, TypeScript is happy. This is not a "we got burned" story.

The reason to enforce this is different: **we are about to add an ESLint rule, and we cannot enforce a rule we already violate.** A linter that flags future violations while silently allowing existing ones gives engineers the wrong signal about what's acceptable.

There is also a forward-looking concern. `config/` — the layer that assembles `PhaseConfig`, `PhaseEntityConfig`, and `ViewConfig` into a studio definition — is intended to move out of `packages/core` into consumer applications (see `app/App.tsx`). When that happens, consumer code needs to import the config vocabulary from `@michelangelo-ai/core`. That vocabulary needs to live in `types/` — the foundation — not inside `components/`, which is an implementation detail of the rendering layer.

## Current State

The codebase has **organically converged on import zone purity for most layers**. This happened without any stated rule:

| Layer | Imports from `components/`? | Status |
|---|---|---|
| `types/` | Yes — 1 file | ⚠ violates the rule |
| `utils/` | No | ✓ naturally pure |
| `hooks/` | No | ✓ naturally pure |
| `providers/` | Yes — 1 file | ⚠ violates the rule |
| `interpolation/` | Yes | ✓ permitted by zone ordering |
| `components/` | N/A (it is the top) | — |

The two violations:

1. **`types/common/studio-types.ts`** imports `ViewConfig` from `#core/components/views/types`. This was a deliberate pragmatic choice (introduced August 2025) when `PhaseEntityConfig` was extracted into `types/`. The developer needed `ViewConfig` and took the shortest path.

2. **`providers/cell-provider/types.ts`** imports `CellRenderer` from `#core/components/cell/types`. Same pattern — a provider type that references a component type.

## The Fix

The violations exist because certain types are colocated with their component implementations when they should be in `types/`. Specifically, these types describe the **schema for configuring rendering** — they are the vocabulary a developer writes when defining a studio, not implementation details of how rendering works:

- `ViewConfig`, `ListViewConfig`, `DetailViewConfig` — what views an entity has
- `TableConfig` — how a list view's table is configured
- `ColumnConfig` — how each column in a table is defined
- `Cell` — what a single cell displays

These move from `components/*/types.ts` into a new file: **`types/config-types.ts`**.

Types that stay colocated are the ones no config author ever writes — rendering internals like `FilteringCapability`, `ColumnRenderState`, `CellStyleFunction`, `MainViewContainerProps`.

After the move, `studio-types.ts` imports `ViewConfig` from a sibling in `types/` instead of reaching up into `components/`. The violation is gone, and the ESLint rule becomes enforceable.

## What We Are Not Doing

**Not splitting `types/` into subdirectories.** The current `types/common/` subdirectory has no coherent organizing principle — `TimeZone` at the root is equally "common" as `PhaseConfig` inside `common/`. We are flattening `types/common/` into `types/` and naming files by what they describe (`time-types.ts`, `config-types.ts`), not by how widely they're used.

**Not doing a large-scale migration.** Only the types in the `PhaseEntityConfig → ViewConfig → TableConfig → ColumnConfig → Cell` chain move. Types like `FormState`, `TagColor`, and `ExecutionDetailViewSchema` are evaluated separately — they are not part of the current violation and moving them is not required to enforce the rule.

**Not arguing this is urgent.** The existing violations are stable and contained. The motivation is principle consistency and forward compatibility, not firefighting.

## The ESLint Rule

Once the two violations are fixed, a single `no-restricted-imports` rule on each lower layer enforces the boundary permanently. Any future upward import is a lint error at the point of authorship — before it reaches review or CI.

The rule makes the implicit convention that `utils/` and `hooks/` have already been following explicit and machine-checked across all layers.
