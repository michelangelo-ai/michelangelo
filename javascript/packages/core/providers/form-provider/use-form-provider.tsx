import { useContext } from 'react';

import { FormContext } from './form-context';

/**
 * Accesses custom field and layout renderers registered via FormProvider.
 *
 * @returns Form context containing custom renderer mappings, or undefined if no FormProvider exists
 *
 * @example
 * ```typescript
 * function useCustomFieldRenderer(type: string) {
 *   const formContext = useFormProvider();
 *   return formContext?.renderers[type];
 * }
 * ```
 */
export const useFormProvider = () => {
  return useContext(FormContext);
};
