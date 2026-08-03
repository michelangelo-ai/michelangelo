---
name: ma-design-interview
description: >
  Structured interview for designing and implementing changes to the Michelangelo platform.
  Auto-discovers proto schemas, API hooks, controllers, and existing configuration, then
  asks targeted questions to reach shared understanding before generating code. Domain-specific
  question trees live in references/ files. Use when the user says "design interview",
  "grill me", or when a task benefits from structured discovery before implementation.
---

# /design-interview — Structured Design Interview

Interview a contributor about a design task — from backend entity onboarding to UI
configuration to API design. Reads the codebase first, asks questions second, generates
code at the end. Domain-specific guidance is loaded from references files.

Interview discipline derived from [mattpocock/skills](https://github.com/mattpocock/skills) (MIT License).

## Interview rules

1. **One question at a time.** Never batch. Each answer informs the next question.
2. **Recommend an answer for every question.** Propose what you'd do and why, then confirm.
3. **Explore before asking.** If the codebase can answer it, don't ask the contributor.
4. **Resolve dependencies in order.** When decision A constrains decision B, settle A first.
5. **Stop only at shared understanding.** Continue until every branch of the design tree has a resolved answer.

---

## Phase 1: Context Discovery

Start by understanding what the contributor wants to do. The input can range from broad
("onboard X to the UI") to narrow ("add Y fields to the Z form").

### Resolve resource names before discovery

Before launching any discovery, resolve every resource-like noun in the input to an
actual proto file. Run `ls proto/api/v2/*.proto` and match the input against filenames.

Examples of why this matters:
- "create a trigger run for a pipeline" → `trigger_run.proto` + `pipeline.proto`, NOT
  `pipeline_run.proto`. The input contains "run" and "pipeline" but the resource is
  TriggerRun, not PipelineRun.
- "add notifications to pipeline runs" → `pipeline_run.proto` + `notification.proto`.

If the input maps cleanly to one or more proto files, proceed with those. If the mapping
is ambiguous (e.g., multiple plausible resources, or no proto file matches), ask one
scoping question to resolve.

### Determine scope

Once the resource(s) are resolved:

- **New entity**: resource name is sufficient (e.g., "Deployment"). Discover everything.
- **New surface on existing entity**: resource name + which surface (list, detail, form).
- **Extend existing surface**: resource name + which fields to add. This is critical —
  the fields being added may reference a _different_ proto message (e.g., adding
  `notifications` to PipelineRunSpec, where `Notification` is defined in
  `notification.proto`). The skill must follow those references and discover the
  referenced message's schema too.

### 1a. Discover schema artifacts

From the resource name (and any referenced messages), locate:

| Artifact         | Convention                                      | What to extract                                                    |
| ---------------- | ----------------------------------------------- | ------------------------------------------------------------------ |
| Proto definition | `proto/api/v2/<snake_case>.proto`               | Fields, types, enums, repeated fields, nested messages             |
| API hooks        | `go/components/<lowercase>/apihook/`            | BeforeCreate/BeforeUpdate validation — business rules beyond proto |
| Proto validation | `proto-go/api/v2/<snake_case>.pb.validation.go` | Generated field constraints                                        |
| Controller       | `go/components/<lowercase>/controller.go`       | Side effects on create/update                                      |

If any path doesn't resolve, flag it — that's a directory structure problem, not something
to work around.

### 1b. Discover existing UI config

Search `packages/core/config/entities/` and `app/config/` for existing configuration
matching the resource name. Determine the scope:

- **Existing form found** → extend mode. Read the form component, entity config, and TS type.
- **Existing list/detail config found** → extend mode. Read the column/metadata configs.
- **Entity config exists but missing a surface** → add the missing surface.
- **Nothing found** → new entity. Will need `PhaseEntityConfig`, views, and form.

Tell the contributor what you found and confirm.

### 1c. Discover available UI components

Read these directories at runtime to build an inventory of what's already available:

| Directory                                               | What it provides                                                                     |
| ------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| `javascript/packages/core/components/form/fields/`      | Field components (string, select, boolean, date, etc.)                               |
| `javascript/packages/core/components/form/layout/`      | Layout components (FormGroup, FormRow, ArrayFormGroup, ArrayFormRow, FormGrid, etc.) |
| `javascript/packages/core/components/form/validation/`  | Built-in validators (required, min, max, minLength, maxLength, regex, url)           |
| `javascript/packages/core/components/cell/constants.ts` | CellType enum — available column/metadata renderers                                  |

**Read the `types.ts` file for each component, not just the directory listing.** The prop
types determine what each component can do. Listing names without reading props leads to
underusing what's already available or reinventing behavior that a prop already handles.

This inventory determines what can be built with existing components vs what needs custom work.

### 1d. Build a field inventory

Scope depends on the task:

- **New entity or new surface**: inventory all spec fields from the proto definition.
- **Extending an existing surface**: inventory only the fields being added. If the new
  fields reference another proto message, inventory that message's fields instead.

For each field, capture: name, type, whether required, enum values (if applicable), and
whether repeated. This table drives the interview.

---

## Phase 2: Interview

Present what you discovered in Phase 1, then ask only about things the codebase can't answer.

Load the appropriate references file for the domain being discussed. The references file
defines the question tree — what to ask, in what order, and what answers the codebase can
provide vs what requires contributor input.

See `references/ui-onboarding.md` for UI configuration interviews.

---

## Phase 2.5: Design Alternatives

When the interview surfaces a decision between meaningfully different approaches, don't
ask the contributor to choose in the abstract. Build the alternatives and let them react
to something concrete.

### Trigger

Enter this phase when a Phase 2 question involves the **shape** of the solution — not
a detail within a settled shape.

Triggers: "one thing or many?", "flat or nested?", "explicit or convention-based?",
"user-configured or hardcoded?", "separate resources or inline?"

Does not trigger: "required or optional?", "what label?", "which renderer?"

### Process

1. Generate up to **3 alternatives**, each labeled by its tradeoff posture — not
   "Option 1/2/3". The name should tell the contributor what they're optimizing for.
2. Present them for comparison. For UI work with a running dev server, screenshot each
   one. For backend or API design, present the alternatives inline.
3. The contributor picks one, or asks to revisit a specific alternative.
4. **No new interview questions** during this phase. If the alternatives surface new
   questions, loop back to Phase 2.

### Output

The chosen alternative becomes the input to Phase 3.

---

## Phase 3: Generation

Generate the highest-quality implementation possible. The goal is a working prototype on a
branch that either ships as-is (self-service) or gets refined by a frontend engineer.

Forms are currently component-driven in the OSS repository. The field and layout components
serve as a pseudo-configuration language — generation output is React components that compose
these primitives.

### For extending an existing surface

- **Adding columns**: Add `ColumnConfig` entries to the existing list/detail config.
  This is pure data — map proto field paths to CellType values.
- **Adding detail metadata**: Same pattern as columns — `ColumnConfig` entries in the
  detail view's `metadata` array.
- **Adding form fields**: Add field components and layout to the existing form.
  Update the entity's TypeScript type to include the new fields.

### For new entity forms

- Form component using discovered field/layout components from `javascript/packages/core/components/form/`
- Validation using built-in validators from `javascript/packages/core/components/form/validation/`
- Any custom field components identified during the interview

### For new entities (full config)

- `PhaseEntityConfig` with service binding
- View configs (list with columns, detail with metadata and pages)
- Form configs as above
- Wire into the appropriate phase config
