import type { IconProps } from 'baseui/icon';

/**
 * Represents a React component that can be used as an icon
 */
export type IconComponent = React.ComponentType<Omit<IconProps, 'icon' | 'overrides'>>;

/**
 * Map of icon names to their component implementations.
 * Icon names are strings that can be used to reference icons in the registry.
 */
export type IconMap = Record<string, IconComponent>;

export type IconProviderContext = {
  icons: IconMap;
};
