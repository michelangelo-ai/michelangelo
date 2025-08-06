import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { TaskBodyStruct } from '../task-body-struct';

describe('TaskBodyStruct', () => {
  it('should render struct data as formatted JSON', async () => {
    const user = userEvent.setup();
    const structValue = {
      fields: {
        dataset: { stringValue: 'training_data.csv', kind: 'stringValue' },
        batchSize: { numberValue: 32, kind: 'numberValue' },
      },
    };

    render(<TaskBodyStruct label="Input Parameters" value={structValue} />);

    const accordionButton = screen.getByRole('button', { name: /Input Parameters/ });
    expect(accordionButton).toBeInTheDocument();

    await user.click(accordionButton);

    expect(screen.getByRole('textbox')).toBeInTheDocument();
    expect(screen.getByText('"training_data.csv"')).toBeInTheDocument();
    expect(screen.getByText('32')).toBeInTheDocument();
  });

  it('should handle non-struct objects', async () => {
    const user = userEvent.setup();
    const regularObject = {
      name: 'test-pipeline',
      version: '1.0.0',
    };

    render(<TaskBodyStruct label="Configuration" value={regularObject} />);

    const accordionButton = screen.getByRole('button', { name: /Configuration/ });
    await user.click(accordionButton);

    expect(screen.getByText('"test-pipeline"')).toBeInTheDocument();
    expect(screen.getByText('"1.0.0"')).toBeInTheDocument();
  });

  it('should handle null and undefined values', async () => {
    const user = userEvent.setup();

    const { rerender } = render(<TaskBodyStruct label="Empty Value" value={null} />);

    const accordionButton = screen.getByRole('button', { name: /Empty Value/ });
    await user.click(accordionButton);

    expect(screen.getByText('null')).toBeInTheDocument();

    rerender(<TaskBodyStruct label="Empty Value" value={undefined} />);
    await user.click(accordionButton);

    expect(screen.getByRole('textbox')).toBeInTheDocument();
  });

  it('should not allow editing the JSON content', async () => {
    const user = userEvent.setup();
    const testValue = { test: 'data' };

    render(<TaskBodyStruct label="Read Only Test" value={testValue} />);

    const accordionButton = screen.getByRole('button', { name: /Read Only Test/ });
    await user.click(accordionButton);

    const textEditor = screen.getByRole('textbox');

    // Try to type in the editor - should not change content
    await user.click(textEditor);
    await user.type(textEditor, 'should not appear');
    expect(screen.getByText(/"test":/i)).toBeInTheDocument();
    expect(screen.getByText(/"data"/i)).toBeInTheDocument();
    expect(screen.queryByText('should not appear')).not.toBeInTheDocument();
  });

  it('should expand accordion to show content', async () => {
    const user = userEvent.setup();
    const testValue = { test: 'data' };

    render(<TaskBodyStruct label="Toggle Test" value={testValue} />);

    const accordionButton = screen.getByRole('button', { name: /Toggle Test/ });

    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();

    await user.click(accordionButton);
    expect(screen.getByRole('textbox')).toBeInTheDocument();
  });
});
