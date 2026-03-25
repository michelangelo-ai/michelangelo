import { render, screen } from '@testing-library/react';

import { DateField } from '#core/components/form/fields/date/date-field';
import { DateFormat } from '#core/components/form/fields/date/types';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getFormProviderWrapper } from '#core/test/wrappers/get-form-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';

describe('DateField', () => {
  it('renders with label', () => {
    render(
      <DateField name="date" label="Start Date" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByText('Start Date')).toBeInTheDocument();
  });

  it('shows required indicator when required', () => {
    render(
      <DateField name="date" label="Start Date" required />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(
      screen.getAllByText((_, element) => element?.textContent === 'Start Date*').length
    ).toBeGreaterThan(0);
  });

  it('displays caption text', () => {
    render(
      <DateField name="date" label="Start Date" caption="Select a start date" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByText('Select a start date')).toBeInTheDocument();
  });

  it('displays help tooltip when description is provided', () => {
    render(
      <DateField name="date" label="Start Date" description="When the pipeline should start" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByRole('img', { name: 'help' })).toBeInTheDocument();
  });

  it('renders with placeholder', () => {
    render(
      <DateField name="date" label="Start Date" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByPlaceholderText('MM/dd/yyyy')).toBeInTheDocument();
  });

  it('hides placeholder when disabled', () => {
    render(
      <DateField name="date" label="Start Date" disabled />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.queryByPlaceholderText('MM/dd/yyyy')).not.toBeInTheDocument();
  });

  it('hides placeholder when readOnly', () => {
    render(
      <DateField name="date" label="Start Date" readOnly />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.queryByPlaceholderText('MM/dd/yyyy')).not.toBeInTheDocument();
  });

  it('displays initial value with ISO format', () => {
    render(
      <DateField name="date" label="Start Date" dateFormat={DateFormat.ISO_DATE_STRING} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ initialValues: { date: '2024-01-15T00:00:00.000Z' } }),
      ])
    );

    expect(screen.getByDisplayValue('01/15/2024')).toBeInTheDocument();
  });

  it('displays initial value with epoch format', () => {
    // 1705276800 = 2024-01-15T00:00:00.000Z
    render(
      <DateField name="date" label="Start Date" dateFormat={DateFormat.EPOCH_SECONDS} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ initialValues: { date: '1705276800' } }),
      ])
    );

    expect(screen.getByDisplayValue('01/15/2024')).toBeInTheDocument();
  });

  it('applies readOnly overrides when readOnly is true', () => {
    render(
      <DateField name="date" label="Start Date" readOnly />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByRole('textbox')).toHaveAttribute('readonly');
  });
});
