import type { ReactNode } from 'react';
import type { ArrayFieldOptions } from '#core/components/form/types';

export interface ArrayLayoutProps extends ArrayFieldOptions {
  /**
   * Dot-notation path to the root field of the array layout.
   * @note The field value must be an array.
   *
   * @example 'addresses'
   * @example 'contacts[0].emails'
   */
  rootFieldPath: string;

  /**
   * Render function called once per array item.
   *
   * @param indexedFieldPath - The indexed, dot-notation path for this item,
   *   e.g. `"addresses[0]"`, `"contacts.emails[1]"`.
   *   Prefix nested field names with this value:
   *   `<StringField name={`${indexedFieldPath}.street`} />`
   * @param index - Zero-based position of this item in the array. Use for display labels,
   *   e.g. `Address ${index + 1}`.
   */
  children: (indexedFieldPath: string, index: number) => ReactNode;
}
