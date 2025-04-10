import { Cell, Grid } from 'baseui/layout-grid';

import type { Props } from './types';

import { BREADCRUMB_BAR_HEIGHT } from './constants';

/**
 * MainViewContainer is a layout wrapper component that provides consistent padding and spacing
 * for full-page layouts and components. It automatically handles top margin spacing based on
 * breadcrumb visibility.
 *
 * This component should be used to wrap any full-page content to ensure consistent layout
 * alignment and spacing across the application.
 *
 * @remarks
 * MainViewContainer leverages the `Grid` component from `baseui/layout-grid` to provide a
 * responsive layout system. By default, `Grid` leverages the application's theme.grid.margins
 * configuration. For components that are _outside_ of the `MainViewContainer`, you should directly
 * leverage the `Grid` component to ensure consistent spacing.
 *
 * @example
 * ```tsx
 * <MainViewContainer>
 *   <YourPageContent />
 * </MainViewContainer>
 * ```
 *
 * @property {boolean} [hasBreadcrumb=true] - Whether the page has a breadcrumb navigation. Controls top margin spacing.
 * @property {React.ReactNode} children - The content to be rendered within the container.
 */
export const MainViewContainer = ({ hasBreadcrumb = true, children }: Props) => {
  return (
    <Grid
      gridColumns={1}
      gridGutters={0}
      gridGaps={0}
      overrides={{
        Grid: {
          style: {
            marginTop: hasBreadcrumb ? BREADCRUMB_BAR_HEIGHT : 0,
          },
        },
      }}
    >
      <Cell>{children}</Cell>
    </Grid>
  );
};
