# no-event-handler-prefix

## What this rule enforces

Event handler names passed as JSX props must describe *what they do*, not just the event type. A name that mirrors the prop adds no information for the reader.

## Flagged patterns

```tsx
// ❌ mirrors the prop name exactly
<Button onClick={onClick} />

// ❌ "handle" + event name — still no context
<RadioGroup onChange={handleChange} />
<Select onChange={handleChange} />

// ❌ "handle" + full prop name
<Form onClick={handleOnClick} />
```

## Correct patterns

```tsx
// ✓ names the effect, not the trigger
<RadioGroup onChange={persistSelection} />
<Select onChange={commitSelection} />
<Checkbox onChange={applyToggle} />
<Form onSubmit={submitAndClose} />

// ✓ member expression pass-through — explicitly forwarded
<Child onClick={props.onClick} />
```

## Naming guidance

Name the **effect**, not the trigger. Drop the `handle` prefix when the function describes a domain action.

| Context | Instead of | Use |
|---------|-----------|-----|
| Radio coerces DOM string to typed value | `handleChange` | `persistSelection` |
| Select maps option objects to typed IDs | `handleChange` | `commitSelection` |
| Checkbox stops propagation + toggles | `handleChange` | `applyToggle` |
| Form submit that also closes the dialog | `handleSubmit` | `submitAndClose` |
| Confirm dialog executes mutation/route | `onConfirm` | `executeAction` |
| Row delete on click | `handleClick` | `deleteRow` |

The name should answer "what does this function do to the application state?" — not "what event is it responding to?"

## Prop forwarding

When a component receives a callback prop and forwards it directly to a child **without adding any logic**, the pattern is a pass-through and does not need renaming:

```tsx
// ✓ pass-through — no intermediate logic, auto-detected by the rule
const FilterOption = ({ onClick }: Props) => (
  <Item onClick={onClick} />
);
```

The rule detects pass-throughs via scope analysis — if the value identifier is a function parameter, it is exempt. Where the rule cannot detect it (e.g. `const { onClick } = props`), use direct parameter destructuring instead.
