import { cloneDeep, get, isNil, set, unset } from 'lodash';

import { getObjectValue } from '#core/utils/object-utils';
import { applyScaffold } from './apply-scaffold';

import type { StudioParamsBase } from '#core/hooks/routing/use-studio-params/types';
import type { MiddlewareOptions, MiddlewareSchema } from './types';

export function applyMiddleware<T extends object>(
  record: T,
  schema: MiddlewareSchema,
  context?: StudioParamsBase,
  options?: MiddlewareOptions
): T {
  const clone = applyScaffold(cloneDeep(record), schema);

  if (!schema.operations) return clone;

  const sourceObject = options?.sourceFromObject ?? clone;

  for (const op of schema.operations) {
    const subType = getObjectValue<string>(clone, schema.subTypePath!) ?? '';
    if (op.subTypes && !op.subTypes.includes(subType)) {
      continue;
    }

    if (op.transformation === 'unset') {
      unset(clone, op.destination);
      continue;
    }

    const sourceValue: unknown = op.source !== undefined ? get(sourceObject, op.source) : undefined;

    if (!isNil(sourceValue) && typeof op.transformation === 'function') {
      set(clone, op.destination, op.transformation(sourceValue));
    } else if (isNil(sourceValue) && 'default' in op) {
      const defaultVal =
        typeof op.default === 'function' ? op.default({ studio: context! }) : op.default;
      set(clone, op.destination, defaultVal);
    }
  }

  return clone;
}
