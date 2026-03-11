import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { SelectField } from '#core/components/form/fields/select/select-field';
import { Form } from '#core/components/form/form';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getFormProviderWrapper } from '#core/test/wrappers/get-form-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';

describe('SelectField', () => {
  const options = [
    { id: 'low', label: 'Low Priority' },
    { id: 'medium', label: 'Medium Priority' },
    { id: 'high', label: 'High Priority' },
  ];

  it('renders with label and placeholder', () => {
    render(
      <SelectField
        name="priority"
        label="Priority"
        options={options}
        placeholder="Select priority level"
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByRole('combobox', { name: 'Priority' })).toBeInTheDocument();
    expect(screen.getByText('Select priority level')).toBeInTheDocument();
  });

  it('shows required indicator when required', () => {
    render(
      <SelectField name="priority" label="Priority" required options={options} />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(
      screen.getAllByText((_, element) => element?.textContent === 'Priority*').length
    ).toBeGreaterThan(0);
  });

  it('displays help tooltip when description is provided', async () => {
    const user = userEvent.setup();

    render(
      <SelectField
        name="priority"
        label="Priority"
        description="Select the priority level for this task"
        options={options}
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    await user.hover(screen.getByRole('img', { name: 'help' }));
    await screen.findByText('Select the priority level for this task');
  });

  it('submits the selected value', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    render(
      <>
        <SelectField name="priority" label="Priority" options={options} />
        <button type="submit">Submit</button>
      </>,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ onSubmit }),
      ])
    );

    const select = screen.getByRole('combobox', { name: 'Priority' });
    await user.click(select);

    expect(screen.getByRole('option', { name: 'Low Priority' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'Medium Priority' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'High Priority' })).toBeInTheDocument();

    await user.click(screen.getByRole('option', { name: 'Low Priority' }));
    await user.click(screen.getByRole('button', { name: 'Submit' }));
    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(
        { priority: 'low' },
        expect.anything(),
        expect.anything()
      )
    );
  });

  it('filters options by search input', async () => {
    const user = userEvent.setup();

    render(
      <SelectField name="priority" label="Priority" options={options} searchable />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper()])
    );

    const select = screen.getByRole('combobox', { name: 'Priority' });
    await user.type(select, 'Low');

    expect(screen.getAllByRole('option')).toHaveLength(1);
    expect(screen.getByRole('option', { name: 'Low Priority' })).toBeInTheDocument();
  });

  it('supports numeric ids', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    render(
      <>
        <SelectField
          name="priority"
          label="Priority"
          options={[
            { id: 0, label: 'Low Priority' },
            { id: 1, label: 'Medium Priority' },
            { id: 2, label: 'High Priority' },
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

    const select = screen.getByRole('combobox', { name: 'Priority' });
    await user.click(select);

    await user.click(screen.getByRole('option', { name: 'Low Priority' }));
    await user.click(screen.getByRole('button', { name: 'Submit' }));
    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith({ priority: 0 }, expect.anything(), expect.anything())
    );
  });

  it('supports multi-select mode', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    render(
      <>
        <SelectField name="priority" label="Priority" options={options} multi />
        <button type="submit">Submit</button>
      </>,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ onSubmit }),
      ])
    );

    // Select Low Priority
    let select = screen.getByRole('combobox');
    await user.click(select);
    await user.click(await screen.findByRole('option', { name: 'Low Priority' }));

    // Select Medium Priority
    select = await screen.findByRole('combobox', { name: /Selected Low Priority/ });
    await user.click(select);
    await user.click(await screen.findByRole('option', { name: 'Medium Priority' }));

    await user.click(screen.getByRole('button', { name: 'Submit' }));
    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(
        { priority: ['low', 'medium'] },
        expect.anything(),
        expect.anything()
      )
    );
  });

  it('supports creatable mode', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    render(
      <>
        <SelectField name="priority" label="Priority" options={options} creatable />
        <button type="submit">Submit</button>
      </>,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ onSubmit }),
      ])
    );

    const select = screen.getByRole('combobox', { name: 'Priority' });
    await user.type(select, 'new');
    await user.click(await screen.findByRole('option', { name: 'new' }));

    await user.click(screen.getByRole('button', { name: 'Submit' }));
    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(
        { priority: 'new' },
        expect.anything(),
        expect.anything()
      )
    );
  });

  it('supports nonexisting options in creatable mode', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    render(
      <>
        <SelectField name="priority" label="Priority" options={options} creatable />
        <button type="submit">Submit</button>
      </>,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ initialValues: { priority: 'new' }, onSubmit }),
      ])
    );

    expect(screen.getByText('new')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Submit' }));
    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(
        { priority: 'new' },
        expect.anything(),
        expect.anything()
      )
    );
  });

  it('omits clear button when readOnly or disabled', () => {
    render(
      <>
        <SelectField name="disabled" label="Disabled" options={options} disabled />
        <SelectField name="readonly" label="Read only" options={options} readOnly />
      </>,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ initialValues: { disabled: 'low', readonly: 'low' } }),
      ])
    );

    expect(screen.getAllByRole('combobox', { hidden: true })).toHaveLength(2);
    expect(screen.queryByRole('button', { name: 'Clear' })).not.toBeInTheDocument();
  });

  it('omits placeholder when readOnly or disabled', () => {
    render(
      <>
        <SelectField
          name="disabled"
          label="Disabled"
          options={options}
          disabled
          placeholder="Disabled placeholder"
        />
        <SelectField
          name="readonly"
          label="Read only"
          options={options}
          readOnly
          placeholder="Read only placeholder"
        />
      </>,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper()])
    );

    expect(screen.getByRole('combobox', { name: 'Disabled' })).toBeInTheDocument();
    expect(screen.getByRole('combobox', { name: 'Read only' })).toBeInTheDocument();
    expect(screen.queryByText('Disabled placeholder')).not.toBeInTheDocument();
    expect(screen.queryByText('Read only placeholder')).not.toBeInTheDocument();
  });

  it('focuses on failed submit when form has focusOnError enabled', async () => {
    const user = userEvent.setup();

    render(
      <Form onSubmit={vi.fn()} focusOnError>
        <SelectField name="priority" label="Priority" options={options} required />
        <button type="submit">Submit</button>
      </Form>,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
    );

    await user.click(screen.getByRole('button', { name: 'Submit' }));

    await waitFor(() => {
      expect(document.activeElement).toBe(screen.getByRole('combobox', { name: 'Priority *' }));
    });
  });

  it('displays caption text', () => {
    render(
      <SelectField
        name="priority"
        label="Priority"
        options={options}
        caption="Choose the appropriate priority level"
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByText('Choose the appropriate priority level')).toBeInTheDocument();
  });

  it('submits object id as the field value', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    const objectOptions = [
      { id: { tier: 'free', limit: 100 }, label: 'Free Tier' },
      { id: { tier: 'pro', limit: 10000 }, label: 'Pro Tier' },
    ];

    render(
      <>
        <SelectField name="plan" label="Plan" options={objectOptions} />
        <button type="submit">Submit</button>
      </>,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ onSubmit }),
      ])
    );

    const select = screen.getByRole('combobox', { name: 'Plan' });
    await user.click(select);
    await user.click(screen.getByRole('option', { name: 'Pro Tier' }));

    await user.click(screen.getByRole('button', { name: 'Submit' }));
    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(
        { plan: { tier: 'pro', limit: 10000 } },
        expect.anything(),
        expect.anything()
      )
    );
  });

  it('clears initial value that does not match any option', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    render(
      <>
        <SelectField name="priority" label="Priority" options={options} />
        <button type="submit">Submit</button>
      </>,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ initialValues: { priority: 'nonexistent' }, onSubmit }),
      ])
    );

    expect(screen.queryByText('nonexistent')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Submit' }));
    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith({}, expect.anything(), expect.anything())
    );
  });

  it('keeps only valid values in multi-select when some are invalid', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    render(
      <>
        <SelectField name="priority" label="Priority" options={options} multi />
        <button type="submit">Submit</button>
      </>,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({
          initialValues: { priority: ['low', 'nonexistent', 'high'] },
          onSubmit,
        }),
      ])
    );

    await waitFor(() => {
      expect(screen.getByText('Low Priority')).toBeInTheDocument();
      expect(screen.getByText('High Priority')).toBeInTheDocument();
    });
    expect(screen.queryByText('nonexistent')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Submit' }));
    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(
        { priority: ['low', 'high'] },
        expect.anything(),
        expect.anything()
      )
    );
  });

  it('limits the number of visible options with visibleOptionLimit', async () => {
    const user = userEvent.setup();
    const manyOptions = Array.from({ length: 10 }, (_, i) => ({
      id: `opt-${i}`,
      label: `Option ${i}`,
    }));

    render(
      <SelectField name="choice" label="Choice" options={manyOptions} visibleOptionLimit={3} />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper()])
    );

    const select = screen.getByRole('combobox', { name: 'Choice' });
    await user.click(select);

    expect(screen.getAllByRole('option')).toHaveLength(3);
  });

  it('does not clear value while options are loading', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    render(
      <>
        <SelectField name="priority" label="Priority" options={[]} isLoading />
        <button type="submit">Submit</button>
      </>,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ initialValues: { priority: 'low' }, onSubmit }),
      ])
    );

    await user.click(screen.getByRole('button', { name: 'Submit' }));
    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(
        { priority: 'low' },
        expect.anything(),
        expect.anything()
      )
    );
  });

  it('matches object id regardless of key order', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    const objectOptions = [
      { id: { tier: 'free', limit: 100 }, label: 'Free Tier' },
      { id: { tier: 'pro', limit: 10000 }, label: 'Pro Tier' },
    ];

    render(
      <>
        <SelectField
          name="plan"
          label="Plan"
          options={objectOptions}
          initialValue={{ limit: 10000, tier: 'pro' }}
        />
        <button type="submit">Submit</button>
      </>,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ onSubmit }),
      ])
    );

    expect(screen.getByText('Pro Tier')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Submit' }));
    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(
        { plan: { tier: 'pro', limit: 10000 } },
        expect.anything(),
        expect.anything()
      )
    );
  });

  it('matches nested object id regardless of key order', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    const objectOptions = [
      { id: { config: { region: 'us', zone: 'east' }, name: 'prod' }, label: 'Production' },
      { id: { config: { region: 'eu', zone: 'west' }, name: 'staging' }, label: 'Staging' },
    ];

    render(
      <>
        <SelectField
          name="env"
          label="Environment"
          options={objectOptions}
          initialValue={{ name: 'staging', config: { zone: 'west', region: 'eu' } }}
        />
        <button type="submit">Submit</button>
      </>,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ onSubmit }),
      ])
    );

    expect(screen.getByText('Staging')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Submit' }));
    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(
        { env: { config: { region: 'eu', zone: 'west' }, name: 'staging' } },
        expect.anything(),
        expect.anything()
      )
    );
  });

  it('does not clear value when creatable even if it has no matching option', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    render(
      <>
        <SelectField name="priority" label="Priority" options={options} creatable />
        <button type="submit">Submit</button>
      </>,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ initialValues: { priority: 'custom' }, onSubmit }),
      ])
    );

    expect(screen.getByText('custom')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Submit' }));
    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(
        { priority: 'custom' },
        expect.anything(),
        expect.anything()
      )
    );
  });
});
