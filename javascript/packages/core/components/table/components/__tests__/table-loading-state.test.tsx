import { render, screen } from '@testing-library/react';

import { TableLoadingState } from '../table-loading-state';

describe('TableLoadingState', () => {
  it('should render loading skeleton rows', () => {
    render(
      <table>
        <TableLoadingState />
      </table>
    );

    expect(screen.getByTestId('table-loading-state')).toBeInTheDocument();
  });

  it('should render 3 skeleton rows by default', () => {
    render(
      <table>
        <TableLoadingState />
      </table>
    );

    const rows = screen.getAllByRole('row');
    expect(rows).toHaveLength(3);
  });

  it('should render cells with full width colspan', () => {
    render(
      <table>
        <TableLoadingState />
      </table>
    );

    const cells = screen.getAllByRole('cell');
    cells.forEach((cell) => {
      expect(cell).toHaveAttribute('colSpan', '100');
    });
  });
});
