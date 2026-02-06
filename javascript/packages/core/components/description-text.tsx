import { styled } from 'baseui';

import type { StyleObject } from 'styletron-react';

/**
 * Styled component for displaying secondary descriptive text with consistent typography.
 *
 * This component applies the theme's small paragraph typography with secondary content
 * color, making it ideal for help text, descriptions, or supplementary information.
 *
 * Features:
 * - Automatic theme typography (ParagraphSmall)
 * - Secondary content color from theme
 * - Flexbox layout with vertical alignment
 * - Support for style overrides via $styleOverrides prop
 *
 * @example
 * ```tsx
 * // Basic usage
 * <DescriptionText>
 *   This pipeline runs every hour
 * </DescriptionText>
 *
 * // In a form field
 * <label>
 *   Pipeline Name
 *   <Input />
 *   <DescriptionText>
 *     Must be unique within the project
 *   </DescriptionText>
 * </label>
 *
 * // With custom styles
 * <DescriptionText $styleOverrides={{ marginTop: '8px' }}>
 *   Additional information here
 * </DescriptionText>
 * ```
 */
export const DescriptionText = styled<'div', { $styleOverrides?: StyleObject }>(
  'div',
  ({ $theme, $styleOverrides }) => ({
    ...$theme.typography.ParagraphSmall,
    color: $theme.colors.contentSecondary,
    display: 'flex',
    alignItems: 'center',
    ...$styleOverrides,
  })
);
