import { Cell, Grid } from 'baseui/layout-grid';

import type { MainViewContainerProps } from './types';

/**
 * MainViewContainer is a layout wrapper component that provides consistent padding and spacing
 * for full-page layouts and components.
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
 */
export const MainViewContainer = ({ children }: MainViewContainerProps) => {
  return (
    <Grid gridColumns={1} gridGutters={0} gridGaps={0}>
      <Cell>{children}</Cell>
    </Grid>
  );
};
