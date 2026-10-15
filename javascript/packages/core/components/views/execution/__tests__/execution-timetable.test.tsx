import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { vi } from 'vitest';

import { buildWrapper } from '#core/test/wrappers/build-wrapper';
import { getRouterWrapper } from '#core/test/wrappers/get-router-wrapper';
import {
  buildExecutionSchemaFactory,
  buildTaskStepFactory,
} from '../__fixtures__/execution-schema-factory';
import { Execution } from '../execution';

const EM_DASH = '—';

describe('Execution - timetable view', () => {
  const buildSchema = buildExecutionSchemaFactory();
  const buildStep = buildTaskStepFactory();

  let executionData: Record<string, unknown>;

  beforeEach(() => {
    executionData = {
      status: {
        steps: [
          buildStep({
            displayName: 'Image Build',
            startTime: '1670007850',
            endTime: '1670008414',
          }),
          buildStep({ displayName: 'Compile Workflow', state: 'PIPELINE_RUN_STEP_STATE_PENDING' }),
        ],
      },
    };
  });

  function findTimetableRow(taskName: string): HTMLElement {
    const row = screen
      .getAllByRole('button')
      .find((element) => element.textContent?.includes(taskName));

    return row!;
  }

  it('should show the flow view until the timetable is selected', async () => {
    const user = userEvent.setup();
    render(
      <Execution schema={buildSchema()} data={executionData} />,
      buildWrapper([getRouterWrapper()])
    );

    expect(screen.getByRole('option', { name: 'Flow' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.queryByText('Duration')).not.toBeInTheDocument();

    await user.click(screen.getByRole('option', { name: 'Timetable' }));

    expect(screen.getByRole('option', { name: 'Timetable' })).toHaveAttribute(
      'aria-selected',
      'true'
    );
    expect(screen.getByText('Duration')).toBeInTheDocument();
  });

  it('should show how long each task took', async () => {
    const user = userEvent.setup();
    render(
      <Execution schema={buildSchema()} data={executionData} />,
      buildWrapper([getRouterWrapper()])
    );

    await user.click(screen.getByRole('option', { name: 'Timetable' }));

    expect(within(findTimetableRow('Image Build')).getByText('9m 24s')).toBeInTheDocument();
  });

  it('should mark tasks that never ran as having no duration', async () => {
    const user = userEvent.setup();
    render(
      <Execution schema={buildSchema()} data={executionData} />,
      buildWrapper([getRouterWrapper()])
    );

    await user.click(screen.getByRole('option', { name: 'Timetable' }));

    expect(within(findTimetableRow('Compile Workflow')).getByText(EM_DASH)).toBeInTheDocument();
  });

  it('should scroll to the matching task section when a row is clicked', async () => {
    const user = userEvent.setup();
    window.scrollTo = vi.fn();
    render(
      <Execution schema={buildSchema()} data={executionData} />,
      buildWrapper([getRouterWrapper()])
    );

    await user.click(screen.getByRole('option', { name: 'Timetable' }));
    await user.click(findTimetableRow('Image Build'));

    expect(window.scrollTo).toHaveBeenCalled();
  });

  it('should return to the flow view when Flow is selected again', async () => {
    const user = userEvent.setup();
    render(
      <Execution schema={buildSchema()} data={executionData} />,
      buildWrapper([getRouterWrapper()])
    );

    await user.click(screen.getByRole('option', { name: 'Timetable' }));
    await user.click(screen.getByRole('option', { name: 'Flow' }));

    expect(screen.getByRole('option', { name: 'Flow' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.queryByText('Duration')).not.toBeInTheDocument();
  });

  it('should explain when no task carries timing information', async () => {
    const user = userEvent.setup();
    render(
      <Execution
        schema={buildSchema()}
        data={{ status: { steps: [buildStep({ displayName: 'Untimed' })] } }}
      />,
      buildWrapper([getRouterWrapper()])
    );

    await user.click(screen.getByRole('option', { name: 'Timetable' }));

    expect(
      screen.getByText('No timing information is available for these tasks.')
    ).toBeInTheDocument();
  });
});
