import { forwardRef } from 'react';
import { mergeOverrides, useStyletron } from 'baseui';
import { KIND as BASE_KIND, Tag as BaseTag } from 'baseui/tag';

import { TAG_BEHAVIOR, TAG_COLOR, TAG_HIERARCHY, TAG_SIZE } from './constants';
import { getTagOverrides } from './styled-components';

import type { TagKind as BaseTagKind, TagSize as BaseTagSize } from 'baseui/tag';
import type { Props } from './types';

/**
 * Displays labeled tags with customizable colors, sizes, behaviors, and visual hierarchy.
 *
 * Tag extends BaseUI's Tag component with Michelangelo-specific styling and configuration
 * options. It supports different visual styles for various use cases like status indicators,
 * labels, filters, and interactive elements.
 *
 * Features:
 * - Multiple color options (gray, purple, magenta, blue, green, red, orange, yellow, brown)
 * - Four size variants (xSmall, small, medium, large)
 * - Two hierarchies (primary, secondary) for visual emphasis
 * - Two behaviors (display, interactive) for different interaction patterns
 * - Theme integration
 * - Customizable through BaseUI overrides
 * - Ref forwarding support
 *
 * @param props.children - Tag content (typically text)
 * @param props.size - Tag size variant. Defaults to 'small'
 * @param props.color - Tag color variant. Defaults to 'gray'
 * @param props.behavior - Tag behavior type. Defaults to 'display'
 *   - 'display': Static tag for labels/status
 *   - 'interactive': Clickable tag for filters/actions
 * @param props.hierarchy - Visual hierarchy level. Defaults to 'secondary'
 *   - 'primary': More prominent styling
 *   - 'secondary': Subtle styling
 * @param props.overrides - BaseUI overrides for customization
 *
 * @example
 * ```tsx
 * // Status indicator
 * <Tag color={TAG_COLOR.green}>Running</Tag>
 *
 * // Multiple sizes
 * <Tag size={TAG_SIZE.xSmall}>XS</Tag>
 * <Tag size={TAG_SIZE.small}>Small</Tag>
 * <Tag size={TAG_SIZE.medium}>Medium</Tag>
 *
 * // Interactive filter tag
 * <Tag
 *   color={TAG_COLOR.blue}
 *   behavior={TAG_BEHAVIOR.interactive}
 *   onClick={() => removeFilter('status')}
 * >
 *   Status: Active ×
 * </Tag>
 *
 * // Primary hierarchy for emphasis
 * <Tag color={TAG_COLOR.red} hierarchy={TAG_HIERARCHY.primary}>
 *   Critical
 * </Tag>
 * ```
 */
export const Tag = forwardRef<HTMLElement, Props>(
  (
    {
      children,
      overrides,
      size = TAG_SIZE.small,
      color = TAG_COLOR.gray,
      behavior = TAG_BEHAVIOR.display,
      hierarchy = TAG_HIERARCHY.secondary,
      ...rest
    },
    ref
  ) => {
    const [_, theme] = useStyletron();

    const baseSize: BaseTagSize | undefined = size === TAG_SIZE.xSmall ? TAG_SIZE.small : size;
    const baseKind: BaseTagKind | undefined =
      color === TAG_COLOR.gray || color === TAG_COLOR.purple || color === TAG_COLOR.magenta
        ? BASE_KIND.black
        : color;

    return (
      <BaseTag
        {...rest}
        ref={ref}
        kind={baseKind}
        size={baseSize}
        hierarchy={hierarchy}
        overrides={mergeOverrides(
          overrides,
          getTagOverrides(theme, {
            size,
            color,
            behavior,
            hierarchy,
          })
        )}
      >
        {children}
      </BaseTag>
    );
  }
);

Tag.displayName = 'Tag';
