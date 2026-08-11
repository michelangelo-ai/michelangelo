import { createContext } from 'react';

import type { FormContextType } from './types';

export const FormContext = createContext<FormContextType | null>(null);
