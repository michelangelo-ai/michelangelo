import type { Size as DialogSize } from 'baseui/dialog';
import type { FormProps } from '#core/components/form/types';

export interface FormDialogProps
  extends Pick<FormProps, 'onSubmit' | 'initialValues' | 'children'> {
  isOpen: boolean;
  onDismiss: () => void;
  heading: string;
  size?: DialogSize;
  submitLabel?: string;
}
