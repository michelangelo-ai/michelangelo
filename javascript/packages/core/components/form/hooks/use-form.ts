import { useForm as useReactFinalForm } from 'react-final-form';

import { useFormContext } from '#core/components/form/form-context';

import type { FormApi } from '../types';

export function useForm(): FormApi {
  const { fieldRegistry } = useFormContext();
  const form = useReactFinalForm();
  return {
    fieldRegistry,
    change: form.change,
    submit: form.submit,
  };
}
