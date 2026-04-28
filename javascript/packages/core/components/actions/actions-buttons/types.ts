import type { ActionConfig, Data } from '#core/components/actions/types';

export type PartitionedActions<T extends Data> = {
  primary: ActionConfig<T> | undefined;
  secondary: ActionConfig<T>[];
  tertiary: ActionConfig<T>[];
};
