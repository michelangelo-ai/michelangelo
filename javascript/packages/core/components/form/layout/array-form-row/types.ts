import type { FormRowProps } from '#core/components/form/layout/form-row/types';
import type { ArrayLayoutProps } from '#core/components/form/layout/types';

export interface ArrayFormRowProps extends Omit<FormRowProps, 'children'>, ArrayLayoutProps {
  /**
   * Label for the add button.
   * Use when "Add more" is insufficient, e.g. for acronyms: `addLabel="Add ML model"`.
   *
   * @default "Add more"
   */
  addLabel?: string;
}
