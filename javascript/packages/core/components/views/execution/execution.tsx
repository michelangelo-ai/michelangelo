import { useState } from 'react';
import { useStyletron } from 'baseui';
import { Segment, SegmentedControl } from 'baseui/segmented-control';

import { Box } from '#core/components/box/box';
import { CircleExclamationMark } from '#core/components/illustrations/circle-exclamation-mark/circle-exclamation-mark';
import { CircleExclamationMarkKind } from '#core/components/illustrations/circle-exclamation-mark/types';
import { Signpost } from '#core/components/signpost/signpost';
import { TaskDetails } from './components/task-details/task-details';
import { TaskFlow } from './components/task-flow';
import { TaskStateIcon } from './components/task-state-icon';
import { TaskTimetable } from './components/task-timetable/task-timetable';
import { taskListHasTiming } from './components/task-timetable/utils/build-timetable';
import { EXECUTION_VIEW, EXECUTION_VIEW_OPTIONS } from './constants';
import { OverviewHeader, TaskContentStack } from './styled-components';
import { buildTaskList } from './utils/build-task-list';
import { buildTaskMatrix } from './utils/build-task-matrix';
import { determineExecutionState } from './utils/determine-execution-state';
import { handleScrollToTask } from './utils/scroll-to-task';

import type { ExecutionDetailViewSchema, ExecutionOverrides, ExecutionView } from './types';

export function Execution<
  TData extends object = object,
  TTaskRecord extends object = object,
>(props: {
  schema: ExecutionDetailViewSchema<TData, TTaskRecord>;
  data: TData;
  overrides?: ExecutionOverrides<TTaskRecord>;
}) {
  const { schema, data, overrides } = props;
  const [css, theme] = useStyletron();
  const [view, setView] = useState<ExecutionView>(EXECUTION_VIEW.FLOW);
  const taskList = overrides?.taskList ?? buildTaskList(schema, data);

  if (!taskList.length) {
    return (
      <Signpost
        illustration={
          <CircleExclamationMark
            height="64px"
            width="64px"
            kind={CircleExclamationMarkKind.PRIMARY}
          />
        }
        title={schema.emptyState.title}
        description={schema.emptyState.description}
      />
    );
  }

  const matrix = buildTaskMatrix(taskList);
  // Timetable needs at least one timestamp; hide the option when none exist.
  const viewOptions: { view: ExecutionView; label: string }[] = taskListHasTiming(taskList)
    ? [...EXECUTION_VIEW_OPTIONS]
    : [{ view: EXECUTION_VIEW.FLOW, label: 'Flow' }];
  const activeView = viewOptions.some((option) => option.view === view)
    ? view
    : EXECUTION_VIEW.FLOW;

  return (
    <TaskContentStack>
      <Box
        title={
          <OverviewHeader>
            <div
              className={css({ display: 'flex', alignItems: 'center', gap: theme.sizing.scale500 })}
            >
              <TaskStateIcon state={determineExecutionState(taskList)} />
              Overview
            </div>
            {viewOptions.length > 1 ? (
              <SegmentedControl
                activeKey={activeView}
                onChange={({ activeKey }) => {
                  // cast: segment keys are always EXECUTION_VIEW values, see EXECUTION_VIEW_OPTIONS
                  setView(activeKey as ExecutionView);
                }}
              >
                {viewOptions.map((option) => (
                  <Segment key={option.view} label={option.label} />
                ))}
              </SegmentedControl>
            ) : null}
          </OverviewHeader>
        }
      >
        <TaskContentStack>
          {activeView === EXECUTION_VIEW.FLOW ? (
            <TaskFlow
              matrix={matrix}
              onTaskClick={(clickedTask) => {
                handleScrollToTask(clickedTask);
              }}
              overrides={{
                TaskListRenderer: overrides?.TaskListRenderer,
                SubTaskListRenderer: overrides?.SubTaskListRenderer,
              }}
            />
          ) : (
            <TaskTimetable taskList={taskList} />
          )}
        </TaskContentStack>
      </Box>

      <TaskContentStack>
        {taskList.map((task, index) => (
          <TaskDetails
            key={index}
            task={task}
            metadata={schema.tasks.header.metadata}
            bodySchema={schema.tasks.body}
            overrides={overrides}
          />
        ))}
      </TaskContentStack>
    </TaskContentStack>
  );
}
