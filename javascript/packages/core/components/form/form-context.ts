import { createContext, useContext } from 'react';

import type { FormInstance } from './types';

export const FormContext = createContext<FormInstance | null>(null);

export function useFormContext(): FormInstance {
  const formContext = useContext(FormContext);
  if (!formContext) throw new Error('useFormContext must be used within a <Form>');
  return formContext;
}
