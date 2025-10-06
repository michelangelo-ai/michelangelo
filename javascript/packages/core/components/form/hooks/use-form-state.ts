import { useFormState as useReactFinalFormState } from 'react-final-form';

import type { FormState } from '../types';

/**
 * Hook for accessing form state with customizable subscriptions.
 *
 * @param subscription - Object specifying which state properties to subscribe to.
 *                      If not provided, subscribes to all available properties.
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
export function useFormState(): FormState;
export function useFormState(
  subscription: Partial<Record<keyof FormState, boolean>>
): Partial<FormState>;
export function useFormState(
  subscription?: Partial<Record<keyof FormState, boolean>>
): FormState | Partial<FormState> {
  const reactFinalFormSubscription = subscription
    ? {
        submitting: subscription.submitting,
        submitError: subscription.submitError,
      }
    : undefined;

  const formState = useReactFinalFormState({
    subscription: reactFinalFormSubscription as Record<string, boolean>,
  });

  return {
    submitting: formState.submitting,
    submitError: formState.submitError as unknown,
  } as FormState | Partial<FormState>;
}
