import { useMemo } from 'react';

import { FormContext } from './form-context';

import type { FormContextType } from './types';

/**
 * @description
 * Provider component that allows consumers to register custom field renderers.
 * Custom renderers are checked before falling back to built-in renderers.
 *
 * @example
 * ```tsx
 * const fieldRenderers = {
 *   'hive-select': HiveSelectField,
 * };
 *
 * <FormProvider renderers={fieldRenderers}>
 *   <ConfigDrivenForm config={formConfig} onSubmit={handleSubmit} />
 * </FormProvider>
 * ```
 */
export const FormProvider = ({
  children,
  renderers = {},
}: { children: React.ReactNode } & Partial<FormContextType>) => {
  const contextValue = useMemo<FormContextType>(() => ({ renderers }), [renderers]);

  return <FormContext.Provider value={contextValue}>{children}</FormContext.Provider>;
};
