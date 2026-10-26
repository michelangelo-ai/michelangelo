---
last_verified: 2026-10-26
---

# Detail View Configuration Reference

A detail view is the page you reach by clicking a record in an entity's list, at `/<projectId>/<phase>/<entity>/<entityId>/<entityTab>`. It has two parts:

- **A header**: the record's name, the entity's [actions](./action-configuration-reference.md), and a row of metadata items
- **Tabs (pages)**: each tab is a table, an execution (step) view, or a custom React component

You configure it with a `DetailViewConfig` in the entity's `views` array.

This page is for contributors adding or changing detail pages in Michelangelo AI Studio. It assumes you've read the [Entity Configuration Reference](./entity-configuration-reference.md).

## DetailViewConfig Interface

| Property   | Type                  | Description                                                              | Required    |
| ---------- | --------------------- | ------------------------------------------------------------------------ | ----------- |
| `type`     | `'detail'`            | Marks this view as the detail view                                       | ✅ Required |
| `metadata` | `RowCell[]`           | Items shown in the header row, rendered by `Row`                         | ✅ Required |
| `pages`    | `DetailPageConfig[]`  | The tabs. **The first page is the default tab**                          | ✅ Required |

## How the Route Uses the Config

`EntityDetailRoute` reads the config like this:

1. It finds the entity whose `id` matches the URL's `entity` segment in `phases[phase].entities`.
2. It uses the **first** view in `entityConfig.views` with `type: 'detail'`. An entity with no detail view has no detail page.
3. If the URL has no `entityTab`, or one that isn't any `pages[].id`, it redirects (with `replace`) to the first page. Page order matters: put the tab you want people to land on first.
4. It runs the whole detail config through the interpolation resolver with `{ page: record }`, where `record` is the fetched entity. Any `${page.…}` string or `interpolate(...)` value anywhere in the config resolves against that record. See [Interpolation System](../core-systems.md#interpolation-system).

## `metadata`: RowCell

`RowCell` is a [`Cell`](./cell-type-reference.md) with two extra properties:

| Property      | Type      | Description                                                                    | Default     |
| ------------- | --------- | ------------------------------------------------------------------------------ | ----------- |
| `description` | `string`  | Help text displayed next to the item's label                                   | `undefined` |
| `hideEmpty`   | `boolean` | Hide the item entirely when its value is empty, instead of rendering it blank  | `false`     |

Use `hideEmpty` for fields that only apply to some records, such as mutually exclusive fields.

## `pages`: DetailPageConfig

Every page has these properties, whatever its type:

| Property | Type     | Description                                                          | Required    |
| -------- | -------- | -------------------------------------------------------------------- | ----------- |
| `id`     | `string` | Unique page ID. Also the `entityTab` segment of the URL               | ✅ Required |
| `label`  | `string` | Tab label shown in the header                                         | ✅ Required |
| `type`   | `'table' \| 'execution' \| 'custom'` | Selects one of the three page types below  | ✅ Required |

`DetailViewPageRenderer` renders a page according to its `type`. A page whose `type` isn't one of the three (possible when a config bypasses TypeScript) renders the text **"Page type '…' not yet supported"** instead of throwing.

### `type: 'table'`

A table of related records, fetched with its own query.

| Property      | Type          | Description                                                                 | Required    |
| ------------- | ------------- | --------------------------------------------------------------------------- | ----------- |
| `queryConfig` | `QueryConfig` | What to fetch: `endpoint` (e.g. `'list'`), `service` (e.g. `'pipelineRun'`), `serviceOptions`, optional `clientOptions` | ✅ Required |
| `tableConfig` | `TableConfig` | Columns and table behavior. See [Table Configuration Reference](./table-configuration-reference.md) | ✅ Required |

`tableConfig.actions` works on detail-page tables; each row's actions resolve against that row.

### `type: 'execution'`

A list of steps (tasks) with states, such as a pipeline run's steps. The page config extends `ExecutionDetailViewSchema`:

| Property                 | Type                              | Description                                                                                       | Required    |
| ------------------------ | --------------------------------- | ------------------------------------------------------------------------------------------------- | ----------- |
| `emptyState.title`       | `string`                          | Shown when the task accessor returns no tasks                                                     | ✅ Required |
| `emptyState.description` | `string`                          | Extra context under the title                                                                     | Optional    |
| `tasks.accessor`         | `Accessor<unknown, TTaskRecord[]>` | Path (e.g. `'status.steps'`) or function returning the task records from the entity              | ✅ Required |
| `tasks.subTasksAccessor` | `Accessor<unknown, TTaskRecord[]>` | Path or function returning a task's child tasks. Omit for a flat list                            | Optional    |
| `tasks.header.heading`   | `Accessor<unknown, string>`        | Path or function giving each task's display name. Falls back to the record's `name` if it returns a falsy value | ✅ Required |
| `tasks.header.metadata`  | `RowCell[]`                       | Items shown under each task's heading, rendered with cell renderers                               | Optional    |
| `tasks.body`             | `TaskBodySchema[]`                | Detail sections shown for a task, e.g. `{ type: 'struct', label: 'Input', accessor: 'input' }`     | Optional    |
| `tasks.stateBuilder`     | `(taskRecord, taskIndex, siblingTasks, rootData) => TaskState` | Maps each raw task record to a `TASK_STATE` value                | ✅ Required |

`TASK_STATE` (exported from `@michelangelo-ai/core`) has five values: `ERROR`, `PENDING`, `RUNNING`, `SUCCESS` and `SKIPPED`.

A minimal sketch, adapted from the `ExecutionDetailViewSchema` doc comment:

```typescript
import { TASK_STATE } from '#core/components/views/execution/constants';

// Real configs compare against the generated step-state enum, as in the example below.
stateBuilder: (step) => {
  if (step.state === 'SUCCEEDED') return TASK_STATE.SUCCESS;
  if (step.state === 'FAILED') return TASK_STATE.ERROR;
  if (step.state === 'RUNNING') return TASK_STATE.RUNNING;
  return TASK_STATE.PENDING;
},
```

### `type: 'custom'`

Any React component, for content the other two types can't express.

| Property    | Type                                                                 | Description                              | Required    |
| ----------- | -------------------------------------------------------------------- | ---------------------------------------- | ----------- |
| `component` | `React.ComponentType<{ data: T \| undefined; isLoading: boolean }>`   | Receives the entity record and its loading state | ✅ Required |

The built-in "Information" tabs (`RunInfoPage`, `ModelInfoPage` and others) are custom pages. Several of them display values in a read-only [`Form`](./form-reference.md#read-only-display-forms).

## Revisioned Entities

When the entity sets `revisioned: true` (see [Entity Configuration Reference](./entity-configuration-reference.md)), the detail view renders a **Revision** snapshot through the same detail config. That's the Revision named by `?revisionId=`, or the one `status.latestRevision` points at when there's no `revisionId`. A `RevisionSelector` appears next to the title, and selecting a Revision changes only the query string, so the current tab is kept. Because `page` is then the Revision rather than the live entity, accessors read the snapshot's paths: `PIPELINE_DETAIL_CONFIG` reads `spec.content.spec.type` and `spec.baseResource.name`, for example.

## Example

Trimmed from `config/entities/run/detail.ts` (custom and execution pages) and `config/entities/pipeline/detail.ts` (table page):

```typescript
import { CellType } from '#core/components/cell/constants';
import { TASK_STATE } from '#core/components/views/execution/constants';
// RunInfoPage, SHARED_RUN_CELL_CONFIG and PipelineRunStepState are defined in
// neighboring files of the source tree.
import { RunInfoPage } from './run-info-page';
import { SHARED_RUN_CELL_CONFIG } from './shared';
import { PipelineRunStepState } from './types';

import type { DetailViewConfig } from '#core/components/views/types';

export const RUN_DETAIL_CONFIG: DetailViewConfig = {
  type: 'detail',
  metadata: SHARED_RUN_CELL_CONFIG,
  pages: [
    {
      id: 'information', // default tab: first in the array
      label: 'Information',
      type: 'custom',
      component: RunInfoPage,
    },
    {
      id: 'steps',
      label: 'Steps',
      type: 'execution',
      emptyState: {
        title: 'No execution data',
        description: 'No steps available for this pipeline run',
      },
      tasks: {
        accessor: 'status.steps',
        subTasksAccessor: 'subSteps',
        header: {
          heading: 'displayName',
          metadata: [
            { id: 'startTime', label: 'Start time', type: CellType.DATE },
            { id: 'retry', label: 'Actions', type: CellType.RETRY, accessor: 'activityId', hideEmpty: true },
          ],
        },
        body: [{ type: 'struct', label: 'Input Parameters', accessor: 'input' }],
        stateBuilder: (record: { state: PipelineRunStepState }) => {
          switch (record.state) {
            case PipelineRunStepState.SUCCEEDED:
              return TASK_STATE.SUCCESS;
            case PipelineRunStepState.FAILED:
              return TASK_STATE.ERROR;
            // …RUNNING, KILLED and SKIPPED cases trimmed
            default:
              return TASK_STATE.PENDING;
          }
        },
      },
    },
  ],
};

// A table page, from PIPELINE_DETAIL_CONFIG: the query is filtered by the viewed record.
export const PIPELINE_DETAIL_CONFIG: DetailViewConfig = {
  type: 'detail',
  metadata: [{ id: 'spec.owner.name', label: 'Owner', type: CellType.TEXT }],
  pages: [
    {
      id: 'triggers',
      label: 'Triggers',
      type: 'table',
      queryConfig: {
        endpoint: 'list',
        service: 'triggerRun',
        serviceOptions: {
          listOptions: { fieldSelector: 'pipeline_name=${page.spec.baseResource.name}' },
        },
      },
      tableConfig: {
        columns: [
          {
            id: 'metadata.name',
            label: 'Name',
            url: '/${studio.projectId}/${studio.phase}/triggers/${row.metadata.name}',
          },
        ],
      },
    },
  ],
};
```

## Source Files

**Type definitions:**
- [javascript/packages/core/components/views/types.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/views/types.ts) - `DetailViewConfig`, `TableConfig`
- [javascript/packages/core/components/views/detail-view/types/detail-view-schema-types.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/views/detail-view/types/detail-view-schema-types.ts) - `DetailPageConfig` and the three page types
- [javascript/packages/core/components/views/execution/types.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/views/execution/types.ts) - `ExecutionDetailViewSchema`, `TaskState`
- [javascript/packages/core/components/views/execution/constants.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/views/execution/constants.ts) - `TASK_STATE`
- [javascript/packages/core/components/row/types.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/row/types.ts) - `RowCell`

**Behavior:**
- [javascript/packages/core/router/entity-detail-route.tsx](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/router/entity-detail-route.tsx) - config lookup, default-tab redirect, interpolation, revisions
- [javascript/packages/core/components/views/detail-view/components/detail-view-page-renderer/detail-view-page-renderer.tsx](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/views/detail-view/components/detail-view-page-renderer/detail-view-page-renderer.tsx) - page-type dispatch

**Real examples:**
- [javascript/packages/core/config/entities/run/detail.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/config/entities/run/detail.ts) - custom and execution pages
- [javascript/packages/core/config/entities/pipeline/detail.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/config/entities/pipeline/detail.ts) - table pages on a revisioned entity
- [javascript/packages/core/config/entities/targets/detail.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/config/entities/targets/detail.ts) - targets (inference servers)

## Related Documentation

- [Entity Configuration Reference](./entity-configuration-reference.md) - Where the detail view sits in an entity's `views`
- [Action Configuration Reference](./action-configuration-reference.md) - Actions in the detail header
- [Table Configuration Reference](./table-configuration-reference.md) - `tableConfig` for table pages
- [Cell Types Reference](./cell-type-reference.md) - Cells used in `metadata`
- [Core Systems: Interpolation](../core-systems.md#interpolation-system)
