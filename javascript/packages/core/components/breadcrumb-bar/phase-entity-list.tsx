import { useLocation, useNavigate } from 'react-router-dom-v5-compat';
import { useStyletron } from 'baseui';

import { capitalizeFirstLetter } from '#core/utils/string-utils';
import { EntityItem } from './styled-components';

import type { PhaseConfig } from '#core/types/common/studio-types';

export function PhaseEntityList({
  phase,
  projectId,
  onSelect,
}: {
  phase: PhaseConfig;
  projectId: string;
  onSelect: () => void;
}) {
  const [css, theme] = useStyletron();
  const navigate = useNavigate();
  const { pathname } = useLocation();

  const pathParts = pathname.split('/').filter(Boolean);
  const currentPhase = pathParts[1];
  const currentEntity = pathParts[2];

  return (
    <ul className={css({ listStyleType: 'none', padding: 0, margin: 0 })}>
      {phase.entities.map((entity) => {
        const isDisabled = entity.state !== 'active';
        const isSelected = !isDisabled && currentPhase === phase.id && currentEntity === entity.id;

        return (
          <EntityItem
            key={entity.id}
            $disabled={isDisabled}
            onClick={() => {
              if (isDisabled) return;
              onSelect();
              navigate(`/${projectId}/${phase.id}/${entity.id}`);
            }}
          >
            <span
              className={css({
                ...theme.typography.ParagraphSmall,
                fontWeight: isSelected ? 'bold' : undefined,
                color: isDisabled ? theme.colors.contentTertiary : undefined,
              })}
            >
              {capitalizeFirstLetter(entity.name)}
            </span>
          </EntityItem>
        );
      })}
    </ul>
  );
}
