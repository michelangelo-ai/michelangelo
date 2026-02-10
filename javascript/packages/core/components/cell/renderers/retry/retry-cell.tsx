import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useStyletron } from 'baseui';
import { Button, KIND, SIZE } from 'baseui/button';
import { Textarea } from 'baseui/textarea';
import { create } from '@bufbuild/protobuf';
import { RetryInfoSchema } from '@michelangelo/rpc';

import { Dialog } from '#core/components/dialog/dialog';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioMutation } from '#core/hooks/use-studio-mutation';
import { useStudioQuery } from '#core/hooks/use-studio-query';

import type { CellRendererProps } from '#core/components/cell/types';
import type { PipelineRunData } from './types';

const TERMINATED_STATES = new Set([3, 4, 5, 6]);

export const RetryCell = (props: CellRendererProps<string>) => {
  const { value } = props;
  const [css, theme] = useStyletron();
  const [showRetryModal, setShowRetryModal] = useState(false);
  const [retryReason, setRetryReason] = useState('Manual retry from UI');
  const queryClient = useQueryClient();

  const { projectId, entityId } = useStudioParams('detail');

  const { data: pipelineRunData } = useStudioQuery<PipelineRunData>({
    queryName: 'GetPipelineRun',
    serviceOptions: {
      namespace: projectId,
      name: entityId,
    },
    clientOptions: {
      enabled: !!projectId && !!entityId,
    },
  });

  const updatePipelineRunMutation = useStudioMutation<
    { pipelineRun: Record<string, unknown> },
    { pipelineRun: Record<string, unknown> }
  >({ mutationName: 'UpdatePipelineRun' });

  const hasActivityId = !!value;
  const pipelineRunState = pipelineRunData?.pipelineRun?.status?.state;
  const isPipelineRunTerminated =
    pipelineRunState !== undefined && TERMINATED_STATES.has(pipelineRunState);

  if (!hasActivityId || !isPipelineRunTerminated) {
    return null;
  }

  const submitRetry = async () => {
    if (updatePipelineRunMutation.isPending || !pipelineRunData?.pipelineRun) {
      return;
    }

    const { pipelineRun } = pipelineRunData;
    const { workflowId, workflowRunId } = pipelineRun.status;

    if (!value || !workflowId || !workflowRunId) {
      return;
    }

    const updatedPipelineRun = {
      ...pipelineRun,
      spec: {
        ...pipelineRun.spec,
        retryInfo: create(RetryInfoSchema, {
          activityId: value,
          workflowId,
          // Must match status.workflowRunId to trigger backend retry processing
          workflowRunId,
          reason: retryReason,
        }),
      },
    };

    try {
      await updatePipelineRunMutation.mutateAsync({ pipelineRun: updatedPipelineRun });
      setShowRetryModal(false);
      setRetryReason('Manual retry from UI');

      await queryClient.invalidateQueries({
        queryKey: ['GetPipelineRun', { namespace: projectId, name: entityId }],
      });
    } catch {
      // Error is captured in updatePipelineRunMutation.error and displayed in the modal
    }
  };

  return (
    <>
      <Button
        size={SIZE.mini}
        kind={KIND.secondary}
        onClick={() => setShowRetryModal(true)}
        disabled={updatePipelineRunMutation.isPending}
      >
        Retry
      </Button>

      <Dialog
        isOpen={showRetryModal}
        onDismiss={() => setShowRetryModal(false)}
        heading="Retry Task"
        buttonDock={{
          primaryAction: (
            <Button
              kind={KIND.primary}
              onClick={submitRetry}
              isLoading={updatePipelineRunMutation.isPending}
            >
              Retry Task
            </Button>
          ),
          dismissiveAction: (
            <Button
              kind={KIND.tertiary}
              onClick={() => setShowRetryModal(false)}
              disabled={updatePipelineRunMutation.isPending}
            >
              Cancel
            </Button>
          ),
        }}
      >
        {updatePipelineRunMutation.error && (
          <div
            className={css({
              color: theme.colors.negative,
              marginBottom: theme.sizing.scale600,
              ...theme.typography.ParagraphSmall,
            })}
          >
            {updatePipelineRunMutation.error.message}
          </div>
        )}
        <div className={css({ marginBottom: theme.sizing.scale600 })}>
          Are you sure you want to retry this task?
        </div>
        <div className={css({ marginBottom: theme.sizing.scale400 })}>
          <label className={css({ ...theme.typography.LabelMedium })}>Retry Reason:</label>
        </div>
        <Textarea
          value={retryReason}
          onChange={(e) => setRetryReason(e.target.value)}
          placeholder="Enter reason for retry..."
          overrides={{
            Input: {
              style: {
                resize: 'vertical',
                minHeight: '80px',
              },
            },
          }}
        />
      </Dialog>
    </>
  );
};
