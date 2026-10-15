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
import { EXECUTION_VIEW, EXECUTION_VIEW_OPTIONS } from './constants';
import { TaskContentStack } from './styled-components';
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

  return (
    <TaskContentStack>
      <Box
        title={
          <div
            className={css({
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'space-between',
              gap: theme.sizing.scale500,
              width: '100%',
            })}
          >
            <div
              className={css({ display: 'flex', alignItems: 'center', gap: theme.sizing.scale500 })}
            >
              <TaskStateIcon state={determineExecutionState(taskList)} />
              Overview
            </div>
            <SegmentedControl
              activeKey={view}
              onChange={({ activeKey }) => {
                // cast: segment keys are always EXECUTION_VIEW values, see EXECUTION_VIEW_OPTIONS
                setView(activeKey as ExecutionView);
              }}
            >
              {EXECUTION_VIEW_OPTIONS.map((option) => (
                <Segment key={option.view} label={option.label} />
              ))}
            </SegmentedControl>
          </div>
        }
      >
        <TaskContentStack>
          {view === EXECUTION_VIEW.FLOW ? (
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
            <TaskTimetable
              taskList={taskList}
              onTaskClick={(clickedTask, ancestors) => {
                handleScrollToTask(clickedTask, ancestors);
              }}
            />
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
