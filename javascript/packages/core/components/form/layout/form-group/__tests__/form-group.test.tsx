import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { FormGroup } from '#core/components/form/layout/form-group/form-group';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';

describe('FormGroup', () => {
  it('renders children with optional title, description, tooltip, and endEnhancer', async () => {
    const user = userEvent.setup();

    render(
      <FormGroup
        title="User Settings"
        description="This is **important** information"
        tooltip="Help text"
        endEnhancer={<button>Action</button>}
      >
        <div>Test content</div>
      </FormGroup>,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    expect(screen.getByText('Test content')).toBeInTheDocument();
    expect(screen.getByText('User Settings')).toBeInTheDocument();

    // Renders markdown content wrapped in HTML tags
    expect(screen.queryByText('This is important information')).not.toBeInTheDocument();
    expect(
      screen.getAllByText((_, element) => element?.textContent === 'This is important information')
        .length
    ).toBeGreaterThan(0);

    expect(screen.getByRole('button', { name: 'Action' })).toBeInTheDocument();

    await user.hover(screen.getByRole('img', { name: 'help' }));
    await screen.findByText('Help text');
  });

  it('renders as non-interactive box by default', () => {
    render(
      <FormGroup title="Settings">
        <div>Content</div>
      </FormGroup>,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    expect(screen.queryByRole('button', { name: /Settings/i })).not.toBeInTheDocument();
    expect(screen.getByText('Content')).toBeInTheDocument();
  });

  it('renders as collapsible when collapsible prop is true', async () => {
    const user = userEvent.setup();

    render(
      <FormGroup title="Settings" tooltip="Help text" endEnhancer={<span>Extra</span>} collapsible>
        <div>Expandable content</div>
      </FormGroup>,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    const header = screen.getByRole('button', { name: /Settings/i });
    expect(header).toBeInTheDocument();
    expect(screen.getByRole('img', { name: 'help' })).toBeInTheDocument();
    expect(screen.getByText('Extra')).toBeInTheDocument();

    expect(screen.queryByText('Expandable content')).not.toBeInTheDocument();

    await user.click(header);
    expect(screen.getByText('Expandable content')).toBeInTheDocument();
  });
});
