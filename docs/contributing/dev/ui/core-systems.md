---
sidebar_position: 2
---

# Core Systems

## Service Provider

The Service Provider implements dependency injection for RPC requests. The core library doesn't know about specific backends - applications inject their own `request` function.

### Context Definition

[`ServiceContextType`](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/providers/service-provider/types.ts#L10):

```typescript
export type ServiceContextType = {
  request: (requestId: string, args: unknown) => Promise<unknown>;
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

The [`ErrorProvider`](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/providers/error-provider/types.ts#L3) supplies error normalization functions to convert framework-specific errors into `ApplicationError`:

```typescript
export type ErrorNormalizer = (error: unknown) => ApplicationError | null;

export interface ErrorContextValue {
  normalizeError: ErrorNormalizer;
}
```

### Connect RPC Error Normalization

The RPC package provides [`normalizeConnectError`](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/rpc/normalize-connect-error.ts#L22) for Connect RPC errors:

```typescript
import { ConnectError } from '@connectrpc/connect';

export const normalizeConnectError: ErrorNormalizer = (error) => {
  if (!(error instanceof ConnectError)) {
    return null;
  }

  return new ApplicationError(error.message, mapConnectCodeToGrpc(error.code), {
    source: 'connect-rpc',
    meta: {
      connectErrorName: error.name,
      details: error.details,
      metadata: error.metadata,
    },
    cause: error.cause ?? error,
  });
};
```

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
export interface InterpolationContext<U extends StudioParamsView = 'base'> {
  page: any;           // Detail/form view data
  row: any;            // Table row data
  initialValues: any;  // Form initial state
  response: any;       // Mutation response data
  data: any;           // Resolved from row ?? page
  studio: ViewTypeToParamType<U>;  // URL parameters
  repeatedLayoutContext?: RepeatedLayoutState;
}
```

### Interpolatable Type

Fields that accept interpolation use the `Interpolatable` type:

```typescript
export type Interpolatable<T> = T | string | FunctionInterpolation<T> | StringInterpolation;

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

[`useStudioMutation`](https://github.com/michelangelo-ai/michelangelo/blob/main/javascript/packages/core/hooks/use-studio-mutation.ts#L10) performs mutations with error normalization:

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
| `detail` | `projectId`, `phase`, `entity`, `entityId`, `entityTab`, `revisionId?` |
| `form` | `projectId`, `phase`, `entity`, `entityId`, `entitySubType?`, `revisionId?` |
| `base` | All available parameters with optional fields |

## Provider Composition

Providers are composed in a specific hierarchy. The application typically wraps components with:

```tsx
<ServiceProvider request={rpcRequest}>
  <ErrorProvider normalizeError={normalizeConnectError}>
    <InterpolationProvider>
      <IconProvider icons={iconMap}>
        <UserProvider user={currentUser}>
          {children}
        </UserProvider>
      </IconProvider>
    </InterpolationProvider>
  </ErrorProvider>
</ServiceProvider>
```

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
