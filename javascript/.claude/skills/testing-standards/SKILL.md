---
name: Testing standards
description: 'Trigger when testing is any part of the task — writing tests, adding coverage, adding test cases, or implementing code that will need tests. Project-specific mocking rules that OVERRIDE standard defaults.'
user-invocable: false
---

# Testing Patterns

## What to Test

- **User-facing behavior** — `screen.getByText()`, `screen.getByRole()`, not `container.firstChild`
- **Business logic** — test your decisions, not the tools implementing them
- **Success path + key edge cases** — skip scenarios already covered by dependencies
- **No duplication** — if higher-level tests cover the behavior, don't also test the implementation details

## Mocking Strategy

| Mock                                          | Don't mock                                  |
| --------------------------------------------- | ------------------------------------------- |
| External APIs, RPC calls, server dependencies | Internal hooks and components               |
|                                               | React context                               |
|                                               | Well-tested utilities                       |
|                                               | Dependencies already comprehensively tested |

## Test Organization

- Place tests in the closest `__tests__/` directory to the tested code (create it if it does not exist)
- Use `describe` blocks only when tests benefit from grouping (shared setup, related assertions)
- Keep tests flat when individual test names provide enough context
- Prefer `expect(fn(args)).toEqual(result)` over assigning to intermediate variables

## Anti-Patterns

- ❌ Testing implementation details (`container.firstChild`, verifying props passed to children)
- ❌ Duplicating behavior across unit and integration tests — pick the right level
- ❌ Testing scenarios already covered by the dependency being used
- ❌ Module-level constants for test setup — don't define wrappers, props, or component configurations at the module level. Inline everything per test:

```typescript
// ❌ Avoid
const wrapper = buildWrapper([getBaseProviderWrapper()]);
const OPTIONS = [{ value: 'a', label: 'Option A' }];
it('renders', () => { render(<Foo options={OPTIONS} />, wrapper); });

// ✅ Prefer
it('renders', () => {
  render(
    <Foo options={[{ value: 'a', label: 'Option A' }]} />,
    buildWrapper([getBaseProviderWrapper()])
  );
});
```
