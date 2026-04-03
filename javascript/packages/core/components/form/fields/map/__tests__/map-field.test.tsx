import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { MapField } from '#core/components/form/fields/map/map-field';
import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getBaseProviderWrapper } from '#core/test/wrappers/get-base-provider-wrapper';
import { getFormProviderWrapper } from '#core/test/wrappers/get-form-provider-wrapper';
import { getIconProviderWrapper } from '#core/test/wrappers/get-icon-provider-wrapper';

describe('MapField', () => {
  it('renders with label', () => {
    render(
      <MapField name="metadata" label="Metadata" />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ onSubmit: vi.fn() }),
      ])
    );
    expect(screen.getByText('Metadata')).toBeInTheDocument();
  });

  it('shows empty message when no entries exist', () => {
    render(
      <MapField name="metadata" label="Metadata" emptyMessage="No entries yet" />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ onSubmit: vi.fn() }),
      ])
    );
    expect(screen.getByText('No entries yet')).toBeInTheDocument();
  });

  it('renders initial values as key-value rows', () => {
    render(
      <MapField name="metadata" label="Metadata" />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({
          onSubmit: vi.fn(),
          initialValues: { metadata: { host: 'localhost', port: '8080' } },
        }),
      ])
    );

    const inputs = screen.getAllByRole('textbox');
    expect(inputs).toHaveLength(4);
    expect(inputs[0]).toHaveValue('host');
    expect(inputs[1]).toHaveValue('localhost');
    expect(inputs[2]).toHaveValue('port');
    expect(inputs[3]).toHaveValue('8080');
  });

  it('adds a new row via add button', async () => {
    const user = userEvent.setup();
    render(
      <MapField name="metadata" label="Metadata" />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ onSubmit: vi.fn() }),
      ])
    );

    await user.click(screen.getByText('Add more'));
    expect(screen.getAllByRole('textbox')).toHaveLength(2);
  });

  it('removes a row via delete button', async () => {
    const user = userEvent.setup();
    render(
      <MapField name="metadata" label="Metadata" />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({
          onSubmit: vi.fn(),
          initialValues: { metadata: { a: '1', b: '2' } },
        }),
      ])
    );

    expect(screen.getAllByRole('textbox')).toHaveLength(4);
    const deleteButtons = screen.getAllByRole('button', { name: 'Delete' });
    await user.click(deleteButtons[0]);

    const remaining = screen.getAllByRole('textbox');
    expect(remaining).toHaveLength(2);
    expect(remaining[0]).toHaveValue('b');
    expect(remaining[1]).toHaveValue('2');
  });

  it('submits form with correct Record shape', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    render(
      <>
        <MapField name="metadata" label="Metadata" />
        <button type="submit">Submit</button>
      </>,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ onSubmit }),
      ])
    );

    await user.click(screen.getByText('Add more'));
    const inputs = screen.getAllByRole('textbox');
    await user.type(inputs[0], 'host');
    await user.type(inputs[1], 'localhost');

    await user.click(screen.getByText('Submit'));
    await waitFor(() => {
      expect(onSubmit).toHaveBeenCalled();
      expect(onSubmit.mock.calls[0][0]).toEqual(
        expect.objectContaining({ metadata: { host: 'localhost' } })
      );
    });
  });

  it('shows required indicator', () => {
    render(
      <MapField name="metadata" label="Metadata" required />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ onSubmit: vi.fn() }),
      ])
    );
    expect(
      screen.getAllByText((_, element) => element?.textContent === 'Metadata*').length
    ).toBeGreaterThan(0);
  });

  it('singleValue: renders one row with no add/delete buttons', () => {
    render(
      <MapField name="metadata" label="Metadata" singleValue />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ onSubmit: vi.fn() }),
      ])
    );

    expect(screen.getAllByRole('textbox')).toHaveLength(2);
    expect(screen.queryByText('Add more')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Delete' })).not.toBeInTheDocument();
  });

  it('readOnly: hides add and delete buttons, makes inputs read-only', () => {
    render(
      <MapField name="metadata" label="Metadata" readOnly />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({
          onSubmit: vi.fn(),
          initialValues: { metadata: { key: 'val' } },
        }),
      ])
    );

    expect(screen.queryByText('Add more')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Delete' })).not.toBeInTheDocument();
    const inputs = screen.getAllByRole('textbox');
    for (const input of inputs) {
      expect(input).toHaveAttribute('readonly');
    }
  });

  it('creatable=false: hides add button', () => {
    render(
      <MapField name="metadata" label="Metadata" creatable={false} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({
          onSubmit: vi.fn(),
          initialValues: { metadata: { a: '1' } },
        }),
      ])
    );
    expect(screen.queryByText('Add more')).not.toBeInTheDocument();
  });

  it('deletable=false: keeps add button but hides delete buttons', () => {
    render(
      <MapField name="metadata" label="Metadata" deletable={false} />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({
          onSubmit: vi.fn(),
          initialValues: { metadata: { a: '1' } },
        }),
      ])
    );
    expect(screen.getByText('Add more')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Delete' })).not.toBeInTheDocument();
  });

  it('uses custom placeholders from keyConfig and valueConfig', async () => {
    const user = userEvent.setup();
    render(
      <MapField
        name="metadata"
        label="Metadata"
        keyConfig={{ placeholder: 'Feature' }}
        valueConfig={{ placeholder: 'Transformation' }}
      />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ onSubmit: vi.fn() }),
      ])
    );

    await user.click(screen.getByText('Add more'));
    expect(screen.getByPlaceholderText('Feature')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('Transformation')).toBeInTheDocument();
  });

  it('shows error on duplicate keys', async () => {
    const user = userEvent.setup();
    render(
      <MapField name="metadata" label="Metadata" />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({
          onSubmit: vi.fn(),
          initialValues: { metadata: { a: '1' } },
        }),
      ])
    );

    await user.click(screen.getByText('Add more'));
    const keyInputs = screen.getAllByRole('textbox').filter((_, i) => i % 2 === 0);
    await user.type(keyInputs[1], 'a');

    await waitFor(() => {
      expect(screen.getAllByTitle(/cannot have duplicated values/).length).toBeGreaterThan(0);
    });
  });

  it('add then delete then add: renders new empty row', async () => {
    const user = userEvent.setup();
    render(
      <MapField name="metadata" label="Metadata" />,
      buildWrapper([
        getBaseProviderWrapper(),
        getIconProviderWrapper(),
        getFormProviderWrapper({ onSubmit: vi.fn() }),
      ])
    );

    await user.click(screen.getByText('Add more'));
    expect(screen.getAllByRole('textbox')).toHaveLength(2);

    await user.click(screen.getByRole('button', { name: 'Delete' }));
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();

    await user.click(screen.getByText('Add more'));
    const inputs = screen.getAllByRole('textbox');
    expect(inputs).toHaveLength(2);
    expect(inputs[0]).toHaveValue('');
    expect(inputs[1]).toHaveValue('');
  });
});
