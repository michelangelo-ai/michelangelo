import { render, screen } from '@testing-library/react';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getInterpolationProviderWrapper } from '#core/test/wrappers/get-interpolation-provider-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import { TableCell } from '../table-cell';

import type { ColumnConfig } from '../../../types/column-types';

describe('TableCell', () => {
  it('should render basic text cell', () => {
    const column: ColumnConfig = {
      id: 'name',
      label: 'Name',
      type: 'text',
    };

    render(
      <TableCell column={column} record={{ name: 'John Doe' }} value="John Doe" />,
      buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
    );

    expect(screen.getByText('John Doe')).toBeInTheDocument();
  });

  it('should resolve interpolations in column config', () => {
    const column: ColumnConfig = {
      id: 'name',
      label: 'label',
      type: 'text',
      url: 'https://${row.name}.com',
    };

    const record = { name: 'Jane Smith' };

    render(
      <TableCell column={column} record={record} value="Jane Smith" />,
      buildWrapper([getInterpolationProviderWrapper(), getRouterWrapper()])
    );

    expect(screen.getByRole('link', { name: 'Jane Smith' })).toHaveAttribute(
      'href',
      'https://Jane Smith.com'
    );
  });
});
