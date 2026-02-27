import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { TextareaField } from '#core/components/form/fields/textarea/textarea-field';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getFormProviderWrapper } from '#core/test/wrappers/get-form-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';

describe('TextareaField', () => {
  it('renders with label', () => {
    render(
      <TextareaField name="notes" label="Notes" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByRole('textbox', { name: 'Notes' })).toBeInTheDocument();
  });

  it('shows required indicator when required', () => {
    render(
      <TextareaField name="notes" label="Notes" required />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByRole('textbox', { name: 'Notes *' })).toBeInTheDocument();
  });

  it('handles user input', async () => {
    const user = userEvent.setup();

    render(
      <TextareaField name="notes" label="Notes" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    const textarea = screen.getByRole('textbox', { name: 'Notes' });
    await user.type(textarea, 'Some notes here');

    expect(textarea).toHaveValue('Some notes here');
  });

  it('displays character count when maxLength is provided', () => {
    render(
      <TextareaField name="notes" label="Notes" maxLength={100} />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByText('0 / 100')).toBeInTheDocument();
  });

  it('updates character count as user types', async () => {
    const user = userEvent.setup();

    render(
      <TextareaField name="notes" label="Notes" maxLength={100} />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    await user.type(screen.getByRole('textbox', { name: 'Notes' }), 'Hello');

    expect(screen.getByText('5 / 100')).toBeInTheDocument();
  });

  it('does not display character count when maxLength is not provided', () => {
    render(
      <TextareaField name="notes" label="Notes" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.queryByText(/\d+ \/ \d+/)).not.toBeInTheDocument();
  });
});
