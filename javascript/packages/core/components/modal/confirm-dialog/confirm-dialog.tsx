import { useEffect, useState } from 'react';
import { Banner, KIND as BANNER_KIND } from 'baseui/banner';
import { Button, KIND } from 'baseui/button';
import { PLACEMENT, SIZE } from 'baseui/dialog';

import { Dialog } from '#core/components/dialog/dialog';

import type { ConfirmDialogProps } from './types';

/**
 * Modal dialog component for confirming a user action.
 *
 * Follows the FormDialog pattern: onConfirm is a plain async function that throws
 * on failure. The dialog auto-closes on success and stays open with an error message
 * on failure. Cancel is disabled while the confirmation is in progress.
 *
 * @example
 * ```tsx
 * <ConfirmDialog
 *   isOpen={showModal}
 *   onDismiss={() => setShowModal(false)}
 *   heading="Delete pipeline"
 *   onConfirm={handleDelete}
 *   confirmLabel="Delete"
 * >
 *   Are you sure you want to delete this pipeline? This action cannot be undone.
 * </ConfirmDialog>
 * ```
 */
export const ConfirmDialog: React.FC<ConfirmDialogProps> = ({
  isOpen,
  onDismiss,
  heading,
  onConfirm,
  confirmLabel = 'Confirm',
  confirmButtonColor,
  children,
  size = SIZE.small,
}) => {
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Reset state when the dialog closes so re-opening starts fresh.
  useEffect(() => {
    if (!isOpen) {
      setIsLoading(false);
      setError(null);
    }
  }, [isOpen]);

  const handleConfirm = async () => {
    setIsLoading(true);
    try {
      await onConfirm();
      onDismiss(); // Auto-close on success
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'An unexpected error occurred.');
      setIsLoading(false);
    }
  };

  return (
    <Dialog
      isOpen={isOpen}
      onDismiss={onDismiss}
      heading={heading}
      size={size}
      placement={PLACEMENT.topCenter}
      buttonDock={{
        primaryAction: (
          <Button
            isLoading={isLoading}
            onClick={() => void handleConfirm()}
            style={confirmButtonColor ? { backgroundColor: confirmButtonColor } : undefined}
          >
            {confirmLabel}
          </Button>
        ),
        dismissiveAction: (
          <Button kind={KIND.tertiary} onClick={onDismiss} disabled={isLoading}>
            Cancel
          </Button>
        ),
      }}
    >
      {children}
      {error && <Banner kind={BANNER_KIND.negative}>{error}</Banner>}
    </Dialog>
  );
};
