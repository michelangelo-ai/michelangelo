# no-handler-mirror

## What this rule enforces

Event handler names passed as JSX props must add context beyond the event type. A name that merely mirrors the prop tells the reader nothing about what is being handled.

This rule works in tandem with `react/jsx-handler-names`, which requires all handlers to begin with `handle`. Together they enforce: the name must carry the `handle` prefix **and** go beyond repeating the event type.

## Flagged patterns

```tsx
// ❌ mirrors the prop name exactly
<Button onClick={onClick} />

// ❌ "handle" + bare event name — no context added
<RadioGroup onChange={handleChange} />
<Select onChange={handleChange} />

// ❌ "handle" + full prop name — still no context
<Form onClick={handleOnClick} />
```

## Correct patterns

```tsx
// ✓ "handle" prefix with a descriptive suffix
<RadioGroup onChange={handleSelectionChange} />
<Select onChange={handleCommitSelection} />
<Form onSubmit={handleFormSubmit} />

// ✓ member expression pass-through — explicitly forwarded
<Child onClick={props.onClick} />
```

## Naming guidance

Name the **effect**, not the trigger. The `handle` prefix is required (enforced by `react/jsx-handler-names`), and the suffix should describe what the handler does to application state.

| Context                                 | Instead of     | Use                     |
| --------------------------------------- | -------------- | ----------------------- |
| Radio coerces DOM string to typed value | `handleChange` | `handleSelectionChange` |
| Select maps option objects to typed IDs | `handleChange` | `handleCommitSelection` |
| Checkbox stops propagation + toggles    | `handleChange` | `handleToggleSelection` |
| Form submit that also closes the dialog | `handleSubmit` | `handleFormSubmit`      |
| Row delete on click                     | `handleClick`  | `handleRowDelete`       |

The name should answer "what does this function do to the application state?" — not "what event is it responding to?"

## Prop forwarding

When a component receives a callback prop and forwards it directly to a child **without adding any logic**, the pattern is a pass-through and does not need renaming:

```tsx
// ✓ pass-through — no intermediate logic, auto-detected by the rule
const FilterOption = ({ onClick }: Props) => <Item onClick={onClick} />;
```

The rule detects pass-throughs via scope analysis — if the value identifier is a function parameter, it is exempt. Where the rule cannot detect it (e.g. `const { onClick } = props`), use direct parameter destructuring instead.
