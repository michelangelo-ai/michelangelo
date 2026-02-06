# Phase Configuration Reference

Phases group entities by ML lifecycle stage, providing structure to the Michelangelo UI workflow. Each phase represents a step in the machine learning operations lifecycle (e.g., data preparation, training, deployment, monitoring).

**Phases define:**
- URL routing structure (`/train`, `/deploy`, etc.)
- Navigation organization in the UI
- Which entities belong to each workflow stage
- Phase availability (active, coming soon, disabled)

## PhaseConfig Interface

| Property | Type | Description | Required |
|----------|------|-------------|----------|
| `id` | `string` | Unique identifier for the phase, used in URL routing | ✅ Yes |
| `name` | `string` | Display name shown in navigation | ✅ Yes |
| `icon` | `string` | Icon name from the application's icon provider system | ✅ Yes |
| `state` | `PhaseState` | Controls phase availability and behavior | ✅ Yes |
| `entities` | `PhaseEntityConfig[]` | List of entities that belong to this phase | ✅ Yes |
| `description` | `string` | Optional descriptive text explaining what this phase does | No |
| `docUrl` | `string` | Optional URL to external documentation for this phase | No |

## Phase States

The `state` property controls overall phase behavior and appearance:

| State | Description | Use When |
|-------|-------------|----------|
| `active` | Phase is fully functional and can be interacted with | Phase is implemented and ready for users |
| `comingSoon` | Phase is not yet available but will be in the future | Feature is planned but not implemented |
| `disabled` | Phase is not available and cannot be interacted with | Feature is temporarily disabled or deprecated |

## Property Details

### `id`
- **Must be unique** across all phases
- Used in URL routing: `/${id}`
- Convention: lowercase, hyphenated (e.g., `train`, `deploy`, `genai-finetune`)

### `name`
- Displayed in navigation and page headers
- Can include special characters and formatting
- Examples: `"Train & Evaluate"`, `"Prepare & Analyze Data"`, `"Deploy & Predict"`

### `icon`
- Icon name from your application's icon provider
- Examples: `'chartLine'`, `'deploy'`, `'database'`
- Icons are registered in your icon system

### `description`
- Optional help text
- Explains the purpose of this phase

### `docUrl`
- Optional link to external documentation

### `entities`
- Array of entity configurations (see [Entity Configuration Reference](./entity-configuration-reference))
- Order in array affects display order in navigation

## Source Files

**Type definitions:** [javascript/packages/core/types/common/studio-types.ts](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/types/common/studio-types.ts) - `PhaseConfig` interface, `PhaseState` type

**Real examples:** [javascript/packages/core/config/phases](https://github.com/michelangelo-ai/michelangelo/tree/main/javascript/packages/core/config/phases)

## Related Documentation

- [Entity Configuration Reference](./entity-configuration-reference) - Configure entities within phases
- [Configuration API](./configuration-api) - overview of configuration system
