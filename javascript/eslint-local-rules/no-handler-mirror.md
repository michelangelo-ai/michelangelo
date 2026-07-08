# no-handler-mirror

## What this rule enforces

Event handler names passed as JSX props must add context beyond the event type. A name that merely mirrors the prop tells the reader nothing about what is being handled.

This rule works in tandem with `require-handler-prefix`, which requires all locally defined handlers to begin with `handle`. Together they enforce: the name must carry the `handle` prefix **and** go beyond repeating the event type.

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

Name the **effect**, not the trigger — the suffix should describe what the handler does, not what event fired it.

## Prop forwarding

When a component receives a callback prop and forwards it directly to a child **without adding any logic**, the pattern is a pass-through and does not need renaming:

```tsx
// ✓ pass-through — no intermediate logic, auto-detected by the rule
const FilterOption = ({ onClick }: Props) => <Item onClick={onClick} />;
```

The rule detects pass-throughs via scope analysis — if the value identifier is a function parameter, it is exempt. Where the rule cannot detect it (e.g. `const { onClick } = props`), use direct parameter destructuring instead.
