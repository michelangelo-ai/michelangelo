import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';
import { ConfirmDialog } from '../confirm-dialog';

beforeEach(() => {
  vi.clearAllMocks();
});

it('renders dialog with heading and buttons', async () => {
  const handleDialogDismiss = vi.fn();
  const handleDialogConfirm = vi.fn();
  render(
    <ConfirmDialog
      isOpen={true}
      onDismiss={handleDialogDismiss}
      heading="Confirm modal title"
      onConfirm={handleDialogConfirm}
      confirmLabel="Confirm button text"
    />,
    buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
  );

  await screen.findByRole('dialog', { name: 'Confirm modal title' });
  expect(screen.getByRole('button', { name: 'Confirm button text' })).toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument();
});

it('renders body content as children', async () => {
  const handleDialogDismiss = vi.fn();
  const handleDialogConfirm = vi.fn();
  render(
    <ConfirmDialog
      isOpen={true}
      onDismiss={handleDialogDismiss}
      heading="Confirm modal title"
      onConfirm={handleDialogConfirm}
      confirmLabel="Confirm button text"
    >
      <p>Confirm modal body</p>
    </ConfirmDialog>,
    buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
  );

  await screen.findByRole('dialog', { name: 'Confirm modal title' });
  expect(screen.getByText('Confirm modal body')).toBeInTheDocument();
});

it('renders with default confirm label when confirmLabel is omitted', async () => {
  const handleDialogDismiss = vi.fn();
  const handleDialogConfirm = vi.fn();
  render(
    <ConfirmDialog
      isOpen={true}
      onDismiss={handleDialogDismiss}
      heading="Minimal"
      onConfirm={handleDialogConfirm}
    />,
    buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
  );

  await screen.findByRole('dialog', { name: 'Minimal' });
  expect(screen.getByRole('button', { name: 'Confirm' })).toBeInTheDocument();
});

it('does not render when closed', async () => {
  const handleDialogDismiss = vi.fn();
  const handleDialogConfirm = vi.fn();
  render(
    <ConfirmDialog
      isOpen={false}
      onDismiss={handleDialogDismiss}
      heading="Confirm modal title"
      onConfirm={handleDialogConfirm}
      confirmLabel="Confirm button text"
    />,
    buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
  );

  try {
    await screen.findByRole('dialog', {}, { timeout: 100 });
    throw new Error('Dialog should not be in the document');
  } catch (e: unknown) {
    if (e instanceof Error && e.name !== 'TestingLibraryElementError') throw e;
  }
});

it('calls onConfirm and auto-closes on success', async () => {
  const user = userEvent.setup();
  const handleDialogConfirm = vi.fn().mockResolvedValue(undefined);
  const handleDialogDismiss = vi.fn();

  render(
    <ConfirmDialog
      isOpen={true}
      onDismiss={handleDialogDismiss}
      heading="Confirm modal title"
      onConfirm={handleDialogConfirm}
      confirmLabel="Confirm button text"
    />,
    buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
  );

  await user.click(screen.getByRole('button', { name: 'Confirm button text' }));

  await waitFor(() => expect(handleDialogConfirm).toHaveBeenCalledTimes(1));
  await waitFor(() => expect(handleDialogDismiss).toHaveBeenCalledTimes(1));
});

it('calls onDismiss when cancel is clicked', async () => {
  const user = userEvent.setup();
  const handleDialogDismiss = vi.fn();
  const handleDialogConfirm = vi.fn();

  render(
    <ConfirmDialog
      isOpen={true}
      onDismiss={handleDialogDismiss}
      heading="Confirm modal title"
      onConfirm={handleDialogConfirm}
      confirmLabel="Confirm button text"
    />,
    buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
  );

  await user.click(screen.getByRole('button', { name: 'Cancel' }));
  expect(handleDialogDismiss).toHaveBeenCalledTimes(1);
});

it('shows error message and stays open when onConfirm throws', async () => {
  const user = userEvent.setup();
  const handleDialogConfirm = vi.fn().mockRejectedValue(new Error('Delete failed'));
  const handleDialogDismiss = vi.fn();

  render(
    <ConfirmDialog
      isOpen={true}
      onDismiss={handleDialogDismiss}
      heading="Confirm modal title"
      onConfirm={handleDialogConfirm}
      confirmLabel="Confirm button text"
    />,
    buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
  );

  await user.click(screen.getByRole('button', { name: 'Confirm button text' }));

  await screen.findByText('Delete failed');
  expect(handleDialogDismiss).not.toHaveBeenCalled();
  expect(screen.getByRole('dialog', { name: 'Confirm modal title' })).toBeInTheDocument();
});

it('re-enables confirm button after error', async () => {
  const user = userEvent.setup();
  const handleDialogConfirm = vi.fn().mockRejectedValue(new Error('Failed'));
  const handleDialogDismiss = vi.fn();

  render(
    <ConfirmDialog
      isOpen={true}
      onDismiss={handleDialogDismiss}
      heading="Confirm modal title"
      onConfirm={handleDialogConfirm}
      confirmLabel="Confirm button text"
    />,
    buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
  );

  await user.click(screen.getByRole('button', { name: 'Confirm button text' }));
  await screen.findByText('Failed');

  expect(screen.getByRole('button', { name: 'Confirm button text' })).not.toBeDisabled();
});

it('disables cancel button while loading', async () => {
  const user = userEvent.setup();
  let resolveConfirm!: () => void;
  const handleDialogConfirm = vi.fn(
    () =>
      new Promise<void>((resolve) => {
        resolveConfirm = resolve;
      })
  );
  const handleDialogDismiss = vi.fn();

  render(
    <ConfirmDialog
      isOpen={true}
      onDismiss={handleDialogDismiss}
      heading="Confirm modal title"
      onConfirm={handleDialogConfirm}
      confirmLabel="Confirm button text"
    />,
    buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
  );

  await user.click(screen.getByRole('button', { name: 'Confirm button text' }));

  await waitFor(() => expect(handleDialogConfirm).toHaveBeenCalled());
  expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled();

  resolveConfirm();
});

it('applies confirmButtonColor as inline background on the confirm button', async () => {
  const handleDialogDismiss = vi.fn();
  const handleDialogConfirm = vi.fn();
  render(
    <ConfirmDialog
      isOpen={true}
      onDismiss={handleDialogDismiss}
      heading="Confirm modal title"
      onConfirm={handleDialogConfirm}
      confirmLabel="Confirm button text"
      destructive
    />,
    buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
  );

  await screen.findByRole('dialog', { name: 'Confirm modal title' });
  expect(screen.getByRole('button', { name: 'Confirm button text' })).toHaveStyle({
    backgroundColor: '#DE1135',
  });
});

it('clears error and resets state when dialog is reopened', async () => {
  const user = userEvent.setup();
  const handleDialogConfirm = vi.fn().mockRejectedValue(new Error('Failed'));
  const handleDialogDismiss = vi.fn();

  const { rerender } = render(
    <ConfirmDialog
      isOpen={true}
      onDismiss={handleDialogDismiss}
      heading="Confirm modal title"
      onConfirm={handleDialogConfirm}
      confirmLabel="Confirm button text"
    />,
    buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
  );

  await user.click(screen.getByRole('button', { name: 'Confirm button text' }));
  await screen.findByText('Failed');

  // Close and reopen
  rerender(
    <ConfirmDialog
      isOpen={false}
      onDismiss={handleDialogDismiss}
      heading="Confirm modal title"
      onConfirm={handleDialogConfirm}
      confirmLabel="Confirm button text"
    />
  );
  rerender(
    <ConfirmDialog
      isOpen={true}
      onDismiss={handleDialogDismiss}
      heading="Confirm modal title"
      onConfirm={handleDialogConfirm}
      confirmLabel="Confirm button text"
    />
  );

  await screen.findByRole('dialog', { name: 'Confirm modal title' });
  expect(screen.queryByText('Failed')).not.toBeInTheDocument();
});
