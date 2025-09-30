import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { CollapsibleBox } from '../collapsible-box';

describe('CollapsibleBox', () => {
  describe('Controlled mode warnings', () => {
    const expectedWarning =
      'CollapsibleBox: `expanded` prop provided without `onToggle`. ' +
      'This will make the component unresponsive to user interaction. ' +
      'Either provide `onToggle` or use `defaultExpanded` instead.';

    let consoleWarnSpy: ReturnType<typeof vi.spyOn>;

    beforeEach(() => {
      consoleWarnSpy = vi.spyOn(console, 'warn').mockImplementation(() => null);
    });

    afterEach(() => {
      consoleWarnSpy.mockRestore();
    });

    it('warns when expanded is provided without onToggle', () => {
      render(
        <CollapsibleBox expanded={false} title="Test">
          Content
        </CollapsibleBox>,
        buildWrapper([getBaseProviderWrapper()])
      );

      expect(consoleWarnSpy).toHaveBeenCalledWith(expectedWarning);
    });

    it('does not warn when expanded is provided with onToggle', () => {
      render(
        <CollapsibleBox expanded={false} onToggle={() => null} title="Test">
          Content
        </CollapsibleBox>,
        buildWrapper([getBaseProviderWrapper()])
      );

      const calls = consoleWarnSpy.mock.calls.filter((call) => call[0] === expectedWarning);
      expect(calls).toHaveLength(0);
    });

    it('does not warn when using defaultExpanded (uncontrolled)', () => {
      render(
        <CollapsibleBox defaultExpanded={true} title="Test">
          Content
        </CollapsibleBox>,
        buildWrapper([getBaseProviderWrapper()])
      );

      const calls = consoleWarnSpy.mock.calls.filter((call) => call[0] === expectedWarning);
      expect(calls).toHaveLength(0);
    });
  });

  it('renders collapsed by default', () => {
    render(
      <CollapsibleBox title="Collapsible Section">
        <div>Hidden Content</div>
      </CollapsibleBox>,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByRole('button', { expanded: false })).toBeInTheDocument();
    expect(screen.queryByText('Hidden Content')).not.toBeInTheDocument();
  });

  it('renders expanded when defaultExpanded is true', () => {
    render(
      <CollapsibleBox title="Collapsible Section" defaultExpanded={true}>
        <div>Content</div>
      </CollapsibleBox>,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByRole('button', { expanded: true })).toBeInTheDocument();
    expect(screen.getByText('Content')).toBeInTheDocument();
  });

  it('toggles expansion when clicked in uncontrolled mode', async () => {
    const user = userEvent.setup();

    render(
      <CollapsibleBox title="Toggle Test">
        <div>Collapsible Content</div>
      </CollapsibleBox>,
      buildWrapper([getBaseProviderWrapper()])
    );

    await user.click(screen.getByRole('button', { expanded: false }));
    expect(screen.getByRole('button', { expanded: true })).toBeInTheDocument();

    await user.click(screen.getByRole('button', { expanded: true }));
    expect(screen.getByRole('button', { expanded: false })).toBeInTheDocument();
  });

  it('calls onToggle when provided in uncontrolled mode', async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();

    render(
      <CollapsibleBox title="Toggle Test" onToggle={onToggle}>
        Content
      </CollapsibleBox>,
      buildWrapper([getBaseProviderWrapper()])
    );

    await user.click(screen.getByRole('button'));
    expect(onToggle).toHaveBeenCalledWith(true);

    await user.click(screen.getByRole('button'));
    expect(onToggle).toHaveBeenCalledWith(false);
  });

  it('starts with defaultExpanded state in uncontrolled mode', async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();

    render(
      <CollapsibleBox title="Test" defaultExpanded={true} onToggle={onToggle}>
        Content
      </CollapsibleBox>,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByRole('button', { expanded: true })).toBeInTheDocument();

    await user.click(screen.getByRole('button'));
    expect(onToggle).toHaveBeenCalledWith(false);
  });

  it('uses expanded prop to control state in controlled mode', async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();

    const { rerender } = render(
      <CollapsibleBox title="Controlled Test" expanded={false} onToggle={onToggle}>
        Content
      </CollapsibleBox>,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByRole('button', { expanded: false })).toBeInTheDocument();

    await user.click(screen.getByRole('button'));
    expect(onToggle).toHaveBeenCalledWith(true);
    expect(screen.getByRole('button', { expanded: false })).toBeInTheDocument();

    rerender(
      <CollapsibleBox title="Controlled Test" expanded={true} onToggle={onToggle}>
        Content
      </CollapsibleBox>
    );

    expect(screen.getByRole('button', { expanded: true })).toBeInTheDocument();
  });

  it('remains in controlled state when toggled without onToggle', async () => {
    const user = userEvent.setup();

    render(
      <CollapsibleBox title="Stuck Test" expanded={false}>
        Content
      </CollapsibleBox>,
      buildWrapper([getBaseProviderWrapper()])
    );

    await user.click(screen.getByRole('button', { expanded: false }));
    expect(screen.getByRole('button', { expanded: false })).toBeInTheDocument();
  });

  it('does not toggle when disabled', async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();

    render(
      <CollapsibleBox title="Disabled Test" disabled={true} onToggle={onToggle}>
        Content
      </CollapsibleBox>,
      buildWrapper([getBaseProviderWrapper()])
    );

    await user.click(screen.getByRole('button', { expanded: false }));

    expect(onToggle).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { expanded: false })).toBeInTheDocument();
  });

  it('applies overrides to elements', () => {
    const testId = 'test-container';

    render(
      <CollapsibleBox
        title="Override Test"
        overrides={{
          Container: {
            props: { 'data-testid': testId },
          },
          HeaderTitle: {
            props: { 'data-testid': 'test-title' },
          },
        }}
      >
        Content
      </CollapsibleBox>,
      buildWrapper([getBaseProviderWrapper()])
    );

    expect(screen.getByTestId(testId)).toBeInTheDocument();
    expect(screen.getByTestId('test-title')).toHaveTextContent('Override Test');
  });
});
