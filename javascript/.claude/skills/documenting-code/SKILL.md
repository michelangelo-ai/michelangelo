---
name: Documenting code
description: "Apply when adding, writing, or reviewing comments or JSDoc in any file. Also apply when making significant code changes that introduce non-obvious logic, public APIs, or edge cases that likely need documentation. Covers when to write JSDoc vs. when to skip it entirely, and inline comment standards."
user-invocable: false
---

# Documentation Patterns

## JSDoc

**Add JSDoc when a function has:**
- Non-obvious behavior (error handling, side effects, special logic)
- Edge cases that need clarification (null handling, validation, fallback behavior)
- A public API shared across multiple features

**Skip JSDoc for:**
- Simple getters/setters that match their TypeScript signature
- Internal implementation details

**Focus on "why" over "what"** — explain behavior and decisions, not syntax. Include examples for complex functions.

## Inline Comments

Use for non-obvious implementation decisions, not to narrate obvious code:

```typescript
// ✅ Explains a non-obvious decision
// Ignore localStorage errors (quota exceeded, private browsing, etc.)

// ✅ Explains a non-obvious transformation
// Convert MM/DD/YYYY to YYYY/MM/DD for API consistency

// ❌ Narrates what the code already says
// gets the pipeline run based on name
```

## Prefer Renaming Over Commenting

Before adding a comment to explain what a function does, ask whether a better name would make the comment unnecessary. A well-named function with a clear TypeScript signature is self-documenting.

```typescript
// ❌ Compensating comment because the name is weak
// Recursively flattens a nested error object into dot-notation string keys
function flattenErrors(obj: Record<string, unknown>): Record<string, string>

// ✅ Name makes the comment redundant
function flattenErrorsToDotPaths(obj: Record<string, unknown>): Record<string, string>
```

## Avoid

- Restating what the TypeScript signature already expresses
- AI/agent references in documentation
