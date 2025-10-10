import type { Size as DialogSize } from 'baseui/dialog';
import type { FormData, FormProps } from '#core/components/form/types';

export interface FormDialogProps<FieldValues extends FormData = FormData>
  extends Pick<FormProps<FieldValues>, 'onSubmit' | 'initialValues' | 'children'> {
  isOpen: boolean;
  onDismiss: () => void;
  heading: string;
  size?: DialogSize;
  submitLabel?: string;
}
