import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { HelpTooltip } from '../help-tooltip';

const helpIcon = () => <div>circleI</div>;

describe('HelpTooltip', () => {
  it('should render help icon', () => {
    render(
      <HelpTooltip text="Help text" />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper({
          icons: {
            circleI: helpIcon,
          },
        }),
      ])
    );

    expect(screen.getByText('circleI')).toBeInTheDocument();
  });

  it('should have help cursor style', () => {
    render(
      <HelpTooltip text="Help text" />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper({
          icons: {
            circleI: helpIcon,
          },
        }),
      ])
    );

    const iconContainer = screen.getByText('circleI').closest('span');
    expect(iconContainer).toHaveStyle({ cursor: 'help' });
  });

  it('should render markdown content in tooltip', async () => {
    const user = userEvent.setup();
    render(
      <HelpTooltip text="**Bold** and *italic* text" />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper({
          icons: {
            circleI: helpIcon,
          },
        }),
      ])
    );

    await user.hover(screen.getByText('circleI'));
    const tooltipContent = await screen.findByText('Bold');
    expect(tooltipContent).toMatchInlineSnapshot(`
      <strong
        class="ba be bc bd"
      >
        Bold
      </strong>
    `);
  });
});
