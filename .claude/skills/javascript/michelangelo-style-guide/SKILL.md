---
name: Michelangelo JavaScript/TypeScript Style Guide
description: "Apply Michelangelo project-specific conventions, patterns, and best practices for JavaScript/TypeScript/React code. Use when writing, reviewing, or refactoring code in the Michelangelo codebase."
---

# Michelangelo JavaScript/TypeScript Style Guide

Apply project-specific conventions and patterns from the Michelangelo codebase to ensure consistency and maintainability.

## When to Apply

Use this skill automatically when:
- Writing new JavaScript/TypeScript/React code in Michelangelo
- Reviewing JavaScript/TypeScript code
- Refactoring existing implementations

## Core Principle

**Follow existing patterns in the codebase first.**

### Pattern Discovery Process

1. **Examine existing code**: Look for similar functionality already in the codebase
2. **Follow established conventions**: Use the same approach, naming, and structure
3. **Document what you find**: Note the patterns for consistency
4. **When no pattern exists**: Make minimal decisions and establish new patterns carefully

---

## File Organization

### Import Strategy

- **Direct imports**: Avoid `index.ts` barrel exports - import directly from source files
- **Co-location**: Related code (types, context, hooks, tests, utils) should live together
- **Flat over nested**: Prefer flat structures when file count is manageable (~10 files)

### Import Ordering

The project uses `eslint-plugin-simple-import-sort` with specific grouping rules. Imports should be organized in this order:

1. **React and third-party packages** (React always first)
   ```typescript
   import React from 'react';
   import { useState } from 'react';
   import { Button } from 'baseui/button';
   ```

2. **Internal imports (#) and relative imports**
   ```typescript
   import { useAuth } from '#auth/hooks';
   import { formatDate } from './utils';
   ```

3. **Type imports** (both third-party and local)
   ```typescript
   import type { User } from '#types';
   import type { Props } from './types';
   ```

4. **Style imports**
   ```typescript
   import './styles.css';
   ```

**ESLint will automatically sort these groups for you.**

### Directory Patterns (Established in Codebase)

- **Components**: Use `components/` for feature building blocks
- **Tests**: Place in closest `__tests__/` directory to tested code
- **Constants/Types**: Single files `constants.ts`, `types.ts` in feature root
- **Styled components**: In `styled-components.ts` for reusable styles

### Code Organization Strategy

- **Start local, promote when needed**: Begin with `components/my-component/utils/`, move to `utils/` when multiple parts need it
- **Follow recursive folder structure**: Use consistent pattern regardless of file size
- **Namespace component names**: `table/components/table-action-button.tsx` not `action-button.tsx`

### Hooks/Utils Patterns

**Common pattern variations**:
- **Single file**: `use-hook-name.ts`, `validation-util.ts` directly in component directory
- **Directory approach**: `hooks/`, `utils/`, `components/` with multiple files

---

## Naming Conventions

### Files and Identifiers

- **Files**: kebab-case with `.tsx` for JSX, `.ts` otherwise
- **Variables**: camelCase
- **Constants**: UPPER_SNAKE_CASE
- **Components**: PascalCase (matching kebab-case filename)
- **Types**: PascalCase without T suffix
- **Component props**: Follow existing patterns in the area
  - **Simple "Props"**: When component file contains single component
  - **"ComponentNameProps"**: When multiple components exist or props are exported

### Event Handlers

- **Use semantic function names**: Focus on intent rather than event type
- **Avoid redundant prefixes**: Don't use `handleOn*` since "handle" already implies event handling

**❌ Avoid:**
```typescript
const handleOnMouseEnter = () => { /* show tooltip */ };
const handleOnClick = () => { /* toggle menu */ };
```

**✅ Prefer:**
```typescript
const showTooltipAfterDelay = () => { /* show tooltip */ };
const toggleMenu = () => { /* toggle menu */ };
```

---

## Type Safety

### TypeScript Best Practices

- **Prefer `unknown` over `any`**: For better compile-time safety when creating new types
- **Create focused types**: Map from generated types, include only needed properties
- **Avoid type suppression**: Unless stress testing with invalid input
- **No T suffix**: Don't use `TProps`, `TUser`, etc. - just use `Props`, `User`

### Unused Variables

The project uses `@typescript-eslint/no-unused-vars` with special patterns:
- **Prefix with underscore for intentionally unused variables**: `_unused`, `_param`
  ```typescript
  function handler(_event: Event, data: Data) {
    // Only using data, event is required by signature
  }
  ```

This allows function signatures to document all parameters while acknowledging some aren't used.

---

## React Best Practices

### React Hooks

The project enforces React Hooks rules (`eslint-plugin-react-hooks`):
- **Follow Rules of Hooks**: Only call hooks at the top level
- **Exhaustive dependencies**: Include all dependencies in `useEffect`, `useMemo`, `useCallback`
- ESLint will error on violations to prevent bugs

### Component Exports

The project uses `eslint-plugin-react-refresh` for fast refresh compatibility:
- **Only export components**: Keep non-component exports to a minimum in component files
- **Constant exports allowed**: You can export constants alongside components
- **Warning on violations**: ESLint warns when non-component exports might break fast refresh

### BaseUI Guidelines

The project uses BaseUI components and enforces specific patterns:
- **No deep imports**: Import from `baseui/component-name`, not `baseui/component-name/subpath`
  ```typescript
  // ✅ Good
  import { Button } from 'baseui/button';

  // ❌ Avoid
  import { SIZE } from 'baseui/button/constants';
  ```
- **Use `useStyletron` for custom styling**: Prefer BaseUI's theming system over custom CSS

---

## Styled Components Guidelines

### Styling Approach Strategy

**Follow BaseUI's philosophy: Start with `useStyletron`, extract when needed.**

### When to Use Each Approach

**1. useStyletron (Inline Styles) - Start Here**

```typescript
const [css, theme] = useStyletron();

// Simple layouts and basic styling
<div className={css({ display: 'flex', gap: theme.sizing.scale400 })}>
<div className={css({ padding: theme.sizing.scale600 })}>
```

**2. styled() - Extract When Complex**

```typescript
export const TaskSeparator = styled('div', ({ $theme }) => ({
  height: '1px',
  backgroundColor: $theme.colors.borderOpaque,
  margin: `${$theme.sizing.scale600} 0`,
}));
```

### Decision Criteria for Extraction

**Extract to styled component when you find:**

1. **Complex Multi-Property Styling** (4+ CSS properties, computed values, pseudo-selectors)
2. **Pattern Reuse** (Used in 2+ places, clear semantic meaning)
3. **JSX Readability** (Inline styles would make JSX hard to read)

### Naming Conventions

- **❌ Avoid Generic Names**: `Container`, `Card`, `Wrapper` (cause collisions)
- **✅ Use Semantic Names**: `TaskSeparator`, `ExecutionMatrix`, `PipelineHeader`

---

## Testing Guidelines

### What to Test

- **Real interactions over mocks**: Reduce dependency mocking to test closer to production
- **Mock server dependencies**: Don't connect to real servers, use established RPC mocking
- **Business logic over implementation**: Test your business decisions and logic, not the tools you're using to implement them
  - Follow React Testing Library's guidance: test what users see (`screen.getByText()`, `screen.getByRole()`)
  - Don't test component internals (`container.firstChild`, prop verification)
- **Avoid test duplication**: Don't test the same behavior in both unit and integration tests
  - Choose the most appropriate level
  - If higher-level tests verify user-facing behavior, don't also test implementation details
- **Success path plus key edge cases**: Test core functionality and meaningful edge cases
  - Skip testing scenarios already covered by dependencies
  - Skip testing unrealistic scenarios

### Test Organization

- **Use `describe` blocks selectively**: Only when tests benefit from grouping (shared setup, complex state, related assertions)
- **Keep tests flat when descriptive**: Individual test names often provide enough context
- **Place tests near code**: Use closest `__tests__/` directory
- **Well-named mocks**: Use descriptive names that explain test intent
- **Inline when appropriate**: For simple cases tied directly to test expectations
- **Inline expect calls**: Prefer `expect(functionCall(args)).toEqual(result)` over assigning to variables

### When to Mock vs Not Mock

- **✅ DO mock**: External APIs, RPC calls, server dependencies
- **✅ DO use real**: Internal hooks, components, React context
- **✅ DO use real**: Well-tested utilities with error handling
- **✅ DO use real**: Dependencies already comprehensively tested

---

## Documentation Guidelines

### When to Add Documentation

**Add JSDoc when functions have**:
- **Non-obvious behaviors**: Error handling, side effects, or special logic
- **Edge cases**: Null handling, validation, or fallback behavior
- **Public APIs**: Utilities shared across multiple features

**Skip documentation for**:
- Simple getters/setters that match their TypeScript signature
- Internal implementation details

### JSDoc Style

- **Focus on "why" over "what"**: Explain behavior, not syntax
- **Include examples for complex functions**: Show real usage scenarios
- **Document edge cases and error handling**: What happens when inputs are invalid?
- **Be concise but complete**: Cover important behavior without redundancy

### Inline Comments

**Use inline comments for**:
- **Non-obvious implementation decisions**: Why a specific approach was chosen
- **Complex logic explanation**: Clarify intricate algorithms or transformations
- **Important context**: Business rules or constraints that affect the code

**Good examples**:
```typescript
// Ignore localStorage errors (quota exceeded, private browsing, etc.)
// Convert MM/DD/YYYY to YYYY/MM/DD format for consistency
```

### Avoid

- **Type duplication**: Don't restate what TypeScript already expresses
- **Obvious comments**: `// gets the pipeline run based on name`
- **AI/agent references**: Keep documentation focused on code purpose

---

## Code Formatting

### Prettier Integration

The project uses Prettier for automatic code formatting:
- **Prettier errors fail builds**: `prettier/prettier` is set to `error` in ESLint
- **Run Prettier before committing**: Use your IDE's format-on-save or run `npm run format`
- **No manual formatting**: Let Prettier handle whitespace, line breaks, and semicolons
- **Consistent style**: Prettier ensures consistent formatting across the entire codebase

All Prettier style rules are automatically enforced by ESLint, so you don't need to manually follow formatting guidelines.

---

## Performance

### Optimization Strategy

- **Avoid premature optimization**: Only optimize with data justifying the addition
- **Memoization**: Use `useMemo`/`useCallback` only to solve real user-facing problems
- **Profile before optimizing**: Use React DevTools and browser profiling

---

## Key Anti-Patterns to Avoid

- ❌ Using `index.ts` barrel exports instead of direct imports
- ❌ Generic styled component names (`Container`, `Wrapper`)
- ❌ Event handler names like `handleOnClick` (redundant prefix)
- ❌ Type names with T suffix (`TProps`, `TUser`)
- ❌ Using `any` instead of `unknown`
- ❌ Premature optimization without data
- ❌ Testing implementation details instead of user behavior
- ❌ Obvious or redundant comments
- ❌ Using `useStyletron` for complex multi-property styling (extract to styled component)
- ❌ Deep imports from BaseUI packages (use top-level exports)
- ❌ Exporting non-components from component files (breaks fast refresh)
- ❌ Violating React Hooks rules (non-exhaustive dependencies, conditional hooks)
- ❌ Unused variables without underscore prefix
- ❌ Manual import ordering (let ESLint auto-sort)

---

## Summary Checklist

When writing code in Michelangelo:

- [ ] Examined existing patterns in the codebase
- [ ] Used direct imports (no barrel exports)
- [ ] Followed import ordering (React first, then third-party, internal, types, styles)
- [ ] Followed established naming conventions (kebab-case files, PascalCase components)
- [ ] Used semantic event handler names (no `handleOn*`)
- [ ] Prefixed intentionally unused variables with underscore
- [ ] Started with `useStyletron`, extracted to styled component when complex
- [ ] Named styled components semantically (not generically)
- [ ] Used top-level BaseUI imports (no deep imports)
- [ ] Followed React Hooks rules (exhaustive dependencies, top-level only)
- [ ] Exported only components from component files (for fast refresh)
- [ ] Preferred `unknown` over `any`
- [ ] Co-located related code (types, tests, utils)
- [ ] Tested user behavior, not implementation details
- [ ] Added JSDoc only for non-obvious or public APIs
- [ ] Avoided premature optimization
- [ ] Let Prettier handle all code formatting
