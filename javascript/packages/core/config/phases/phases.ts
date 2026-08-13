import { DATA_PHASE } from './data';
import { DEPLOY_PHASE } from './deploy';
import { EXPERIMENT_PRODUCTIONIZE_PHASE } from './experiment-productionize';
import { TRAIN_PHASE } from './train';

export const PHASES = {
  data: DATA_PHASE,
  train: TRAIN_PHASE,
  'experiment-productionize': EXPERIMENT_PRODUCTIONIZE_PHASE,
  deploy: DEPLOY_PHASE,
};
