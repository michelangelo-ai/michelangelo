import type { FormGroupProps } from '#core/components/form/layout/form-group/types';
import type { ArrayLayoutProps } from '#core/components/form/layout/types';

export interface ArrayFormGroupProps
  extends Omit<FormGroupProps, 'children' | 'title' | 'endEnhancer'>,
    ArrayLayoutProps {
  /**
   * Prefix for auto-numbered group titles.
   * e.g. `groupLabel="Address"` → "Address 1", "Address 2", ...
   * Omit to render groups without a title.
   */
  groupLabel?: string;

  /**
   * Label for the add button. Overrides the default derived from `groupLabel`.
   * Use when the default lowercased label is inappropriate, e.g. for acronyms:
   * `addLabel="Add ML model"` instead of `"Add ml model"`.
   *
   * @default `"Add ${groupLabel.toLowerCase()}"` or `"Add more"` if no groupLabel
   */
  addLabel?: string;
}
