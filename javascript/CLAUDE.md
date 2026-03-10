# JavaScript Codebase

## Architecture

Michelangelo Studio is a **pluggable ML UI**. The `javascript/` directory contains two packages and an application:

| Directory       | Name                      | Purpose                                              |
| --------------- | ------------------------- | ---------------------------------------------------- |
| `packages/core` | `@uber/michelangelo-core` | UI rendering engine — the primary library            |
| `packages/rpc`  | `@michelangelo/rpc`       | Optional ConnectRPC/gRPC-Web client                  |
| `app/`          | `@michelangelo/app`       | Reference application — served via Docker/Kubernetes |

### Component-driven first, configuration-driven on top

**Component-driven is the default and first-class approach.** Build with standard React patterns — props, composition, TypeScript interfaces — no configuration required.

**Configuration-driven is layered on top.** Deployments that need dynamic behavior, interpolation, or CRUD automation can wrap the same base components in a configuration layer. New functionality should be built component-driven first; a configuration layer can be added later as a thin wrapper.

### Pluggability via providers

Customization is achieved through React context providers. Consumers register custom implementations (renderers, icons, services, etc.) via the `dependencies` prop on `CoreApp` — no forking of core required.

### `app/` — the reference application

`app/` is not just a local dev sandbox. It is the application built into the Docker image (see `docker/ui.Dockerfile`) and published via CI (see `.github/workflows/ui-release.yml`). It is what users of the Kubernetes sandbox (`python/michelangelo/cli/sandbox/`) and anyone pulling the published image consume.

`app/` also shows how to leverage pluggability implemented by `packages/core`. For example, the request function from `@michelangelo/rpc` is provided to `CoreApp`'s `request` property.

```tsx
// app/App.tsx — reference integration using @michelangelo/rpc
import { request } from '@michelangelo/rpc';

<CoreApp dependencies={{ service: { request } }} />;
```

---

## Commands

Run from the `javascript/` directory:

| Command           | Purpose                                                            |
| ----------------- | ------------------------------------------------------------------ |
| `yarn dev`        | Start dev server (`app/`)                                          |
| `yarn test`       | Run all tests (vitest)                                             |
| `yarn test:core`  | Run only `packages/core` tests                                     |
| `yarn test:rpc`   | Run only `packages/rpc` tests                                      |
| `yarn test:watch` | Run tests in watch mode                                            |
| `yarn lint`       | ESLint across all packages                                         |
| `yarn typecheck`  | Full TypeScript type check                                         |
| `yarn format`     | Check Prettier formatting                                          |
| `yarn build`      | Build all workspaces (runs `generate` first)                       |
| `yarn generate`   | Regenerate gRPC clients from protobuf definitions                  |
| `yarn setup`      | Fresh install + generate (use after cloning or switching branches) |

---

## Skills

Coding standards are enforced through skills that fire automatically based on context.

| Context                     | Skill                         | Trigger                                           |
| --------------------------- | ----------------------------- | ------------------------------------------------- |
| Any code change             | `ui-coding-principles`        | Always — governs complexity and pattern-following |
| React components            | `react-component-development` | All `.tsx` modifications                          |
| Tests                       | `testing-standards`           | Writing, editing, or refactoring test files       |
| TypeScript types            | `typescript-cookbook`         | Type definitions, generics, type errors           |
| Files, directories, imports | `file-directory-structure`    | Creating/moving files, adding imports             |
| Comments, JSDoc             | `documenting-code`            | Adding or reviewing documentation                 |

## Always-On Rules

- **No barrel exports**: Never use `index.ts` to re-export — always import directly from source files
- **Follow existing patterns first**: Examine similar code in the codebase before establishing new patterns
- **Prettier owns formatting**: Never manually adjust whitespace, semicolons, or line breaks
