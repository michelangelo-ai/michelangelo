import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { StringField } from '#core/components/form/fields/string/string-field';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getFormProviderWrapper } from '#core/test/wrappers/get-form-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';

describe('StringField', () => {
  it('renders with label', () => {
    render(
      <StringField name="email" label="Email Address" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByRole('textbox', { name: 'Email Address' })).toBeInTheDocument();
  });

  it('shows required indicator when required', () => {
    render(
      <StringField name="email" label="Email" required />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    expect(screen.getByRole('textbox', { name: 'Email *' })).toBeInTheDocument();
  });

  it('handles user input', async () => {
    const user = userEvent.setup();

    render(
      <StringField name="email" label="Email" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper({})])
    );

    const input = screen.getByRole('textbox', { name: 'Email' });
    await user.type(input, 'test@example.com');

    expect(input).toHaveValue('test@example.com');
  });

  it('displays help tooltip when description is provided', () => {
    render(
      <StringField name="email" label="Email" description="Your email address for notifications" />,
      buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper()])
    );

    expect(screen.getByRole('img', { name: 'help' })).toBeInTheDocument();
  });

  describe('multi', () => {
    it('adds a tag on enter and submits a string array', async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn();

      render(
        <>
          <StringField name="tags" label="Tags" multi />
          <button type="submit">Submit</button>
        </>,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getFormProviderWrapper({ onSubmit }),
        ])
      );

      const input = screen.getByRole('textbox', { name: 'Tags' });
      await user.type(input, 'first-value');
      await user.keyboard('{Enter}');

      expect(await screen.findByText('first-value')).toBeInTheDocument();
      expect(input).toHaveValue('');

      await user.click(screen.getByRole('button', { name: 'Submit' }));
      await waitFor(() =>
        expect(onSubmit).toHaveBeenCalledWith(
          { tags: ['first-value'] },
          expect.anything(),
          expect.anything()
        )
      );
    });

    it('persists an in-progress value on blur', async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn();

      render(
        <>
          <StringField name="tags" label="Tags" multi />
          <button type="submit">Submit</button>
        </>,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getFormProviderWrapper({ onSubmit }),
        ])
      );

      await user.type(screen.getByRole('textbox', { name: 'Tags' }), 'unfinished-value');
      await user.click(screen.getByRole('button', { name: 'Submit' }));

      await waitFor(() =>
        expect(onSubmit).toHaveBeenCalledWith(
          { tags: ['unfinished-value'] },
          expect.anything(),
          expect.anything()
        )
      );
    });

    it('removes the last tag on backspace only when the input is empty', async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn();

      render(
        <>
          <StringField name="tags" label="Tags" multi />
          <button type="submit">Submit</button>
        </>,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getFormProviderWrapper({
            initialValues: { tags: ['first-value', 'second-value'] },
            onSubmit,
          }),
        ])
      );

      await user.type(screen.getByRole('textbox', { name: 'Tags' }), 'not-empty');
      await user.keyboard('{Backspace}');
      expect(screen.getByText('second-value')).toBeInTheDocument();

      await user.clear(screen.getByRole('textbox', { name: 'Tags' }));
      await user.keyboard('{Backspace}');
      await waitFor(() => expect(screen.queryByText('second-value')).not.toBeInTheDocument());
      expect(screen.getByText('first-value')).toBeInTheDocument();

      await user.click(screen.getByRole('button', { name: 'Submit' }));
      await waitFor(() =>
        expect(onSubmit).toHaveBeenCalledWith(
          { tags: ['first-value'] },
          expect.anything(),
          expect.anything()
        )
      );
    });

    it('removes a single tag via its remove control', async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn();

      render(
        <>
          <StringField name="tags" label="Tags" multi />
          <button type="submit">Submit</button>
        </>,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getFormProviderWrapper({
            initialValues: { tags: ['first-value', 'second-value'] },
            onSubmit,
          }),
        ])
      );

      const firstTag = screen.getByRole('button', { name: /first-value/ });
      await user.click(within(firstTag).getAllByTitle('Delete')[0]);

      await waitFor(() => expect(screen.queryByText('first-value')).not.toBeInTheDocument());
      expect(screen.getByText('second-value')).toBeInTheDocument();

      await user.click(screen.getByRole('button', { name: 'Submit' }));
      await waitFor(() =>
        expect(onSubmit).toHaveBeenCalledWith(
          { tags: ['second-value'] },
          expect.anything(),
          expect.anything()
        )
      );
    });

    it('clears all tags and refocuses the input', async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn();

      render(
        <>
          <StringField name="tags" label="Tags" multi />
          <button type="submit">Submit</button>
        </>,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getFormProviderWrapper({
            initialValues: { tags: ['first-value', 'second-value'] },
            onSubmit,
          }),
        ])
      );

      await user.click(screen.getAllByTitle('Delete Alt')[0]);

      await waitFor(() => expect(screen.queryByText('first-value')).not.toBeInTheDocument());
      expect(screen.queryByText('second-value')).not.toBeInTheDocument();
      expect(screen.getByRole('textbox', { name: 'Tags' })).toHaveFocus();

      await user.click(screen.getByRole('button', { name: 'Submit' }));
      await waitFor(() =>
        expect(onSubmit).toHaveBeenCalledWith({ tags: [] }, expect.anything(), expect.anything())
      );
    });

    it('edits a tag in place and confirms with enter', async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn();

      render(
        <>
          <StringField name="tags" label="Tags" multi />
          <button type="submit">Submit</button>
        </>,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getFormProviderWrapper({ initialValues: { tags: ['first-value'] }, onSubmit }),
        ])
      );

      await user.click(screen.getByRole('button', { name: /first-value/ }));
      const editInput = screen.getByDisplayValue('first-value');
      await user.type(editInput, '-edited');
      await user.keyboard('{Enter}');

      expect(await screen.findByText('first-value-edited')).toBeInTheDocument();

      await user.click(screen.getByRole('button', { name: 'Submit' }));
      await waitFor(() =>
        expect(onSubmit).toHaveBeenCalledWith(
          { tags: ['first-value-edited'] },
          expect.anything(),
          expect.anything()
        )
      );
    });

    it('reverts an edit when the tag loses focus without confirming', async () => {
      const user = userEvent.setup();

      render(
        <StringField name="tags" label="Tags" multi />,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getFormProviderWrapper({ initialValues: { tags: ['first-value'] } }),
        ])
      );

      await user.click(screen.getByRole('button', { name: /first-value/ }));
      const editInput = screen.getByDisplayValue('first-value');
      await user.type(editInput, '-edited');
      await user.click(screen.getByText('Tags'));

      await waitFor(() => expect(screen.queryByText('first-value-edited')).not.toBeInTheDocument());
      expect(screen.getByText('first-value')).toBeInTheDocument();
    });

    it('shows placeholder only when there are no tags', async () => {
      const user = userEvent.setup();

      render(
        <StringField name="tags" label="Tags" multi placeholder="Add a tag" />,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper(), getFormProviderWrapper()])
      );

      const input = screen.getByRole('textbox', { name: 'Tags' });
      expect(input).toHaveAttribute('placeholder', 'Add a tag');

      await user.type(input, 'first-value');
      await user.keyboard('{Enter}');

      await waitFor(() => expect(input).toHaveAttribute('placeholder', ''));
    });

    it('prevents typing, editing, and removing tags when read-only', async () => {
      const user = userEvent.setup();

      render(
        <StringField name="tags" label="Tags" multi readOnly />,
        buildWrapper([
          getBaseProviderWrapper(),
          getIconProviderWrapper(),
          getFormProviderWrapper({ initialValues: { tags: ['first-value', 'second-value'] } }),
        ])
      );

      const input = screen.getByRole('textbox', { name: 'Tags' });
      await user.type(input, 'another-value');
      expect(input).toHaveValue('');

      await user.keyboard('{Backspace}');
      expect(screen.getByText('first-value')).toBeInTheDocument();
      expect(screen.getByText('second-value')).toBeInTheDocument();

      await user.click(screen.getByText('first-value'));
      expect(screen.queryByDisplayValue('first-value')).not.toBeInTheDocument();

      expect(screen.queryByTitle('Delete Alt')).not.toBeInTheDocument();
    });
  });
});
