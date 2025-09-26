import React from 'react';
import { Form as FinalForm } from 'react-final-form';

import type { FormProps } from './types';

export const Form: React.FC<FormProps> = ({ onSubmit, initialValues, id, children }) => {
  return (
    <FinalForm
      onSubmit={onSubmit}
      initialValues={initialValues}
      render={({ handleSubmit }) => (
        // react-final-form internally uses a promise to handle the form submission
        // so we need to disable the eslint rule. I tested the execution of handleSubmit
        // and it is synchronous.

        // eslint-disable-next-line @typescript-eslint/no-misused-promises
        <form id={id} onSubmit={handleSubmit}>
          {children}
        </form>
      )}
    />
  );
};
