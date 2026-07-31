import { useMemo } from 'react';

import { FormContext } from './form-context';

import type { FormContextType } from './types';

export const FormProvider = ({
  children,
  renderers = {},
  validators = {},
  layouts = {},
}: { children: React.ReactNode } & Partial<FormContextType>) => {
  const contextValue = useMemo<FormContextType>(
    () => ({ renderers, validators, layouts }),
    [renderers, validators, layouts]
  );

  return <FormContext.Provider value={contextValue}>{children}</FormContext.Provider>;
};
