import type { LayoutItem } from '#core/components/form/types/config-types';

type BaseConditionLayoutConfig = {
  type: 'condition';
  when: string;
  items: LayoutItem[];
};

/** Renders `items` only when the `when` field's value strictly equals `is`. */
type LiteralCondition = BaseConditionLayoutConfig & { is: unknown };

/**
 * Renders `items` when the `when` field's value differs from `isNot`.
 *
 * An empty value is treated as "not yet determined" and is also hidden, even
 * though it technically differs from `isNot` — this avoids flashing content
 * before the user has made a choice.
 */
type NegationCondition = BaseConditionLayoutConfig & { isNot: unknown };

/** Renders `items` based on whether the `when` field's value is empty (null/undefined/''/[]). */
type EmptyCondition = BaseConditionLayoutConfig & { isEmpty: boolean };

/**
 * Renders `items` when the `when` field's value overlaps with `containsAny`.
 *
 * If the field value is an array, membership is checked against each of its
 * elements; otherwise the scalar value itself is checked for membership.
 */
type ContainsAnyCondition = BaseConditionLayoutConfig & { containsAny: unknown[] };

export type ConditionLayoutConfig =
  | LiteralCondition
  | NegationCondition
  | EmptyCondition
  | ContainsAnyCondition;
