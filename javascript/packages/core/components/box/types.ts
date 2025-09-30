import type { Override } from 'baseui/overrides';
import type { ReactNode } from 'react';

export type BoxContentProps = {
  children: ReactNode;
  title?: ReactNode | string;
  description?: ReactNode | string;
};

export type BoxOverrides = {
  BoxContainer?: Override;
  BoxDescription?: Override;
  BoxHeader?: Override;
  BoxTitle?: Override;
};

export type BoxProps = BoxContentProps & {
  overrides?: BoxOverrides;
};

type CollapsibleBoxOverrides = {
  Container?: Override;
  Header?: Override;
  HeaderTitle?: Override;
  Content?: Override;
  ToggleIcon?: Override;
};

export interface CollapsibleBoxProps extends BoxContentProps {
  expanded?: boolean;

  /**
   * Whether the box begins in an expanded state
   *
   * @default false
   */
  defaultExpanded?: boolean;
  onToggle?: (expanded: boolean) => void;

  /**
   * Controls whether the box's expansion state can be toggled
   *
   * @default false
   */
  disabled?: boolean;

  /**
   * Overrides for styling the collapsible box elements.
   *
   * @example
   * <CollapsibleBox overrides={{
   *   Container: { style: { backgroundColor: 'gray' } },
   *   HeaderTitle: { style: { fontWeight: 'bold' } }
   * }} />
   */
  overrides?: CollapsibleBoxOverrides;
}
