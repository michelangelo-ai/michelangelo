import React, { useId } from 'react';
import { Button, KIND } from 'baseui/button';
import { PLACEMENT, SIZE } from 'baseui/dialog';
import { FORM_ERROR } from 'final-form';

import { Dialog } from '#core/components/dialog/dialog';
import { FormErrorBanner } from '#core/components/form/components/form-error-banner';
import { SubmitButton } from '#core/components/form/components/submit-button/submit-button';
import { Form } from '#core/components/form/form';

import type { FormDialogProps } from './types';

/**
 * Modal dialog component that wraps a form with automatic submit button integration.
 *
 * Combines Form and Dialog components using render prop pattern. Automatically
 * generates form IDs and connects external submit buttons. Auto-closes modal on
 * successful form submission but stays open if submission fails.
 *
 * @example
 * ```tsx
 * // Basic usage with form fields as children
 * <FormDialog
 *   isOpen={showModal}
 *   onDismiss={() => setShowModal(false)}
 *   heading="Create User"
 *   onSubmit={handleSubmit}
 *   submitLabel="Create"
 * >
 *   <StringField name="email" label="Email" />
 *   <FormGroup title="Settings">
 *     <BooleanField name="isActive" label="Active" />
 *   </FormGroup>
 * </FormDialog>
 * ```
 */
export const FormDialog: React.FC<FormDialogProps> = ({
  isOpen,
  onDismiss,
  heading,
  size = SIZE.large,
  onSubmit,
  initialValues,
  submitLabel = 'Submit',
  children,
}) => {
  const formId = useId();

  const handleSubmit = async (values: Record<string, unknown>) => {
    try {
      await onSubmit(values);
      onDismiss(); // Auto-close on successful submit
    } catch (error: unknown) {
      return { [FORM_ERROR]: error };
    }
  };

  return (
    <Form
      id={formId}
      initialValues={initialValues}
      onSubmit={handleSubmit}
      render={(formElement) => (
        <Dialog
          isOpen={isOpen}
          onDismiss={onDismiss}
          heading={heading}
          size={size}
          placement={PLACEMENT.topCenter}
          buttonDock={{
            primaryAction: <SubmitButton formId={formId}>{submitLabel}</SubmitButton>,
            dismissiveAction: (
              <Button
                kind={KIND.tertiary}
                onClick={onDismiss}
                overrides={{
                  BaseButton: {
                    style: {
                      minWidth: '150px',
                    },
                  },
                }}
              >
                Cancel
              </Button>
            ),
          }}
        >
          {formElement}
        </Dialog>
      )}
    >
      {children}
      <FormErrorBanner />
    </Form>
  );
};
