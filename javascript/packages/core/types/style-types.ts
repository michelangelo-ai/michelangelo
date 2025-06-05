import type { Theme } from 'baseui';
import type { CSSProperties } from 'react';

/**
 * @description
 * A function that takes a theme and returns style properties.
 * This follows baseui's ConfigurationOverrideFunction pattern but with proper typing.
 * The style object follows React's CSSProperties type for proper CSS property typing.
 *
 * @example
 * ```ts
 * const styleFunction: StyleFunction = (theme) => ({
 *   color: theme.colors.primary,
 *   backgroundColor: theme.colors.background,
 * });
 * ```
 */
export type StyleFunction<T extends CSSProperties = CSSProperties> = (
  theme: Theme
) => T | undefined | null;
