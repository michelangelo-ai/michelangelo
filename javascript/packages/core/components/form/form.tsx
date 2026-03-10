import { useRef } from 'react';
import { Form as FinalForm } from 'react-final-form';
import { useStyletron } from 'baseui';
import createFocusOnErrorDecorator from 'final-form-focus';

import { FormContext } from './form-context';

import type { FieldRegistry, FormData, FormProps } from './types';

const focusOnErrorDecorator = createFocusOnErrorDecorator();

export const Form = <FieldValues extends FormData = FormData>({
  onSubmit,
  initialValues,
  id,
  children,
  render,
  focusOnError = true,
}: FormProps<FieldValues>) => {
  const [css, theme] = useStyletron();
  const registryRef = useRef<FieldRegistry>(new Map());

  return (
    <FormContext.Provider value={{ fieldRegistry: registryRef.current }}>
      <FinalForm
        onSubmit={onSubmit}
        initialValues={initialValues}
        decorators={focusOnError ? [focusOnErrorDecorator] : undefined}
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
    </FormContext.Provider>
  );
};
