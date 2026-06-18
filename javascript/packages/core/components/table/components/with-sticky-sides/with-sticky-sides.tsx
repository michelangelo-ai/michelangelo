import React, { useRef } from 'react';
import { useStyletron } from 'baseui';

import { useHover } from '#core/hooks/use-hover';
import { buildShadowStyles } from './build-shadow-styles';
import { getTableStickyConfigs } from './get-table-sticky-configs';

import type { WithStickySidesProps } from './types';

/**
 * Higher-Order Component that adds sticky column functionality to table rows.
 *
 * Wraps table row components to make specified columns stick to the left/right edges
 * during horizontal scrolling, with shadow effects to indicate hidden content.
 *
 * Automatically handles:
 * - First column (+ selection column if enabled) sticking to left
 * - Last column sticking to right
 * - Shadow effects based on scroll position
 * - Hover background colors for better UX
 *
 * @param Component - The table row component to wrap (typically StyledTableHeadRow or StyledTableBodyRow)
 * @returns Enhanced component with sticky column behavior
 */
export function withStickySides<P extends object>(
  Component: React.ComponentType<P>
): React.ComponentType<P & WithStickySidesProps> {
  return function StickySidesHoc(props: P & WithStickySidesProps) {
    const {
      enableStickySides,
      enableRowSelection,
      lastColumnIndex,
      scrollRatio = -1,
      role,
      children,
      ...componentProps
    } = props;

    const [css, theme] = useStyletron();
    const hoverContainerRef = useRef<HTMLElement>(null);
    const isContainerHovered = useHover(hoverContainerRef);

    if (!enableStickySides) {
      return <Component {...(componentProps as P)}>{children}</Component>; // cast: rest spread after destructuring known props; remaining props match Component's P
    }

    const backgroundColor =
      isContainerHovered && role === 'row'
        ? theme.colors.tableStripedBackground
        : theme.colors.tableHeadBackgroundColor;

    const sharedStickyStyles = {
      position: 'sticky !important' as 'sticky', // cast: CSS-in-JS requires the literal 'sticky'; the !important suffix makes the string not assignable without cast
      backgroundColor: `${backgroundColor} !important`,
    };

    const stickyConfigs = getTableStickyConfigs(enableRowSelection, lastColumnIndex);

    return (
      // cast: rest spread after destructuring WithStickySidesProps; remaining props match Component's generic P
      <Component {...(componentProps as P)} ref={hoverContainerRef}>
        {React.Children.map(children, (child, index) => {
          const config = stickyConfigs[index];
          if (!config) return child;

          const stickyStyles = {
            ...sharedStickyStyles,
            [config.stickySide]: `${config.position}px !important`,
          };

          // cast: React.Children.map yields ReactNode; we know each sticky cell child is a ReactElement
          return React.cloneElement(child as React.ReactElement<Record<string, unknown>>, {
            className: `${css(stickyStyles)} ${css(buildShadowStyles(config.shadowSide, scrollRatio))}`,
            'data-testid': `sticky-cell-${config.stickySide}-sticky`,
          });
        })}
      </Component>
    );
  };
}
