import type { ButtonProps } from 'baseui/button';
import type { ReactNode } from 'react';

export interface SubmitButtonProps extends Omit<ButtonProps, 'type'> {
  children: ReactNode;

  /**
   * Form ID to associate this button with a form element
   * Use when button is rendered outside the form (e.g., in modal footer)
   */
  formId?: string;
}
