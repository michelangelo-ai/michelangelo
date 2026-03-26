import type { BaseFieldProps } from '../types';

export interface BooleanFieldProps extends BaseFieldProps<boolean> {
  /**
   * Custom label for the checkbox itself.
   *
   * @default "Enabled" when checked or "Disabled" when unchecked.
   */
  checkboxLabel?: string;

  /**
   * Renders as a toggle switch instead of a standard checkbox.
   *
   * @default false
   */
  toggle?: boolean;
}
