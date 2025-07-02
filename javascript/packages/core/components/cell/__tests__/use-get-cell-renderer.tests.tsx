import { render, screen } from '@testing-library/react';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { CellType } from '../constants';
import { useGetCellRenderer } from '../use-get-cell-renderer';

import type { LinkCellConfig } from '../renderers/link/types';
import type { CellRenderer, CellRendererProps } from '../types';

function TestCellRenderer({ props }: { props: CellRendererProps<unknown> }) {
  const getCellRenderer = useGetCellRenderer();
  const CellComponent = getCellRenderer(props);
  return <CellComponent {...props} />;
}

describe('useGetCellRenderer', () => {
  it('should return custom cell renderer when provided', () => {
    const CustomCell: CellRenderer<string> = (props: CellRendererProps<string>) => (
      <div>Custom: {props.value}</div>
    );
    const props: CellRendererProps<string> = {
      column: { id: 'test', Cell: CustomCell },
      record: {},
      value: 'test value',
    };

    render(
      <TestCellRenderer props={props} />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    expect(screen.getByText('Custom: test value')).toBeInTheDocument();
  });

  it('should return cell renderer for known type', () => {
    const props: CellRendererProps<boolean> = {
      column: { id: 'test', type: CellType.BOOLEAN, label: 'Is Active' },
      record: {},
      value: true,
    };

    render(
      <TestCellRenderer props={props} />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    expect(screen.getByText('Is Active')).toBeInTheDocument();
  });

  it('should return link renderer for URL values', () => {
    const props: CellRendererProps<string> = {
      column: { id: 'test' },
      record: {},
      value: 'https://example.com',
    };

    render(
      <TestCellRenderer props={props} />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    const link = screen.getByRole('link');
    expect(link).toHaveAttribute('href', 'https://example.com');
    expect(link).toHaveTextContent('Click here');
  });

  it('should return text cell renderer for URL values without protocol', () => {
    const props: CellRendererProps<string> = {
      column: { id: 'test' },
      record: {},
      value: 'example.com',
    };

    render(
      <TestCellRenderer props={props} />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    expect(screen.queryByRole('link')).not.toBeInTheDocument();
    expect(screen.getByText('example.com')).toBeInTheDocument();
  });

  it('should return text cell renderer for unknown type', () => {
    const props: CellRendererProps<string> = {
      column: { id: 'test', type: 'unknown' },
      record: {},
      value: 'test value',
    };

    render(
      <TestCellRenderer props={props} />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    expect(screen.getByText('test value')).toBeInTheDocument();
  });

  it('should return text cell renderer for no type', () => {
    const props: CellRendererProps<string> = {
      column: { id: 'test' },
      record: {},
      value: 'test value',
    };

    render(
      <TestCellRenderer props={props} />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    expect(screen.getByText('test value')).toBeInTheDocument();
  });

  it('should return link renderer when url is provided in column', () => {
    const props: CellRendererProps<string, LinkCellConfig> = {
      column: { id: 'test', url: 'https://example.com' },
      record: {},
      value: 'Click me',
    };

    render(
      <TestCellRenderer props={props} />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    const link = screen.getByRole('link');
    expect(link).toHaveAttribute('href', 'https://example.com');
    expect(link).toHaveTextContent('Click me');
  });
});
