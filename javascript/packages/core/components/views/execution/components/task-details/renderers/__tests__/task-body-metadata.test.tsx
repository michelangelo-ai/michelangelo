import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { CellType } from '#core/components/cell/constants';
import { TaskBodyMetadata } from '../task-body-metadata';

describe('TaskBodyMetadata', () => {
  const mockCells = [
    {
      id: 'status',
      label: 'Status',
      type: CellType.TEXT,
      accessor: 'status',
    },
    {
      id: 'duration',
      label: 'Duration',
      type: CellType.TEXT,
      accessor: 'duration',
    },
    {
      id: 'startTime',
      label: 'Started',
      type: CellType.DATE,
      accessor: 'startTime',
    },
  ];

  it('should render metadata in accordion with label', async () => {
    const user = userEvent.setup();
    const mockData = {
      status: 'Success',
      duration: '5m 30s',
      startTime: '2025-01-01T08:00:00Z',
    };

    render(<TaskBodyMetadata label="Task Metadata" value={mockData} cells={mockCells} />);

    const accordionButton = screen.getByRole('button', { name: /Task Metadata/ });
    expect(accordionButton).toBeInTheDocument();

    await user.click(accordionButton);

    expect(screen.getByText('Success')).toBeInTheDocument();
    expect(screen.getByText('5m 30s')).toBeInTheDocument();
  });

  it('should handle undefined value gracefully', async () => {
    const user = userEvent.setup();

    render(<TaskBodyMetadata label="Empty Metadata" value={undefined} cells={mockCells} />);

    const accordionButton = screen.getByRole('button', { name: /Empty Metadata/ });
    await user.click(accordionButton);

    expect(screen.queryByText('Success')).not.toBeInTheDocument();
  });

  it('should handle empty cells array', async () => {
    const user = userEvent.setup();
    const mockData = { status: 'Success' };

    render(<TaskBodyMetadata label="No Cells" value={mockData} cells={[]} />);

    const accordionButton = screen.getByRole('button', { name: /No Cells/ });
    expect(accordionButton).toBeInTheDocument();

    await user.click(accordionButton);
  });

  it('should render with partial data', async () => {
    const user = userEvent.setup();
    const partialData = {
      status: 'Running',
    };

    render(<TaskBodyMetadata label="Partial Metadata" value={partialData} cells={mockCells} />);

    const accordionButton = screen.getByRole('button', { name: /Partial Metadata/ });
    await user.click(accordionButton);

    expect(screen.getByText('Running')).toBeInTheDocument();
    expect(screen.queryByText('5m 30s')).not.toBeInTheDocument();
  });

  it('should render cells with different cell types', async () => {
    const user = userEvent.setup();
    const cellsWithStates = [
      {
        id: 'state',
        label: 'State',
        type: CellType.STATE,
        accessor: 'state',
        stateTextMap: {
          SUCCESS: 'Completed',
          FAILED: 'Failed',
        },
        stateColorMap: {
          SUCCESS: 'green',
          FAILED: 'red',
        },
      },
    ];

    const stateData = { state: 'SUCCESS' };

    render(<TaskBodyMetadata label="State Metadata" value={stateData} cells={cellsWithStates} />);

    const accordionButton = screen.getByRole('button', { name: /State Metadata/ });
    await user.click(accordionButton);

    expect(screen.getByText('Completed')).toBeInTheDocument();
  });
});
