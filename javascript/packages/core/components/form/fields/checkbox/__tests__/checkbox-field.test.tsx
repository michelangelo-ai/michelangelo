import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { CheckboxField } from '#core/components/form/fields/checkbox/checkbox-field';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getFormProviderWrapper } from '#core/test/wrappers/get-form-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';

describe('CheckboxField', () => {
  it('renders all options', () => {
    render(
      <CheckboxField
        name="items"
        label="Select Items"
        options={[
          { id: 'a', label: 'Option A' },
          { id: 'b', label: 'Option B' },
          { id: 'c', label: 'Option C' },
        ]}
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByText('Option A')).toBeInTheDocument();
    expect(screen.getByText('Option B')).toBeInTheDocument();
    expect(screen.getByText('Option C')).toBeInTheDocument();
  });

  it('shows "No options available" when options is empty', () => {
    render(
      <CheckboxField name="items" label="Select Items" options={[]} />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByText('No options available')).toBeInTheDocument();
  });

  it('toggles options on click and submits selected values', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    render(
      <>
        <CheckboxField
          name="items"
          label="Select Items"
          options={[
            { id: 'a', label: 'Option A' },
            { id: 'b', label: 'Option B' },
            { id: 'c', label: 'Option C' },
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

    await user.click(screen.getByRole('checkbox', { name: 'Option A' }));
    await user.click(screen.getByRole('checkbox', { name: 'Option C' }));
    await user.click(screen.getByRole('button', { name: 'Submit' }));

    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(
        { items: ['a', 'c'] },
        expect.anything(),
        expect.anything()
      )
    );
  });

  it('unchecks an already-selected option', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    render(
      <>
        <CheckboxField
          name="items"
          label="Select Items"
          options={[
            { id: 'a', label: 'Option A' },
            { id: 'b', label: 'Option B' },
            { id: 'c', label: 'Option C' },
          ]}
        />
        <button type="submit">Submit</button>
      </>,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ initialValues: { items: ['a', 'b'] }, onSubmit }),
      ])
    );

    await user.click(screen.getByRole('checkbox', { name: 'Option A' }));
    await user.click(screen.getByRole('button', { name: 'Submit' }));

    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith({ items: ['b'] }, expect.anything(), expect.anything())
    );
  });

  it('renders option descriptions when provided', () => {
    render(
      <CheckboxField
        name="items"
        label="Select Items"
        options={[
          { id: 'a', label: 'Option A', description: 'Desc for A' },
          { id: 'b', label: 'Option B' },
        ]}
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByText('Desc for A')).toBeInTheDocument();
  });

  it('renders label', () => {
    render(
      <CheckboxField
        name="items"
        label="Select Items"
        options={[{ id: 'a', label: 'Option A' }]}
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByText('Select Items')).toBeInTheDocument();
  });

  it('is disabled when disabled prop is true', () => {
    render(
      <CheckboxField
        name="items"
        label="Select Items"
        options={[
          { id: 'a', label: 'Option A' },
          { id: 'b', label: 'Option B' },
        ]}
        disabled
      />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    const checkboxes = screen.getAllByRole('checkbox');
    checkboxes.forEach((cb) => expect(cb).toBeDisabled());
  });
});
