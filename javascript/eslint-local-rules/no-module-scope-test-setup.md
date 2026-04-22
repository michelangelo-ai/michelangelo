Flags shared variable declarations at module scope in test files — object/array literals, `buildWrapper()` calls, and wrapper helper functions.

## Why this rule exists

Shared test data creates invisible coupling between tests. When `const defaultProps` lives at the top of a file, every test silently inherits it. This causes three problems:

1. **Fixtures grow to satisfy the most demanding test.** A simple "renders label" test inherits a 20-field props object because a sibling test needed all those fields. The reader can't tell which fields matter for _this_ test.

2. **Failures are harder to diagnose.** When a test fails, you have to scroll up to find the shared data, then figure out which parts of it are relevant.

3. **Shared state resists change.** Adding a field to `defaultProps` affects every test. Removing one might break tests that silently depended on it. The blast radius of a "simple" change is the entire file.

The fix is straightforward: each test declares exactly what it needs. Duplication is a feature — it makes each test a self-contained document of its preconditions.

## What counts as module scope

The outermost `describe()` in a test file is structural — it's just how files are organized. Variables declared there are shared across all tests, same as module scope.

**Nested describes are different.** A `describe('disabled state')` is a semantic group — tests inside it share a precondition by design. The rule allows shared state in nested describes.

## How to fix violations

### Inline per test

For plain data — props, options, config objects — inline directly:

```tsx
// Before: shared data, invisible coupling
describe('SelectField', () => {
  const options = [
    { id: 'low', label: 'Low' },
    { id: 'high', label: 'High' },
  ];

  it('renders options', () => {
    render(<SelectField options={options} />);
  });

  it('submits selected value', () => {
    render(<SelectField options={options} />);
  });
});

// After: each test declares its own preconditions
describe('SelectField', () => {
  it('renders options', () => {
    render(
      <SelectField
        options={[
          { id: 'low', label: 'Low' },
          { id: 'high', label: 'High' },
        ]}
      />
    );
  });

  it('submits selected value', () => {
    render(
      <SelectField
        options={[
          { id: 'low', label: 'Low' },
          { id: 'high', label: 'High' },
        ]}
      />
    );
  });
});
```

### Group into nested describes

When many tests share a precondition, create a nested describe. Use `beforeEach` for side-effectful setup like `render()`, and query with `screen`:

```tsx
describe('SelectField', () => {
  describe('disabled state', () => {
    beforeEach(() => {
      render(<SelectField disabled options={[{ id: 'low', label: 'Low' }]} />);
    });

    it('shows disabled indicator', () => {
      expect(screen.getByRole('combobox')).toBeDisabled();
    });

    it('prevents interaction', () => {
      expect(screen.getByRole('combobox')).toHaveAttribute('aria-disabled');
    });
  });
});
```

The describe name tells you the precondition. `beforeEach` renders it. Each test just asserts.

### Use a factory function

When tests exercise the same subject with mostly the same inputs, a factory function inside the `describe` block keeps each test explicit without repeating everything:

```tsx
describe('Foo', () => {
  const buildProps = (overrides: Partial<ComponentProps<typeof Foo>> = {}) => ({
    label: 'Default',
    disabled: false,
    ...overrides,
  });

  it('renders label', () => {
    render(<Foo {...buildProps()} />);
  });

  it('handles disabled state', () => {
    render(<Foo {...buildProps({ disabled: true })} />);
  });
});
```

The same pattern works for any function call — not just component props:

```ts
describe('processItem', () => {
  const buildItem = (overrides: Partial<Item> = {}): Item => ({
    id: 'item-1',
    status: 'active',
    ...overrides,
  });

  it('handles inactive items', () => {
    expect(processItem(buildItem({ status: 'inactive' }))).toBe(false);
  });
});
```

**Pass all varying props through the overrides parameter.** Do not spread the factory result and then add extra props — that defeats the purpose and hides what each test actually needs:

```tsx
// Wrong — props needed by some tests aren't visible in the factory call
render(<Foo {...buildProps()} disabled={true} />);

// Right — everything a test needs flows through overrides
render(<Foo {...buildProps({ disabled: true })} />);
```
