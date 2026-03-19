import type { Size as DialogSize } from 'baseui/dialog';
import type { ReactNode } from 'react';

export interface ConfirmDialogProps {
  isOpen: boolean;
  onDismiss: () => void;
  heading: string;
  onConfirm: () => Promise<void> | void;
  /** Label for the confirm button. Defaults to 'Confirm'. */
  confirmLabel?: string;
  /** Renders the confirm button in red using the design system's negative color. Use for irreversible actions (e.g. delete). */
  destructive?: boolean;
  children?: ReactNode;
  size?: DialogSize;
}
