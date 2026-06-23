# require-handler-prefix

## What this rule enforces

Locally defined functions passed as handlers to `on*` JSX props must use the `handle*` prefix.

```tsx
// ✗ Bad — toggleMenu doesn't signal it's an event handler
const toggleMenu = () => setOpen(true);
<Button onClick={toggleMenu} />

// ✓ Good — handleMenuOpen makes the role clear
const handleMenuOpen = () => setOpen(true);
<Button onClick={handleMenuOpen} />
```

## Why

The `handle*` prefix is the React convention for event handler implementations. It distinguishes "this function responds to an event" from general utility functions. Without it, a reader has to trace the function to understand its role.

## What is exempt

**Passthrough props** — a value forwarded directly from the component's own props is already named at the call site where the handler was defined. Requiring `handle*` on the receiving end would force an alias with no information gain.

```tsx
// ✓ Exempt — onClick was named handleX by the caller
function FilterOption({ onClick }: Props) {
  return <Button onClick={onClick} />;
}
<FilterOption onClick={handleFilterChange} />
```

This includes props destructured from a `props` object in `forwardRef` components:

```tsx
// ✓ Exempt — onClose is forwarded from props
const C = forwardRef((props, ref) => {
  const { onClose } = props;
  return <Modal onEsc={onClose} />;
});
```

**Test files** — this rule does not apply to test files. Mocks are often named after the prop they stand in for (`const onToggle = vi.fn()`) to make the connection to the component interface explicit.

## Relationship to no-handler-mirror

These two rules work together:

- `no-handler-mirror` — prevents names that mirror the prop without adding context (`handleChange` for `onChange`, `onClick` for `onClick`)
- `require-handler-prefix` — requires that locally defined handlers start with `handle*`

Together they ensure handlers are named `handle` + something descriptive.
