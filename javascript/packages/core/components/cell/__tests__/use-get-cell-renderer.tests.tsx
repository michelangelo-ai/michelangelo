import { render, screen } from '@testing-library/react';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getCellProviderWrapper } from '#core/test/wrappers/get-cell-provider-wrapper';
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

  describe('CellProvider functionality', () => {
    const CustomBadgeRenderer: CellRenderer<string> = (props: CellRendererProps<string>) => (
      <div data-testid="custom-badge">Badge: {props.value}</div>
    );

    const CustomSpecialRenderer: CellRenderer<string> = (props: CellRendererProps<string>) => (
      <div data-testid="custom-special">Special: {props.value}</div>
    );

    it('should use custom renderer from provider when type matches', () => {
      const renderers = {
        CUSTOM_BADGE: CustomBadgeRenderer,
      };

      const props: CellRendererProps<string> = {
        column: { id: 'test', type: 'CUSTOM_BADGE' },
        record: {},
        value: 'test value',
      };

      render(
        <TestCellRenderer props={props} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getCellProviderWrapper({ renderers }),
        ])
      );

      expect(screen.getByTestId('custom-badge')).toBeInTheDocument();
      expect(screen.getByText('Badge: test value')).toBeInTheDocument();
    });

    it('should support multiple custom renderers', () => {
      const renderers = {
        CUSTOM_BADGE: CustomBadgeRenderer,
        CUSTOM_SPECIAL: CustomSpecialRenderer,
      };

      const badgeProps: CellRendererProps<string> = {
        column: { id: 'test1', type: 'CUSTOM_BADGE' },
        record: {},
        value: 'badge value',
      };

      const specialProps: CellRendererProps<string> = {
        column: { id: 'test2', type: 'CUSTOM_SPECIAL' },
        record: {},
        value: 'special value',
      };

      render(
        <div>
          <TestCellRenderer props={badgeProps} />
          <TestCellRenderer props={specialProps} />
        </div>,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getCellProviderWrapper({ renderers }),
        ])
      );

      expect(screen.getByText('Badge: badge value')).toBeInTheDocument();
      expect(screen.getByText('Special: special value')).toBeInTheDocument();
    });

    it('should fall back to built-in renderers when custom renderer not found', () => {
      const renderers = {
        CUSTOM_BADGE: CustomBadgeRenderer,
      };

      const props: CellRendererProps<boolean> = {
        column: { id: 'test', type: CellType.BOOLEAN, label: 'Is Active' },
        record: {},
        value: true,
      };

      render(
        <TestCellRenderer props={props} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getCellProviderWrapper({ renderers }),
        ])
      );

      expect(screen.getByText('Is Active')).toBeInTheDocument();
    });

    it('should prioritize column Cell over provider custom renderers', () => {
      const ColumnCustomCell: CellRenderer<string> = (props: CellRendererProps<string>) => (
        <div>Column Custom: {props.value}</div>
      );

      const renderers = {
        CUSTOM_BADGE: CustomBadgeRenderer,
      };

      const props: CellRendererProps<string> = {
        column: { id: 'test', type: 'CUSTOM_BADGE', Cell: ColumnCustomCell },
        record: {},
        value: 'test value',
      };

      render(
        <TestCellRenderer props={props} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getCellProviderWrapper({ renderers }),
        ])
      );

      expect(screen.getByText('Column Custom: test value')).toBeInTheDocument();
      expect(screen.queryByTestId('custom-badge')).not.toBeInTheDocument();
    });

    it('should prioritize provider custom renderers over built-in renderers', () => {
      const CustomTextRenderer: CellRenderer<string> = (props: CellRendererProps<string>) => (
        <div data-testid="custom-text">Custom Text: {props.value}</div>
      );

      const renderers = {
        [CellType.TEXT]: CustomTextRenderer,
      };

      const props: CellRendererProps<string> = {
        column: { id: 'test', type: CellType.TEXT },
        record: {},
        value: 'test value',
      };

      render(
        <TestCellRenderer props={props} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getCellProviderWrapper({ renderers }),
        ])
      );

      expect(screen.getByTestId('custom-text')).toBeInTheDocument();
      expect(screen.getByText('Custom Text: test value')).toBeInTheDocument();
    });

    it('should work with empty renderers', () => {
      const props: CellRendererProps<string> = {
        column: { id: 'test', type: CellType.TEXT },
        record: {},
        value: 'test value',
      };

      render(
        <TestCellRenderer props={props} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getCellProviderWrapper({ renderers: {} }),
        ])
      );

      expect(screen.getByText('test value')).toBeInTheDocument();
    });

    it('should work without CellProvider (backward compatibility)', () => {
      const props: CellRendererProps<string> = {
        column: { id: 'test', type: CellType.TEXT },
        record: {},
        value: 'test value',
      };

      render(
        <TestCellRenderer props={props} />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.getByText('test value')).toBeInTheDocument();
    });

    it('should maintain URL detection with custom renderers present', () => {
      const renderers = {
        CUSTOM_BADGE: CustomBadgeRenderer,
      };

      const props: CellRendererProps<string> = {
        column: { id: 'test' },
        record: {},
        value: 'https://example.com',
      };

      render(
        <TestCellRenderer props={props} />,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getCellProviderWrapper({ renderers }),
        ])
      );

      const link = screen.getByRole('link');
      expect(link).toHaveAttribute('href', 'https://example.com');
      expect(link).toHaveTextContent('Click here');
    });

    it('should handle undefined renderers gracefully', () => {
      const props: CellRendererProps<string> = {
        column: { id: 'test', type: CellType.TEXT },
        record: {},
        value: 'test value',
      };

      render(
        <TestCellRenderer props={props} />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getCellProviderWrapper()])
      );

      expect(screen.getByText('test value')).toBeInTheDocument();
    });
  });
});
