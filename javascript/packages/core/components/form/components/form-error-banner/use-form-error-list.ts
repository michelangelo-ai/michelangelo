import { FORM_ERROR } from 'final-form';

import { useFormContext } from '#core/components/form/form-context';
import { useFormState } from '#core/components/form/hooks/use-form-state';
import { toFlatDotPathMap } from '#core/utils/object-utils';

import type { ErrorEntry } from './types';

/**
 * @returns A list of error entries for the form, including form-level
 * submission errors and field-level validation errors.
 */
export function useFormErrorList(): ErrorEntry[] {
  const { fieldRegistry } = useFormContext();
  const { submitErrors, errors, touched } = useFormState({
    submitErrors: true,
    errors: true,
    touched: true,
  });

  const formLevelError = submitErrors?.[FORM_ERROR] as Error | undefined;
  const formLevelEntry: ErrorEntry[] = formLevelError
    ? [{ fieldPath: FORM_ERROR, errorMessage: String(formLevelError) }]
    : [];

  if (!errors) return formLevelEntry;

  const fieldEntries: ErrorEntry[] = [];

  for (const [fieldPath, errorMessage] of Object.entries(toFlatDotPathMap(errors))) {
    if (typeof errorMessage !== 'string') continue;
    if (!touched?.[fieldPath]) continue;

    fieldEntries.push({
      fieldPath,
      fieldLabel: fieldRegistry.get(fieldPath)?.label,
      errorMessage,
      focus: () => {
        document.querySelector<HTMLElement>(`[name="${fieldPath}"]`)?.focus();
      },
    });
  }

  return [...formLevelEntry, ...fieldEntries];
}
