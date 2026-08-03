import { useMemo } from 'react';

import { FormContext } from './form-context';

import type { FormContextType } from './types';

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
