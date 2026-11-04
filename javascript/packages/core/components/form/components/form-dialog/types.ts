import type { Size as DialogSize } from 'baseui/dialog';
import type { FormData, FormProps } from '#core/components/form/types/form-types';

export interface FormDialogProps<FieldValues extends FormData = FormData>
  extends Pick<FormProps<FieldValues>, 'onSubmit' | 'initialValues' | 'children'> {
  isOpen: boolean;
  onDismiss: () => void;
  heading: string;
  size?: DialogSize;
  submitLabel?: string;
  /** Disables the submit button. */
  submitDisabled?: boolean;
  /** Tooltip on the submit button explaining why it is disabled. Display only. */
  submitDisabledReason?: string;
}
