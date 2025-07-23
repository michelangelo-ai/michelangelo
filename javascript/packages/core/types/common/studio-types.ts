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
