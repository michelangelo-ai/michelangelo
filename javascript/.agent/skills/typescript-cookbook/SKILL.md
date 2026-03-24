---
name: TypeScript cookbook
description: "Trigger when implementing anything in .ts or .tsx — new types, interfaces, hooks, or any feature that will involve TypeScript decisions. Don't wait for type errors; consult this before writing types, not after. Covers patterns that OVERRIDE TypeScript defaults."
user-invocable: false
---

# TypeScript Patterns

## Type Design

- **`unknown` over `any`** — better compile-time safety for new types
- **Create focused types** — map from generated types, include only the properties you need
- **No type suppression** — unless stress-testing with invalid input

## Naming

- **No T suffix** — use `Props`, `User`, not `PropsT`, `UserT`
- **PascalCase** for all types and interfaces
