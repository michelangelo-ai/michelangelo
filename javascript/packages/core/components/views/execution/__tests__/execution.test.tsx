import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

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
            displayName: 'Unfocused Step',
            state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            subSteps: [
              buildStep({
                displayName: 'Unfocused Subtask 1',
                state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
              }),
              buildStep({
                displayName: 'Unfocused Subtask 2',
                state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
              }),
            ],
          }),
          buildStep({
            displayName: 'Focused Step',
            state: 'PIPELINE_RUN_STEP_STATE_RUNNING',
            subSteps: [
              buildStep({
                displayName: 'Focused Subtask 1',
                state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
              }),
              buildStep({
                displayName: 'Focused Subtask 2',
                state: 'PIPELINE_RUN_STEP_STATE_RUNNING',
              }),
            ],
          }),
        ],
      },
    };

    render(<Execution schema={buildSchema()} data={executionData} />);

    // Should render parent task in both sections
    expect(screen.getAllByText('Unfocused Step')).toHaveLength(2);
    expect(screen.getAllByText('Focused Step')).toHaveLength(2);

    // Accordion for focused task is expanded
    expect(screen.getByRole('button', { name: 'Focused Step Down Small' })).toHaveAttribute(
      'aria-expanded',
      'true'
    );

    expect(screen.getByRole('button', { name: 'Unfocused Step Down Small' })).toHaveAttribute(
      'aria-expanded',
      'false'
    );

    // Unfocused subtasks are not visible in collapsed accordion or overview
    expect(screen.queryByText('Unfocused Subtask 1')).not.toBeInTheDocument();
    expect(screen.queryByText('Unfocused Subtask 2')).not.toBeInTheDocument();

    // Unfocused subtasks are visible in expanded accordion
    await user.click(screen.getByRole('button', { name: 'Unfocused Step Down Small' }));
    expect(screen.getAllByText('Unfocused Subtask 1')).toHaveLength(2);
    expect(screen.getAllByText('Unfocused Subtask 2')).toHaveLength(2);

    // Focused subtasks are visible twice in expanded accordion and once in overview
    expect(screen.getAllByText('Focused Subtask 1')).toHaveLength(3);
    expect(screen.getAllByText('Focused Subtask 2')).toHaveLength(3);
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
            displayName: 'Hidden Task',
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

    // All body schema labels should be present for focused task
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

    // Expanding unfocused task should show body content
    await user.click(screen.getByRole('button', { name: 'Hidden Task Down Small' }));
    expect(screen.getAllByText('Input Parameters')).toHaveLength(2);
  });

  describe('scroll navigation integration', () => {
    const mockScrollTo = vi.fn();

    beforeEach(() => {
      vi.clearAllMocks();
      Object.defineProperty(window, 'scrollTo', {
        value: mockScrollTo,
        writable: true,
      });
    });

    it('should scroll when clicking task name in overview', async () => {
      const user = userEvent.setup();
      const executionData = {
        status: {
          steps: [
            buildStep({
              displayName: 'Build Pipeline',
              state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
            }),
          ],
        },
      };

      render(<Execution schema={buildSchema()} data={executionData} />);

      // Click task name in overview
      await user.click(screen.getAllByText('Build Pipeline')[0]);

      expect(mockScrollTo).toHaveBeenCalledWith({
        top: expect.any(Number) as number,
        behavior: 'smooth',
      });
    });

    it('should navigate to subtasks', async () => {
      const user = userEvent.setup();
      const executionData = {
        status: {
          steps: [
            buildStep({
              displayName: 'Execute Workflow',
              state: 'PIPELINE_RUN_STEP_STATE_RUNNING',
              subSteps: [
                buildStep({
                  displayName: 'feature_prep',
                  state: 'PIPELINE_RUN_STEP_STATE_SUCCEEDED',
                }),
              ],
            }),
          ],
        },
      };

      render(<Execution schema={buildSchema()} data={executionData} />);

      // Click subtask name in overview
      await user.click(screen.getAllByText('feature_prep')[0]);

      expect(mockScrollTo).toHaveBeenCalledWith({
        top: expect.any(Number) as number,
        behavior: 'smooth',
      });
    });
  });
});
