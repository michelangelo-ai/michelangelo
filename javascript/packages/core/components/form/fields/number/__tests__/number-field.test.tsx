import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { NumberField } from '#core/components/form/fields/number/number-field';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getFormProviderWrapper } from '#core/test/wrappers/get-form-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';

const wrapper = buildWrapper([
  getBaseProviderWrapper(),
  getIconProviderWrapper(),
  getFormProviderWrapper({}),
]);

describe('NumberField', () => {
  it('renders with label', () => {
    render(<NumberField name="count" label="Count" />, wrapper);

    expect(screen.getByText('Count')).toBeInTheDocument();
    expect(screen.getByRole('spinbutton')).toBeInTheDocument();
  });

  it('submits numeric value', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    render(
      <>
        <NumberField name="count" label="Count" />
        <button type="submit">Submit</button>
      </>,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ onSubmit }),
      ])
    );

    await user.type(screen.getByRole('spinbutton'), '42');
    await user.click(screen.getByRole('button', { name: 'Submit' }));

    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(
        { count: 42 },
        expect.anything(),
        expect.anything()
      )
    );
  });

  it('shows initial value', async () => {
    render(
      <NumberField name="count" label="Count" />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ initialValues: { count: 7 } }),
      ])
    );

    expect(await screen.findByDisplayValue('7')).toBeInTheDocument();
  });

  it('is disabled when disabled prop is true', () => {
    render(<NumberField name="count" label="Count" disabled />, wrapper);

    expect(screen.getByRole('spinbutton')).toBeDisabled();
  });

  it('submits undefined when input is cleared', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    render(
      <>
        <NumberField name="count" label="Count" />
        <button type="submit">Submit</button>
      </>,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ onSubmit }),
      ])
    );

    // type then clear — browser type=number returns '' for empty/invalid
    await user.type(screen.getByRole('spinbutton'), '5');
    await user.clear(screen.getByRole('spinbutton'));
    await user.click(screen.getByRole('button', { name: 'Submit' }));

    await waitFor(() =>
      expect(onSubmit).toHaveBeenCalledWith(
        { count: undefined },
        expect.anything(),
        expect.anything()
      )
    );
  });

  it('displays caption text', () => {
    render(<NumberField name="count" label="Count" caption="Enter a number" />, wrapper);

    expect(screen.getByText('Enter a number')).toBeInTheDocument();
  });
});
