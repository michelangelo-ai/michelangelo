import { useFormState as useReactFinalFormState } from 'react-final-form';

import type { FormData, FormState } from '../types';

/**
 * Hook for accessing form state with customizable subscriptions.
 *
 * @param subscription - Object specifying which state properties to subscribe to.
 *                      If not provided, subscribes to all available properties.
 *
 * @generic FieldValues - The shape of the form data. Defaults to {@link FormData}
 *
 * @example
 * ```tsx
 * // Subscribe only to submitting state (for submit buttons)
 * const { submitting } = useFormState({ submitting: true });
 *
 * // Subscribe to submit error state
 * const { submitError } = useFormState({ submitError: true });
 *
 * // Subscribe to all state (default behavior)
 * const formState = useFormState();
 * ```
 */
export function useFormState<FieldValues extends FormData = FormData>(): FormState<FieldValues>;
export function useFormState<FieldValues extends FormData = FormData>(
  subscription: Partial<Record<keyof FormState<FieldValues>, boolean>>
): Partial<FormState<FieldValues>>;
export function useFormState<FieldValues extends FormData = FormData>(
  subscription?: Partial<Record<keyof FormState<FieldValues>, boolean>>
): FormState<FieldValues> | Partial<FormState<FieldValues>> {
  const reactFinalFormSubscription = subscription
    ? {
        submitting: subscription.submitting,
        submitError: subscription.submitError,
        values: subscription.values,
        submitFailed: subscription.submitFailed,
        hasValidationErrors: subscription.hasValidationErrors,
        errors: subscription.errors,
        submitErrors: subscription.submitErrors,
        touched: subscription.touched,
        modifiedSinceLastSubmit: subscription.modifiedSinceLastSubmit,
      }
    : undefined;

  const formState = useReactFinalFormState<FieldValues>({
    subscription: reactFinalFormSubscription,
  });

  return {
    submitting: formState.submitting,
    // cast: react-final-form types submitError as any; matches SubmitErrors[FORM_ERROR]'s string |
    // Error shape
    submitError: formState.submitError as string | Error | undefined,
    values: formState.values,
    submitFailed: formState.submitFailed,
    hasValidationErrors: formState.hasValidationErrors,
    errors: formState.errors,
    submitErrors: formState.submitErrors,
    touched: formState.touched,
    modifiedSinceLastSubmit: formState.modifiedSinceLastSubmit,
  };
}
