# Config-Driven Form: Studio-Web Mergeback Compatibility Analysis

Analysis of whether OSS `packages/core`'s config-driven form system can serve as a
drop-in replacement for studio-web's `generic-form` engine, based on 58 form view
declarations across 13 entity config directories.

**Verdict: Architecturally compatible — no blockers.** Every studio-web capability has
a defined path in the new system.

---

## Action Items (implement in core)

### Add `immutable` to SharedFieldConfig

**68 usages.** Fields editable on create, disabled on update. `ConfigDrivenForm` receives
`formOperation` from view resolution. The field rendering layer checks
`config.immutable && formOperation === 'update'` and sets `disabled=true`.

### Add `virtual` to SharedFieldConfig

**37 usages.** Fields excluded from API submission (UI-only discriminators in oneof
patterns). The middleware/submission layer filters virtual fields before RPC.

### Expand `FieldValidator` to receive form state

~15 configs use `validation.validate(value, page)` where `page` is full form state.
Core's `FieldValidator` only receives `value`. The config-driven form's validation
adapter must inject form state via `useFormState().values`.

### Add `FormViewConfig` extensions

`withSteps`, `stickyFooter`, `action` (submit button customization), `valueAccessor`
(transform initial values). These are view-level properties, not schema-level — they
belong on `FormViewConfig`, not `FormConfig`.

### Group `actions` property

Studio's `GroupLayoutT` has `actions?: ActionSchemaT[]` for inline action buttons
within a group header (e.g., "Add feature"). Either add to core's built-in group
layout or let consumer override the group layout renderer.

---

## Accepted Mergeback Divergences

### `repeated` → `multi` rename

**79 usages.** Studio uses `repeated: true` for multi-value primitive fields. Core
already has `multi: boolean` on StringField — same concept, different name. The
consumer adapter maps `repeated` → `multi` in field config. We accept this naming
divergence rather than adding a redundant `repeated` property.

### `name` → `label` rename

Studio uses `name` for field labels and group titles. Core uses `label`. Mechanical
find-replace in consumer adapter. Universal but trivial.

### Condition variants

Studio has 5 condition discriminants (`is`, `isNot`, `isEmpty`, `containsAny`,
`Interpolatable<boolean>`). Core uses `Interpolatable<boolean>` only. The consumer
registers a condition layout renderer that handles all 5 variants internally —
existing configs work without modification.

---

## Extension Model Coverage

### 14 field types → `FieldConfigExtensions` + `FormProvider` renderers

| Type | Complexity | Notes |
|------|------------|-------|
| `code` | Medium | Language, dataFormat, autocomplete, height |
| `list` | Medium | Async (queryConfig) + sync (options) variants |
| `table` | High | Full table-select with queryConfig, columns, multi |
| `custom` | High | Arbitrary React component — escape hatch |
| `asl` | Low | ASL workflow editor |
| `struct` | Low | JSON struct editor |
| `card` | Medium | queryConfig + items (RowCell[]) |
| `file` | Low | accept, maxFileSize, multiple |
| `terrablobFile` | Medium | Uber-specific Terrablob upload |
| `hive` | Low | Hive table selector |
| `querypath` | Medium | queryPathIndex, sourceField interpolation |
| `targetPartitions` | Low | Uber-specific |
| `searchUOwn` | Low | Uber uOwn asset search |
| `metricList` | High | Full metric selection UI |

### 9 layout types → `LayoutConfigExtensions` + `FormProvider` layouts

| Type | Notes |
|------|-------|
| `condition` | Consumer handles 5 studio variants internally |
| `oneof` | Simple (auto-detect) + explicit (switchEntityId + valueToConfig) |
| `step` | Wizard steps — pairs with `withSteps` on FormViewConfig |
| `column` | Side-by-side with `sticky` flag |
| `section` | Thin wrapper, 1 usage |
| `tabs` | Tab-level conditions handled by consumer renderer |
| `banner` | title, kind, dismissible, content |
| `note` | content string only |
| `repeated` | RepeatedRowT / RepeatedGroupT, rootFieldPath, minItems |

### Consumer-only field properties (not in core)

| Property | Usages | Path |
|----------|--------|------|
| `isResourceIdentifier` | 27 | Consumer field wrapper adds parse/format |
| `searchable` | 15 | Pipeline-specific, flow through config bag |
| `instruction` | 5 | Maps to `labelEndEnhancer` |

---

## Migration Estimate

| Category | % of configs | What's needed |
|----------|-------------|---------------|
| Mechanical adapter | ~70% | Register types, `name`→`label`, validation adapter |
| Structural adaptation | ~30% | Interpolated configs, nested oneof, FormViewConfig extensions |

One-time adapter layer makes ~70% of configs work. Complex forms (deployment, pipeline,
prompt, agent) need the interpolation + oneof + FormViewConfig work from PRs 4-5.
