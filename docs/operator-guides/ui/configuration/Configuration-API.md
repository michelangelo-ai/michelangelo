# Configuration API

Michelangelo UI is configuration-driven, meaning common UI patterns are abstracted into configuration objects. Think of these as component APIs or contracts that cover the standard cases for ML platform interfaces.

**Configuration objects are simply structured interfaces for defining UI behavior:**
- React developers: Component props interfaces
- Backend/API developers: API contracts
- Everyone: A structured way to define UI without writing component code

We've identified common patterns in ML platform UIs (entity tables, detail views, workflow phases) and abstracted them into configurations. Use these configurations when they fit your needs, write custom components when they don't.

## How Configuration Works

Configuration in Michelangelo UI follows a hierarchy:

**Phase → Entity → View**

Views can be tables, forms, or detail pages.

### The Configuration Hierarchy

**1. Phase Configuration** - Groups entities by ML lifecycle stage

```typescript
// Example: config/phases/train.ts
export const TRAIN_PHASE: PhaseConfig = {
  id: 'train',
  icon: 'chartLine',
  name: 'Train & Evaluate',
  state: 'active',
  entities: [PIPELINE_ENTITY_CONFIG, RUN_ENTITY_CONFIG, ...]
}
```

- **id**: URL routing (`/train`)
- **entities**: Related objects for this lifecycle phase
- **state**: Phase availability (`active`, `comingSoon`, `disabled`)

See [`javascript/packages/core/config/phases`](https://github.com/michelangelo-ai/michelangelo/tree/main/javascript/packages/core/config/phases) for complete examples.

---

**2. Entity Configuration** - Defines data models and their properties

```typescript
// Example: config/entities/run/run.ts
export const RUN_ENTITY_CONFIG: PhaseEntityConfig = {
  id: 'runs',
  name: 'Pipeline Runs',
  service: 'pipelineRun',
  state: 'active',
  views: [RUN_LIST_CONFIG, RUN_DETAIL_CONFIG],
};
```

- **id**: URL routing (`/<phase>/runs`)
- **service**: Maps to protobuf service name
- **views**: Supported views (list, detail)

See [`javascript/packages/core/config/entities/`](https://github.com/michelangelo-ai/michelangelo/tree/main/javascript/packages/core/config/entities) for complete examples.

---

**3. View Configuration** - Controls presentation

```typescript
// List view example
export const PIPELINE_LIST_CONFIG: ListViewConfig<object> = {
  type: "list",
  tableConfig: {
    columns: [
      {
        id: "metadata.name",
        label: "Name",
        tooltip: {
          content: "Click to filter by this pipeline name",
          action: "filter",
        },
      },
      {
        id: "status.state",
        label: "State",
        type: CellType.STATE,
        stateTextMap: { 0: 'Invalid', 1: 'Created', 2: 'Building', ... },
        stateColorMap: { 0: 'red', 1: 'green', 2: 'yellow', ... },
      },
    ],
  },
};

// Detail view example
export const RUN_DETAIL_CONFIG: DetailViewConfig = {
  type: 'detail',
  metadata: [...], // Header metadata cells
  pages: [
    {
      id: 'steps',
      label: 'Steps',
      type: 'execution',
      tasks: { accessor: 'status.steps', ... }
    }
  ],
};
```

See `javascript/packages/core/config/entities/*/list.ts` and `*/detail.ts` for complete examples.

---

## When to Use Configuration vs Custom Components

**✅ Use configuration when the standard patterns fit:**

- Entity list views (tables with columns, sorting, filtering)
- Entity detail views (metadata headers, content sections)
- Data input forms _(coming soon)_
- Workflow phases grouping related entities
- Standard display types (dates, states, text, links)

**🔧 Write custom components when you need:**

- UI patterns not covered by existing configurations
- Complex interactive visualizations or custom interactions
- Business logic beyond what accessor functions can handle
- Integration with external systems requiring custom logic

**💡 Engineering best practice: Dual interfaces**

When building new UI components for Michelangelo, provide both interfaces:

```typescript
// Base component - standard React props
export function MyView({ data, onAction, ... }: MyViewProps) { ... }

// Config wrapper - declarative configuration
export function ConfigurableMyView({ config }: { config: MyViewConfig }) {
  return <MyView {...mapConfigToProps(config)} />;
}
```

This allows consumers to choose the interface that fits their needs.

## Common Configuration Patterns

These patterns apply across different view types (list views, detail views, forms):

### Display Type Mapping

Map enum values to display text and colors:

```typescript
{
  id: 'status.state',
  label: 'State',
  type: CellType.STATE,
  stateTextMap: { 0: 'Queued', 2: 'Running', 3: 'Succeeded', ... },
  stateColorMap: { 0: 'gray', 2: 'blue', 3: 'green', ... },
}
```

Works in list views and detail view metadata. See [`config/entities/run/shared.ts`](https://github.com/michelangelo-ai/michelangelo/blob/45d238f458785d5badf866ab7e3641737f12d3a5/javascript/packages/core/config/entities/run/shared.ts#L30-L51).

### Computed Values with Accessor Functions

Calculate derived values from your data:

```typescript
{
  id: 'duration',
  label: 'Duration',
  type: CellType.TEXT,
  accessor: (record) => {
    const start = parseInt(record.startTime.seconds) * 1000;
    const end = parseInt(record.endTime.seconds) * 1000;
    return `${Math.round((end - start) / 1000)}s`;
  },
}
```

Use accessor functions for any computed field. See [`config/entities/run/detail.ts`](https://github.com/michelangelo-ai/michelangelo/blob/45d238f458785d5badf866ab7e3641737f12d3a5/javascript/packages/core/config/entities/run/detail.ts#L39-L51).

### Composite Display

Show multiple related fields together:

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

Useful for showing primary/secondary information. Works in list views and detail view metadata. See [`config/entities/run/shared.ts`](https://github.com/michelangelo-ai/michelangelo/blob/45d238f458785d5badf866ab7e3641737f12d3a5/javascript/packages/core/config/entities/run/shared.ts#L11-L24).

## Next Steps

### Explore Existing Configurations

The best way to learn is to explore working examples:

- **Phases**: [`javascript/packages/core/config/phases/`](https://github.com/michelangelo-ai/michelangelo/tree/main/javascript/packages/core/config/phases)
  - `train.ts`, `deploy.ts`, `data.ts` - Complete phase configurations

- **Entities**: [`javascript/packages/core/config/entities/`](https://github.com/michelangelo-ai/michelangelo/tree/main/javascript/packages/core/config/entities)
  - `run/`, `pipeline/`, `trigger/` - Complete entity configurations with list and detail views

- **Type Definitions**: `javascript/packages/core/types/common/studio-types.ts`
  - `PhaseConfig`, `PhaseEntityConfig` - TypeScript interfaces

- **View Types**: `javascript/packages/core/components/views/types.ts`
  - `ListViewConfig`, `DetailViewConfig` - View configuration interfaces

### Reference Documentation

Detailed API references for each configuration type:

- [Cell Types Reference](cell-type-reference)
- [Table Configuration Reference](table-configuration-reference)
- [Phase Configuration Reference](phase-configuration-reference)
- [Entity Configuration Reference](entity-configuration-reference)

### FAQ

**Q: How do I version control configuration changes?**
A: Configurations are code - commit them to your repository alongside component code.

**Q: Can I mix configuration and custom components?**
Yes! The configuration system provides escape hatches for injecting custom components. For example, detail views support a `custom` page type:

```typescript
export const MODEL_DETAIL_CONFIG: DetailViewConfig = {
  type: 'detail',
  metadata: [...],
  pages: [
    // Standard configuration-driven pages
    { type: 'table', id: 'runs', label: 'Training Runs', tableConfig: {...} },
    { type: 'execution', id: 'steps', label: 'Steps', tasks: {...} },

    // Custom component for unique visualization
    {
      type: 'custom',
      id: 'metrics',
      label: 'Metrics Dashboard',
      component: MetricsVisualization
    }
  ],
};
```

Your custom component receives the data and loading state:

```typescript
function MetricsVisualization({ data, isLoading }: {
  data: Model | undefined;
  isLoading: boolean
}) {
  // Your custom visualization logic
  return <YourCustomUI />;
}
```

**Q: How do I handle complex business logic?**
A: Use accessor functions for simple computed values. For complex logic, write a custom component and integrate it into the configuration system.
