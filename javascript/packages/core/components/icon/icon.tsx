import { memo } from 'react';
import { useStyletron } from 'baseui';

import { useIconProvider } from '#core/providers/icon-provider/use-icon-provider';
import { capitalizeFirstLetter } from '#core/utils/string-utils';
import { IconKind } from './types';

import type { Props } from './types';

/**
 * Renders icons with automatic theme-based coloring and consistent sizing.
 *
 * This component provides a unified way to display icons throughout the application,
 * automatically applying the correct color based on the icon kind (primary, secondary,
 * tertiary) and the current theme. Icons are retrieved from the IconProvider registry.
 *
 * Features:
 * - Automatic theme color application based on kind
 * - Icon registry lookup by name
 * - Direct icon component support
 * - Memoized for performance
 * - Accessible with role="img"
 * - Default size from theme (scale550)
 *
 * @param props.name - Name of the icon in the icon registry (e.g., 'play', 'pause', 'circleI')
 * @param props.icon - Direct icon component to render (alternative to name lookup)
 * @param props.kind - Icon styling variant. Determines the color from theme.
 *   - PRIMARY: contentPrimary color
 *   - SECONDARY: contentSecondary color
 *   - TERTIARY: contentTertiary color
 *   - ACCENT: contentAccent color
 * @param props.size - Icon size. Defaults to theme.sizing.scale550
 * @param props.color - Custom color override. When provided, overrides kind-based coloring
 * @param props.title - Accessible title for the icon
 *
 * @example
 * ```tsx
 * // Icon from registry by name
 * <Icon name="play" kind={IconKind.PRIMARY} />
 *
 * // With custom size and color
 * <Icon name="error" size="24px" color="red" />
 *
 * // Using direct icon component
 * import { Play } from 'baseui/icon';
 * <Icon icon={Play} kind={IconKind.SECONDARY} />
 *
 * // With accessibility title
 * <Icon name="circleI" title="help" kind={IconKind.TERTIARY} />
 * ```
 */
export const Icon = memo<Props>(function Icon(props: Props) {
  const [, theme] = useStyletron();
  const { color, name, icon, kind = IconKind.PRIMARY, size = theme.sizing.scale550 } = props;
  const { icons } = useIconProvider();

  const IconComponent = icon ?? (name ? icons[name] : null);

  if (!IconComponent) return null;

  return (
    <IconComponent
      {...props}
      role="img"
      size={size}
      color={color ?? theme.colors[`content${capitalizeFirstLetter(kind)}`]}
      style={{ minWidth: 'fit-content' }}
    />
  );
});
