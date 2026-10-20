---
sidebar_position: 2
last_verified: 2026-10-14
---

# Core Systems

## Service Provider

The Service Provider implements dependency injection for RPC requests. The core library doesn't know about specific backends - applications inject their own `request` function.

### Context Definition

[`ServiceContextType`](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/providers/service-provider/types.ts#L10):

```typescript
export type ServiceContextType = {
  request: (requestId: string, args: unknown, headers?: Record<string, string>) => Promise<unknown>;
  /** Resolvers the RPC layer applies to enrich responses before consumers see them, keyed by concern. */
  resolvers?: {
    team?: (uuids: string[]) => Promise<Record<string, TeamInfo>>;
  };
};
```

### Usage

The `request` function is injected by the application and called by hooks:

```typescript
import { useServiceProvider } from '#core/providers/service-provider/use-service-provider';

function MyComponent() {
  const { request } = useServiceProvider();

  const handleFetch = async () => {
    const result = await request('GetPipeline', { name: 'my-pipeline' });
  };
}
```

## Error Handling

### ApplicationError

[`ApplicationError`](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/types/error-types.ts#L5) is the standard error class used throughout the application:

```typescript
export class ApplicationError extends Error {
  name = 'ApplicationError' as const;
  code: number;        // gRPC status code
  meta?: Record<string, unknown>;
  cause?: unknown;
  source?: string;     // Error source identifier
}
```

### ErrorProvider

The [`ErrorProvider`](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/providers/error-provider/types.ts#L3) supplies an error normalization function to convert transport-specific errors into `ApplicationError`. A normalizer returns `null` for errors it does not recognize, and the provider then falls back to `normalizeUniversalError`, which handles plain `Error` objects, strings, and other shapes. `ErrorNormalizer` is defined in [`types/error-types.ts`](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/types/error-types.ts#L43):

```typescript
export type ErrorNormalizer = (error: unknown) => ApplicationError | null;

export interface ErrorContextValue {
  normalizeError: ErrorNormalizer;
}
```

### Transcoder Error Normalization

The UI talks to the API server with proto3 JSON over HTTP, through Envoy's gRPC JSON transcoder. The RPC package's fetch transport throws a [`GrpcTranscoderError`](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/rpc/grpc-transcoder-error.ts#L3) (with `code` and `details` fields) for any non-OK response. It builds the error from the `google.rpc.Status` JSON body when present, falls back to the `grpc-status` and `grpc-message` response headers, and otherwise uses gRPC code `2` (`UNKNOWN`).

The RPC package provides [`normalizeTranscoderError`](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/rpc/normalize-transcoder-error.ts#L17) to convert these into `ApplicationError`:

```typescript
export const normalizeTranscoderError: ErrorNormalizer = (error: unknown) => {
  if (!(error instanceof GrpcTranscoderError)) {
    return null;
  }

  const code = (
    error.code in GrpcStatusCode ? error.code : GrpcStatusCode.UNKNOWN
  ) as GrpcStatusCode;

  return new ApplicationError(error.message, code, {
    source: 'grpc-transcoder',
    meta: { details: error.details },
    cause: error,
  });
};
```

The resulting `ApplicationError` has `source: 'grpc-transcoder'`, a gRPC status `code` (unrecognized codes become `UNKNOWN`), the status `details` array in `meta.details`, and the original `GrpcTranscoderError` as `cause`. Any other error type returns `null` and is handled by `normalizeUniversalError`.

## Interpolation System

Configuration objects can contain dynamic values that get resolved at runtime. Two forms are supported: string templates and functions.

### String Interpolation

[`StringInterpolation`](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/interpolation/string-interpolation.ts#L20) replaces `${path.to.value}` patterns with context values:

```typescript
const interpolation = new StringInterpolation('Hello ${page.title}');
const result = interpolation.execute({ page: { title: 'Dashboard' } });
// result: "Hello Dashboard"
```

### Function Interpolation

[`FunctionInterpolation`](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/interpolation/function-interpolation.ts#L17) executes functions with full context access:

```typescript
const interpolation = new FunctionInterpolation(
  ({ page, row }) => `${page.title}: ${row.status}`
);
const result = interpolation.execute({
  page: { title: 'Pipeline' },
  row: { status: 'Running' }
});
// result: "Pipeline: Running"
```

### Interpolation Context

[`InterpolationContext`](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/interpolation/types.ts#L74) is available to interpolation functions:

```typescript
export interface InterpolationContext<U extends StudioParamsView = 'base'>
  extends InterpolationContextExtensions,
    UserDataSources {
  repeatedLayoutContext?: RepeatedLayoutState;
  studio: ViewTypeToParamType<U>;  // URL parameters
  data: any;                       // Resolved from row ?? page
}

// UserDataSources
//   page: any;           // Detail/form view data
//   row: any;            // Table row data
//   initialValues: any;  // Form initial state
//   response: any;       // Mutation response data
//   mutationName?: string;
```

`InterpolationContextExtensions` is an empty interface that applications can augment through module declaration to add their own context.

### Interpolatable Type

Fields that accept interpolation use the `Interpolatable` type:

```typescript
export type Interpolatable<T, U extends StudioParamsView = 'base'> =
  | T
  | string
  | FunctionInterpolation<T, U>
  | StringInterpolation<U>;

// Usage in configuration
interface ActionConfig {
  title: Interpolatable<string>;
  disabled: Interpolatable<boolean>;
}
```

## Hook Patterns

### useStudioQuery

[`useStudioQuery`](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/hooks/use-studio-query.ts#L63) fetches data with automatic error normalization and interpolation:

```typescript
const { data, isLoading, error } = useStudioQuery<Pipeline>({
  queryName: 'GetPipeline',
  serviceOptions: { name: 'my-pipeline' },
  clientOptions: { enabled: true },
});

// With interpolation
const { data } = useStudioQuery({
  queryName: 'GetPipeline',
  serviceOptions: { name: interpolate('${row.pipelineName}') },
});
```

The hook:
- Resolves interpolated values in `serviceOptions`
- Uses `projectId` as namespace by default
- Normalizes errors to `ApplicationError`
- Integrates with TanStack Query

### useStudioMutation

[`useStudioMutation`](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/hooks/use-studio-mutation/use-studio-mutation.ts#L13) performs mutations with error normalization:

```typescript
const { mutate, isPending, error } = useStudioMutation<Pipeline, CreatePipelineInput>({
  mutationName: 'CreatePipeline',
  clientOptions: {
    onSuccess: (data) => { /* handle success */ },
    onError: (error) => { /* handle error */ },
  },
});

mutate({ name: 'new-pipeline', config: { /* ... */ } });
```

### useStudioParams

[`useStudioParams`](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/hooks/routing/use-studio-params/use-studio-params.ts#L27) extracts and transforms URL parameters based on view type:

```typescript
// For list views
const { projectId, phase, entity } = useStudioParams('list');

// For detail views
const { projectId, phase, entity, entityId, entityTab } = useStudioParams('detail');

// For form views
const { projectId, phase, entity, entityId, entitySubType } = useStudioParams('form');
```

View types and their parameters:

| View Type | Parameters |
|-----------|------------|
| `list` | `projectId`, `phase`, `entity` |
| `detail` | `projectId`, `phase`, `entity`, `entityId`, `entityTab`, `revisionId?`, `pipelineName?` |
| `form` | `projectId`, `phase`, `entity`, `entityId`, `entitySubType?`, `revisionId?` |
| `form-detail` | Parameters shared by form and detail views; the rest are optional |
| `base` | All available parameters with optional fields |
| `unregistered` | Untyped `Record<string, string \| undefined>` of route and query params |

## Provider Composition

The application passes its injected dependencies to `CoreApp`, which composes the providers in a fixed hierarchy:

```tsx
// app/App.tsx
const dependencies = {
  error: { normalizeError: normalizeTranscoderError },
  theme: { icons: ICONS },
  service: { request, resolvers: { team: resolveTeams } },
  user: { name, email, role, timeZone },
};

<CoreApp dependencies={dependencies} />
```

```tsx
// packages/core/index.tsx (simplified)
<ServiceProvider {...dependencies.service}>
  <ErrorProvider {...dependencies.error}>
    <IconProvider icons={dependencies.theme.icons}>
      <UserProvider {...dependencies.user}>
        {children}
      </UserProvider>
    </IconProvider>
  </ErrorProvider>
</ServiceProvider>
```

`InterpolationProvider` is not part of this composition; it is mounted where interpolation context extensions are needed.

Available providers:

| Provider | Purpose | Source |
|----------|---------|--------|
| ServiceProvider | RPC request injection | `providers/service-provider/` |
| ErrorProvider | Error normalization | `providers/error-provider/` |
| InterpolationProvider | Value interpolation context | `providers/interpolation-provider/` |
| IconProvider | Icon component mapping | `providers/icon-provider/` |
| UserProvider | Current user context | `providers/user-provider/` |
| CellProvider | Cell rendering context | `providers/cell-provider/` |

## Related Documentation

- [Architecture Overview](./index.md) - Technology stack and build process
- [Types and Patterns](./types-and-patterns.md) - TypeScript types and conventions
- [UI Components](./ui-components.md) - Table, cell, and form systems
