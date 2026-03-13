import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { DeleteAlt, Plus } from 'baseui/icon';

import { StringField } from '#core/components/form/fields/string/string-field';
import { ArrayFormRow } from '#core/components/form/layout/array-form-row/array-form-row';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getFormProviderWrapper } from '#core/test/wrappers/get-form-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';

const icons = { plus: Plus, deleteAlt: DeleteAlt };

it('renders children for each initial item', () => {
  render(
    <ArrayFormRow rootFieldPath="tags" minItems={2}>
      {(name) => <StringField name={`${name}.value`} label="Tag" />}
    </ArrayFormRow>,
    buildWrapper([
      getBaseProviderWrapper(),
      getIconProviderWrapper({ icons }),
      getFormProviderWrapper({}),
    ])
  );

  expect(screen.getAllByRole('textbox', { name: 'Tag' })).toHaveLength(2);
});

it('prepopulates rows to meet minItems on mount', async () => {
  render(
    <ArrayFormRow rootFieldPath="tags" minItems={3}>
      {(name) => <StringField name={`${name}.value`} label="Tag" />}
    </ArrayFormRow>,
    buildWrapper([
      getBaseProviderWrapper(),
      getIconProviderWrapper({ icons }),
      getFormProviderWrapper({}),
    ])
  );

  await waitFor(() => expect(screen.getAllByRole('textbox', { name: 'Tag' })).toHaveLength(3));
});

it('adds a row when "Add more" is clicked', async () => {
  const user = userEvent.setup();

  render(
    <ArrayFormRow rootFieldPath="tags" minItems={1}>
      {(name) => <StringField name={`${name}.value`} label="Tag" />}
    </ArrayFormRow>,
    buildWrapper([
      getBaseProviderWrapper(),
      getIconProviderWrapper({ icons }),
      getFormProviderWrapper({}),
    ])
  );

  await waitFor(() => screen.getByRole('textbox', { name: 'Tag' }));
  await user.click(screen.getByRole('button', { name: /add more/i }));

  expect(screen.getAllByRole('textbox', { name: 'Tag' })).toHaveLength(2);
});

it('hides remove button when at minItems', async () => {
  render(
    <ArrayFormRow rootFieldPath="tags" minItems={2}>
      {(name) => <StringField name={`${name}.value`} label="Tag" />}
    </ArrayFormRow>,
    buildWrapper([
      getBaseProviderWrapper(),
      getIconProviderWrapper({ icons }),
      getFormProviderWrapper({}),
    ])
  );

  await waitFor(() => expect(screen.getAllByRole('textbox', { name: 'Tag' })).toHaveLength(2));
  expect(screen.queryByRole('button', { name: /remove/i })).not.toBeInTheDocument();
});

it('removes a row when remove is clicked', async () => {
  const user = userEvent.setup();

  render(
    <ArrayFormRow rootFieldPath="tags" minItems={1}>
      {(name) => <StringField name={`${name}.value`} label="Tag" />}
    </ArrayFormRow>,
    buildWrapper([
      getBaseProviderWrapper(),
      getIconProviderWrapper({ icons }),
      getFormProviderWrapper({}),
    ])
  );

  await waitFor(() => screen.getByRole('textbox', { name: 'Tag' }));

  // Add a second row so the remove button appears
  await user.click(screen.getByRole('button', { name: /add more/i }));
  expect(screen.getAllByRole('textbox', { name: 'Tag' })).toHaveLength(2);

  // Remove one row
  await user.click(screen.getAllByRole('button', { name: /remove/i })[0]);

  await waitFor(() => expect(screen.getAllByRole('textbox', { name: 'Tag' })).toHaveLength(1));
});

it('uses addLabel for the add button', () => {
  render(
    <ArrayFormRow rootFieldPath="tags" addLabel="Add tag">
      {(name) => <StringField name={`${name}.value`} label="Tag" />}
    </ArrayFormRow>,
    buildWrapper([
      getBaseProviderWrapper(),
      getIconProviderWrapper({ icons }),
      getFormProviderWrapper({}),
    ])
  );

  expect(screen.getByRole('button', { name: 'Add tag' })).toBeInTheDocument();
});

it('Hides add and remove buttons when readOnly is true', async () => {
  render(
    <ArrayFormRow rootFieldPath="tags" minItems={2} readOnly>
      {(name) => <StringField name={`${name}.value`} label="Tag" />}
    </ArrayFormRow>,
    buildWrapper([
      getBaseProviderWrapper(),
      getIconProviderWrapper({ icons }),
      getFormProviderWrapper({}),
    ])
  );

  await waitFor(() => expect(screen.getAllByRole('textbox', { name: 'Tag' })).toHaveLength(2));
  expect(screen.queryByRole('button', { name: /add more/i })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /remove/i })).not.toBeInTheDocument();
});
