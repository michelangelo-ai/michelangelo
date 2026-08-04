import { useMemo } from 'react';

import { FormContext } from './form-context';

import type { FormContextType } from './types';

/**
 * @description
 * Provider component that allows consumers to register custom field renderers
 * and layout renderers. Custom renderers are checked before falling back to
 * built-in renderers.
 *
 * @example
 * ```tsx
 * const fieldRenderers = {
 *   'hive-select': HiveSelectField,
 * };
 *
 * const layoutRenderers = {
 *   tabs: TabsLayoutRenderer,
 * };
 *
 * <FormProvider renderers={fieldRenderers} layouts={layoutRenderers}>
 *   <ConfigDrivenForm config={formConfig} onSubmit={handleSubmit} />
 * </FormProvider>
 * ```
 */
export const FormProvider = ({
  children,
  renderers = {},
  layouts = {},
}: { children: React.ReactNode } & Partial<FormContextType>) => {
  const contextValue = useMemo<FormContextType>(
    () => ({ renderers, layouts }),
    [renderers, layouts]
  );

  return <FormContext.Provider value={contextValue}>{children}</FormContext.Provider>;
};
