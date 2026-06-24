import { FORM_ERROR, getIn } from 'final-form';

import { useFormContext } from '#core/components/form/form-context';
import { useFormState } from '#core/components/form/hooks/use-form-state';

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

  const rawFormLevelError = submitErrors?.[FORM_ERROR];
  let formLevelErrorMessage: string | undefined;
  if (rawFormLevelError instanceof Error) {
    formLevelErrorMessage = rawFormLevelError.message;
  } else if (typeof rawFormLevelError === 'string') {
    formLevelErrorMessage = rawFormLevelError;
  }
  const formLevelEntry: ErrorEntry[] =
    formLevelErrorMessage != null
      ? [{ fieldPath: FORM_ERROR, errorMessage: formLevelErrorMessage }]
      : [];

  if (!errors) return formLevelEntry;

  const fieldEntries: ErrorEntry[] = [];

  for (const [fieldPath, isTouched] of Object.entries(touched ?? {})) {
    if (!isTouched) continue;

    const errorMessage: unknown = getIn(errors, fieldPath);
    if (typeof errorMessage !== 'string') continue;

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
