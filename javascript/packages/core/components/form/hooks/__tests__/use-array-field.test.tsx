import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { useArrayField } from '#core/components/form/hooks/use-array-field';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getFormProviderWrapper } from '#core/test/wrappers/get-form-provider-wrapper';

it('remove does not drop below minItems even when called directly without an isRemovable check', async () => {
  function CustomArrayField() {
    const { entries, add, remove } = useArrayField('items', { minItems: 2 });
    return (
      <div>
        {entries.map(({ id }, index) => (
          <div key={id}>
            <span>Item {index + 1}</span>
            <button type="button" aria-label={`Remove item ${index + 1}`} onClick={() => remove(index)}>
              Remove
            </button>
          </div>
        ))}
        <button type="button" onClick={add}>Add</button>
      </div>
    );
  }

  const user = userEvent.setup();
  render(<CustomArrayField />, buildWrapper([getBaseProviderWrapper(), getFormProviderWrapper()]));

  await waitFor(() => expect(screen.getAllByText(/Item \d+/)).toHaveLength(2));

  await user.click(screen.getByRole('button', { name: 'Add' }));
  expect(screen.getAllByText(/Item \d+/)).toHaveLength(3);

  await user.click(screen.getByRole('button', { name: 'Remove item 3' }));
  await waitFor(() => expect(screen.getAllByText(/Item \d+/)).toHaveLength(2));

  // Hook guard should block this — count stays at 2
  await user.click(screen.getByRole('button', { name: 'Remove item 2' }));
  expect(screen.getAllByText(/Item \d+/)).toHaveLength(2);
});
