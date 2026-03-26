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

  const formState = useReactFinalFormState({
    subscription: reactFinalFormSubscription as Record<string, boolean>,
  });

  return {
    submitting: formState.submitting,
    submitError: formState.submitError as unknown,
    values: formState.values as FieldValues | undefined,
    submitFailed: formState.submitFailed,
    hasValidationErrors: formState.hasValidationErrors,
    errors: formState.errors as Record<string, unknown> | undefined,
    submitErrors: formState.submitErrors as Record<string, unknown> | undefined,
    touched: formState.touched as Record<string, boolean> | undefined,
    modifiedSinceLastSubmit: formState.modifiedSinceLastSubmit,
  } as FormState<FieldValues> | Partial<FormState<FieldValues>>;
}
