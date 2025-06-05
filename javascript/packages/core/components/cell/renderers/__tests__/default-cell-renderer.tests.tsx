import { render, screen } from '@testing-library/react';

import { CellType } from '#core/components/cell/constants';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { DefaultCellRenderer } from '../default-cell-renderer';

describe('DefaultCellRenderer', () => {
  it('should render with default styles when no style is provided', () => {
    render(
      <DefaultCellRenderer
        column={{ id: 'test', type: CellType.TEXT }}
        record={{}}
        value="test value"
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    expect(screen.getByText('test value')).toBeInTheDocument();
  });

  it('should apply custom styles when provided', () => {
    const customStyle = { color: 'red' };
    render(
      <DefaultCellRenderer
        column={{ id: 'test', type: CellType.TEXT, style: customStyle }}
        record={{}}
        value="test value"
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    const cell = screen.getByText('test value');
    expect(cell.parentElement).toHaveStyle({ color: 'rgb(255, 0, 0)' });
  });

  it('should render provided cell type', () => {
    render(
      <DefaultCellRenderer
        column={{ id: 'test', type: CellType.BOOLEAN, label: 'Is Active' }}
        record={{}}
        value={true}
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    expect(screen.getByText('Is Active')).toBeInTheDocument();
  });

  it('should handle undefined values', () => {
    const { container } = render(
      <DefaultCellRenderer
        column={{ id: 'test', type: CellType.TEXT }}
        record={{}}
        value={undefined}
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    expect(container).not.toBeEmptyDOMElement();
  });

  it('should handle null values', () => {
    const { container } = render(
      <DefaultCellRenderer column={{ id: 'test', type: CellType.TEXT }} record={{}} value={null} />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    expect(container).not.toBeEmptyDOMElement();
  });
});
