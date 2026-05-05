import { cloneDeep, get, isNil, set, unset } from 'lodash';

import type { StudioParamsBase } from '#core/hooks/routing/use-studio-params/types';
import type { MiddlewareSchema } from './types';

export function applyMiddleware<T extends object>(
  record: T,
  schema: MiddlewareSchema,
  context?: StudioParamsBase
): T {
  const clone = cloneDeep(record);

  if (!schema.operations) return clone;

  for (const op of schema.operations) {
    if (op.transformation === 'unset') {
      unset(clone, op.destination);
      continue;
    }

    const sourceValue: unknown = op.source !== undefined ? get(clone, op.source) : undefined;

    if (!isNil(sourceValue) && typeof op.transformation === 'function') {
      set(clone, op.destination, op.transformation(sourceValue));
    } else if (isNil(sourceValue) && 'default' in op) {
      const defaultVal =
        typeof op.default === 'function'
          ? (op.default as (args: { studio: StudioParamsBase }) => unknown)({ studio: context! })
          : op.default;
      set(clone, op.destination, defaultVal);
    }
  }

  return clone;
}
