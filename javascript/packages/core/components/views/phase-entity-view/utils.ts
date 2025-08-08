import type { PhaseEntityConfig } from '#core/types/common/studio-types';
import type { ListableEntity } from './types';

export function isListableEntity(
  entity: Pick<PhaseEntityConfig, 'state' | 'views'>
): entity is ListableEntity {
  return entity.state === 'active' && entity.views.some((view) => view.type === 'list');
}
