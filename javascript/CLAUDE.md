# CLAUDE.md - Project Guidelines

## Core Principle

**Follow existing patterns in the codebase first.**

### Pattern Discovery Process

1. **Examine existing code**: Look for similar functionality already in the codebase
2. **Follow established conventions**: Use the same approach, naming, and structure
3. **Document what you find**: Note the patterns for consistency
4. **When no pattern exists**: Make minimal decisions and establish new patterns carefully

---

## General Code Practices

### File Organization

- **Direct imports**: Avoid `index.ts` barrel exports - import directly from source files
- **Co-location**: Related code (types, context, hooks, tests, utils) should live together
- **Flat over nested**: Prefer flat structures when file count is manageable (~10 files)

### Naming Conventions

- **Files**: kebab-case with `.tsx` for JSX, `.ts` otherwise
- **Variables**: camelCase
- **Constants**: UPPER_SNAKE_CASE
- **Components**: PascalCase (matching kebab-case filename)
- **Types**: PascalCase without T suffix
- **Component props**: Follow existing patterns in the area
  - **Simple "Props"**: When component file contains single component
  - **"ComponentNameProps"**: When multiple components exist or props are exported

### Type Safety

- **Prefer `unknown` over `any`**: For better compile-time safety when creating new types
- **Create focused types**: Map from generated types, include only needed properties
- **Avoid type suppression**: Unless stress testing with invalid input

---

## Development Guidelines

### Code Organization

- **Start local, promote when needed**: Begin with `components/my-component/utils/`, move to `utils/` when multiple parts need it
- **Follow recursive folder structure**: Use consistent pattern regardless of file size
- **Namespace component names**: `table/components/table-action-button.tsx` not `action-button.tsx`

### Specific Directory Patterns (Established in Codebase)

- **Components**: Use `components/` for feature building blocks
- **Tests**: Place in closest `__tests__/` directory to tested code
- **Constants/Types**: Single files `constants.ts`, `types.ts` in feature root
- **Styled components**: In `styled-components.ts` for reusable styles

#### Hooks/Utils

**Common pattern variations**:

- **Single file**: `use-hook-name.ts`, `validation-util.ts` directly in component directory
- **Directory approach**: `hooks/`, `utils/`, `components/` with multiple files

### Performance

- **Avoid premature optimization**: Only optimize with data justifying the addition
- **Memoization**: Use `useMemo`/`useCallback` only to solve real user-facing problems

---

## Testing Guidelines

### What to Test

- **Real interactions over mocks**: Reduce dependency mocking to test closer to production
- **Mock server dependencies**: Don't connect to real servers, use established RPC mocking

### Test Organization

- **Follow existing test structure**: Use `describe` to group, `it('should...')` to document
- **Place tests near code**: Use closest `__tests__/` directory
- **Well-named mocks**: Use descriptive names that explain test intent
- **Inline when appropriate**: For simple cases tied directly to test expectations

### Fixtures and Mocks

- Well-named mocks: Use descriptive names that explain test intent
- Inline when appropriate: For simple cases tied directly to test expectations

When to mock vs not mock:

- ✅ DO mock: External APIs, RPC calls, server dependencies
- ✅ DO use real: Internal hooks, components, React context
