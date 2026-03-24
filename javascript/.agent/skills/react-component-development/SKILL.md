---
name: react-patterns
description: 'Trigger when implementing, designing, or modifying any React component or hook — including planning new components before any file is touched. Enforces styling and naming conventions that OVERRIDE default React patterns.'
user-invocable: false
---

# React Component Patterns

## Styling: useStyletron vs styled()

**Start with `useStyletron`. Extract to `styled()` when it earns it.**

```typescript
// Default — inline with useStyletron
const [css, theme] = useStyletron();
<div className={css({ display: 'flex', gap: theme.sizing.scale400 })}>
```

Extract to `styled()` when you find:

- 4+ CSS properties, computed values, or pseudo-selectors
- Used in 2+ places with clear semantic meaning
- Inline styles would make JSX hard to read

```typescript
export const TaskSeparator = styled('div', ({ $theme }) => ({
  height: '1px',
  backgroundColor: $theme.colors.borderOpaque,
  margin: `${$theme.sizing.scale600} 0`,
}));
```

**Name styled components semantically — never generically:**

- ❌ `Container`, `Card`, `Wrapper` (collide everywhere)
- ✅ `TaskSeparator`, `ExecutionMatrix`, `PipelineHeader`

## Event Handler Naming

Name handlers by intent, not by event type:

```typescript
// ❌
const handleOnMouseEnter = () => {};
const handleOnClick = () => {};

// ✅
const showTooltipAfterDelay = () => {};
const toggleMenu = () => {};
```

## Component Props Naming

- `Props` — single component in the file
- `ComponentNameProps` — multiple components in the file, or props are exported
