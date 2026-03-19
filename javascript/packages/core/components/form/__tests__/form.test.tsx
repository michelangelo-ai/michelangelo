import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { DeleteAlt, Plus } from 'baseui/icon';
import { FORM_ERROR } from 'final-form';
import { vi } from 'vitest';

import { FormDialog } from '#core/components/form/components/form-dialog/form-dialog';
import { FormErrorBanner } from '#core/components/form/components/form-error-banner/form-error-banner';
import { StringField } from '#core/components/form/fields/string/string-field';
import { Form } from '#core/components/form/form';
import { useForm } from '#core/components/form/hooks/use-form';
import { ArrayFormGroup } from '#core/components/form/layout/array-form-group/array-form-group';
import { ArrayFormRow } from '#core/components/form/layout/array-form-row/array-form-row';
import { combineValidators } from '#core/components/form/validation/combine-validators';
import { minLength, required } from '#core/components/form/validation/validators';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';

describe('Form', () => {
  describe('integration', () => {
    it('submits form with multiple field values', async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn();

      render(
        <Form onSubmit={onSubmit}>
          <StringField name="email" label="Email" />
          <StringField name="name" label="Name" />
          <button type="submit">Submit</button>
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      await user.type(screen.getByLabelText('Email'), 'test@example.com');
      await user.type(screen.getByLabelText('Name'), 'John Doe');
      await user.click(screen.getByRole('button', { name: 'Submit' }));

      await waitFor(() =>
        expect(onSubmit).toHaveBeenCalledWith(
          {
            email: 'test@example.com',
            name: 'John Doe',
          },
          expect.anything(),
          expect.anything()
        )
      );
    });

    it('provides initial values to fields', async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn();
      const initialValues = { email: 'initial@example.com', name: 'Initial User' };

      render(
        <div>
          <Form onSubmit={onSubmit} initialValues={initialValues}>
            <StringField name="email" label="Email" />
            <StringField name="name" label="Name" />
            <button type="submit">Submit</button>
          </Form>
        </div>,

        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.getByRole('textbox', { name: 'Email' })).toHaveValue('initial@example.com');
      expect(screen.getByRole('textbox', { name: 'Name' })).toHaveValue('Initial User');
      await user.click(screen.getByRole('button', { name: 'Submit' }));
      await waitFor(() =>
        expect(onSubmit).toHaveBeenCalledWith(initialValues, expect.anything(), expect.anything())
      );
    });

    it('populates the field with defaultValue', () => {
      render(
        <Form onSubmit={vi.fn()}>
          <StringField name="email" label="Email" defaultValue="from-default" />
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.getByRole('textbox', { name: 'Email' })).toHaveValue('from-default');
    });

    it('uses initialValues over defaultValue when both are provided', () => {
      render(
        <Form onSubmit={vi.fn()} initialValues={{ email: 'from-form' }}>
          <StringField name="email" label="Email" defaultValue="from-default" />
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.getByRole('textbox', { name: 'Email' })).toHaveValue('from-form');
    });

    it('uses field-level initialValue over defaultValue when both are provided', () => {
      render(
        <Form onSubmit={vi.fn()}>
          <StringField
            name="email"
            label="Email"
            defaultValue="from-default"
            initialValue="from-field-initial"
          />
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.getByRole('textbox', { name: 'Email' })).toHaveValue('from-field-initial');
    });

    it('uses field-level initialValue when all three value sources are provided', () => {
      render(
        <Form onSubmit={vi.fn()} initialValues={{ email: 'from-form' }}>
          <StringField
            name="email"
            label="Email"
            defaultValue="from-default"
            initialValue="from-field-initial"
          />
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.getByRole('textbox', { name: 'Email' })).toHaveValue('from-field-initial');
    });

    it('applies parse to transform input before storing in form state', async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn();

      render(
        <Form onSubmit={onSubmit}>
          <StringField
            name="code"
            label="Code"
            parse={(value: unknown) => String(value).toUpperCase()}
          />
          <button type="submit">Submit</button>
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      await user.type(screen.getByLabelText('Code'), 'abc');
      await user.click(screen.getByRole('button', { name: 'Submit' }));

      await waitFor(() =>
        expect(onSubmit).toHaveBeenCalledWith({ code: 'ABC' }, expect.anything(), expect.anything())
      );
    });

    it('applies format to transform stored value for display', () => {
      render(
        <Form onSubmit={vi.fn()} initialValues={{ price: '1000' }}>
          <StringField
            name="price"
            label="Price"
            format={(value: string) => (value ? `$${value}` : '')}
          />
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.getByRole('textbox', { name: 'Price' })).toHaveValue('$1000');
    });

    it('applies format and parse together as inverse transforms', async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn();

      render(
        <Form onSubmit={onSubmit} initialValues={{ tag: 'initial' }}>
          <StringField
            name="tag"
            label="Tag"
            format={(value: string) => (value ? `#${value}` : '')}
            parse={(value: unknown) => String(value).replace(/^#/, '')}
          />
          <button type="submit">Submit</button>
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.getByRole('textbox', { name: 'Tag' })).toHaveValue('#initial');

      await user.clear(screen.getByRole('textbox', { name: 'Tag' }));
      await user.type(screen.getByRole('textbox', { name: 'Tag' }), '#updated');
      await user.click(screen.getByRole('button', { name: 'Submit' }));

      await waitFor(() =>
        expect(onSubmit).toHaveBeenCalledWith(
          { tag: 'updated' },
          expect.anything(),
          expect.anything()
        )
      );
    });

    it('supports external submit button via form id', async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn();

      render(
        <div>
          <Form id="test-form" onSubmit={onSubmit}>
            <StringField name="email" label="Email" />
          </Form>
          <button type="submit" form="test-form">
            External Submit
          </button>
        </div>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      await user.type(screen.getByLabelText('Email'), 'test@example.com');
      await user.click(screen.getByRole('button', { name: 'External Submit' }));

      await waitFor(() =>
        expect(onSubmit).toHaveBeenCalledWith(
          { email: 'test@example.com' },
          expect.anything(),
          expect.anything()
        )
      );
    });

    it('supports render prop for wrapping form element', async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn();

      render(
        <Form
          id="wrapped-form"
          onSubmit={onSubmit}
          render={(formElement) => (
            <div data-testid="wrapper">
              <div data-testid="header">Header Content</div>
              {formElement}
              <div data-testid="footer">Footer Content</div>
            </div>
          )}
        >
          <StringField name="email" label="Email" />
          <button type="submit">Submit</button>
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.getByTestId('wrapper')).toBeInTheDocument();
      expect(screen.getByTestId('header')).toHaveTextContent('Header Content');
      expect(screen.getByTestId('footer')).toHaveTextContent('Footer Content');
      expect(screen.getByLabelText('Email')).toBeInTheDocument();

      await user.type(screen.getByLabelText('Email'), 'test@example.com');
      await user.click(screen.getByRole('button', { name: 'Submit' }));

      await waitFor(() =>
        expect(onSubmit).toHaveBeenCalledWith(
          { email: 'test@example.com' },
          expect.anything(),
          expect.anything()
        )
      );
    });

    it('allows external submit button in render prop wrapper via form id', async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn();

      render(
        <Form
          id="wrapped-form"
          onSubmit={onSubmit}
          render={(formElement) => (
            <div data-testid="wrapper">
              {formElement}
              <div data-testid="footer">
                <button type="submit" form="wrapped-form">
                  External Submit
                </button>
              </div>
            </div>
          )}
        >
          <StringField name="email" label="Email" />
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      await user.type(screen.getByLabelText('Email'), 'test@example.com');
      await user.click(screen.getByRole('button', { name: 'External Submit' }));

      await waitFor(() =>
        expect(onSubmit).toHaveBeenCalledWith(
          { email: 'test@example.com' },
          expect.anything(),
          expect.anything()
        )
      );
    });

    it('useForm change() updates field value before submit', async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn();

      function SetStatusButton() {
        const { change } = useForm();
        return <button onClick={() => change('status', 'in-progress')}>Set Status</button>;
      }

      render(
        <Form onSubmit={onSubmit}>
          <StringField name="status" label="Status" />
          <SetStatusButton />
          <button type="submit">Submit</button>
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      await user.click(screen.getByRole('button', { name: 'Set Status' }));
      await user.click(screen.getByRole('button', { name: 'Submit' }));

      await waitFor(() =>
        expect(onSubmit).toHaveBeenCalledWith(
          { status: 'in-progress' },
          expect.anything(),
          expect.anything()
        )
      );
    });

    it('useForm multiple change() calls accumulate', async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn();

      function SetFieldsButton() {
        const { change } = useForm();
        return (
          <button
            onClick={() => {
              change('first', 'alpha');
              change('second', 'beta');
            }}
          >
            Set Fields
          </button>
        );
      }

      render(
        <Form onSubmit={onSubmit}>
          <StringField name="first" label="First" />
          <StringField name="second" label="Second" />
          <SetFieldsButton />
          <button type="submit">Submit</button>
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      await user.click(screen.getByRole('button', { name: 'Set Fields' }));
      await user.click(screen.getByRole('button', { name: 'Submit' }));

      await waitFor(() =>
        expect(onSubmit).toHaveBeenCalledWith(
          { first: 'alpha', second: 'beta' },
          expect.anything(),
          expect.anything()
        )
      );
    });

    it('useForm submit() triggers onSubmit programmatically', async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn();

      function ProgrammaticSubmitButton() {
        const { submit } = useForm();
        return <button onClick={() => void submit()}>Programmatic Submit</button>;
      }

      render(
        <Form onSubmit={onSubmit} initialValues={{ email: 'test@example.com' }}>
          <StringField name="email" label="Email" />
          <ProgrammaticSubmitButton />
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      await user.click(screen.getByRole('button', { name: 'Programmatic Submit' }));

      await waitFor(() =>
        expect(onSubmit).toHaveBeenCalledWith(
          { email: 'test@example.com' },
          expect.anything(),
          expect.anything()
        )
      );
    });

    it('submits form via sticky footer shorthand right content', async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn();

      render(
        <Form onSubmit={onSubmit} footer={{ right: <button type="submit">Save</button> }}>
          <StringField name="email" label="Email" />
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      await user.type(screen.getByLabelText('Email'), 'test@example.com');
      await user.click(screen.getByRole('button', { name: 'Save' }));

      await waitFor(() =>
        expect(onSubmit).toHaveBeenCalledWith(
          { email: 'test@example.com' },
          expect.anything(),
          expect.anything()
        )
      );
    });

    it('renders sticky footer with shorthand left and right content', () => {
      render(
        <Form
          onSubmit={vi.fn()}
          footer={{ left: <span>Last saved 2m ago</span>, right: <button>Save</button> }}
        >
          <StringField name="email" label="Email" />
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );
      expect(screen.getByText('Last saved 2m ago')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Save' })).toBeInTheDocument();
    });

    it('renders custom footer ReactNode directly', () => {
      render(
        <Form onSubmit={vi.fn()} footer={<div data-testid="custom-footer">Custom</div>}>
          <StringField name="email" label="Email" />
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );
      expect(screen.getByTestId('custom-footer')).toBeInTheDocument();
    });

    describe('Repeated layouts', () => {
      const icons = { plus: Plus, deleteAlt: DeleteAlt };

      it('submits ArrayFormGroup data as a nested array', async () => {
        const user = userEvent.setup();
        const onSubmit = vi.fn();

        render(
          <Form onSubmit={onSubmit}>
            <ArrayFormGroup rootFieldPath="addresses" groupLabel="Address" minItems={1}>
              {(name) => <StringField name={`${name}.street`} label="Street" />}
            </ArrayFormGroup>
            <button type="submit">Submit</button>
          </Form>,
          buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper({ icons })])
        );

        await waitFor(() => screen.getByRole('textbox', { name: 'Street' }));
        await user.type(screen.getByRole('textbox', { name: 'Street' }), '123 Main St');
        await user.click(screen.getByRole('button', { name: 'Submit' }));

        await waitFor(() =>
          expect(onSubmit).toHaveBeenCalledWith(
            { addresses: [{ street: '123 Main St' }] },
            expect.anything(),
            expect.anything()
          )
        );
      });

      it('submits ArrayFormRow data as a nested array', async () => {
        const user = userEvent.setup();
        const onSubmit = vi.fn();

        render(
          <Form onSubmit={onSubmit}>
            <ArrayFormRow rootFieldPath="tags" minItems={1}>
              {(name) => <StringField name={`${name}.value`} label="Tag" />}
            </ArrayFormRow>
            <button type="submit">Submit</button>
          </Form>,
          buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper({ icons })])
        );

        await waitFor(() => screen.getByRole('textbox', { name: 'Tag' }));
        await user.type(screen.getByRole('textbox', { name: 'Tag' }), 'ml');
        await user.click(screen.getByRole('button', { name: 'Submit' }));

        await waitFor(() =>
          expect(onSubmit).toHaveBeenCalledWith(
            { tags: [{ value: 'ml' }] },
            expect.anything(),
            expect.anything()
          )
        );
      });

      it('populates ArrayFormGroup fields from initialValues', async () => {
        render(
          <Form
            onSubmit={vi.fn()}
            initialValues={{
              addresses: [{ street: '1 Infinite Loop' }, { street: '1600 Amphitheatre' }],
            }}
          >
            <ArrayFormGroup rootFieldPath="addresses" groupLabel="Address">
              {(name) => <StringField name={`${name}.street`} label="Street" />}
            </ArrayFormGroup>
            <button type="submit">Submit</button>
          </Form>,
          buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper({ icons })])
        );

        const fields = await screen.findAllByRole('textbox', { name: 'Street' });
        expect(fields).toHaveLength(2);
        expect(fields[0]).toHaveValue('1 Infinite Loop');
        expect(fields[1]).toHaveValue('1600 Amphitheatre');
      });

      it('submits multiple added items', async () => {
        const user = userEvent.setup();
        const onSubmit = vi.fn();

        render(
          <Form onSubmit={onSubmit}>
            <ArrayFormGroup rootFieldPath="contacts" groupLabel="Contact" minItems={1}>
              {(name) => <StringField name={`${name}.email`} label="Email" />}
            </ArrayFormGroup>
            <button type="submit">Submit</button>
          </Form>,
          buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper({ icons })])
        );

        await waitFor(() => screen.getByRole('textbox', { name: 'Email' }));
        await user.type(screen.getByRole('textbox', { name: 'Email' }), 'a@example.com');
        await user.click(screen.getByRole('button', { name: /add contact/i }));

        const fields = await screen.findAllByRole('textbox', { name: 'Email' });
        await user.type(fields[1], 'b@example.com');
        await user.click(screen.getByRole('button', { name: 'Submit' }));

        await waitFor(() =>
          expect(onSubmit).toHaveBeenCalledWith(
            { contacts: [{ email: 'a@example.com' }, { email: 'b@example.com' }] },
            expect.anything(),
            expect.anything()
          )
        );
      });
    });
  });

  describe('validation', () => {
    it('allows submission after required field is filled', async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn();

      render(
        <Form onSubmit={onSubmit}>
          <StringField name="username" label="Username" required />
          <button type="submit">Submit</button>
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      await user.click(screen.getByRole('button', { name: 'Submit' }));
      expect(await screen.findByText('This field is required.')).toBeInTheDocument();

      await user.type(screen.getByRole('textbox', { name: 'Username *' }), 'johndoe');
      await user.click(screen.getByRole('button', { name: 'Submit' }));

      await waitFor(() =>
        expect(onSubmit).toHaveBeenCalledWith(
          { username: 'johndoe' },
          expect.anything(),
          expect.anything()
        )
      );
    });

    it('shows first error when composed validators fail sequentially', async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn();

      render(
        <Form onSubmit={onSubmit}>
          <StringField
            name="username"
            label="Username"
            required
            validate={combineValidators(required(), minLength(6))}
          />
          <button type="submit">Submit</button>
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      await user.click(screen.getByRole('button', { name: 'Submit' }));
      expect(await screen.findByText('This field is required.')).toBeInTheDocument();
      expect(onSubmit).not.toHaveBeenCalled();

      await user.type(screen.getByRole('textbox', { name: 'Username *' }), 'abc');
      await user.click(screen.getByRole('button', { name: 'Submit' }));

      expect(await screen.findByText('Must be at least 6 characters.')).toBeInTheDocument();
      expect(onSubmit).not.toHaveBeenCalled();
    });

    it('focuses first field with error on failed submit', async () => {
      const user = userEvent.setup();

      render(
        <Form onSubmit={vi.fn()}>
          <StringField name="email" label="Email" required />
          <StringField name="name" label="Name" required />
          <button type="submit">Submit</button>
        </Form>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      await user.click(screen.getByRole('button', { name: 'Submit' }));

      await waitFor(() => {
        expect(document.activeElement).toBe(screen.getByRole('textbox', { name: 'Email *' }));
      });
    });
  });

  describe('error display', () => {
    const wrapper = buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()]);

    function getErrorEntry(banner: HTMLElement, label: string, errorMessage: string) {
      const labelButton = within(banner).getByRole('button', { name: label });
      const entry = labelButton.closest('div')!;
      expect(entry).toHaveTextContent(errorMessage);
      return entry;
    }

    it('shows errors after failed submit', async () => {
      const user = userEvent.setup();

      render(
        <Form onSubmit={vi.fn()}>
          <StringField name="email" label="Email Address" required />
          <FormErrorBanner />
          <button type="submit">Submit</button>
        </Form>,
        wrapper
      );

      expect(screen.queryByRole('complementary')).not.toBeInTheDocument();

      await user.click(screen.getByRole('button', { name: 'Submit' }));
      const banner = await screen.findByRole('complementary');
      expect(banner).toHaveTextContent('This field is required.');
    });

    it('clears errors when field is corrected', async () => {
      const user = userEvent.setup();

      render(
        <Form onSubmit={vi.fn()}>
          <StringField name="email" label="Email Address" required />
          <FormErrorBanner />
          <button type="submit">Submit</button>
        </Form>,
        wrapper
      );

      await user.click(screen.getByRole('button', { name: 'Submit' }));
      expect(await screen.findByRole('complementary')).toBeInTheDocument();

      await user.type(screen.getByRole('textbox', { name: 'Email Address *' }), 'test@example.com');

      await waitFor(() => {
        expect(screen.queryByRole('complementary')).not.toBeInTheDocument();
      });
    });

    it('focuses the correct field when clicking a label among multiple errors', async () => {
      const user = userEvent.setup();

      render(
        <Form onSubmit={vi.fn()}>
          <StringField name="email" label="Email" required />
          <StringField name="name" label="Name" required />
          <FormErrorBanner />
          <button type="submit">Submit</button>
        </Form>,
        wrapper
      );

      await user.click(screen.getByRole('button', { name: 'Submit' }));

      const banner = await screen.findByRole('complementary');
      await user.click(within(banner).getByText('Name'));

      expect(document.activeElement).toBe(screen.getByRole('textbox', { name: 'Name *' }));
      expect(document.activeElement).not.toBe(screen.getByRole('textbox', { name: 'Email *' }));
    });

    it('does not show a clickable button for form-level errors', async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn().mockResolvedValue({ [FORM_ERROR]: 'Something went wrong' });

      render(
        <Form onSubmit={onSubmit}>
          <StringField name="email" label="Email" />
          <FormErrorBanner />
          <button type="submit">Submit</button>
        </Form>,
        wrapper
      );

      await user.click(screen.getByRole('button', { name: 'Submit' }));

      const banner = await screen.findByRole('complementary');
      expect(banner).toHaveTextContent('Something went wrong');
      expect(within(banner).queryByRole('button')).not.toBeInTheDocument();
    });

    it('shows separate errors for sibling fields in the same nested object', async () => {
      const user = userEvent.setup();

      render(
        <Form onSubmit={vi.fn()}>
          <StringField name="address.street" label="Street" required />
          <StringField name="address.city" label="City" required />
          <FormErrorBanner />
          <button type="submit">Submit</button>
        </Form>,
        wrapper
      );

      await user.click(screen.getByRole('button', { name: 'Submit' }));

      const banner = await screen.findByRole('complementary');
      getErrorEntry(banner, 'Street', 'This field is required.');
      getErrorEntry(banner, 'City', 'This field is required.');
    });

    it('shows form-level error alongside field validation errors', async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn().mockResolvedValue({ [FORM_ERROR]: 'Server unavailable' });

      render(
        <Form onSubmit={onSubmit}>
          <StringField name="email" label="Email" required />
          <FormErrorBanner />
          <button type="submit">Submit</button>
        </Form>,
        wrapper
      );

      // First submit — field error fires, onSubmit is never called because validation blocks it
      await user.click(screen.getByRole('button', { name: 'Submit' }));
      const banner = await screen.findByRole('complementary');
      getErrorEntry(banner, 'Email', 'This field is required.');

      // Fill the field and resubmit — now onSubmit is called and returns server error
      await user.type(screen.getByRole('textbox', { name: 'Email *' }), 'test@example.com');
      await user.click(screen.getByRole('button', { name: 'Submit' }));

      expect(await screen.findByText(/Server unavailable/)).toBeInTheDocument();
    });

    it('removes only the corrected field error when one of multiple errors is fixed', async () => {
      const user = userEvent.setup();

      render(
        <Form onSubmit={vi.fn()}>
          <StringField name="email" label="Email" required />
          <StringField name="name" label="Name" required />
          <FormErrorBanner />
          <button type="submit">Submit</button>
        </Form>,
        wrapper
      );

      await user.click(screen.getByRole('button', { name: 'Submit' }));
      const banner = await screen.findByRole('complementary');

      getErrorEntry(banner, 'Email', 'This field is required.');
      getErrorEntry(banner, 'Name', 'This field is required.');

      // Fix only the email field
      await user.type(screen.getByRole('textbox', { name: 'Email *' }), 'test@example.com');

      await waitFor(() => {
        expect(
          within(screen.getByRole('complementary')).queryByText('Email')
        ).not.toBeInTheDocument();
      });

      // Name entry still present with label and error together
      getErrorEntry(screen.getByRole('complementary'), 'Name', 'This field is required.');
    });

    it('shows error without label when field has no label', async () => {
      const user = userEvent.setup();

      render(
        <Form onSubmit={vi.fn()}>
          <StringField name="username" required />
          <FormErrorBanner />
          <button type="submit">Submit</button>
        </Form>,
        wrapper
      );

      await user.click(screen.getByRole('button', { name: 'Submit' }));

      const errorDisplay = await screen.findByRole('complementary');
      expect(within(errorDisplay).getByText('This field is required.')).toBeInTheDocument();
      expect(within(errorDisplay).queryByRole('button')).not.toBeInTheDocument();
    });
  });

  describe('FormDialog', () => {
    const defaultProps = {
      isOpen: true,
      onDismiss: vi.fn(),
      heading: 'Test Dialog',
      onSubmit: vi.fn(),
    };

    beforeEach(() => {
      vi.clearAllMocks();
    });

    it('renders dialog with form when open', async () => {
      render(
        <FormDialog {...defaultProps}>
          <StringField name="email" label="Email" />
        </FormDialog>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      await screen.findByRole('dialog', { name: 'Test Dialog' });
      expect(screen.getByLabelText('Email')).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Submit' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument();
    });

    it('does not render when closed', async () => {
      render(
        <FormDialog {...defaultProps} isOpen={false}>
          <StringField name="email" label="Email" />
        </FormDialog>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      try {
        await screen.findByRole('dialog', {}, { timeout: 100 });
        throw new Error('Dialog should not be in the document');
      } catch (e: unknown) {
        if (e instanceof Error) {
          if (e.name !== 'TestingLibraryElementError') throw e;
        } else {
          throw e;
        }

        // Success!
      }
    });

    it('submits form data and auto-closes on success', async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn().mockResolvedValue(undefined);
      const onDismiss = vi.fn();

      render(
        <FormDialog {...defaultProps} onSubmit={onSubmit} onDismiss={onDismiss}>
          <StringField name="email" label="Email" />
        </FormDialog>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      await user.type(screen.getByLabelText('Email'), 'test@example.com');
      await user.click(screen.getByRole('button', { name: 'Submit' }));

      await waitFor(() => {
        expect(onSubmit).toHaveBeenCalledWith({ email: 'test@example.com' });
      });

      await waitFor(() => {
        expect(onDismiss).toHaveBeenCalledTimes(1);
      });
    });

    it('calls onDismiss when cancel is clicked', async () => {
      const user = userEvent.setup();
      const onDismiss = vi.fn();

      render(
        <FormDialog {...defaultProps} onDismiss={onDismiss}>
          <StringField name="email" label="Email" />
        </FormDialog>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      await user.click(screen.getByRole('button', { name: 'Cancel' }));
      expect(onDismiss).toHaveBeenCalledTimes(1);
    });

    it('handles submit errors without auto-closing', async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn().mockRejectedValue(new Error('Submit failed'));
      const onDismiss = vi.fn();

      render(
        <FormDialog {...defaultProps} onSubmit={onSubmit} onDismiss={onDismiss}>
          <StringField name="email" label="Email" />
        </FormDialog>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      await user.click(screen.getByRole('button', { name: 'Submit' }));

      await waitFor(() => {
        expect(onSubmit).toHaveBeenCalled();
      });

      expect(onDismiss).not.toHaveBeenCalled();
      expect(screen.getByRole('dialog', { name: 'Test Dialog' })).toBeInTheDocument();
      await screen.findByText(/Submit failed/);
    });

    it('supports custom submit label and initial values', () => {
      render(
        <FormDialog
          {...defaultProps}
          submitLabel="Create Item"
          initialValues={{ email: 'preset@example.com' }}
        >
          <StringField name="email" label="Email" />
        </FormDialog>,
        buildWrapper([getBaseProviderWrapper(), getIconProviderWrapper()])
      );

      expect(screen.getByRole('button', { name: 'Create Item' })).toBeInTheDocument();
      expect(screen.getByRole('textbox', { name: 'Email' })).toHaveValue('preset@example.com');
    });
  });
});
