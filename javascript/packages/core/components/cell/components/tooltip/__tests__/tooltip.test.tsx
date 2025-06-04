import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { CellRendererProps } from '#core/components/cell/types';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { CellTooltipContentRenderer } from '../cell-tooltip-content-renderer';
import { cellTooltipHOC } from '../cell-tooltip-hoc';
import { CellTooltipWrapper } from '../cell-tooltip-wrapper';

describe('Cell Tooltip Components', () => {
  describe('CellTooltipWrapper', () => {
    const defaultContent = <div>Tooltip Content</div>;
    const defaultChildren = <div>Hover Me</div>;

    describe('basic rendering', () => {
      it('renders children', () => {
        render(
          <CellTooltipWrapper content={defaultContent}>{defaultChildren}</CellTooltipWrapper>,
          buildWrapper([getBaseProviderWrapper()])
        );

        expect(screen.getByText('Hover Me')).toBeInTheDocument();
      });

      it('shows tooltip content on hover', async () => {
        const user = userEvent.setup();
        render(
          <CellTooltipWrapper content={defaultContent}>{defaultChildren}</CellTooltipWrapper>,
          buildWrapper([getBaseProviderWrapper()])
        );

        await user.hover(screen.getByText('Hover Me'));
        await screen.findByText('Tooltip Content');
      });
    });

    describe('with action handler', () => {
      const actionHandler = vi.fn();

      beforeEach(() => {
        actionHandler.mockClear();
      });

      it('renders action icon and handles click', async () => {
        const user = userEvent.setup();
        render(
          <CellTooltipWrapper content={defaultContent} actionHandler={actionHandler}>
            {defaultChildren}
          </CellTooltipWrapper>,
          buildWrapper([
            getBaseProviderWrapper(),
            getIconProviderWrapper({
              icons: {
                chevronRight: () => <div>Chevron Right</div>,
              },
            }),
          ])
        );

        await user.hover(screen.getByText('Hover Me'));
        await screen.findByText('Chevron Right');

        const tooltipContent = await screen.findByText('Tooltip Content');
        await user.click(tooltipContent);
        expect(actionHandler).toHaveBeenCalled();
      });
    });
  });

  describe('CellTooltipContentRenderer', () => {
    describe('content rendering', () => {
      it('renders string content', () => {
        const props = {
          column: {
            id: 'test',
            tooltip: {
              content: 'Test Content',
            },
          },
          record: {},
          value: 'Test Value',
        };

        render(<CellTooltipContentRenderer {...props} />, buildWrapper([getBaseProviderWrapper()]));
        expect(screen.getByText('Test Content')).toBeInTheDocument();
      });

      it('renders custom component when content is a function', () => {
        const CustomComponent = () => <div>Custom Content</div>;
        const props = {
          column: {
            id: 'test',
            tooltip: {
              content: CustomComponent,
            },
          },
          record: {},
          value: 'Test Value',
        };

        render(<CellTooltipContentRenderer {...props} />, buildWrapper([getBaseProviderWrapper()]));
        expect(screen.getByText('Custom Content')).toBeInTheDocument();
      });
    });
  });

  describe('cellTooltipHOC', () => {
    const TestComponent = (props: CellRendererProps<string>) => <div>{props.value}</div>;
    const defaultProps = {
      value: 'Test Value',
      column: {
        id: 'test',
        tooltip: {
          content: 'Tooltip Content',
        },
      },
      record: {},
    };

    it('wraps component with tooltip functionality', async () => {
      const user = userEvent.setup();
      const WrappedComponent = cellTooltipHOC(TestComponent);

      render(<WrappedComponent {...defaultProps} />, buildWrapper([getBaseProviderWrapper()]));

      await user.hover(screen.getByText('Test Value'));
      await screen.findByText('Tooltip Content');
    });
  });
});
