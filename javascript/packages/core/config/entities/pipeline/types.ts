import type { PipelineRun } from '#core/config/entities/run/types';
import type { ManifestTrigger } from '#core/config/entities/trigger/types';

export interface Pipeline {
  metadata: {
    name: string;
    namespace: string;
  };
  spec: {
    owner: {
      name: string;
    };
    /** Optional because a pipeline can be registered without a manifest. */
    manifest?: {
      /** Named triggers declared for this pipeline, keyed by trigger name. */
      triggerMap?: Record<string, ManifestTrigger>;
    };
  };
}

/**
 * Form-only shape submitted by {@link CreatePipelineRunForm}.
 *
 * The notification fields here have no proto counterpart — `notifyOnCompletion` is a UI-only
 * toggle, and the email/Slack lists are the raw tag values from a `StringField multi`. All three
 * are read out of the form values and discarded; `handleRunSubmit` builds the real
 * `PipelineRunNotification[]` from them instead of forwarding them as-is.
 */
export type PipelineRunFormValues = PipelineRun & {
  notifyOnCompletion?: boolean;
  notificationEmails?: string[];
  notificationSlackDestinations?: string[];
};

/**
 * Values held by {@link RunTriggerForm}.
 *
 * `isBackfill`, `startTimestamp`, `endTimestamp`, `selectedParams`, and
 * `maxConcurrencyOverride` only matter once `isBackfill` is set — the backfill fields are
 * hidden otherwise and never reach the submit handler with a meaningful value.
 *
 * Declared as a type alias rather than an interface so it satisfies the `FormData`
 * (`Record<string, unknown>`) constraint on `FormDialog`.
 */
export type RunTriggerFormValues = {
  sourceTriggerName: string;
  autoFlip?: boolean;
  isBackfill?: boolean;
  startTimestamp?: string;
  endTimestamp?: string;
  selectedParams?: string[];
  maxConcurrencyOverride?: number;
};
