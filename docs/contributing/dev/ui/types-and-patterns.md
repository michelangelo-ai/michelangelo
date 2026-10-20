---
sidebar_position: 3
last_verified: 2026-10-14
---

# Types and Patterns

## Key TypeScript Types

### Phase

The [`Phase`](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/types/common/studio-types.ts#L28) enum represents different stages in the ML workflow. Phases are used in URL routing and define the application's navigation structure.

```typescript
export enum Phase {
  Project = 'project',
  Assistants = 'assistants',
  Agents = 'agents',
  // Traditional ML workflow
  Data = 'data',
  Train = 'train',
  Retrain = 'retrain',
  Deploy = 'deploy',
  Monitor = 'monitor',
  // GenAI workflow
  GenaiLLM = 'genai-llm',
  GenaiData = 'genai-data',
  GenaiPrompt = 'genai-prompt',
  GenaiFinetune = 'genai-finetune',
  GenaiMonitor = 'genai-monitor',
  // Agent workflow
  AgentData = 'agent-data',
  AgentDevelop = 'agent-develop',
  AgentDeploy = 'agent-deploy',
  AgentMonitor = 'agent-monitor',
}
```

### PhaseConfig

[`PhaseConfig`](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/types/common/studio-types.ts#L133) defines a workflow phase's appearance and entities:

```typescript
export interface PhaseConfig {
  id: string;           // URL segment
  icon: string;         // Icon name from IconProvider
  name: string;         // Display name (e.g., "Train & Evaluate")
  description?: string; // Optional description
  docUrl?: string;      // Link to documentation
  state: PhaseState;    // 'active' | 'comingSoon' | 'disabled'
  entities: PhaseEntityConfig[];
  pipelineTypes?: string[]; // Restrict pipeline-derived entities to these pipeline types
}
```

### PhaseEntityConfig

[`PhaseEntityConfig`](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/types/common/studio-types.ts#L68) defines an entity within a phase:

```typescript
export interface PhaseEntityConfig<T extends object = object> {
  name: string;     // Display name (plural, lowercase): "trained models"
  id: string;       // URL segment (plural, no whitespace): "models"
  service: QueryConfig['service'];  // RPC service name: "pipeline", "pipelineRun"
  state: PhaseEntityState;  // 'active' | 'disabled'
  revisioned?: boolean;     // Render the detail view with a Revision
  views: ViewConfig<T>[];   // List and detail view configurations
  actions?: ActionConfigSchema<T>[];  // Row actions in list views
  createAction?: CreateActionConfig;  // Page-level "create" button in the phase header
}
```

### ViewConfig

[`ViewConfig`](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/views/types.ts#L15) defines how an entity is displayed:

```typescript
export type ViewConfig<T extends object = object> = ListViewConfig<T> | DetailViewConfig<T>;

export interface ListViewConfig<T extends object = object> {
  type: 'list';
  tableConfig: TableConfig<T>;
  variants?: ListViewVariant<T>[];  // Alternate data sources the list can toggle between
}

export interface DetailViewConfig<T extends object = object> {
  type: 'detail';
  metadata: RowCell[];
  pages: DetailPageConfig<T>[];
}
```

### Accessor

A flexible way to access object properties, supporting both dot-notation strings and functions:

```typescript
export type Accessor<TIn = unknown, TOut = unknown> = AccessorFn<TIn, TOut> | string;
export type AccessorFn<TIn = unknown, TOut = unknown> = (object: TIn) => TOut | undefined;

// Examples
const accessor1: Accessor = 'metadata.name';
const accessor2: Accessor = 'users[0].email';
const accessor3: Accessor<{ user?: { name: string } }, string> = (obj) => obj.user?.name;
```

## Type Conventions

### Naming

| Pattern | Example | Usage |
|---------|---------|-------|
| PascalCase | `PhaseConfig`, `ViewConfig` | Types and interfaces |
| No T suffix | `TableData` not `TTableData` | Generic type parameters |
| Singular | `ColumnConfig` | Type names |
| Plural | `columns: ColumnConfig[]` | Array properties |

### Props Types

Follow the pattern established in each component area:

```typescript
// Simple "Props" when single component per file
interface Props {
  data: TableData[];
  columns: ColumnConfig[];
}

// "ComponentNameProps" when multiple components or exported
export interface TableCellProps<T extends TableData = TableData>
  extends CellRendererProps<T, ColumnConfig<T>> {
  columnFilterValue?: unknown;
  row: TableRow<T>;
  setColumnFilterValue?: (value: unknown) => void;
}
```

### State Types

Use union types for finite states:

```typescript
export type PhaseState = 'active' | 'comingSoon' | 'disabled';
export type PhaseEntityState = 'active' | 'disabled';
export type TableViewState = 'loading' | 'empty' | 'ready' | 'error' | 'filtered-empty' | 'no-columns';
```

## React Component Patterns

### Functional Components

All components use functional components with hooks:

```typescript
export function Table<T extends TableData>({ data, columns }: TableProps<T>) {
  const [css] = useStyletron();
  // ...
}
```

### Props Naming

Event handlers focus on intent rather than event type:

```typescript
// Preferred
const showTooltipAfterDelay = () => { /* ... */ };
const toggleMenu = () => { /* ... */ };

// Avoid
const handleOnMouseEnter = () => { /* ... */ };
const handleOnClick = () => { /* ... */ };
```

### Generic Components

Components accepting data use generic type parameters:

```typescript
interface TableProps<T extends TableData = TableData> {
  data: T[];
  columns: ColumnConfig<T>[];
}

export function Table<T extends TableData>(props: TableProps<T>) {
  // T is inferred from data
}
```

## File and Naming Conventions

### File Names

- Use kebab-case: `use-studio-query.ts`, `table-cell.tsx`
- Use `.tsx` for files with JSX, `.ts` otherwise
- Tests go in `__tests__/` directories: `__tests__/use-studio-query.test.ts`

### Directory Structure

Components follow a recursive folder structure:

```
components/
├── table/
│   ├── components/
│   │   ├── table-header/
│   │   ├── table-body/
│   │   └── table-cell/
│   ├── hooks/
│   ├── types/
│   ├── utils/
│   └── table.tsx
```

### Import Style

Prefer direct imports over barrel exports:

```typescript
// Preferred
import { useStudioQuery } from '#core/hooks/use-studio-query';

// Avoid index re-exports
import { useStudioQuery } from '#core/hooks';
```

## Related Documentation

- [Architecture Overview](./index.md) - Technology stack and build process
- [Core Systems](./core-systems.md) - Providers, hooks, and error handling
- [UI Components](./ui-components.md) - Table, cell, and form systems
