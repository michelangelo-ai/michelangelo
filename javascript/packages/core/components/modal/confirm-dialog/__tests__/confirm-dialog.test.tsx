import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { ConfirmDialog } from '../confirm-dialog';

const wrapper = buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()]);

const defaultProps = {
  isOpen: true,
  onDismiss: vi.fn(),
  heading: 'Confirm modal title',
  onConfirm: vi.fn(),
  confirmLabel: 'Confirm button text',
};

beforeEach(() => {
  vi.clearAllMocks();
});

it('renders dialog with heading and buttons', async () => {
  render(<ConfirmDialog {...defaultProps} />, wrapper);

  await screen.findByRole('dialog', { name: 'Confirm modal title' });
  expect(screen.getByRole('button', { name: 'Confirm button text' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument();
});

it('renders body content as children', async () => {
  render(
    <ConfirmDialog {...defaultProps}>
      <p>Confirm modal body</p>
    </ConfirmDialog>,
    wrapper
  );

  await screen.findByRole('dialog', { name: 'Confirm modal title' });
  expect(screen.getByText('Confirm modal body')).toBeInTheDocument();
});

it('renders with default confirm label when confirmLabel is omitted', async () => {
  render(
    <ConfirmDialog isOpen={true} onDismiss={vi.fn()} heading="Minimal" onConfirm={vi.fn()} />,
    wrapper
  );

  await screen.findByRole('dialog', { name: 'Minimal' });
  expect(screen.getByRole('button', { name: 'Confirm' })).toBeInTheDocument();
});

it('does not render when closed', async () => {
  render(<ConfirmDialog {...defaultProps} isOpen={false} />, wrapper);

  try {
    await screen.findByRole('dialog', {}, { timeout: 100 });
    throw new Error('Dialog should not be in the document');
  } catch (e: unknown) {
    if (e instanceof Error && e.name !== 'TestingLibraryElementError') throw e;
  }
});

it('calls onConfirm and auto-closes on success', async () => {
  const user = userEvent.setup();
  const onConfirm = vi.fn().mockResolvedValue(undefined);
  const onDismiss = vi.fn();

  render(<ConfirmDialog {...defaultProps} onConfirm={onConfirm} onDismiss={onDismiss} />, wrapper);

  await user.click(screen.getByRole('button', { name: 'Confirm button text' }));

  await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
  await waitFor(() => expect(onDismiss).toHaveBeenCalledTimes(1));
});

it('calls onDismiss when cancel is clicked', async () => {
  const user = userEvent.setup();
  const onDismiss = vi.fn();

  render(<ConfirmDialog {...defaultProps} onDismiss={onDismiss} />, wrapper);

  await user.click(screen.getByRole('button', { name: 'Cancel' }));
  expect(onDismiss).toHaveBeenCalledTimes(1);
});

it('shows error message and stays open when onConfirm throws', async () => {
  const user = userEvent.setup();
  const onConfirm = vi.fn().mockRejectedValue(new Error('Delete failed'));
  const onDismiss = vi.fn();

  render(<ConfirmDialog {...defaultProps} onConfirm={onConfirm} onDismiss={onDismiss} />, wrapper);

  await user.click(screen.getByRole('button', { name: 'Confirm button text' }));

  await screen.findByText('Delete failed');
  expect(onDismiss).not.toHaveBeenCalled();
  expect(screen.getByRole('dialog', { name: 'Confirm modal title' })).toBeInTheDocument();
});

it('re-enables confirm button after error', async () => {
  const user = userEvent.setup();
  const onConfirm = vi.fn().mockRejectedValue(new Error('Failed'));

  render(<ConfirmDialog {...defaultProps} onConfirm={onConfirm} />, wrapper);

  await user.click(screen.getByRole('button', { name: 'Confirm button text' }));
  await screen.findByText('Failed');

  expect(screen.getByRole('button', { name: 'Confirm button text' })).not.toBeDisabled();
});

it('disables cancel button while loading', async () => {
  const user = userEvent.setup();
  let resolveConfirm!: () => void;
  const onConfirm = vi.fn(
    () =>
      new Promise<void>((resolve) => {
        resolveConfirm = resolve;
      })
  );

  render(<ConfirmDialog {...defaultProps} onConfirm={onConfirm} />, wrapper);

  await user.click(screen.getByRole('button', { name: 'Confirm button text' }));

  await waitFor(() => expect(onConfirm).toHaveBeenCalled());
  expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled();

  resolveConfirm();
});

it('applies confirmButtonColor as inline background on the confirm button', async () => {
  render(<ConfirmDialog {...defaultProps} confirmButtonColor="#DE1135" />, wrapper);

  await screen.findByRole('dialog', { name: 'Confirm modal title' });
  expect(screen.getByRole('button', { name: 'Confirm button text' })).toHaveStyle({
    backgroundColor: '#DE1135',
  });
});

it('clears error and resets state when dialog is reopened', async () => {
  const user = userEvent.setup();
  const onConfirm = vi.fn().mockRejectedValue(new Error('Failed'));
  const onDismiss = vi.fn();

  const { rerender } = render(
    <ConfirmDialog {...defaultProps} onConfirm={onConfirm} onDismiss={onDismiss} />,
    wrapper
  );

  await user.click(screen.getByRole('button', { name: 'Confirm button text' }));
  await screen.findByText('Failed');

  // Close and reopen
  rerender(
    <ConfirmDialog {...defaultProps} isOpen={false} onConfirm={onConfirm} onDismiss={onDismiss} />
  );
  rerender(
    <ConfirmDialog {...defaultProps} isOpen={true} onConfirm={onConfirm} onDismiss={onDismiss} />
  );

  await screen.findByRole('dialog', { name: 'Confirm modal title' });
  expect(screen.queryByText('Failed')).not.toBeInTheDocument();
});
