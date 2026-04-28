import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { DeleteAlt, Plus } from 'baseui/icon';

import { StringField } from '#core/components/form/fields/string/string-field';
import { ArrayFormGroup } from '#core/components/form/layout/array-form-group/array-form-group';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getFormProviderWrapper } from '#core/test/wrappers/get-form-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';

it('renders numbered group titles when groupLabel is provided', async () => {
  render(
    <ArrayFormGroup rootFieldPath="addresses" groupLabel="Address" minItems={2}>
      {(name) => <StringField name={`${name}.street`} label="Street" />}
    </ArrayFormGroup>,
    buildWrapper([
      getBaseProviderWrapper(),
      getIconProviderWrapper({ icons: { plus: Plus, deleteAlt: DeleteAlt } }),
      getFormProviderWrapper({}),
    ])
  );

  await waitFor(() => expect(screen.getAllByRole('textbox', { name: 'Street' })).toHaveLength(2));
  expect(screen.getByText('Address 1')).toBeInTheDocument();
  expect(screen.getByText('Address 2')).toBeInTheDocument();
});

it('renders no title when groupLabel is omitted', async () => {
  render(
    <ArrayFormGroup rootFieldPath="addresses" minItems={1}>
      {(name) => <StringField name={`${name}.street`} label="Street" />}
    </ArrayFormGroup>,
    buildWrapper([
      getBaseProviderWrapper(),
      getIconProviderWrapper({ icons: { plus: Plus, deleteAlt: DeleteAlt } }),
      getFormProviderWrapper({}),
    ])
  );

  await waitFor(() => screen.getByRole('textbox', { name: 'Street' }));
  expect(screen.queryByText(/\d+/)).not.toBeInTheDocument();
});

it('uses groupLabel in add button label', () => {
  render(
    <ArrayFormGroup rootFieldPath="addresses" groupLabel="Address">
      {(name) => <StringField name={`${name}.street`} label="Street" />}
    </ArrayFormGroup>,
    buildWrapper([
      getBaseProviderWrapper(),
      getIconProviderWrapper({ icons: { plus: Plus, deleteAlt: DeleteAlt } }),
      getFormProviderWrapper({}),
    ])
  );

  expect(screen.getByRole('button', { name: /add address/i })).toBeInTheDocument();
});

it('falls back to "Add more" when no groupLabel or addLabel is provided', () => {
  render(
    <ArrayFormGroup rootFieldPath="addresses">
      {(name) => <StringField name={`${name}.street`} label="Street" />}
    </ArrayFormGroup>,
    buildWrapper([
      getBaseProviderWrapper(),
      getIconProviderWrapper({ icons: { plus: Plus, deleteAlt: DeleteAlt } }),
      getFormProviderWrapper({}),
    ])
  );

  expect(screen.getByRole('button', { name: /add more/i })).toBeInTheDocument();
});

it('uses addLabel over the derived groupLabel label', () => {
  render(
    <ArrayFormGroup rootFieldPath="models" groupLabel="ML Model" addLabel="Add ML model">
      {(name) => <StringField name={`${name}.name`} label="Name" />}
    </ArrayFormGroup>,
    buildWrapper([
      getBaseProviderWrapper(),
      getIconProviderWrapper({ icons: { plus: Plus, deleteAlt: DeleteAlt } }),
      getFormProviderWrapper({}),
    ])
  );

  expect(screen.getByRole('button', { name: /Add ML model/ })).toBeInTheDocument();
});

it('prepopulates groups to meet minItems on mount', async () => {
  render(
    <ArrayFormGroup rootFieldPath="addresses" groupLabel="Address" minItems={3}>
      {(name) => <StringField name={`${name}.street`} label="Street" />}
    </ArrayFormGroup>,
    buildWrapper([
      getBaseProviderWrapper(),
      getIconProviderWrapper({ icons: { plus: Plus, deleteAlt: DeleteAlt } }),
      getFormProviderWrapper({}),
    ])
  );

  await waitFor(() => expect(screen.getAllByRole('textbox', { name: 'Street' })).toHaveLength(3));
});

it('adds a group when the add button is clicked', async () => {
  const user = userEvent.setup();

  render(
    <ArrayFormGroup rootFieldPath="addresses" groupLabel="Address" minItems={1}>
      {(name) => <StringField name={`${name}.street`} label="Street" />}
    </ArrayFormGroup>,
    buildWrapper([
      getBaseProviderWrapper(),
      getIconProviderWrapper({ icons: { plus: Plus, deleteAlt: DeleteAlt } }),
      getFormProviderWrapper({}),
    ])
  );

  await waitFor(() => screen.getByRole('textbox', { name: 'Street' }));
  await user.click(screen.getByRole('button', { name: /add address/i }));

  expect(screen.getAllByRole('textbox', { name: 'Street' })).toHaveLength(2);
  expect(screen.getByText('Address 2')).toBeInTheDocument();
});

it('hides remove when at minItems', async () => {
  render(
    <ArrayFormGroup rootFieldPath="addresses" groupLabel="Address" minItems={2}>
      {(name) => <StringField name={`${name}.street`} label="Street" />}
    </ArrayFormGroup>,
    buildWrapper([
      getBaseProviderWrapper(),
      getIconProviderWrapper({ icons: { plus: Plus, deleteAlt: DeleteAlt } }),
      getFormProviderWrapper({}),
    ])
  );

  await waitFor(() => expect(screen.getAllByRole('textbox', { name: 'Street' })).toHaveLength(2));
  expect(screen.queryByRole('button', { name: /remove/i })).not.toBeInTheDocument();
});

it('hides add and remove buttons when readOnly', async () => {
  render(
    <ArrayFormGroup rootFieldPath="addresses" groupLabel="Address" minItems={2} readOnly>
      {(name) => <StringField name={`${name}.street`} label="Street" />}
    </ArrayFormGroup>,
    buildWrapper([
      getBaseProviderWrapper(),
      getIconProviderWrapper({ icons: { plus: Plus, deleteAlt: DeleteAlt } }),
      getFormProviderWrapper({}),
    ])
  );

  await waitFor(() => expect(screen.getAllByRole('textbox', { name: 'Street' })).toHaveLength(2));
  expect(screen.queryByRole('button', { name: /add address/i })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /remove/i })).not.toBeInTheDocument();
});

it('typing in the remaining group after removing the first does not corrupt values', async () => {
  const user = userEvent.setup();

  render(
    <ArrayFormGroup rootFieldPath="contacts" groupLabel="Contact" minItems={1}>
      {(name) => <StringField name={`${name}.name`} label="Name" />}
    </ArrayFormGroup>,
    buildWrapper([
      getBaseProviderWrapper(),
      getIconProviderWrapper({ icons: { plus: Plus, deleteAlt: DeleteAlt } }),
      getFormProviderWrapper({}),
    ])
  );

  await waitFor(() => screen.getByRole('textbox', { name: 'Name' }));

  // Type in group 1, add group 2, type in group 2
  await user.type(screen.getAllByRole('textbox', { name: 'Name' })[0], 'Alice');
  await user.click(screen.getByRole('button', { name: /add contact/i }));
  await user.type(screen.getAllByRole('textbox', { name: 'Name' })[1], 'Bob');

  // Remove group 1
  const removeButtons = screen.getAllByRole('button', { name: /remove/i });
  await user.click(removeButtons[0]);

  // Only 1 group remains, showing Bob's value (which shifted to index 0)
  await waitFor(() => expect(screen.getAllByRole('textbox', { name: 'Name' })).toHaveLength(1));
  expect(screen.getByRole('textbox', { name: 'Name' })).toHaveValue('Bob');
});
