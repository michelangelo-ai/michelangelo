/**
 * Regular expression pattern for matching interpolation syntax.
 * Matches expressions like `${variable.path}`
 *
 * @example
 * ```typescript
 * // Test if string contains interpolation
 * INTERPOLATION_PATTERN.test('Hello ${user.name}'); // true
 * INTERPOLATION_PATTERN.test('Hello world'); // false
 *
 * // Use with global flag for replacement
 * const text = '${user.name} works at ${user.company}';
 * text.replace(new RegExp(INTERPOLATION_PATTERN, 'g'), 'REPLACED');
 * // Result: 'REPLACED works at REPLACED'
 * ```
 */
export const INTERPOLATION_PATTERN = /\$\{[^}]+\}/;

/**
 * Regular expression for removing interpolation syntax characters.
 * Matches `$`, `{`, and `}` characters globally to extract variable paths.
 *
 * @example
 * ```typescript
 * // Extract variable path from interpolation syntax
 * const interpolation = '${user.name}';
 * const path = interpolation.replace(SYNTAX_CHARS, '');
 * // Result: 'user.name'
 *
 * // Works with complex paths
 * '${data.user.profile.email}'.replace(SYNTAX_CHARS, '');
 * // Result: 'data.user.profile.email'
 * ```
 */
export const SYNTAX_CHARS = /[${}]/g;
