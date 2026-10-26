---
last_verified: 2026-10-14
sidebar_position: 4
---

# UI Components

Tables, cells, forms, styling, and testing patterns.

## Table System

The table system is built on [TanStack React Table](https://tanstack.com/table) and provides a declarative API.

### TableConfig

Tables are configured through [`TableConfig`](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/views/types.ts):

```typescript
export interface TableConfig<T extends TableData = TableData> {
  columns: ColumnConfig<T>[];
  emptyState?: EmptyState;
  disablePagination?: boolean;
  disableSorting?: boolean;
  disableSearch?: boolean;
  disableFilters?: boolean;
  pageSizes?: PageSizeOption[];
  enableStickySides?: boolean;
  maxHeight?: string;          // Caps the scrollable area; rows scroll under a pinned header
  actions?: ActionConfigSchema<T>[];
}
```

### ColumnConfig

[`ColumnConfig`](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/table/types/column-types.ts) defines how data is displayed and interacted with:

```typescript
export type ColumnConfig<TData = TableData> = DistributiveOmit<Cell<TData>, 'tooltip'> & {
  filterMode?: FilterMode;     // 'NONE' | 'CLIENT' | 'SERVER'
  enableSorting?: boolean;     // default: true
  enableGrouping?: boolean;    // default: false
  aggregationFn?: AggregationFnOption<TData>;
  aggregatedCell?: ComponentType<CellRendererProps<TData, ColumnConfig<TData>>>;
  sortingFn?: SortingFnOption<TData>;
  tooltip?: ColumnTooltip<TData>;
};
```

### Table State

[`TableState`](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/table/types/table-types.ts) supports both controlled and uncontrolled state:

```typescript
export type TableState = {
  globalFilter: string;
  columnFilters: ColumnFilter[];
  pagination: PaginationState;
  sorting: SortingState;
  columnOrder: ColumnOrderState;
  columnVisibility: ColumnVisibilityState;
  rowSelection: RowSelectionState;
  rowSelectionEnabled: boolean;
  grouping: GroupingState;
};
```

### Table View States

Tables display different views based on their state:

| State | Description |
|-------|-------------|
| `loading` | Data is being fetched |
| `empty` | No data available |
| `ready` | Data is displayed |
| `error` | An error occurred |
| `filtered-empty` | Filters returned no results |
| `no-columns` | No visible columns |

## Cell Renderer System

Cells define how individual data values are rendered in tables and detail views.

### SharedCell Interface

[`SharedCell`](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/cell/types.ts) is the base interface for all cell types:

```typescript
export interface SharedCell<TRecord = unknown, TValue = unknown> {
  id: string;              // Unique identifier, also used as accessor
  accessor?: Accessor<TRecord, TValue>;  // Custom data access path or function
  label?: string;          // Header label
  type?: string;           // Cell renderer type (a CellType value, e.g., 'DATE')
  icon?: string;           // Icon before value
  tooltip?: CellTooltip;   // Hover tooltip
  endEnhancer?: { content: ReactNode; type: 'tooltip' };
  Cell?: CellRenderer<TValue>;  // Custom renderer
  style?: StyleObject | CellStyleFunction;
}
```

### Cell Types

The `Cell` type is a union of all cell configurations:

```typescript
export type Cell<TRecord = unknown, TValue = unknown> = SharedCell<TRecord, TValue> & (
  | DescriptionCellConfig<TRecord>  // Name with description
  | LinkCellConfig<TRecord>         // Clickable link
  | MultiCellConfig                 // Multiple values
  | StateCellConfig<TRecord>        // Status indicator
  | TypeCellConfig<TRecord>         // Type badge
);
```

### CellRenderer

Custom cell renderers receive standardized props:

```typescript
export interface CellRendererProps<T = unknown, CellConfig = SharedCell<unknown, T>> {
  column: CellConfig;      // Column configuration
  record: object;          // Full row data
  value: T | undefined;    // Resolved cell value
  CellComponent?: CellRenderer<T>;  // For recursive rendering
}
```

### Cell Configuration Examples

```typescript
// Simple text cell
{ id: 'metadata.name', label: 'Name' }

// Cell with accessor function
{
  id: 'revision',
  label: 'Revision',
  accessor: (row) => `v${row.spec?.revisionId}`,
}

// State cell with color mapping
{
  id: 'status.state',
  label: 'Status',
  type: CellType.STATE,
  stateTextMap: {
    PIPELINE_RUN_STATE_RUNNING: 'Running',
    PIPELINE_RUN_STATE_FAILED: 'Failed',
    PIPELINE_RUN_STATE_PENDING: 'Pending',
  },
  stateColorMap: {
    PIPELINE_RUN_STATE_RUNNING: 'blue',
    PIPELINE_RUN_STATE_FAILED: 'red',
    PIPELINE_RUN_STATE_PENDING: 'blue',
  },
}

// Link cell
{
  id: 'metadata.name',
  label: 'Pipeline',
  url: '/${studio.projectId}/${studio.phase}/pipelines/${data.metadata.name}',
}
```

## Form System

Forms are built from React components on top of [React Final Form](https://final-form.org/react): `Form` or `FormDialog`, field components (`StringField`, `SelectField`, `BooleanField`, …), layout components (`FormGroup`, `FormRow`, …), validators and the `useForm` / `useFormState` / `useField` hooks. See the [Form Reference](./configuration/form-reference.md) for every prop, field and validator.

## Theme and Styling

### Styletron

Michelangelo AI UI uses [Styletron](https://styletron.org/) with [BaseUI](https://baseweb.design/) for styling:

```typescript
import { useStyletron } from 'baseui';

function MyComponent() {
  const [css, theme] = useStyletron();

  return (
    <div className={css({
      display: 'flex',
      gap: theme.sizing.scale400,
      padding: theme.sizing.scale600,
    })}>
      {/* content */}
    </div>
  );
}
```

### Styled Components

For complex or reusable styles, use `styled()`:

```typescript
import { styled } from 'baseui';

export const TaskSeparator = styled('div', ({ $theme }) => ({
  height: '1px',
  backgroundColor: $theme.colors.borderOpaque,
  margin: `${$theme.sizing.scale600} 0`,
}));
```

### When to Extract Styles

Extract to styled components when:
- 4+ CSS properties
- Used in multiple places
- Complex computed values or pseudo-selectors
- Inline styles make JSX hard to read

### Naming Styled Components

Use semantic names that describe purpose:

```typescript
// Good
TaskSeparator, ExecutionMatrix, PipelineHeader

// Avoid
Container, Card, Wrapper
```

## Testing Patterns

### Test Framework

Tests use [Vitest](https://vitest.dev/) with [React Testing Library](https://testing-library.com/docs/react-testing-library/intro/):

```typescript
import { render, screen } from '@testing-library/react';
import { describe, it, expect } from 'vitest';

describe('MyComponent', () => {
  it('renders content', () => {
    render(<MyComponent />);
    expect(screen.getByText('Hello')).toBeInTheDocument();
  });
});
```

### Test Wrappers

Use [`buildWrapper`](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/test/wrappers/build-wrapper.tsx) to compose provider wrappers:

```typescript
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { getServiceProviderWrapper } from '#core/test/wrappers/get-service-provider-wrapper';

const wrapper = buildWrapper([
  getRouterWrapper(),
  getServiceProviderWrapper({ request: mockRequest }),
]);

render(<MyComponent />, wrapper);
```

Available wrapper functions:

| Wrapper | Purpose |
|---------|---------|
| `getBaseProviderWrapper` | BaseUI theme and Styletron |
| `getRouterWrapper` | React Router context |
| `getServiceProviderWrapper` | Mock RPC requests |
| `getErrorProviderWrapper` | Error normalization |
| `getInterpolationProviderWrapper` | Interpolation context |
| `getIconProviderWrapper` | Icon components |
| `getFormProviderWrapper` | Form context |
| `getCellProviderWrapper` | Cell rendering context |
| `getUserProviderWrapper` | User context |
| `getRepeatedLayoutProviderWrapper` | Repeated layout state |
| `getSnackbarProviderWrapper` | Snackbar notifications |

### Testing Guidelines

- Test user-facing behavior, not implementation details
- Use `screen.getByRole()` and `screen.getByText()` over container queries
- Mock external APIs and RPC calls
- Use real internal hooks and components
- Place tests in nearest `__tests__/` directory

## Related Documentation

- [Architecture Overview](./index.md) - Technology stack and build process
- [Types and Patterns](./types-and-patterns.md) - TypeScript types and conventions
- [Core Systems](./core-systems.md) - Providers, hooks, and error handling
