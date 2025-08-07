import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { CellType } from '#core/components/cell/constants';
import {
  buildExecutionSchemaFactory,
  buildTaskStepFactory,
} from '../__fixtures__/execution-schema-factory';
import { Execution } from '../execution';

describe('Execution view', () => {
  const buildSchema = buildExecutionSchemaFactory();
  const buildStep = buildTaskStepFactory();

  it('should render TaskDetails for each parent task', () => {
    const executionData = {
      status: {
        steps: [
          buildStep({ displayName: 'Build Pipeline', state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED' }),
          buildStep({
            displayName: 'Deploy Pipeline',
            state: 'PIPELINE_RUN_STEP_STATE_RUNNING',
            subSteps: [
              buildStep({ displayName: 'Deploy App', state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED' }),
            ],
          }),
        ],
      },
    };

    render(<Execution schema={buildSchema()} data={executionData} />);

    // Should render each parent task in both overview and details sections
    expect(screen.getAllByText('Build Pipeline')).toHaveLength(2);
    expect(screen.getAllByText('Deploy Pipeline')).toHaveLength(2);
  });

  it('should handle tasks with nested subtasks', async () => {
    const user = userEvent.setup();
    const executionData = {
      status: {
        steps: [
          buildStep({
            displayName: 'Complex Pipeline',
            state: 'PIPELINE_RUN_STEP_STATE_RUNNING',
            subSteps: [
              buildStep({ displayName: 'Stage 1', state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED' }),
              buildStep({ displayName: 'Stage 2', state: 'PIPELINE_RUN_STEP_STATE_RUNNING' }),
            ],
          }),
        ],
      },
    };

    render(<Execution schema={buildSchema()} data={executionData} />);

    // Should render parent task in both sections
    expect(screen.getAllByText('Complex Pipeline')).toHaveLength(2);

    const accordionPanel = screen.getByRole('button', { expanded: false });
    expect(accordionPanel).toBeInTheDocument();

    // Subtasks are visible in Overview section but not in collapsed accordion
    // Note: Overview section shows all tasks in flat structure
    expect(screen.getAllByText('Stage 1')).toHaveLength(1);
    expect(screen.getAllByText('Stage 2')).toHaveLength(1);

    await user.click(accordionPanel);

    expect(screen.getAllByText('Stage 1')).toHaveLength(2);
    expect(screen.getAllByText('Stage 2')).toHaveLength(2);
  });

  it('should render metadata ', () => {
    const schemaWithMetadata = buildSchema({
      tasks: {
        header: {
          metadata: [
            {
              id: 'state',
              label: 'Status',
              type: CellType.STATE,
              stateTextMap: {
                PIPELINE_RUN_STEP_STATE_FAILED: 'Failed',
                PIPELINE_RUN_STEP_STATE_SUCCEEDED: 'Success',
              },
              stateColorMap: {
                PIPELINE_RUN_STEP_STATE_FAILED: 'red',
                PIPELINE_RUN_STEP_STATE_SUCCEEDED: 'green',
              },
            },
            {
              id: 'duration',
              label: 'Duration',
              type: CellType.TEXT,
            },
            {
              id: 'startTime',
              label: 'Started',
              type: CellType.DATE,
            },
          ],
        },
      },
    });

    const executionData = {
      status: {
        steps: [
          buildStep({
            displayName: 'Build Task',
            state: 'PIPELINE_RUN_STEP_STATE_FAILED',
            duration: '5m 30s',
            startTime: '2025-01-01T08:00:00Z',
          }),
        ],
      },
    };

    render(<Execution schema={schemaWithMetadata} data={executionData} />);

    // Should render task name in both sections (Overview + Details)
    expect(screen.getAllByText('Build Task')).toHaveLength(2);

    expect(screen.getByText('Failed')).toBeInTheDocument();
    expect(screen.getByText('5m 30s')).toBeInTheDocument();
    expect(screen.getByText('Started')).toBeInTheDocument();
  });

  it('should handle empty execution data gracefully', () => {
    const emptyData = {};

    render(<Execution schema={buildSchema()} data={emptyData} />);

    expect(screen.getByText('No execution data')).toBeInTheDocument();
    expect(screen.getByText('No tasks available')).toBeInTheDocument();
  });

  it('should render body schema content for leaf tasks', async () => {
    const user = userEvent.setup();
    const schemaWithBody = buildSchema({
      tasks: {
        body: [
          {
            type: 'struct',
            label: 'Input Parameters',
            accessor: 'input',
          },
          {
            type: 'textarea',
            label: 'Logs',
            accessor: 'logs',
            markdown: false,
          },
          {
            type: 'metadata',
            label: 'Performance',
            accessor: 'performance',
            cells: [
              {
                id: 'duration',
                label: 'Duration',
                type: CellType.TEXT,
                accessor: 'duration',
              },
            ],
          },
        ],
      },
    });

    const executionData = {
      status: {
        steps: [
          buildStep({
            displayName: 'Data Processing Task',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            input: {
              fields: {
                dataset: { stringValue: 'training_data.csv', kind: 'stringValue' },
              },
            },
            logs: 'Model training completed',
            performance: {
              duration: '2h 15m',
            },
          }),
        ],
      },
    };

    render(<Execution schema={schemaWithBody} data={executionData} />);

    // No body content should be visible before accordion is expanded
    expect(screen.queryByText('Input Parameters')).not.toBeInTheDocument();
    expect(screen.queryByText('Logs')).not.toBeInTheDocument();
    expect(screen.queryByText('Performance')).not.toBeInTheDocument();

    // All body schema labels should be present after accordion is expanded
    await user.click(screen.getByRole('button', { name: 'Data Processing Task Down Small' }));
    expect(screen.getByText('Input Parameters')).toBeInTheDocument();
    expect(screen.getByText('Logs')).toBeInTheDocument();
    expect(screen.getByText('Performance')).toBeInTheDocument();

    // Textarea content should be immediately visible
    expect(screen.getByLabelText('Logs')).toHaveTextContent('Model training completed');
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
    expect(screen.queryByText('2h 15m')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /Input Parameters/ }));
    expect(screen.getByRole('textbox')).toBeInTheDocument();
    expect(screen.getByText('"training_data.csv"')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /Performance/ }));
    expect(screen.getByText('2h 15m')).toBeInTheDocument();
  });
});
