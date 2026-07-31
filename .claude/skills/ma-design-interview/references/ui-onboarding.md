# UI Onboarding — Interview References

Domain-specific question tree for `/design-interview` when the goal is exposing a backend
entity in the Studio UI. The skill loads this file to determine what questions to ask beyond
what the codebase can answer directly.

Two rendering surfaces can be onboarded: **columns/metadata** and **forms** (create/update).
Columns and metadata are pure configuration — mapping proto field paths to cell renderers.
Forms use field and layout components as a pseudo-configuration language.

---

## Columns & metadata

List view columns and detail view metadata use the same `ColumnConfig` shape:
`{ id, label, type?, url?, tooltip? }`. The `id` is a dot-notation path into the API
response (e.g., `metadata.creationTimestamp.seconds`). The `type` is a `CellType` enum
value `javascript/packages/core/components/cell/constants.ts`

The difference is placement: list columns appear in the table, detail metadata appears
as key-value pairs in the detail header. Detail metadata typically includes more fields
than the list view since there's more space.

### Questions

1. **Which proto fields should appear as columns/metadata?** Recommend based on what a
   user scanning a list would need. Typically: name (with link to detail), created/upadted date,
   type/kind, state. Consider searchability for list columns and properties that helps uniquely
   identify data.

2. **What CellType for each entry?** The skill should recommend based on the proto field type:
   - `string` → TEXT (default, can omit `type`)
   - Timestamp → DATE
   - Enum → TYPE or STATE (STATE for lifecycle enums with color coding)
   - `bool` → BOOLEAN
   - `map<string, string>` → MAP
     Name columns typically omit `type` and instead provide a `url` for navigation.

3. **Should any entry link somewhere?** Name columns typically link to the detail view.
   Resource identifiers should link to their own detail views. URLs and other dynamic
   values use the interpolation engine — see `javascript/packages/core/ARCHITECTURE.md` or
   `javascript/packages/core/utils/interpolation/` for available context variables (`studio`, `data`,
   `row`, `page`). Example: `/${studio.projectId}/${studio.phase}/entities/${data.metadata.name}`.

4. **Ordering?** name first, then descriptive fields, state/status last.

---

## Forms

Forms compose field components (`javascript/packages/core/components/form/fields/`) and layout
components (`javascript/packages/core/components/form/layout/`).

### Per-field questions

#### Identity & intent

6. **Is this field user-facing?** Some proto fields are internal (controller-managed status,
   ownerReferences, resourceVersion). Check if the API hook or controller overwrites the
   field — if so, it's read-only even if the proto allows setting it.

7. **Is this field editable on create? On update? Both?** Example: `metadata.name` is the
   unique identifier for a resource and is immutable

8. **What is the display label?** Recommend converting the proto field name to sentence case
   (`notification_type` → "Notification type").

9. **Does this field have a default value?** Where does it come from?

#### Validation

The skill should present what it found in proto validation, API hooks, and the built-in
validator inventory (`javascript/packages/core/components/form/validation/validators.ts`) before
asking.

Built-in validators: `required()`, `min(n)`, `max(n)`, `minLength(n)`, `maxLength(n)`,
`regex(pattern)`, `url()`. Compose with `combineValidators()`.

10. **Is this field required?** Required for operation is different than required for
    protobuf compatibility. Present what the proto and API hooks enforce, recommend the
    corresponding built-in validators.

11. **Are there format constraints beyond what validators cover?** Email fields need email
    validation (`regex()`). Slack destinations might need channel format validation.
    Check whether a built-in validator handles it before asking.

12. **Are there cross-field dependencies?** Example: if `notification_type` is EMAIL, then
    `emails` is required and `slack_destinations` is irrelevant. These become conditional
    rendering or conditional validation rules.

#### Grouping & layout

13. **Does this field belong to a logical group?** For forms with many fields, group
    related fields using `FormGroup` (collapsible sections) or `FormRow` (side-by-side).

14. **What is the field ordering?** Required fields should appear before optional ones.
    Frequently-used fields before rarely-used ones. Ask the contributor what their users
    care about most.

### Repeated fields

When a field is `repeated` (e.g., `repeated Notification notifications`), the form system
already provides `ArrayFormGroup` and `ArrayFormRow` layout components. These handle
add/remove, auto-numbering, and per-item rendering out of the box.

The skill should recommend the appropriate component based on the nested message complexity:

- **Repeated scalar or simple message (1-3 fields)** → `ArrayFormRow` — compact inline rows
- **Repeated message with 4+ fields** → `ArrayFormGroup` — collapsible numbered sections
  with `groupLabel` for auto-titling (e.g., "Notification 1", "Notification 2")

Questions to ask only when the defaults don't fit:

15. **Is there a practical maximum?** If unbounded in the proto, should the UI suggest a
    limit? (This is uncommon — usually the array layout handles it fine.)

16. **What is the empty state?** No items → show an "Add" button immediately, or hide the
    section until the user opts in? Default: always visible with an add button.

### The custom component decision

Ask this after all fields are understood:

17. **Can the entire form be expressed with existing field and layout components?**

The threshold:

- Standard field type covers it → use it (self-service)
- Dynamic show/hide based on another field → can be handled with conditional rendering
  in the form component (still self-service)
- Repeated messages → `ArrayFormGroup` / `ArrayFormRow` (self-service)
- Complex multi-step wizard or non-standard interaction → custom form component

If everything maps to existing components, this is fully self-service. If custom work is
needed, the generated prototype should be as high-fidelity as possible for handoff.

### Success behavior

18. **What toast message should appear after a successful create/update?**

These configure `successOperations` — see `javascript/packages/core/components/actions/use-success-operations.tsx`
for the available operation types (invalidate, toast, route).
