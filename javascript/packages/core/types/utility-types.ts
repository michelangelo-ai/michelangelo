/**
 * Utility types for common TypeScript patterns
 */

/**
 * Makes all properties of T optional recursively
 */
export type DeepPartial<T> = {
  [P in keyof T]?: T[P] extends object ? DeepPartial<T[P]> : T[P];
};

/**
 * Distributive version of TypeScript's built-in `Omit` utility type.
 *
 * @description
 * Unlike the standard `Omit<T, K>`, this type properly handles union types by
 * distributing the omit operation across each member of the union. This is
 * essential when working with discriminated unions where you need to remove
 * a property from all variants.
 *
 * @example
 * ```typescript
 * // Standard Omit fails with unions:
 * type BadExample = Omit<{ a: string; b: number } | { a: string; c: boolean }, 'a'>;
 * // Result: {} (loses all other properties)
 *
 * // DistributiveOmit works correctly:
 * type GoodExample = DistributiveOmit<{ a: string; b: number } | { a: string; c: boolean }, 'a'>;
 * // Result: { b: number } | { c: boolean } (preserves other properties)
 * ```
 *
 * @template T - The union type to omit properties from
 * @template K - The keys to omit from each member of the union
 *
 * @see https://www.typescriptlang.org/docs/handbook/2/conditional-types.html#distributive-conditional-types
 */
export type DistributiveOmit<T, K extends PropertyKey> = T extends unknown ? Omit<T, K> : never;
