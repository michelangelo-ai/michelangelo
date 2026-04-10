Flags shared variable declarations (object/array literals, `buildWrapper()` calls, wrapper helper functions) at module scope in test files. Treats `describe()` scope the same as module scope.

## Why describe scope = module scope

Most test files wrap everything in a top-level `describe`. A `const defaultProps = {...}` inside that describe is shared by every test in the file -- the same invisible coupling problem as module scope. The `describe` is structural, not a meaningful scope boundary.

This applies to all describe nesting levels, including `describe.each()`.

## The recommended pattern for shared preconditions

When tests genuinely share a precondition (e.g., "all these tests render a disabled phase"), use `beforeEach` with `render()` and query via `screen`:

```tsx
describe('DISABLED phases', () => {
  beforeEach(() => {
    render(<Phase disabled />);
  });

  it('shows disabled state', () => {
    expect(screen.getByText('Disabled')).toBeInTheDocument();
  });

  it('prevents submission', () => {
    expect(screen.getByRole('button')).toBeDisabled();
  });
});
```

The `describe` name tells you the precondition. `beforeEach` does the expensive side-effectful thing (render). Each test queries `screen` directly. No shared mutable data.

## Anti-pattern: `let` + `beforeEach` for data

```tsx
// Don't do this -- trades one smell for another
describe('DISABLED phases', () => {
  let props;
  beforeEach(() => {
    props = { disabled: true };
  });
  it('renders', () => {
    render(<Phase {...props} />);
  });
});
```

This replaces shared constants with shared mutable references. For plain data (props, options, config objects), inline it per test instead.

## When to inline

For plain data -- props objects, option arrays, config literals -- inline per test:

```tsx
describe('DISABLED phases', () => {
  it('shows disabled state', () => {
    render(<Phase disabled />);
    expect(screen.getByText('Disabled')).toBeInTheDocument();
  });

  it('prevents submission', () => {
    render(<Phase disabled />);
    expect(screen.getByRole('button')).toBeDisabled();
  });
});
```

Each test is a complete, readable unit. The duplication cost is low (one line), and each test declares its own preconditions.

## When `beforeEach` earns its place

- `render()` / `renderHook()` -- side-effectful, benefits from centralized cleanup
- Mock setup (`vi.fn()`, `vi.spyOn()`) that needs `.mockClear()` between tests
- DOM setup that requires symmetric mount/unmount

Not for plain data.

## When to suppress

Genuine domain constants shared across tests (not setup data):

```ts
// eslint-disable-next-line local/no-module-scope-test-setup
const COLUMN_DEFINITIONS = [...]; // domain schema, not test setup
```
