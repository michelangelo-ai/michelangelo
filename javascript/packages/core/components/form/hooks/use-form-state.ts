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
    // cast: react-final-form's subscription type is a specific mapped type; Record<string, boolean> is the runtime shape
    subscription: reactFinalFormSubscription as Record<string, boolean>,
  });

  // cast: narrowing the assembled object to our parameterized FormState return type
  return {
    submitting: formState.submitting,
    submitError: formState.submitError as unknown, // cast: react-final-form types submitError as its own internal type; we expose as unknown to avoid coupling
    // cast: react-final-form types values as its internal FormValues generic; FieldValues is our parameterized narrowing
    values: formState.values as FieldValues | undefined,
    submitFailed: formState.submitFailed,
    hasValidationErrors: formState.hasValidationErrors,
    // cast: react-final-form types errors/submitErrors/touched as specific internal types; we expose them as plain records
    errors: formState.errors as Record<string, unknown> | undefined,
    submitErrors: formState.submitErrors as Record<string, unknown> | undefined, // cast: react-final-form internal type; exposed as plain Record
    touched: formState.touched as Record<string, boolean> | undefined, // cast: react-final-form internal type; exposed as plain Record
    modifiedSinceLastSubmit: formState.modifiedSinceLastSubmit,
  } as FormState<FieldValues> | Partial<FormState<FieldValues>>;
}
