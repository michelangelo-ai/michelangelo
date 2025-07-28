import { useStyletron } from 'baseui';

import {
  StyledTableBody,
  StyledTableBodyCell,
  StyledTableBodyRow,
} from '#core/components/table/styled-components';

import type { PropsWithChildren } from 'react';

/**
 * Wrapper component for table states (empty, error, loading) that need to
 * render content within the table's DOM structure while spanning all columns.
 *
 * This maintains proper table semantics and styling when displaying states
 * that replace the normal table content.
 */
export function TableStateWrapper({ children }: PropsWithChildren) {
  const [css, theme] = useStyletron();

  return (
    <StyledTableBody>
      <StyledTableBodyRow>
        <StyledTableBodyCell colSpan={100}>
          <div className={css({ padding: theme.sizing.scale600, width: '100%' })}>{children}</div>
        </StyledTableBodyCell>
      </StyledTableBodyRow>
    </StyledTableBody>
  );
}
