## Overview

Cells are the fundamental building blocks for displaying data in Michelangelo UI. They define how individual fields are rendered across all view types: list views (table columns), detail views (metadata headers), and forms.

**Cell types are powered by the same rendering code** in `components/cell/`, with extensions registered in `components/table/components/table-cell/`.

## Available Cell Types

All cell types are defined in `CellType` enum. Here are the standard types you can use in your configurations:

### TEXT
```typescript
{
  id: 'metadata.name',
  label: 'Name',
  type: CellType.TEXT
}
```
**Description:** Standard non-formatted text  
**Use for:** Names, IDs, simple string values

### DATE
```typescript
{
  id: 'metadata.creationTimestamp.seconds',
  label: 'Created',
  type: CellType.DATE
}
```
**Description:** Formatted timestamp  
**Example output:** `2024/01/09 17:53:49`  
**Use for:** Any timestamp

### STATE
```typescript
{
  id: 'status.state',
  label: 'State',
  type: CellType.STATE,
  stateTextMap: {
    0: 'Queued',
    2: 'Running',
    3: 'Succeeded',
    5: 'Failed',
  },
  stateColorMap: {
    0: 'gray',
    2: 'blue',
    3: 'green',
    5: 'red',
  },
}
```
**Description:** Colored tag with state-specific styling (green for success, red for error, etc.)  
**Use for:** Any status field  
**Required config:** `stateTextMap`, `stateColorMap`

### TYPE
```typescript
{
  id: 'spec.type',
  label: 'Type',
  type: CellType.TYPE,
  typeTextMap: {
    1: 'Train',
    2: 'Evaluation',
    3: 'Performance Evaluation',
  },
}
```
**Description:** Badge with formatted text (sentence case, stripped prefixes/suffixes)  
**Use for:** Entity types, enum values  
**Required config:** `typeTextMap`

### LINK
```typescript
{
  id: 'metadata.name',
  label: 'Name',
  type: CellType.LINK,
  url: '/${studio.projectId}/${studio.phase}/runs/${data.metadata.name}'
}
```
**Description:** Clickable link  
**Use for:** Navigation to entity detail views, external links  
**Note:** Implicitly used when `url` property is provided

### DESCRIPTION
```typescript
{
  id: 'spec.revision.name',
  label: 'Revision',
  type: CellType.DESCRIPTION
}
```
**Description:** Text rendered slightly smaller and more opaque than standard text  
**Use for:** Secondary information, subtitles, descriptions

### BOOLEAN
```typescript
{
  id: 'spec.enabled',
  label: 'Enabled',
  type: CellType.BOOLEAN
}
```
**Description:** Checkmark icon with formatted text  
**Use for:** Boolean flags, true/false values

### TAG
```typescript
{
  id: 'metadata.label',
  label: 'Label',
  type: CellType.TAG
}
```
**Description:** Gray tag with formatted text  
**Use for:** Labels, tags, categories

### MULTI
```typescript
{
  id: 'spec.pipeline.name',
  label: 'Pipeline',
  items: [
    { id: 'spec.pipeline.name', type: CellType.TEXT },
    { id: 'spec.revision.name', type: CellType.DESCRIPTION },
  ],
}
```
**Description:** Renders column items in a vertical list  
**Use for:** Composite fields showing multiple related values  
**Required config:** `items` array

## Shared Cell Properties

All cell types support these common properties (from `SharedCell` interface):

| Property | Type | Description | Required |
|----------|------|-------------|----------|
| `id` | `string` | Unique identifier for the column. Used to access data if no accessor provided | ✅ Yes |
| `label` | `string` | Display label in table header or form field | No |
| `type` | `CellType` | Cell renderer type (TEXT, DATE, STATE, etc.) | No (defaults to TEXT) |
| `accessor` | `string \| function` | Custom JSON path or function to access cell value | No |
| `icon` | `string` | Icon to display before the cell value | No |
| `tooltip` | `CellTooltip` | Tooltip configuration (content + optional action) | No |
| `endEnhancer` | `object` | Content to display after the cell value | No |
| `Cell` | `CellRenderer` | Custom cell renderer component | No |
| `style` | `StyleObject \| function` | Custom styles for the cell | No |

### Property Details

#### `accessor`
More flexible control over cell value extraction:

```typescript
// JSON path string
{ id: 'name', accessor: 'spec.content.metadata.name' }

// Function
{
  id: 'revision',
  accessor: (row) => `Revision ${row?.spec?.revisionId}`
}

// Computed value example
{
  id: 'duration',
  label: 'Duration',
  accessor: (record) => {
    const start = parseInt(record.startTime.seconds) * 1000;
    const end = parseInt(record.endTime.seconds) * 1000;
    return `${Math.round((end - start) / 1000)}s`;
  },
}
```

#### `tooltip`
Interactive tooltips with optional actions.

**Important:** Tooltip behavior differs between table cells and metadata cells:

**In table columns** (list views):
- Supports `action: 'filter'` - Clicking tooltip filters table by that value
- Supports `action: 'custom'` - Custom click handler
- Tooltip content function receives row data: `({ row, value, record }) => ReactNode`
- Filter action automatically wired to table filtering

```typescript
// Simple filter tooltip (table only)
{
  id: 'status',
  label: 'Status',
  tooltip: {
    content: 'Click to filter by this status',
    action: 'filter'
  }
}

// Custom tooltip with row access (table only)
{
  id: 'name',
  label: 'Name',
  tooltip: {
    content: ({ row, value }) => (
      <div>Current: {value}, Row has {row.cells.length} columns</div>
    ),
    action: 'custom'
  }
}
```

**In metadata cells** (detail view headers):
- Tooltips are display-only (no actions)
- Content can be string or function: `(props) => ReactNode`
- No row context available (single entity, not table)

```typescript
// Metadata tooltip (detail view)
{
  id: 'metadata.creationTimestamp.seconds',
  label: 'Created',
  type: CellType.DATE,
  tooltip: {
    content: 'When this pipeline run was created'
    // No action property - metadata tooltips are display-only
  }
}
```

#### `style`
Custom styling per cell:

```typescript
// Static style object
{ id: 'name', style: { color: 'red', fontWeight: 'bold' } }

// Dynamic style function
{
  id: 'status',
  style: ({ record, theme }) => ({
    color: record.failed ? theme.colors.negative : theme.colors.positive
  })
}
```

## Type-Specific Configuration

### StateCellConfig

For `CellType.STATE`, you can configure state text and color mappings:

```typescript
export type StateCellConfig = SharedCell<string> & {
  stateTextMap?: Record<string, string>;
  stateColorMap?: Record<string, TagColor>;
};
```

**Example from codebase:**
```typescript
{
  id: 'status.state',
  label: 'State',
  type: CellType.STATE,
  stateTextMap: {
    0: 'Queued',
    1: 'Pending',
    2: 'Running',
    3: 'Succeeded',
    4: 'Killed',
    5: 'Failed',
    6: 'Skipped',
  },
  stateColorMap: {
    0: 'gray',
    1: 'blue',
    2: 'blue',
    3: 'green',
    4: 'red',
    5: 'red',
    6: 'gray',
  },
}
```

See [`config/entities/run/shared.ts`](https://github.com/michelangelo-ai/michelangelo/blob/47abf4b9e99f29ffc97f769d21103190a1486194/javascript/packages/core/config/entities/run/shared.ts#L30-L51) for complete example.

### TypeCellConfig

For `CellType.TYPE`, you can configure type text mappings:

```typescript
export type TypeCellConfig = SharedCell<string> & {
  typeTextMap?: Record<string, string>;
};
```

**Example:**
```typescript
{
  id: 'spec.type',
  label: 'Type',
  type: CellType.TYPE,
  typeTextMap: {
    1: 'Train',
    2: 'Evaluation',
    3: 'Performance Evaluation',
    4: 'Experiment',
    5: 'Retrain',
    6: 'Prediction',
  },
}
```

See [`config/entities/pipeline/list.ts`](https://github.com/michelangelo-ai/michelangelo/blob/47abf4b9e99f29ffc97f769d21103190a1486194/javascript/packages/core/config/entities/pipeline/list.ts#L17-L39) for complete example.

## Custom Cell Renderers

For unique rendering needs beyond the standard cell types, you can provide a custom renderer:

```typescript
{
  id: 'metrics',
  label: 'Metrics',
  Cell: CustomMetricsCell
}

function CustomMetricsCell({ value, record }: CellRendererProps) {
  return (
    <div>
      {/* Your custom rendering logic */}
    </div>
  );
}
```

**Note:** Custom renderers bypass default functionality (styling, hyperlinking, tooltips). Ensure your custom renderer:
- Doesn't need this functionality, OR
- Applies its own copy of this functionality

## Where Cells Are Used

Cells are used for **displaying data** in configuration:

### List Views (Table Columns)
```typescript
export const PIPELINE_LIST_CONFIG: ListViewConfig = {
  type: 'list',
  tableConfig: {
    columns: [
      { id: 'metadata.name', label: 'Name' },
      { id: 'status.state', label: 'State', type: CellType.STATE, ... },
    ],
  },
};
```

### Detail Views (Metadata Headers)
```typescript
export const RUN_DETAIL_CONFIG: DetailViewConfig = {
  type: 'detail',
  metadata: [
    { id: 'metadata.creationTimestamp.seconds', label: 'Created', type: CellType.DATE },
    { id: 'status.state', label: 'State', type: CellType.STATE, ... },
  ],
  pages: [...],
};
```

## Source Files

**Type definitions:**
- [javascript/packages/core/components/cell/types.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/cell/types.ts) - `SharedCell`, `Cell`, `CellRenderer` interfaces
- [javascript/packages/core/components/cell/constants.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/cell/constants.ts) - `CellType` enum, renderer registry
- [javascript/packages/core/components/cell/renderers/state/types.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/cell/renderers/state/types.ts) - `StateCellConfig`
- [javascript/packages/core/components/cell/renderers/type/types.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/cell/renderers/type/types.ts) - `TypeCellConfig`

**Cell renderers:**
- [javascript/packages/core/components/cell/renderers/](https://github.com/michelangelo-ai/michelangelo/tree/main/javascript/packages/core/components/cell/renderers) - Individual cell type implementations

**Table extensions:**
- [javascript/packages/core/components/table/components/table-cell/](https://github.com/michelangelo-ai/michelangelo/tree/main/javascript/packages/core/components/table/components/table-cell) - Table-specific cell extensions