import type { Size as DialogSize } from 'baseui/dialog';
import type { ReactNode } from 'react';

export interface ConfirmDialogProps {
  isOpen: boolean;
  onDismiss: () => void;
  heading: string;
  onConfirm: () => Promise<void> | void;
  /** Label for the confirm button. Defaults to 'Confirm'. */
  confirmLabel?: string;
  /** Background color for the confirm button (e.g. theme.colors.backgroundNegative for destructive actions). */
  confirmButtonColor?: string;
  children?: ReactNode;
  size?: DialogSize;
}
