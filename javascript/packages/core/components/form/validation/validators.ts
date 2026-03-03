import { isAbsoluteURL } from '#core/utils/string-utils';

import type { FieldValidator } from './types';

/**
 * Checks whether a field value is empty (absent, blank, or structurally empty).
 *
 * Used by all format/range validators to skip validation on empty values.
 * This separates *presence* from *format/range* concerns: an empty value is not
 * a format error — it is a presence error, which belongs to `required()`.
 *
 * This means validators like `minLength` and `min` only apply when a value exists.
 * To enforce both presence and a constraint, compose them:
 *   `combineValidators(required(), minLength(8))`
 *
 * New validators should follow this convention: call `if (isEmpty(value)) return undefined`
 * before applying any format or range check.
 */
const isEmpty = (value: unknown): boolean => {
  if (value === undefined || value === null) return true;
  if (typeof value === 'string') return value.trim() === '';
  if (Array.isArray(value)) return value.length === 0;
  if (typeof value === 'object') return Object.keys(value).length === 0;
  return false;
};

export const required =
  (errorMessage = 'This field is required.'): FieldValidator =>
  (value) => {
    if (typeof value === 'boolean' || typeof value === 'number') return undefined;
    return isEmpty(value) ? errorMessage : undefined;
  };

export const min =
  (minimum: number, errorMessage = `Must be at least ${minimum}.`): FieldValidator =>
  (value) => {
    if (isEmpty(value)) return undefined;
    const num = Number(value);
    return isNaN(num) || num < minimum ? errorMessage : undefined;
  };

export const max =
  (maximum: number, errorMessage = `Must be at most ${maximum}.`): FieldValidator =>
  (value) => {
    if (isEmpty(value)) return undefined;
    const num = Number(value);
    return isNaN(num) || num > maximum ? errorMessage : undefined;
  };

export const minLength =
  (length: number, errorMessage = `Must be at least ${length} characters.`): FieldValidator =>
  (value) => {
    if (isEmpty(value)) return undefined;
    const str = String(value);
    return str.length < length ? errorMessage : undefined;
  };

export const maxLength =
  (length: number, errorMessage = `Must be at most ${length} characters.`): FieldValidator =>
  (value) => {
    if (isEmpty(value)) return undefined;
    const str = String(value);
    return str.length > length ? errorMessage : undefined;
  };

export const regex =
  (pattern: string | RegExp, errorMessage = 'Invalid format.'): FieldValidator =>
  (value) => {
    if (isEmpty(value)) return undefined;
    const re = typeof pattern === 'string' ? new RegExp(pattern) : pattern;
    return re.test(String(value)) ? undefined : errorMessage;
  };

export const url =
  (errorMessage = 'Must be a valid URL.'): FieldValidator =>
  (value) => {
    if (isEmpty(value)) return undefined;
    return isAbsoluteURL(String(value)) ? undefined : errorMessage;
  };
