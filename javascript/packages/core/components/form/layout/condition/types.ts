import type { LayoutItem } from '#core/components/form/types/config-types';

type BaseConditionLayoutConfig = {
  type: 'condition';
  when: string;
  items: LayoutItem[];
};

type LiteralCondition = BaseConditionLayoutConfig & { is: unknown };
type NegationCondition = BaseConditionLayoutConfig & { isNot: unknown };
type EmptyCondition = BaseConditionLayoutConfig & { isEmpty: boolean };
type ContainsAnyCondition = BaseConditionLayoutConfig & { containsAny: unknown[] };

export type ConditionLayoutConfig =
  | LiteralCondition
  | NegationCondition
  | EmptyCondition
  | ContainsAnyCondition;
