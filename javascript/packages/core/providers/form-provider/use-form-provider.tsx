import { useContext } from 'react';

import { FormContext } from './form-context';

export const useFormProvider = () => {
  return useContext(FormContext);
};
