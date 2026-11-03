import { Button } from 'baseui/button';
import { KIND } from 'baseui/button';
// eslint-disable-next-line baseui/no-deep-imports
import { mergeOverrides } from 'baseui/helpers/overrides';
import { StatefulTooltip } from 'baseui/tooltip';

import { useFormState } from '#core/components/form/hooks/use-form-state';

import type { SubmitButtonProps } from './types';

/**
 * Submit button that integrates with React Final Form state.
 *
 * Must be rendered within a Form component's React Final Form context to access form state.
 * Use the `formId` prop when the button is rendered outside the form element (e.g., in modal footers).
 *
 * @example
 * ```tsx
 * // Inside form element
 * <Form onSubmit={handleSubmit}>
 *   <StringField name="email" />
 *   <SubmitButton>Submit</SubmitButton>
 * </Form>
 *
 * // Outside form element (with render prop)
 * <Form
 *   id="my-form"
 *   onSubmit={handleSubmit}
 *   render={(formElement) => (
 *     <Dialog buttonDock={{ primaryAction: <SubmitButton formId="my-form">Submit</SubmitButton> }}>
 *       {formElement}
 *     </Dialog>
 *   )}
 * >
 *   <StringField name="email" />
 * </Form>
 * ```
 */
export const SubmitButton: React.FC<SubmitButtonProps> = ({
  children,
  formId,
  kind = KIND.primary,
  disabled,
  disabledReason,
  overrides,
  ...rest
}) => {
  const { submitting } = useFormState({ submitting: true });

  const button = (
    <Button
      type="submit"
      kind={kind}
      form={formId}
      disabled={!!disabledReason || disabled}
      isLoading={submitting}
      overrides={mergeOverrides(
        {
          BaseButton: {
            style: {
              minWidth: '200px',
            },
          },
        },
        overrides
      )}
      {...rest}
    >
      {children}
    </Button>
  );

  if (!disabledReason) return button;

  // A disabled button swallows pointer events, so the tooltip hangs off a wrapper.
  return (
    <StatefulTooltip content={disabledReason}>
      <span>{button}</span>
    </StatefulTooltip>
  );
};
