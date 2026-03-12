import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { CheckboxField } from '#core/components/form/fields/checkbox/checkbox-field';
import { required } from '#core/components/form/validation/validators';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getFormProviderWrapper } from '#core/test/wrappers/get-form-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';

describe('CheckboxField', () => {
  it('renders all options', () => {
    render(
      <CheckboxField
        name="choices"
        label="Choices"
        options={[
          { value: 'a', label: 'Option A' },
          { value: 'b', label: 'Option B' },
          { value: 'c', label: 'Option C', description: 'Extra info' },
        ]}
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByRole('checkbox', { name: 'Option A' })).toBeInTheDocument();
    expect(screen.getByRole('checkbox', { name: 'Option B' })).toBeInTheDocument();
    expect(screen.getByRole('checkbox', { name: /Option C/ })).toBeInTheDocument();
  });

  it('toggles options on click', async () => {
    const user = userEvent.setup();

    render(
      <CheckboxField
        name="choices"
        label="Choices"
        options={[
          { value: 'a', label: 'Option A' },
          { value: 'b', label: 'Option B' },
        ]}
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    const checkboxA = screen.getByRole('checkbox', { name: 'Option A' });
    expect(checkboxA).not.toBeChecked();

    await user.click(checkboxA);
    expect(checkboxA).toBeChecked();

    await user.click(checkboxA);
    expect(checkboxA).not.toBeChecked();
  });

  it('shows error when touched and invalid', async () => {
    const user = userEvent.setup();

    render(
      <CheckboxField
        name="choices"
        label="Choices"
        options={[{ value: 'a', label: 'Option A' }]}
        validate={required('Pick at least one.')}
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    const checkboxA = screen.getByRole('checkbox', { name: 'Option A' });
    await user.click(checkboxA);
    await user.click(checkboxA);
    // Tab away to trigger blur/touched state
    await user.tab();

    expect(await screen.findByText('Pick at least one.')).toBeInTheDocument();
  });
});
