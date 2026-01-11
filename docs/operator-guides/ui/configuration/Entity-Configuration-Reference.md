## overview

Entities define data models and their properties within a phase. Each entity represents a type of object in your ML platform (pipelines, runs, models, datasets, etc.) and specifies how that object is displayed and interacted with in the UI.

**Entities define:**
- URL routing within a phase (`/<phase>/runs`, `/<phase>/pipelines`)
- Available views for the entity (list view, detail view, forms)
- Connection to backend services (protobuf service mapping)
- Entity availability (active, disabled)

## PhaseEntityConfig Interface

| Property | Type | Description | Required |
|----------|------|-------------|----------|
| `id` | `string` | Unique identifier within the phase, used in URL routing | ✅ Yes |
| `name` | `string` | Display name for the entity (plural, lowercase recommended) | ✅ Yes |
| `service` | `string` | Name of the protobuf service this entity maps to | ✅ Yes |
| `state` | `PhaseEntityState` | Controls entity availability | ✅ Yes |
| `views` | `ViewConfig[]` | Array of view configurations (list, detail, form) | ✅ Yes |

## Entity States

The `state` property controls individual entity behavior within a phase:

| State | Description | Use When |
|-------|-------------|----------|
| `active` | Entity is fully functional and can be interacted with | Entity is implemented with views configured |
| `disabled` | Entity is not available and cannot be interacted with | Entity is planned but views not yet implemented |

## Example

```typescript
// From: config/entities/run/run.ts
export const RUN_ENTITY_CONFIG: PhaseEntityConfig = {
  id: 'runs',
  name: 'Pipeline Runs',
  service: 'pipelineRun',
  state: 'active',
  views: [RUN_LIST_CONFIG, RUN_DETAIL_CONFIG],
};
```

**See also:** [config/entities/](https://github.com/michelangelo-ai/michelangelo/tree/main/javascript/packages/core/config/entities) for other entity examples

## Property Details

### `id`
- **Must be unique** within the phase
- Used in URL routing: `/${phase}/${id}`
- Convention: plural, lowercase, hyphenated (e.g., `runs`, `pipelines`, `feature-groups`)

### `name`
- Display name shown in navigation and headers
- Recommended: plural, descriptive
- Examples: `"Pipeline Runs"`, `"Pipelines"`, `"Trained Models"`
- Can be intentionally not pluralized for special cases (e.g., `"Feature Consistency"`)

### `service`
- Maps to the root protobuf field name of the backend service
- **Must match exactly** for RPC queries to work correctly
- Examples:
  - `'pipeline'` → `PipelineService`, query: `ListPipeline`
  - `'pipelineRun'` → `PipelineRunService`, query: `ListPipelineRun`
  - `'triggerRun'` → `TriggerRunService`, query: `ListTriggerRun`

### `views`
- Array of view configurations defining how the entity is presented

## View Configuration

Views define how entities are presented. See [Cell Types Reference](./cell-type-reference) for column/field configuration.

- **List views**: `config/entities/*/list.ts` - Table columns with cells
- **Detail views**: `config/entities/*/detail.ts` - Metadata header + content pages

## Common Patterns

**Organize entity configs by file:**
```
config/entities/run/
├── run.ts      # Main entity config
├── list.ts     # List view config
├── detail.ts   # Detail view config
└── shared.ts   # Shared configs
```

**Share cell configs across views:**
```typescript
// shared.ts - define once
export const SHARED_RUN_CELL_CONFIG: Cell[] = [...];

// Reuse in list.ts and detail.ts
import { SHARED_RUN_CELL_CONFIG } from './shared';
```

## Service Mapping

The `service` property maps to your protobuf service definition:

```protobuf
// Example protobuf service
service PipelineRunService {
  rpc ListPipelineRun(ListPipelineRunRequest) returns (ListPipelineRunResponse);
  rpc GetPipelineRun(GetPipelineRunRequest) returns (PipelineRun);
}
```

```typescript
// Entity config
{
  id: 'runs',
  name: 'Pipeline Runs',
  service: 'pipelineRun', // ← Must match root field name
  // ...
}
```

The configuration system uses this mapping to:
- Construct RPC query names (`ListPipelineRun`)
- Access the correct service endpoints
- Handle data fetching for views

## Source Files

**Type definitions:** [javascript/packages/core/types/common/studio-types.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/types/common/studio-types.ts) - `PhaseEntityConfig` interface, `PhaseEntityState` type

**View configurations:** [javascript/packages/core/components/views/types.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/components/views/types.ts) - `ListViewConfig`, `DetailViewConfig` interfaces

## Related Documentation

- [Phase Configuration Reference](phase-configuration-reference) - Configure phases that contain entities
- [Cell Types Reference](./cell-type-reference) - Configure columns and fields in views
- [Configuration API](./configuration-api) - overview of configuration system
