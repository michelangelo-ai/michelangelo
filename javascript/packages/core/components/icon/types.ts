import type { IconProps } from 'baseui/icon';

/**
 * @description
 * IconKind controls the color styling applied to Icons
 *
 * New IconKinds should align with an Uber color token. For example,
 * contentPrimary, contentAccent should have associated IconKinds
 * 'primary' and 'accent', respectively.
 */
export enum IconKind {
  PRIMARY = 'primary',
  SECONDARY = 'secondary',
  TERTIARY = 'tertiary',
  ACCENT = 'accent',
}

/**
 * Icon is a wrapper around BaseWeb's Icon component that accepts
 * Icon component, an entity or an icon name and returns the respective icon.
 */
export type Props = {
  icon?: React.ComponentType<IconProps>;
  name?: string;
  kind?: IconKind;
} & IconProps;
