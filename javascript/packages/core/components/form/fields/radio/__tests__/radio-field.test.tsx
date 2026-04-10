import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { RadioField } from '#core/components/form/fields/radio/radio-field';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getFormProviderWrapper } from '#core/test/wrappers/get-form-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';

describe('RadioField', () => {
  it('renders with label and options', () => {
    render(
      <RadioField name="environment" label="Environment" options={[
          { value: 'dev', label: 'Development' },
          { value: 'staging', label: 'Staging' },
          { value: 'prod', label: 'Production' },
        ]} />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByText('Environment')).toBeInTheDocument();
    expect(screen.getByRole('radio', { name: 'Development' })).toBeInTheDocument();
    expect(screen.getByRole('radio', { name: 'Staging' })).toBeInTheDocument();
    expect(screen.getByRole('radio', { name: 'Production' })).toBeInTheDocument();
  });

  it('shows required indicator when required', () => {
    render(
      <RadioField name="environment" label="Environment" required options={[
          { value: 'dev', label: 'Development' },
          { value: 'staging', label: 'Staging' },
          { value: 'prod', label: 'Production' },
        ]} />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(
      screen.getAllByText((_content, node) => node?.textContent === 'Environment*').length
    ).toBeGreaterThan(0);
  });

  it('handles user selection', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    render(
      <>
        <RadioField name="environment" label="Environment" options={[
          { value: 'dev', label: 'Development' },
          { value: 'staging', label: 'Staging' },
          { value: 'prod', label: 'Production' },
        ]} />
        <button type="submit">Submit</button>
      </>,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ onSubmit }),
      ])
    );

    const stagingRadio = screen.getByRole('radio', { name: 'Staging' });
    await user.click(stagingRadio);

    expect(stagingRadio).toBeChecked();
    await user.click(screen.getByRole('button', { name: 'Submit' }));
    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(
        { environment: 'staging' },
        expect.anything(),
        expect.anything()
      )
    );
  });

  it('displays help tooltip when description is provided', async () => {
    const user = userEvent.setup();

    render(
      <RadioField
        name="environment"
        label="Environment"
        description="Select the target environment"
        options={[
          { value: 'dev', label: 'Development' },
          { value: 'staging', label: 'Staging' },
          { value: 'prod', label: 'Production' },
        ]}
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    await user.hover(screen.getByRole('img', { name: 'help' }));
    await screen.findByText('Select the target environment');
  });

  it('handles boolean options correctly', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    render(
      <>
        <RadioField
          name="autoFlip"
          label="Auto Flip"
          options={[
            { value: true, label: 'Enabled' },
            { value: false, label: 'Disabled' },
          ]}
        />
        <button type="submit">Submit</button>
      </>,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ onSubmit }),
      ])
    );

    const enabledRadio = screen.getByRole('radio', { name: 'Enabled' });
    await user.click(enabledRadio);
    await user.click(screen.getByRole('button', { name: 'Submit' }));
    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(
        { autoFlip: true },
        expect.anything(),
        expect.anything()
      )
    );
  });

  it('displays as read-only input when readOnly is true', () => {
    render(
      <RadioField name="environment" label="Environment" options={[
          { value: 'dev', label: 'Development' },
          { value: 'staging', label: 'Staging' },
          { value: 'prod', label: 'Production' },
        ]} readOnly />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ initialValues: { environment: 'staging' } }),
      ])
    );

    const textbox = screen.getByRole('textbox', { name: 'Environment' });
    expect(textbox).toHaveValue('Staging');
    expect(textbox).toHaveAttribute('readOnly');
  });

  it('disables all options when disabled prop is set', () => {
    const options = [
      { value: 'dev', label: 'Development' },
      { value: 'staging', label: 'Staging' },
      { value: 'prod', label: 'Production' },
    ];

    render(
      <RadioField name="environment" label="Environment" options={[
          { value: 'dev', label: 'Development' },
          { value: 'staging', label: 'Staging' },
          { value: 'prod', label: 'Production' },
        ]} disabled />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    options.forEach((option) => {
      expect(screen.getByRole('radio', { name: option.label })).toBeDisabled();
    });
  });

  it('disables individual options when option.disabled is true', () => {
    const optionsWithDisabled = [
      { value: 'dev', label: 'Development' },
      { value: 'staging', label: 'Staging', disabled: true },
      { value: 'prod', label: 'Production' },
    ];

    render(
      <RadioField name="environment" label="Environment" options={optionsWithDisabled} />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByRole('radio', { name: 'Development' })).not.toBeDisabled();
    expect(screen.getByRole('radio', { name: 'Staging' })).toBeDisabled();
    expect(screen.getByRole('radio', { name: 'Production' })).not.toBeDisabled();
  });

  it('displays caption text', () => {
    render(
      <RadioField
        name="environment"
        label="Environment"
        options={[
          { value: 'dev', label: 'Development' },
          { value: 'staging', label: 'Staging' },
          { value: 'prod', label: 'Production' },
        ]}
        caption="Choose your deployment target"
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByText('Choose your deployment target')).toBeInTheDocument();
  });

  it('pre-selects the option matching initial value', () => {
    render(
      <RadioField name="environment" label="Environment" options={[
          { value: 'dev', label: 'Development' },
          { value: 'staging', label: 'Staging' },
          { value: 'prod', label: 'Production' },
        ]} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ initialValues: { environment: 'staging' } }),
      ])
    );

    expect(screen.getByRole('radio', { name: 'Staging' })).toBeChecked();
    expect(screen.getByRole('radio', { name: 'Development' })).not.toBeChecked();
    expect(screen.getByRole('radio', { name: 'Production' })).not.toBeChecked();
  });
});

describe('RadioField with card layout', () => {
  it('renders card tiles with descriptions when options have descriptions', () => {
    const optionsWithDescriptions = [
      { value: 'dev', label: 'Development', description: 'For local testing' },
      { value: 'staging', label: 'Staging', description: 'Pre-production environment' },
      { value: 'prod', label: 'Production', description: 'Live environment' },
    ];

    render(
      <RadioField name="environment" label="Environment" options={optionsWithDescriptions} />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByText('Environment')).toBeInTheDocument();
    expect(screen.getByText('Development')).toBeInTheDocument();
    expect(screen.getByText('For local testing')).toBeInTheDocument();
    expect(screen.getByText('Staging')).toBeInTheDocument();
    expect(screen.getByText('Pre-production environment')).toBeInTheDocument();
    expect(screen.getByText('Production')).toBeInTheDocument();
    expect(screen.getByText('Live environment')).toBeInTheDocument();
  });

  it('handles tile selection', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    render(
      <>
        <RadioField
          name="environment"
          label="Environment"
          options={[
            { value: 'dev', label: 'Development', description: 'For local testing' },
            { value: 'staging', label: 'Staging', description: 'Pre-production environment' },
            { value: 'prod', label: 'Production', description: 'Live environment' },
          ]}
        />
        <button type="submit">Submit</button>
      </>,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ onSubmit }),
      ])
    );

    await user.click(screen.getByText('Staging'));
    await user.click(screen.getByRole('button', { name: 'Submit' }));
    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(
        { environment: 'staging' },
        expect.anything(),
        expect.anything()
      )
    );
  });

  it('disables all tiles when disabled prop is set', () => {
    render(
      <RadioField
        name="environment"
        label="Environment"
        options={[
          { value: 'dev', label: 'Development', description: 'For local testing' },
          { value: 'staging', label: 'Staging', description: 'Pre-production environment' },
          { value: 'prod', label: 'Production', description: 'Live environment' },
        ]}
        disabled
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    const tiles = screen.getAllByRole('radio');
    tiles.forEach((tile) => {
      expect(tile).toBeDisabled();
    });
  });
});
