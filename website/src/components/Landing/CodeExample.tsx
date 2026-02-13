import React, {useState, useCallback, type KeyboardEvent} from 'react';
import {Highlight, themes} from 'prism-react-renderer';
import styles from '../../css/landing.module.css';

type TabKey = 'python' | 'cli' | 'yaml';
const tabKeys: TabKey[] = ['python', 'cli', 'yaml'];

interface Tab {
  key: TabKey;
  label: string;
  language: string;
  code: string;
}

const tabs: Tab[] = [
  {
    key: 'python',
    label: 'Python',
    language: 'python',
    code: `import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask

@uniflow.task(config=RayTask(head_cpu=2, worker_instances=2))
def train(train_data, validation_data, params: dict):
    """Distributed training with Ray."""
    trainer = XGBoostTrainer(
        params=params,
        datasets={"train": train_data, "validation": validation_data},
    )
    return trainer.fit()

@uniflow.workflow()
def train_workflow(dataset: str):
    """End-to-end ML training pipeline."""
    train_data, val_data = load_and_split(dataset)
    result = train(train_data, val_data, params={"max_depth": 5})
    return result

if __name__ == "__main__":
    ctx = uniflow.create_context()
    ctx.run(train_workflow, dataset="s3://data/training.parquet")`,
  },
  {
    key: 'cli',
    label: 'CLI',
    language: 'bash',
    code: `# Create a local sandbox cluster
$ ma sandbox create

# Create your ML project
$ ma pipeline create --file=project.yaml

# Deploy a training pipeline
$ ma pipeline create --file=pipeline.yaml

# List all pipelines
$ ma pipeline list

# Trigger a pipeline run
$ ma pipeline_run create --file=run.yaml

# View pipeline status
$ ma pipeline_run list`,
  },
  {
    key: 'yaml',
    label: 'YAML',
    language: 'yaml',
    code: `apiVersion: michelangelo.api/v2
kind: Pipeline
metadata:
  name: fraud-detection-training
  namespace: ml-team
spec:
  type: PIPELINE_TYPE_TRAIN
  description: Fraud detection model training
  manifest:
    filePath: pipelines.fraud_detection.train_workflow
  commit:
    gitRef: main
    branch: main`,
  },
];

export default function CodeExample(): React.ReactElement {
  const [activeTab, setActiveTab] = useState<TabKey>('python');
  const currentTab = tabs.find((t) => t.key === activeTab) ?? tabs[0];

  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLButtonElement>, tabKey: TabKey) => {
      const currentIndex = tabKeys.indexOf(tabKey);
      let newIndex: number | null = null;

      switch (e.key) {
        case 'ArrowLeft':
          newIndex = currentIndex === 0 ? tabKeys.length - 1 : currentIndex - 1;
          break;
        case 'ArrowRight':
          newIndex = currentIndex === tabKeys.length - 1 ? 0 : currentIndex + 1;
          break;
        case 'Home':
          newIndex = 0;
          break;
        case 'End':
          newIndex = tabKeys.length - 1;
          break;
      }

      if (newIndex !== null) {
        e.preventDefault();
        const newTabKey = tabKeys[newIndex];
        setActiveTab(newTabKey);
        document.getElementById(`tab-${newTabKey}`)?.focus();
      }
    },
    []
  );

  return (
    <section className={styles.codeExample}>
      <div className={styles.codeExampleContainer}>
        <h2 className={styles.codeExampleTitle}>Simple, powerful API</h2>
        <p className={styles.codeExampleSubtitle}>
          From training to production in minutes, not months
        </p>

        <div className={styles.codeBlock}>
          <div className={styles.codeTabs} role="tablist" aria-label="Code examples">
            {tabs.map((tab) => (
              <button
                key={tab.key}
                role="tab"
                aria-selected={activeTab === tab.key}
                aria-controls={`panel-${tab.key}`}
                id={`tab-${tab.key}`}
                tabIndex={activeTab === tab.key ? 0 : -1}
                className={`${styles.codeTab} ${activeTab === tab.key ? styles.codeTabActive : ''}`}
                onClick={() => setActiveTab(tab.key)}
                onKeyDown={(e) => handleKeyDown(e, tab.key)}
              >
                {tab.label}
              </button>
            ))}
          </div>

          <div
            className={styles.codeContent}
            role="tabpanel"
            id={`panel-${currentTab.key}`}
            aria-labelledby={`tab-${currentTab.key}`}
          >
            <Highlight
              theme={themes.dracula}
              code={currentTab.code}
              language={currentTab.language}
            >
              {({className, style, tokens, getLineProps, getTokenProps}) => (
                <pre className={className} style={{...style, margin: 0}}>
                  {tokens.map((line, i) => (
                    <div key={i} {...getLineProps({line})}>
                      <span className={styles.lineNumber}>{i + 1}</span>
                      {line.map((token, key) => (
                        <span key={key} {...getTokenProps({token})} />
                      ))}
                    </div>
                  ))}
                </pre>
              )}
            </Highlight>
          </div>
        </div>
      </div>
    </section>
  );
}
