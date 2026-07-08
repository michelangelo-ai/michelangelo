import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { Form } from '#core/components/form/form';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { MultiInputField } from '../multi-input-field';

describe('MultiInputField', () => {
  it('adds a tag when Enter is pressed and removes it via Backspace', async () => {
    const user = userEvent.setup();

    render(
      <Form onSubmit={vi.fn()}>
        <MultiInputField name="emails" label="Email addresses" />
      </Form>,
      buildWrapper([getBaseProviderWrapper()])
    );

    const input = screen.getByRole('textbox', { name: 'Email addresses' });
    await user.type(input, 'a@example.com');
    await user.keyboard('{Enter}');

    expect(screen.getByText('a@example.com')).toBeInTheDocument();
    expect(input).toHaveValue('');

    // The tag's remove action is a visually-hidden icon; BaseUI wires actual removal to
    // Backspace/Delete on the focused tag itself (the accessible "button").
    screen.getByRole('button', { name: /a@example.com/i }).focus();
    await user.keyboard('{Backspace}');
    expect(screen.queryByText('a@example.com')).not.toBeInTheDocument();
  });

  it('does not add a duplicate or empty tag', async () => {
    const user = userEvent.setup();

    render(
      <Form onSubmit={vi.fn()}>
        <MultiInputField name="emails" label="Email addresses" />
      </Form>,
      buildWrapper([getBaseProviderWrapper()])
    );

    const input = screen.getByRole('textbox', { name: 'Email addresses' });
    await user.type(input, 'a@example.com{Enter}');
    await user.type(input, 'a@example.com{Enter}');
    await user.keyboard('{Enter}');

    expect(screen.getAllByText('a@example.com')).toHaveLength(1);
  });

  it('blocks form submission and shows the error when validation fails', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();

    render(
      <Form onSubmit={onSubmit}>
        <MultiInputField
          name="emails"
          label="Email addresses"
          validate={(value) => ((value as string[])?.length ? undefined : 'Required.')}
        />
        <button type="submit">Submit</button>
      </Form>,
      buildWrapper([getBaseProviderWrapper()])
    );

    await user.click(screen.getByRole('button', { name: 'Submit' }));

    expect(await screen.findByText('Required.')).toBeInTheDocument();
    expect(onSubmit).not.toHaveBeenCalled();
  });
});
