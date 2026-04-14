import type { ActionConfigSchema } from '#core/components/actions/types';
import type { ViewConfig } from '#core/components/views/types';
import type { QueryConfig } from '#core/types/query-types';

/**
 * Represents the different phases in the Michelangelo Studio workflow.
 * Each phase corresponds to a specific stage in the machine learning lifecycle.
 *
 * These phases serve two important purposes:
 * 1. They are used in URLs to navigate between different sections of the application
 * 2. They define the initial grouping of the application's schema and data structure
 *
 * The string values of these enums are used directly in URLs (e.g., /monitor, /train),
 * so they should be kept URL-friendly and consistent.
 *
 * The phases are naturally grouped into three categories:
 * - Phases that are distinct from any workflow, e.g., Project and Assistants
 *
 * - Traditional ML workflow: Data → Train → Retrain → Deploy → Monitor
 *
 * - Generative AI workflow: LLM → Data → Prompt → Finetune → Monitor
 *
 * - Agent workflow: Data → Develop → Deploy → Monitor
 */
export enum Phase {
  /** Initial project setup and configuration phase */
  Project = 'project',
  /** Assistants builder phase*/
  Assistants = 'assistants',
  /** Agents builder phase */
  Agents = 'agents',

  /** Data preparation and preprocessing phase */
  Data = 'data',
  /** Initial model training phase */
  Train = 'train',
  /** Model retraining and fine-tuning phase */
  Retrain = 'retrain',
  /** Model deployment and serving phase */
  Deploy = 'deploy',
  /** Model monitoring and performance tracking phase */
  Monitor = 'monitor',

  /** Large Language Model (LLM) configuration and management */
  GenaiLLM = 'genai-llm',
  /** Data preparation for generative AI models */
  GenaiData = 'genai-data',
  /** Prompt engineering and management */
  GenaiPrompt = 'genai-prompt',
  /** Fine-tuning of generative AI models */
  GenaiFinetune = 'genai-finetune',
  /** Monitoring of generative AI model performance */
  GenaiMonitor = 'genai-monitor',

  /** Agent data preparation and preprocessing phase */
  AgentData = 'agent-data',
  /** Agent development and training phase */
  AgentDevelop = 'agent-develop',
  /** Agent deployment and serving phase */
  AgentDeploy = 'agent-deploy',
  /** Agent monitoring and performance tracking phase */
  AgentMonitor = 'agent-monitor',
}

export interface PhaseEntityConfig<T extends object = object> {
  /**
   * Name of the entity as it appears within MA Studio. Should be plural, lower case
   * version of the name.
   *
   * @example
   * trained models
   * pipelines
   * feature consistency (intentionally not pluralized since this entity is never referred
   *  to as "feature consistencies")
   */
  name: string;
  /**
   * Unique ID for entity within its phase, used in URL, should be plural form with
   * no whitespace
   *
   * @example
   * models
   * pipelines
   * feature-consistency
   */
  id: string;
  /**
   * @description
   * Name of underlying service that the entity is primarily tied to. Should be an
   * exact match to the service's root protobuf field name.
   *
   * This field is used to access queries defined within RPC handlers.
   *
   * @example
   * For PipelineService, root protobuf field name is pipeline. This
   * field should be pipeline. Query will be ListPipeline.
   *
   * For PipelineRunService, root protobuf field name is pipelineRun. This field
   * should be pipelineRun. Query will be ListPipelineRun.
   */
  service: QueryConfig['service'];
  /** State controlling whether this entity is interactive */
  state: PhaseEntityState;
  /** List of view configurations for this entity */
  views: ViewConfig<T>[];
  /**
   * Optional actions to render for this entity.
   * Rendered in table rows for list views.
   */
  actions?: ActionConfigSchema<T>[];
}

/**
 * Simplified phase configuration matching the original studio config structure
 */
export interface PhaseConfig {
  /** Unique ID for the phase, used in URL routing */
  id: string;
  /** Icon name from the application's icon provider system */
  icon: string;
  /**
   * Display name for the phase
   *
   * @example
   * "Prepare & Analyze Data"
   * "Train & Evaluate"
   */
  name: string;
  /** Optional descriptive text explaining what this phase does */
  description?: string;
  /** Optional URL to external documentation for this phase */
  docUrl?: string;
  /** State controlling overall phase behavior and appearance */
  state: PhaseState;
  /** List of entities (like pipelines, models) that belong to this phase */
  entities: PhaseEntityConfig[];
}

/**
 * Groups phases into a logical category for display and filtering purposes.
 *
 * @example
 * ```ts
 * const CATEGORIES: CategoryConfig[] = [
 *   { id: 'core-ml', name: 'Core ML', phases: [DATA_PHASE, TRAIN_PHASE] },
 *   { id: 'gen-ai', name: 'Gen AI', phases: [GENAI_LLM_PHASE, GENAI_DATA_PHASE] },
 * ];
 * ```
 */
export interface CategoryConfig {
  id: string;
  name: string;
  phases: PhaseConfig[];
}

/**
 * Phase state controlling overall phase behavior and appearance
 *
 * @example
 * `active` - The phase is fully functional and can be interacted with
 * `comingSoon` - The phase is not yet available but will be in the future
 * `disabled` - The phase is not available and cannot be interacted with
 */
export type PhaseState = 'active' | 'comingSoon' | 'disabled';

/**
 * Entity state controlling individual entity behavior within a phase
 *
 * @example
 * `active` - The entity is fully functional and can be interacted with
 * `disabled` - The entity is not available and cannot be interacted with
 */
export type PhaseEntityState = 'active' | 'disabled';

/**
 * @description
 * Defines a way to access a specific property or value from an object.
 * This can be either a string representing a dot-notation path, or a function
 * that directly extracts the value.
 *
 * @remarks
 * When `Accessor` is a string, it represents a path using dot notation (e.g., `'name'`, `'address.street'`)
 * and can include array indexing (e.g., `'users[0].name'`). A utility function is typically used to
 * interpret this string path against an object.
 *
 * @example
 * ```ts
 * const accessor: Accessor = 'name';
 * accessor({ name: 'John' }); // 'John'
 *
 * const accessor: Accessor = 'users[0].name';
 * accessor({ users: [{ name: 'John' }] }); // 'John'
 *
 * const accessor: Accessor<string> = (object) => object.name;
 * accessor({ name: 'John' }); // 'John'
 * ```
 */
export type Accessor<K = unknown> = AccessorFn<K> | string;

export type AccessorFn<T = unknown> = (object: unknown) => T | undefined;
