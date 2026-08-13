import type { LayoutItem } from '#core/components/form/types/config-types';

type BaseConditionLayoutConfig = {
  type: 'condition';
  when: string;
  items: LayoutItem[];
};

/** Renders `items` only when the `when` field's value strictly equals `is`. */
type LiteralCondition = BaseConditionLayoutConfig & { is: unknown };

export type ConditionLayoutConfig = LiteralCondition;
