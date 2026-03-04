import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useStyletron } from 'baseui';
import { Button, KIND, SIZE } from 'baseui/button';
import { Textarea } from 'baseui/textarea';

import { Dialog } from '#core/components/dialog/dialog';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioMutation } from '#core/hooks/use-studio-mutation';
import { useStudioQuery } from '#core/hooks/use-studio-query';

import type { CellRendererProps } from '#core/components/cell/types';
import type { PipelineRunData } from './types';

const TERMINATED_STATES = new Set([3, 4, 5, 6]);

export const RetryCell = (props: CellRendererProps<string>) => {
  const { value: originalValue } = props;

  // 🧪 SIMULATE K8S ENVIRONMENT: Simulate race condition where button shows but mutation fails
  // This simulates: button renders with activity_id, but becomes undefined during mutation
  const SIMULATE_K8S_RACE_CONDITION = true; // Set to true to reproduce k8s issue
  const SIMULATE_UNDEFINED_DURING_MUTATION = true;

  // Show button (don't force undefined here)
  const value = originalValue;

  // Log simulation status for debugging
  if (SIMULATE_K8S_RACE_CONDITION && originalValue) {
    console.log('🧪 K8S RACE CONDITION SIMULATION: Button shows with activity_id:', originalValue, 'but mutation will use undefined');
  }
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
    // 🧪 Simulate value becoming undefined during mutation (race condition)
    const mutationValue = SIMULATE_UNDEFINED_DURING_MUTATION ? undefined : value;

    console.log('🔍 DEBUG: submitRetry called with:', {
      originalValue,
      renderValue: value,
      mutationValue: mutationValue,
      isSimulatingRaceCondition: SIMULATE_K8S_RACE_CONDITION,
      valueType: typeof mutationValue,
      hasValue: !!mutationValue,
      isPending: updatePipelineRunMutation.isPending,
      hasPipelineRun: !!pipelineRunData?.pipelineRun
    });

    if (updatePipelineRunMutation.isPending || !pipelineRunData?.pipelineRun) {
      console.log('🔍 DEBUG: Early return - pending or no pipeline run');
      return;
    }

    const { pipelineRun } = pipelineRunData;
    const { workflowId, workflowRunId } = pipelineRun.status;

    console.log('🔍 DEBUG: Extracted values:', {
      mutationValue,
      workflowId,
      workflowRunId,
      status: pipelineRun.status
    });

    if (!mutationValue || !workflowId || !workflowRunId) {
      console.log('🔍 DEBUG: Missing required fields, aborting:', {
        hasValue: !!mutationValue,
        hasWorkflowId: !!workflowId,
        hasWorkflowRunId: !!workflowRunId
      });
      return;
    }

    // Strip protobuf internals so toBinary() re-normalizes nested messages
    const { $typeName: _, $unknown: __, ...specFields } = pipelineRun.spec;

    const updatedPipelineRun = {
      metadata: pipelineRun.metadata,
      spec: {
        ...specFields,
        retryInfo: {
          activityId: mutationValue,
          workflowId,
          // Must match status.workflowRunId to trigger backend retry processing
          workflowRunId,
          reason: retryReason,
        },
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
