---
name: Testing standards
description: 'Trigger when testing is any part of the task — writing tests, adding coverage, adding test cases, or implementing code that will need tests. Project-specific mocking rules that OVERRIDE standard defaults.'
user-invocable: false
---

# Testing Patterns

## What to Test

- **User-facing behavior** — test what users see and interact with, not internal component structure
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

## Querying Elements

Query priority: `getByRole` → `getByLabelText` → `getByText` → `getByTestId`

**Only use `getByTestId` if the element has no semantic role** (spinners, skeletons, custom visual elements) **or is a mock component**. Every other use of `getByTestId` is a bug — fix it by using an accessible query or by adding `aria-label`/`role` to the component under test.

```typescript
// ❌ Wrong — element has a role
screen.getByTestId('submit-button');
// ✅ Fix
screen.getByRole('button', { name: /submit/i });

// ❌ Wrong — element has text content
screen.getByTestId('error-message');
// ✅ Fix
screen.getByText(/something went wrong/i);

// ✅ Correct — spinner has no semantic role
screen.queryByTestId('loading-spinner');

// ✅ Correct — testId is on the mock itself, not the real component.
// The real MetricChart has no accessible role worth testing here;
// the test cares only that the parent rendered it with the right name.
vi.mock('../MetricChart', () => ({
  MetricChart: ({ name }: { name: string }) => (
    <div data-testid={`metric-chart-${name}`} />
  ),
}));
// ...
screen.getByTestId('metric-chart-accuracy');
```

## Anti-Patterns

- ❌ Testing implementation details (`container.firstChild`, verifying props passed to children)
- ❌ Duplicating behavior across unit and integration tests — pick the right level
- ❌ Testing scenarios already covered by the dependency being used
