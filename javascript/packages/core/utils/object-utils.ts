import { get } from 'lodash';

import type { Accessor } from '#core/types/common/studio-types';

/**
 * Excludes arrays and null, unlike a plain `typeof x === 'object'` check.
 * Use to narrow `unknown` before accessing properties by key.
 */
export function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

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
  obj: Record<string, unknown> | unknown[],
  prefix = ''
): Record<string, unknown> {
  const result: Record<string, unknown> = {};

  for (const [key, value] of Object.entries(obj)) {
    const isIndex = /^\d+$/.test(key);
    let path: string;
    if (!prefix) {
      path = key;
    } else if (isIndex) {
      path = `${prefix}[${key}]`;
    } else {
      path = `${prefix}.${key}`;
    }

    if (Array.isArray(value) || isRecord(value)) {
      Object.assign(result, toFlatDotPathMap(value, path));
    } else {
      result[path] = value;
    }
  }

  return result;
}

/**
 * Reads a value from an object using either a function accessor or a lodash-style path.
 *
 * Returns the provided default when the accessor resolves to nullish, and returns
 * undefined when the accessor is not callable or path-like.
 */
export function getObjectValue<K>(
  obj: unknown,
  accessor: Accessor<unknown, K>,
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

/**
 * Copies an object's symbol-keyed properties into a plain symbol record.
 *
 * React and third-party libraries sometimes attach metadata by symbol; this
 * helper preserves those values when object data is transformed.
 */
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
