---
last_verified: 2026-10-26
---

# Action Configuration Reference

Actions are the buttons and menu items that do something to a record: delete a pipeline, retry a run, open an "Update" form. You configure them as `ActionConfigSchema` objects on an entity (`PhaseEntityConfig.actions`) or on a table (`TableConfig.actions`). A separate `CreateActionConfig` (`PhaseEntityConfig.createAction`) adds a page-level "Create …" button.

This page is for contributors adding or changing record actions in Michelangelo AI Studio. It assumes you've read the [Entity Configuration Reference](./entity-configuration-reference.md).

The types on this page are exported from `@michelangelo-ai/core`. The examples use the `#core/...` aliases that code inside `packages/core` uses; from an app, import from `@michelangelo-ai/core`.

## Where Actions Render

| Config                         | Where it renders                                                                                   | Interpolation context   |
| ------------------------------ | -------------------------------------------------------------------------------------------------- | ----------------------- |
| `PhaseEntityConfig.actions`    | Each row of the entity's **list view** table, **and** the **detail view header**                   | `row` in the list, `page` in the header |
| `TableConfig.actions`          | Each row of a **detail page table** (`type: 'table'` pages). Ignored on the list view, see below    | `row`                   |
| `PhaseEntityConfig.createAction` | A button in the list table's action bar (trailing slot), only while that entity's tab is active  | none (no record)        |

:::warning List view: entity `actions` replace `tableConfig.actions`
`PhaseEntityView` builds the list table from `views[0].tableConfig` and then overwrites `actions` with `entity.actions`:

```typescript
tableConfig={{ ...currentEntityConfig.views[0].tableConfig, actions: entity.actions }}
```

So on the list view, any `actions` you put in the list view's `tableConfig` are **always discarded**, even when the entity has no `actions` (the override sets `undefined`). Put list-row actions on the entity. Two more consequences:

- The list view must be `views[0]`.
- `TableConfig.actions` still works for tables inside detail pages.
:::

**Hierarchy only affects the detail header.** In table rows (list view and detail-page tables), every action goes into the row's overflow menu, whatever its `hierarchy`. In the detail header:

| `hierarchy` | Detail header rendering                                                   |
| ----------- | ------------------------------------------------------------------------- |
| `ActionHierarchy.PRIMARY`   | A filled primary button. **Only the first** `PRIMARY` action is rendered; any others are dropped |
| `ActionHierarchy.SECONDARY` | A pill-shaped secondary button, one per action                    |
| `ActionHierarchy.TERTIARY`  | An item in the overflow (`…`) menu                                |
| _(unset)_   | Same as `TERTIARY`: **actions without a hierarchy go to the overflow menu** |

**Write interpolations against `data`, not `row` or `page`.** Entity actions are resolved with `{ row: record }` in list rows and `{ page: record }` in the detail header. The interpolation context's `data` is `row ?? page`, so `({ data }) => …` or `${data.metadata.name}` works in both places. Every built-in entity action uses `data`.

## ActionConfig: the Three Legal Shapes

`ActionConfig` is `ActionConfigBase` plus one of three shapes. TypeScript rejects any other combination (`modal?: never` / `operation?: never`).

| # | `operation`                                 | `modal`                    | What happens on click |
| - | ------------------------------------------- | -------------------------- | --------------------- |
| 1 | `MutationActionConfig` or `RouteActionConfig` | `ConfirmModalConfig`      | A confirm dialog opens. Confirming runs the mutation, or navigates to the route |
| 2 | `MutationActionConfig` or `RouteActionConfig` | _(none)_                  | A **route** navigates immediately. A **mutation does nothing**; see the warning below |
| 3 | _(none)_                                    | `CustomModalConfig`        | Your component opens with `{ record, onClose }` and owns the whole submit flow |

You can't pair a custom modal with an `operation`. If a custom form needs to run a mutation, call `useStudioMutation` inside the component, as `CreatePipelineRunForm` and `DeploymentForm` do.

:::warning A mutation without a modal doesn't run
Shape 2 type-checks with a mutation, but neither the row menu nor the header buttons dispatch it. Their click handlers only open a modal or follow a route. Always pair a mutation with a `ConfirmModalConfig` (shape 1). Every built-in mutation action does.
:::

## ActionConfigBase

| Property    | Type                   | Description                                                                                 | Default/Required |
| ----------- | ---------------------- | ------------------------------------------------------------------------------------------- | ---------------- |
| `display`   | `ActionTriggerDisplay` | `{ label: string; icon?: string }`. `icon` is the name of an icon registered in the IconProvider | ✅ Required |
| `hierarchy` | `ActionHierarchy`      | `PRIMARY`, `SECONDARY` or `TERTIARY`. See [Where Actions Render](#where-actions-render)      | Tertiary (overflow menu) |
| `disabled`  | `{ condition: boolean; message?: string }[]` | Rules that disable the action for a record                            | `undefined` (enabled) |

### `disabled`

Rules are evaluated in order. The **first** rule whose `condition` is `true` disables the action, and its `message` is shown as a tooltip on hover. The condition is usually a function interpolation over the record:

```typescript
// isRetryable is a helper you write for your own record type.
disabled: [
  {
    condition: interpolate(({ data }) => !isRetryable(data)),
    message: 'Only failed or killed runs can be retried',
  },
],
```

## Operations

### `MutationActionConfig`

`{ type: 'mutation'; mutation: MutationConfig }`. The record is sent as the request payload, after `middleware`.

| `MutationConfig` property | Type                 | Description                                                                         | Required    |
| ------------------------- | -------------------- | ----------------------------------------------------------------------------------- | ----------- |
| `mutationName`            | `string`             | RPC handler name, e.g. `'DeletePipeline'`, `'UpdateDeployment'`                     | ✅ Required |
| `clientOptions`           | `MutationOptions`    | `onSuccess(data)` / `onError(error)` callbacks                                      | Optional    |
| `successOperations`       | `SuccessOperation[]` | Side effects to run, **in order**, after the mutation succeeds                       | Optional    |
| `middleware`              | `MiddlewareSchema`   | Reshapes the record before the request is sent (`operations` with `source`, `destination`, `default`, `transformation`; plus `scaffold`). See `MiddlewareSchema` in `packages/core/hooks/use-schema-middleware/types.ts` for the full surface | Optional    |

### `RouteActionConfig`

`{ type: 'route'; route: string }`. Navigates to `route`, which can be interpolated, e.g. `'/${studio.projectId}/${studio.phase}/runs/${data.metadata.name}'`.

### Success Operations

**Nothing is invalidated automatically after a mutation.** If a list or detail page should refetch, add an `invalidate` operation for its query.

| `type`         | Properties                                                                                     | Behavior |
| -------------- | ---------------------------------------------------------------------------------------------- | -------- |
| `'invalidate'` | `targets: (string \| { name: string; serviceOptions: Record<string, unknown> })[]`, `delayMs?: number` | Invalidates queries by name (e.g. `'ListPipeline'`), or by name plus service options. `delayMs` waits before invalidating; use it when a controller reconciles the change asynchronously after the API responds |
| `'toast'`      | `message: string`, `icon?: string`, `action?: { label: string; route?: string }`                | Shows a snackbar. `icon` defaults to `'checkCircle'`. An `action` with a `route` navigates when clicked; without a `route` it dismisses the toast |
| `'route'`      | `route: string`                                                                                | Navigates without showing a toast |

Success operations are resolved against the mutation result, so they can read `${response.…}`:

```typescript
{
  type: 'toast',
  message: 'A new pipeline run has been created from the failed step of this run.',
  action: {
    label: 'See new run',
    route: interpolate(
      '/${studio.projectId}/${studio.phase}/runs/${response.pipelineRun.metadata.name}'
    ),
  },
},
```

## Modals

### `ConfirmModalConfig` (`type: 'confirm'`)

| Property       | Type                           | Description                                                                         | Required    |
| -------------- | ------------------------------ | ----------------------------------------------------------------------------------- | ----------- |
| `type`         | `'confirm'`                    | Discriminator                                                                       | ✅ Required |
| `header.title` | `string`                       | Dialog heading                                                                      | ✅ Required |
| `body`         | `ReactNode`                    | Dialog body, rendered through `Markdown` (so `**bold**` in a string works)          | Optional    |
| `banner`       | `BannerConfig`                 | `{ content: ReactNode; kind?: BannerProps['kind']; icon?: string }`, shown above the body | Optional    |
| `button.label` | `string`                       | Confirm button label                                                                | ✅ Required |
| `button.icon`  | `string`                       | Accepted by the type, but `ConfirmDispatcher` doesn't pass it to the dialog, so **it isn't rendered** | Optional    |
| `destructive`  | `boolean`                      | Renders the confirm button in red. Use for irreversible actions such as delete      | `false`     |
| `size`         | `Size` from `baseui/dialog`    | Dialog size                                                                         | Optional    |

### `CustomModalConfig` (`type: 'custom'`)

| Property    | Type                                      | Description                                                        | Required    |
| ----------- | ----------------------------------------- | ------------------------------------------------------------------ | ----------- |
| `type`      | `'custom'`                                | Discriminator                                                      | ✅ Required |
| `component` | `ComponentType<ActionComponentProps<T>>`  | Rendered when the action is clicked. Props: `record: T`, `onClose: () => void` | ✅ Required |

The component renders its own dialog, usually a [`FormDialog`](./form-reference.md#formdialog), and calls `onClose` when done.

## CreateActionConfig

A page-level "Create …" button. It has no record, so it isn't an `ActionConfig`: there's no `operation`, `hierarchy` or `disabled`.

| Property    | Type                                          | Description                                                  | Required    |
| ----------- | --------------------------------------------- | ------------------------------------------------------------ | ----------- |
| `display`   | `ActionTriggerDisplay`                        | Button `label` and optional `icon`                           | ✅ Required |
| `component` | `ComponentType<CreateActionComponentProps>`   | Rendered when the button is clicked. Props: `onClose: () => void` | ✅ Required |

```typescript
// From: config/entities/targets/target.ts
createAction: {
  display: { label: 'Create target', icon: 'plus' },
  component: CreateInferenceServerForm,
},
```

## Interpolation

Config authors write `ActionConfigSchema<T>`, which is `DeepInterpolatable<ActionConfig<T>>`: every leaf except functions and React components can be a plain value, a `${…}` string or an `interpolate(...)` call. The schema is resolved per record before any rendering code sees it. Components receive the resolved `ActionConfig`. See [Interpolation System](../core-systems.md#interpolation-system) for the context fields (`data`, `row`, `page`, `studio`, `response`).

## Example

Trimmed from `config/entities/pipeline/shared.ts`. Every built-in pattern appears here:
- `PIPELINE_DELETE_ACTION`: a mutation, middleware, three ordered success operations (one delayed), and a destructive confirm with an interpolated body.
- `PIPELINE_ACTIONS`: custom modals and a `disabled` rule.

```typescript
import { ActionHierarchy } from '#core/components/actions/types';
import { interpolate } from '#core/interpolation/interpolate';

import type { ActionConfigSchema } from '#core/components/actions/types';

// pipelineName, hasNoTriggers, CreatePipelineRunForm and RunTriggerForm are
// defined elsewhere in the source file.
export const PIPELINE_DELETE_ACTION: ActionConfigSchema<object> = {
  display: { label: 'Delete', icon: 'trashCan' },
  hierarchy: ActionHierarchy.TERTIARY,
  operation: {
    type: 'mutation',
    mutation: {
      mutationName: 'DeletePipeline',
      middleware: {
        operations: [
          { source: 'spec.baseResource', destination: 'metadata', transformation: (v) => v },
        ],
      },
      successOperations: [
        { type: 'invalidate', targets: ['ListPipeline'] },
        // Revisions are cascade-deleted asynchronously, so wait before refetching them.
        { type: 'invalidate', targets: ['ListRevision'], delayMs: 2000 },
        { type: 'route', route: '/${studio.projectId}/${studio.phase}/pipelines' },
      ],
    },
  },
  modal: {
    type: 'confirm',
    header: { title: 'Delete Pipeline' },
    body: interpolate(({ data }) => `Delete pipeline **${pipelineName(data)}**? This action cannot be undone.`),
    button: { label: 'Delete' },
    destructive: true,
  },
};

export const PIPELINE_ACTIONS: ActionConfigSchema<object>[] = [
  {
    display: { label: 'Run', icon: 'playerPlay' },
    hierarchy: ActionHierarchy.PRIMARY,
    modal: { type: 'custom', component: CreatePipelineRunForm },
  },
  {
    display: { label: 'Run trigger', icon: 'calendarRepeat' },
    hierarchy: ActionHierarchy.SECONDARY,
    disabled: [
      {
        condition: interpolate(({ data }) => hasNoTriggers(data)),
        message: 'No triggers defined for this pipeline',
      },
    ],
    modal: { type: 'custom', component: RunTriggerForm },
  },
  PIPELINE_DELETE_ACTION,
];
```

`pipelineName`, `hasNoTriggers`, `CreatePipelineRunForm` and `RunTriggerForm` are pipeline-specific helpers and forms defined next to this config. (`pipelineName` stands in for the inline expression in the real file.)

## Source Files

**Type definitions:**
- [javascript/packages/core/components/actions/types.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/actions/types.ts) - `ActionConfig`, `ActionConfigSchema`, operations, modals, `CreateActionConfig`
- [javascript/packages/core/types/query-types.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/types/query-types.ts) - `MutationConfig`
- [javascript/packages/core/hooks/use-schema-middleware/types.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/hooks/use-schema-middleware/types.ts) - `MiddlewareSchema`
- [javascript/packages/core/types/common/studio-types.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/types/common/studio-types.ts) - `PhaseEntityConfig.actions` and `createAction`

**Behavior:**
- [javascript/packages/core/components/views/phase-entity-view/phase-entity-view.tsx](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/views/phase-entity-view/phase-entity-view.tsx) - list-view `actions` override, `createAction` button
- [javascript/packages/core/components/actions/actions-buttons/actions-buttons.tsx](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/actions/actions-buttons/actions-buttons.tsx) - detail-header rendering by hierarchy
- [javascript/packages/core/components/actions/actions-popover.tsx](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/actions/actions-popover.tsx) - row overflow menu
- [javascript/packages/core/components/actions/confirm-dispatcher.tsx](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/actions/confirm-dispatcher.tsx) - confirm dialog and mutation dispatch
- [javascript/packages/core/components/actions/use-success-operations.tsx](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/actions/use-success-operations.tsx) - success operations

**Real examples:**
- [javascript/packages/core/config/entities/pipeline/shared.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/config/entities/pipeline/shared.ts) - `PIPELINE_DELETE_ACTION`, `PIPELINE_ACTIONS`
- [javascript/packages/core/config/entities/deployment/deployment.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/config/entities/deployment/deployment.ts) - confirm with a `banner`, `${response.…}` toast, `createAction`
- [javascript/packages/core/config/entities/targets/target.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/config/entities/targets/target.ts) - `createAction` only

## Related Documentation

- [Entity Configuration Reference](./entity-configuration-reference.md) - `actions` and `createAction` on an entity
- [Detail View Configuration Reference](./detail-view-configuration-reference.md) - The detail header where entity actions render
- [Table Configuration Reference](./table-configuration-reference.md) - `TableConfig.actions`
- [Form Reference](./form-reference.md) - Forms for custom modals and `createAction`
- [Core Systems: Interpolation](../core-systems.md#interpolation-system)
