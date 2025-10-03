import { useFormState } from '#core/components/form/hooks/use-form-state';

// TODO: add error banner display styling
export function FormErrorBanner() {
  const { submitError } = useFormState({ submitError: true });

  return submitError ? <span>{String(submitError)}</span> : null;
}
