import type { ManifestTrigger } from '#core/config/entities/trigger/types';

const INTERVAL_UNITS = [
  { unit: 'day', seconds: 86400 },
  { unit: 'hour', seconds: 3600 },
  { unit: 'minute', seconds: 60 },
] as const;

/**
 * Human-readable summary of a manifest trigger's schedule, used to tell one trigger from
 * another in the scheduling dropdown.
 *
 * Returns an empty string when the trigger carries no schedule the reconciler recognizes,
 * so callers can fall back to showing the trigger name on its own.
 */
export const formatTriggerSchedule = (trigger: ManifestTrigger | undefined): string => {
  const triggerType = trigger?.triggerType;

  switch (triggerType?.case) {
    case 'cronSchedule':
      return triggerType.value.cron ? `cron ${triggerType.value.cron}` : '';
    case 'intervalSchedule':
      return formatInterval(triggerType.value.interval?.seconds);
    case 'batchRerun':
      return 'batch rerun';
    default:
      return '';
  }
};

/**
 * Durations reach the client as a protobuf-es `Duration`, whose `seconds` is a bigint —
 * except in tests and hand-built fixtures, where it is whatever the fixture wrote.
 */
function formatInterval(seconds: bigint | number | string | undefined): string {
  const total = Number(seconds ?? 0);
  if (!Number.isFinite(total) || total <= 0) return '';

  for (const { unit, seconds: unitSeconds } of INTERVAL_UNITS) {
    if (total % unitSeconds === 0) {
      const count = total / unitSeconds;
      return count === 1 ? `every ${unit}` : `every ${count} ${unit}s`;
    }
  }

  return `every ${total} seconds`;
}
