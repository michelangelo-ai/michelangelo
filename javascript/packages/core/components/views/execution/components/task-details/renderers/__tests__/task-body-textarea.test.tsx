import { render, screen } from '@testing-library/react';

import { TaskBodyTextarea } from '../task-body-textarea';

describe('TaskBodyTextarea', () => {
  it('should render text content without markdown', () => {
    render(
      <TaskBodyTextarea label="Error Log" value="Pipeline failed at step 3" markdown={false} />
    );

    expect(screen.getByText('Error Log')).toBeInTheDocument();
    expect(screen.getByText('Pipeline failed at step 3')).toBeInTheDocument();
  });

  it('should render text content with markdown by default', () => {
    const markdownText = '# Error\nPipeline **failed** at step 3';

    render(<TaskBodyTextarea label="Error Log" value={markdownText} />);

    expect(screen.getByText('Error Log')).toBeInTheDocument();
    expect(screen.getByRole('heading', { level: 1, name: 'Error' })).toBeInTheDocument();
  });

  it('should truncate text to 10000 characters', () => {
    const longText = 'x'.repeat(20000);
    const expectedText = 'x'.repeat(10000);

    render(<TaskBodyTextarea label="Long Log" value={longText} markdown={false} />);

    expect(screen.getByText(expectedText)).toBeInTheDocument();
    expect(screen.queryByText(longText)).not.toBeInTheDocument();
  });

  it('should not render when value is undefined', () => {
    render(<TaskBodyTextarea label="Empty Log" value={undefined} />);

    expect(screen.queryByText('Empty Log')).not.toBeInTheDocument();
  });

  it('should not render when value is empty string', () => {
    render(<TaskBodyTextarea label="Empty Log" value="" />);

    expect(screen.queryByText('Empty Log')).not.toBeInTheDocument();
  });
});
