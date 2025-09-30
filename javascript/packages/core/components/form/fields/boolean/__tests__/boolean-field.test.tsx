import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { BooleanField } from '#core/components/form/fields/boolean/boolean-field';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getFormProviderWrapper } from '#core/test/wrappers/get-form-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';

describe('BooleanField', () => {
  it('renders with label and checkbox', () => {
    render(
      <BooleanField name="enabled" label="Enable Feature" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByText('Enable Feature')).toBeInTheDocument();
    expect(screen.getByRole('checkbox', { name: 'Disabled' })).not.toBeChecked();
  });

  it('shows required indicator when required', () => {
    render(
      <BooleanField name="enabled" label="Enable Feature" required />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(
      screen.getAllByText((_, element) => element?.textContent === 'Enable Feature*').length
    ).toBeGreaterThan(0);
  });

  it('handles user interaction', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    render(
      <>
        <BooleanField name="enabled" label="Enable Feature" />
        <button type="submit">Submit</button>
      </>,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ onSubmit }),
      ])
    );

    const checkbox = screen.getByRole('checkbox', { name: 'Disabled' });
    expect(checkbox).not.toBeChecked();

    await user.click(checkbox);
    expect(checkbox).toBeChecked();
    await user.click(screen.getByRole('button', { name: 'Submit' }));
    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith({ enabled: true }, expect.anything(), expect.anything())
    );
  });

  it('displays help tooltip when description is provided', async () => {
    const user = userEvent.setup();

    render(
      <BooleanField
        name="enabled"
        label="Enable Feature"
        description="Toggle this feature on or off"
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    await user.hover(screen.getByRole('img', { name: 'help' }));
    await screen.findByText('Toggle this feature on or off');
  });

  it('uses custom checkbox label when provided', () => {
    render(
      <BooleanField
        name="notifications"
        label="Notifications"
        checkboxLabel="Send email notifications"
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByRole('checkbox', { name: 'Send email notifications' })).toBeInTheDocument();
  });

  it('shows initial value when provided', async () => {
    render(
      <BooleanField name="enabled" label="Enable Feature" toggle />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ initialValues: { enabled: true } }),
      ])
    );

    const checkbox = await screen.findByRole('checkbox', { name: 'Enabled' });
    expect(checkbox).toBeChecked();
  });

  it('is disabled when disabled prop is true', async () => {
    render(
      <BooleanField name="enabled" label="Enable Feature" disabled />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    const checkbox = await screen.findByRole('checkbox', { name: 'Disabled' });
    expect(checkbox).toBeDisabled();
  });

  it('is read-only when readOnly prop is true', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    render(
      <>
        <BooleanField name="enabled" label="Enable Feature" readOnly />
        <button type="submit">Submit</button>
      </>,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ initialValues: { enabled: false }, onSubmit }),
      ])
    );

    const checkbox = await screen.findByRole('checkbox', { name: 'Disabled' });
    await user.click(checkbox);
    expect(checkbox).not.toBeChecked();
    await user.click(screen.getByRole('button', { name: 'Submit' }));
    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(
        { enabled: false },
        expect.anything(),
        expect.anything()
      )
    );
  });

  it('displays caption text', () => {
    render(
      <BooleanField
        name="enabled"
        label="Enable Feature"
        caption="This will enable advanced features"
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByText('This will enable advanced features')).toBeInTheDocument();
  });
});
