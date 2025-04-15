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
 */
export enum Phase {
  /** Initial project setup and configuration phase */
  Project = 'project',
  /** Assistants builder phase*/
  Assistants = 'assistants',

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
}
