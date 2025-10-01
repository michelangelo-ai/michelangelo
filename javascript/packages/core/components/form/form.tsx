import React from 'react';
import { Form as FinalForm } from 'react-final-form';
import { useStyletron } from 'baseui';

import type { FormProps } from './types';

export const Form: React.FC<FormProps> = ({ onSubmit, initialValues, id, children, render }) => {
  const [css, theme] = useStyletron();

  return (
    <FinalForm
      onSubmit={onSubmit}
      initialValues={initialValues}
      render={({ handleSubmit }) => {
        const formElement = (
          <form
            className={css({
              display: 'flex',
              flexDirection: 'column',
              gap: theme.sizing.scale600,
            })}
            id={id}
            // react-final-form internally uses a promise to handle the form submission
            // so we need to disable the eslint rule. I tested the execution of handleSubmit
            // and it is synchronous.
            // eslint-disable-next-line @typescript-eslint/no-misused-promises
            onSubmit={handleSubmit}
          >
            {children}
          </form>
        );

        return render ? render(formElement) : formElement;
      }}
    />
  );
};
