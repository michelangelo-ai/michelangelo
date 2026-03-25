import type { HasTypeName } from './types';

/**
 * Type predicate to check if an object has a $typeName property
 */
export function hasTypeName(value: unknown): value is HasTypeName {
  return typeof value === 'object' && value !== null && '$typeName' in value;
}

/**
 * Type guard to check if a response is an entity response (Get, Create, or Update)
 */
export function isSingularResponse<T>(value: T): value is T & HasTypeName {
  return (
    hasTypeName(value) &&
    (value.$typeName.startsWith('michelangelo.api.v2.Get') ||
      value.$typeName.startsWith('michelangelo.api.v2.Create') ||
      value.$typeName.startsWith('michelangelo.api.v2.Update'))
  );
}
