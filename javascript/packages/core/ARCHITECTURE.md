# `@michelangelo-ai/core` — Architecture

This is a working summary of the architectural direction for `@michelangelo-ai/core`. It describes where the package is going — not every section reflects current state in full.

For coding conventions, see `CLAUDE.md` in this directory and in `javascript/`.

## 1. Domain model

Studio organizes around three concepts. Most of this document — and most configuration — is expressed in terms of them.

- **Project** — the top-level workspace; everything else is typically scoped to one. Maps to `:projectId` in the URL and to `namespace` in API requests.
- **Phase** — a stage in the ML lifecycle (e.g. `data`, `train`, `deploy`). Phases group related entity types.
- **Entity** — a kind of resource within a phase (e.g. a `pipeline` or `deployment`). Most entities have list, detail, and form views, and a set of actions attached to them.

When this document refers to "an entity," it means one of these resources. When it refers to "a configuration," it usually means a description of an entity's shape — its views, its actions, its forms.

## 2. Components and configuration

`@michelangelo-ai/core` provides two API surfaces:

- **Components** — standard React primitives. Props, composition, TypeScript interfaces.
- **Configuration** — declarative orchestrators that compose primitives based on configs.

The two are **complementary**. Configuration is the natural fit for most **entities** — resources whose lifecycle follows a declarative CRUD pattern (list, view one, create, update, act). One runtime renders them all from configs rather than each being implemented by hand. Components handle the rest: bespoke UI, app-wide or cross-project resources that don't fit the project hierarchy, and entities that need enough custom logic that bending them to the engine would be costlier than just writing them.

The configuration engine is deliberately kept lightweight. When a use case doesn't fit, the answer is an escape hatch ([§ Customization escape hatches](#customization-escape-hatches)), not more complexity in the engine.

### Configurations live in consumer packages

`core` provides the runtime that interprets configurations. The configurations themselves — the actual `PhaseConfig` objects, entity definitions, action lists, form schemas — live in the consuming application.

Today the reference configurations live in `javascript/app/`. Consumers building their own apps on top of `@michelangelo-ai/core` provide their own. `core` does not ship a default set of entities; it ships the machinery to render them.

## 3. Configuration system

A configuration is a declarative description of a view, action, or form, expressed as a TypeScript object. The `core` runtime walks the configuration and renders the right components. Configurations nest:

- A **`PhaseConfig`** describes a phase (e.g. _train_, _deploy_) and lists the entities it contains.
- An **`EntityConfig`** describes an entity (e.g. _pipeline_, _deployment_) and lists its views and actions.
- A **`ViewConfig`** describes a view of an entity — `list`, `detail`, or `form` — and specifies the columns, tabs, or fields it renders.
- An **`ActionConfig`** describes an action (button, menu item) attached to an entity. It composes _what_ the action does (call a mutation or navigate to a route) with _how_ the user invokes it (immediately, with a confirmation step, or via a custom React component for bespoke flows).
- A **`FormConfig`** describes a form — fields, layout, validation, submission — for create and update flows.
- A **`TableConfig`** describes a table view's columns, sorting, filtering, and action wiring.

### Composition

Configurations are not a flat catalog. They compose:

- **`QueryConfig` and `MutationConfig`** are parallel descriptors for backend calls — `QueryConfig` for reads (consumed by `useStudioQuery`), `MutationConfig` for writes (consumed by `useStudioMutation`). Anything that loads or mutates data references one of these: a `TableConfig`'s row source uses a `QueryConfig`; an `ActionConfig`'s mutation and a `FormConfig`'s submission use a `MutationConfig`.
- **`FormConfig` embeds inside other configs.** An `ActionConfig` can embed a `FormConfig` to render a form when the action is triggered (e.g. "Create Pipeline"); a `ViewConfig` for a form view wraps it directly; a detail view can embed one as a tab.

### Customization escape hatches

The configuration runtime is the path of least resistance. Most customization happens by adjusting configuration — registering custom column renderers, choosing different field types, supplying disabled rules. Specific escape hatches let consumers ship React components when an interaction doesn't fit configuration:

- **Detail view tabs** — a tab can render a custom React component while the standard page chrome and tab navigation stay in place.
- **Forms** — when an interaction is bespoke, swap the entire form for a custom React component.
- **Actions** — mount a custom React component when an action's flow doesn't fit the declarative mutation-or-navigate-with-optional-confirmation patterns.

### Interpolation

Configurations are static data by default. Interpolation makes them dynamic: a config field can be a literal value or an interpolation expression that resolves at render time, with access to the current row, the page entity, the URL state, and any extensions a consumer has registered.

```ts
const action: DeepInterpolatable<ActionConfig> = {
  display: { label: 'Resume', icon: 'play' },
  hierarchy: interpolate(({ data }) => (data.state === 'PAUSED' ? 'primary' : 'tertiary')),
  disabled: interpolate('${data.state == "TERMINATED"}'),
};
```

The resolver runs per-row: in a list view, the same action config sees a different `data` for each row, so `hierarchy` and `disabled` evaluate independently per row. This is what lets one declarative config drive behavior that previously required a custom React component per row.

Two interpolation flavors are supported:

- **String** — `${path.to.value}` template strings. Best for simple field references.
- **Function** — `({ data, studio, ... }) => value`. Best for computed values or branching.

The interpolation engine is pure — it knows nothing about ML or any specific domain. All context is injected through providers, keeping the engine reusable and the domain knowledge with the consumer. The context commonly available to expressions:

- **`data`** — the current row (list) or page entity (detail/form), with `row ?? page` as the convenience reference.
- **`studio`** — URL params from `useStudioParams` (`projectId`, `phase`, `entity`, …).
- **`repeatedLayoutContext`** — index and root field path inside repeated form sections (`<RepeatedLayoutProvider>`).
- Consumer extensions registered via `<InterpolationProvider value={…}>` and typed via `InterpolationContextExtensions` (see [§ Customization via providers](#4-customization-via-providers)).

`useInterpolationResolver` resolves a config object on demand, walking the structure recursively so any leaf can be interpolatable. Higher-level wrappers like `InterpolatableActionsPopover` apply it per-row before delegating to the rendering component, so most call sites don't touch the resolver directly.

## 4. Customization via providers

`core` is framework-agnostic by design. Integrations and consumer-specific data — RPC clients, registries, contextual values — are injected at the application boundary through React context providers, never imported directly inside `core`. Consumers wrap the app in a provider, pass implementations or values, and components inside consume them through hooks.

This is what lets the same `core` package serve different consumers without modification — one might plug in a ConnectRPC client, another a REST adapter — and core's components are unchanged.

```tsx
import { CoreApp } from '@michelangelo-ai/core';
import { request } from '@michelangelo-ai/rpc';

<CoreApp dependencies={{ service: { request } }} />;
```

Inside `core`, components consume injected implementations through hooks (e.g. `useServiceProvider`) and never reach for an SDK directly. `useStudioMutation`, for example, is a thin wrapper around `useServiceProvider().request`.

### Type-safe extension via declaration merging

Any provider in `core` can expose an interface that consumers augment via TypeScript declaration merging. This keeps the runtime API flexible while preserving full type inference.

The pattern: core declares an interface; consumers add members to it from their own code; provider props and consuming code pick up the augmented type automatically.

```ts
declare module '@michelangelo-ai/core' {
  interface InterpolationContextExtensions {
    user: { uuid: string; email: string };
    project: { name: string; namespace: string };
  }
}

// In the app
<InterpolationProvider value={{ user, project }}>
```

After this augmentation, interpolated configs reference `user` and `project` with full type inference. Removing the augmentation surfaces type errors at every consumer site.

## 5. Styling

`core` uses Styletron and BaseUI as its styling stack. Both are injected at the application boundary (`StyletronProvider`, `ThemeProvider`). Consumers customize the visual layer through theme tokens.

Components in `core` come in two styling shapes:

- **Feature components** (e.g. `Table`) take standard React props and own their internal styling — `useStyletron()` for inline styles, a `StyledTable` extracted into `styled-components.ts` when reuse warrants.
- **Reusable styled primitives** (e.g. `Box`) follow the BaseUI overrides pattern: each exposes named slots via `getOverrides`, defaults each slot to its own styled component, and lets consumers replace any slot. `Box` exposes `BoxContainer`, `BoxHeader`, and `BoxTitle`.

For specific styling conventions — when to use `useStyletron()` inline, when to extract a `styled-components.ts`, theme-token usage — see the project's CLAUDE files and related skills.

## 6. Routing

Studio's URL structure mirrors the domain model from [§ 1](#1-domain-model):

```
/                                              project list
/:projectId                                    project detail
/:projectId/:phase/:entity?                    list view — entities of a kind within a phase
/:projectId/:phase/create/:entity              create form view
/:projectId/:phase/:entity/:entityId           detail view — one entity, optional tab
/:projectId/:phase/:entity/:entityId/update    update form view
```

Views read URL state through `useStudioParams`, which is typed by view kind: `useStudioParams('list')` returns `{ projectId, phase, entity }`; `useStudioParams('detail')` adds `entityId` and `entityTab`; `useStudioParams('form')` exposes form-specific fields. Components and configuration both consume this hook — there is no parallel routing system inside `core`.

## 7. Testing

Tests render against real providers, not mocks. The `test/wrappers/` directory exposes per-provider wrapper helpers (`getServiceProviderWrapper`, `getRouterWrapper`, `getInterpolationProviderWrapper`, …) that wrap the production providers; `buildWrapper([...])` composes the ones a test needs.

A test exercises the real integration path — the actual `<ServiceProvider>`, the actual query client, the actual `useStudioMutation` — and swaps out only the boundary it needs to control. For RPC, that's the `request` function: tests pass a mock or use `createQueryMockRouter` to route by query name and args.

```tsx
const request = createQueryMockRouter({
  GetPipelineRun: { pipelineRun: { name: 'test' } },
  ListPipelineRun: { pipelineRunList: { items: [] } },
});

render(<MyComponent />, buildWrapper([getServiceProviderWrapper({ request }), getRouterWrapper()]));
```

Avoid mocking core's hooks (e.g. `vi.mock('#core/hooks/use-studio-mutation', …)`). Mocking the hook bypasses the real query client, the real provider wiring, and the real error normalization — the exact things an integration test should exercise. Mock at the boundary, not the abstraction.

## 8. Package layout

| Package                 | Purpose                                                   |
| ----------------------- | --------------------------------------------------------- |
| `@michelangelo-ai/core` | UI rendering engine: components and configuration runtime |
| `@michelangelo-ai/rpc`  | Optional ConnectRPC/gRPC-Web client                       |

`core` does not depend on `rpc`. It declares the contracts (e.g. the shape of a `request` function); consumers provide implementations.

## 9. Where to look next

- **Coding conventions** — `CLAUDE.md` in this directory (testing patterns) and `javascript/CLAUDE.md` (package layout, scripts, available skills)
- **Reference application** — `javascript/app/` is a complete integration: it imports from `@michelangelo-ai/core`, wires `@michelangelo-ai/rpc` into `<CoreApp dependencies={...}>`, and supplies a configuration tree
- **Entity config examples** — `javascript/packages/core/config/entities/pipeline/pipeline.ts` shows the shape of an entity configuration. These configs live in core today and will migrate to consumer packages.
- **Test wrappers** — `test/wrappers/build-wrapper.tsx` and the `get*ProviderWrapper` helpers in the same directory show how to render `core` components with the right context for tests
