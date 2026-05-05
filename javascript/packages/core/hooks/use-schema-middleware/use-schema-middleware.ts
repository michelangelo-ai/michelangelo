import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { applyMiddleware as applyMiddlewareFn } from './apply-middleware';

import type { MiddlewareSchema } from './types';

export function useSchemaMiddleware(schema?: MiddlewareSchema | null) {
  const studio = useStudioParams('base');

  function applyMiddleware<T extends object>(data: T): T {
    if (!schema) return data;
    return applyMiddlewareFn(data, schema, studio);
  }

  return { applyMiddleware };
}
