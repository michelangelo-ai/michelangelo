import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getInterpolationProviderWrapper } from '#core/test/wrappers/get-interpolation-provider-wrapper';
import { ShareTableUrlButton } from '../share-table-url-button';

const defaultProps = {
  buildShareUrl: vi.fn(() => 'https://example.com?tb.users.gf=test'),
  currentState: { globalFilter: 'test' },
};

function renderButton(overrides = {}) {
  return render(
    <ShareTableUrlButton {...defaultProps} {...overrides} />,
    buildWrapper([getInterpolationProviderWrapper()])
  );
}

describe('ShareTableUrlButton', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('renders with "Share" label by default', () => {
    renderButton();
    expect(screen.getByRole('button', { name: /share/i })).toBeInTheDocument();
  });

  it('calls buildShareUrl with currentState on click', async () => {
    const user = userEvent.setup();
    renderButton();

    await user.click(screen.getByRole('button', { name: /share/i }));

    expect(defaultProps.buildShareUrl).toHaveBeenCalledWith(defaultProps.currentState);
  });

  it('shows "Copied!" confirmation after click', async () => {
    const user = userEvent.setup();
    renderButton();

    await user.click(screen.getByRole('button', { name: /share/i }));

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /copied/i })).toBeInTheDocument();
    });
  });

  it('reverts back to "Share" after 2 seconds', async () => {
    vi.useFakeTimers();
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime.bind(vi) });
    renderButton();

    await user.click(screen.getByRole('button', { name: /share/i }));
    // Let the clipboard Promise resolve (microtask) so setCopied(true) fires
    await act(async () => {
      await Promise.resolve();
    });

    // Advance past the 2-second reset
    act(() => {
      vi.advanceTimersByTime(2100);
    });

    expect(screen.getByRole('button', { name: /share/i })).toBeInTheDocument();

    vi.useRealTimers();
  });
});
