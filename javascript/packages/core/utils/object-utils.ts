import { get } from 'lodash';

import type { Accessor } from '#core/types/common/studio-types';

export function getObjectValue<K>(
  obj: object,
  accessor: Accessor<K>,
  defaultValue?: K
): K | undefined {
  if (typeof accessor === 'function') {
    return accessor(obj) || defaultValue;
  }

  if (typeof accessor === 'string') {
    return get(obj, accessor, defaultValue);
  }

  return undefined;
}

export function getObjectSymbols(obj: unknown): Record<symbol, unknown> {
  if (typeof obj !== 'object' || obj === null) {
    return {};
  }

  const symbols = Object.getOwnPropertySymbols(obj);
  const result: Record<symbol, unknown> = {};

  for (const symbol of symbols) {
    result[symbol] = obj[symbol];
  }

  return result;
}
