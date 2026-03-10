import { get } from 'lodash';

import type { Accessor } from '#core/types/common/studio-types';

/**
 * Recursively flattens a nested object into a flat map with dot-notation keys.
 * Numeric keys (array indices) are formatted with bracket notation.
 *
 * @example toFlatDotPathMap({ address: { street: 'Main St' } })
 * @returns { 'address.street': 'Main St' }
 *
 * @example toFlatDotPathMap({ items: [{ name: 'item1' }, { name: 'item2' }] })
 * @returns { 'items[0].name': 'item1', 'items[1].name': 'item2' }
 */
export function toFlatDotPathMap(
  obj: Record<string, unknown>,
  prefix = ''
): Record<string, unknown> {
  const result: Record<string, unknown> = {};

  for (const [key, value] of Object.entries(obj)) {
    const isIndex = /^\d+$/.test(key);
    const path = prefix ? (isIndex ? `${prefix}[${key}]` : `${prefix}.${key}`) : key;

    if (value !== null && typeof value === 'object') {
      Object.assign(result, toFlatDotPathMap(value as Record<string, unknown>, path));
    } else {
      result[path] = value;
    }
  }

  return result;
}

export function getObjectValue<K>(
  obj: unknown,
  accessor: Accessor<K>,
  defaultValue?: K
): K | undefined {
  if (typeof accessor === 'function') {
    return accessor(obj) ?? defaultValue;
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
