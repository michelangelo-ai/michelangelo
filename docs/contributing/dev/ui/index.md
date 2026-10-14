---
last_verified: 2026-10-14
sidebar_position: 1
---

# UI Architecture

Internal architecture reference for contributors to the Michelangelo AI UI codebase.

## Technology Stack

| Technology | Version | Purpose |
|------------|---------|---------|
| [React](https://react.dev/) | 18.3.1 | UI component library |
| [TypeScript](https://www.typescriptlang.org/) | 5.7.x | Type-safe JavaScript |
| [BaseUI](https://baseweb.design/) | 15.0.0 | Component library |
| [Styletron](https://styletron.org/) | 6.1.x | CSS-in-JS styling engine |
| [TanStack React Query](https://tanstack.com/query) | 5.39.0 | Server state management |
| [TanStack React Table](https://tanstack.com/table) | 8.21.x | Table components |
| [React Final Form](https://final-form.org/react) | 6.5.9 | Form state management |
| [Vite](https://vite.dev/) | 6.2.0 | Build tooling and dev server |
| [Vitest](https://vitest.dev/) | 3.x | Testing framework |
| [Protobuf-ES](https://github.com/bufbuild/protobuf-es) | 2.x | Generated message schemas used to convert proto3 JSON |

## Package Structure

The JavaScript codebase is organized as a Yarn workspace with the following packages:

```
javascript/
├── app/                    # Main application package
│   └── package.json        # @michelangelo/app
├── packages/
│   ├── core/               # Core UI components and utilities
│   │   └── package.json    # @michelangelo-ai/core
│   └── rpc/                # RPC client and error handling
│       └── package.json    # @michelangelo-ai/rpc
└── package.json            # Workspace root
```

### Package Responsibilities

**@michelangelo/app**: The main application that runs in the browser. Contains:
- Application entry point and routing
- Feature-specific views and pages
- Application-level configuration

**@michelangelo-ai/core**: Shared UI components and utilities. Contains:
- React components (table, cell, form, views)
- React hooks (routing, queries, mutations)
- Provider components (service, error, interpolation)
- Type definitions
- Test utilities

**@michelangelo-ai/rpc**: RPC client and error handling. Contains:
- A fetch-based transport that exchanges proto3 JSON with the Envoy gRPC-JSON transcoder
- Generated protobuf-es schemas, used to convert request and response messages
- Error normalization for transcoder errors (`normalizeTranscoderError`)

## Build and Bundling

### Development

Start the development server:

```bash
cd javascript
yarn dev
```

This runs Vite in development mode with hot module replacement.

### Production Build

Build all packages:

```bash
cd javascript
yarn build
```

Build order:
1. Generate gRPC client code (`yarn generate`)
2. Build `@michelangelo-ai/core`
3. Build `@michelangelo-ai/rpc`
4. Build `@michelangelo/app`

### Scripts

| Script | Description |
|--------|-------------|
| `yarn dev` | Start development server |
| `yarn build` | Build all packages |
| `yarn test` | Run all tests |
| `yarn test:core` | Run core package tests |
| `yarn test:rpc` | Run RPC package tests |
| `yarn lint` | Run ESLint |
| `yarn typecheck` | Run TypeScript type checking |
| `yarn format` | Check code formatting with Prettier |

## Import Aliases

The codebase uses TypeScript path aliases:

```typescript
// In @michelangelo-ai/core
import { useStudioQuery } from '#core/hooks/use-studio-query';
import { Phase } from '#core/types/common/studio-types';

// In @michelangelo-ai/rpc
import { normalizeTranscoderError } from '#rpc/normalize-transcoder-error';
```

## Related Documentation

- [Core Systems](./core-systems.md) - Providers, hooks, and error handling
- [Types and Patterns](./types-and-patterns.md) - TypeScript types and React component patterns
- [UI Components](./ui-components.md) - Table, cell, form, and styling systems
- [Configuration API](./configuration/configuration-api.md) - Defining phases, entities, and views
