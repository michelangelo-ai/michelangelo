import React, {useState} from 'react';
import {Highlight, themes} from 'prism-react-renderer';
import styles from '../../css/landing.module.css';

type TabKey = 'python' | 'cli' | 'yaml';

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
    code: `from michelangelo import Model, FeatureStore

# Connect to the feature store
fs = FeatureStore("production")

# Define and train a model
model = Model(
    name="fraud-detection",
    features=fs.get_features(["user_history", "transaction"]),
)
model.train(data=training_data, epochs=100)

# Deploy to production with one line
model.deploy(environment="production", canary_percent=10)

# Monitor in real-time
model.monitor(alert_on_drift=True)`,
  },
  {
    key: 'cli',
    label: 'CLI',
    language: 'bash',
    code: `# Initialize a new ML project
$ michelangelo init my-fraud-model

# Train with distributed computing
$ michelangelo train --config train.yaml --distributed

# Evaluate model performance
$ michelangelo evaluate --model fraud-detection --dataset test

# Deploy with canary release
$ michelangelo deploy fraud-detection:v2 \\
    --environment production \\
    --canary 10%

# Monitor model health
$ michelangelo monitor fraud-detection --watch`,
  },
  {
    key: 'yaml',
    label: 'YAML',
    language: 'yaml',
    code: `# michelangelo.yaml
name: fraud-detection
version: 2.0.0

features:
  source: feature-store/production
  entities:
    - user_history
    - transaction_patterns
    - device_fingerprint

training:
  framework: pytorch
  distributed: true
  resources:
    gpu: 4
    memory: 32Gi

deployment:
  environment: production
  strategy: canary
  canary_percent: 10
  auto_rollback: true`,
  },
];

export default function CodeExample(): React.ReactElement {
  const [activeTab, setActiveTab] = useState<TabKey>('python');
  const currentTab = tabs.find((t) => t.key === activeTab) ?? tabs[0];

  return (
    <section className={styles.codeExample}>
      <div className={styles.codeExampleContainer}>
        <h2 className={styles.codeExampleTitle}>Simple, powerful API</h2>
        <p className={styles.codeExampleSubtitle}>
          From training to production in minutes, not months
        </p>

        <div className={styles.codeBlock}>
          <div className={styles.codeTabs}>
            {tabs.map((tab) => (
              <button
                key={tab.key}
                className={`${styles.codeTab} ${activeTab === tab.key ? styles.codeTabActive : ''}`}
                onClick={() => setActiveTab(tab.key)}
              >
                {tab.label}
              </button>
            ))}
          </div>

          <div className={styles.codeContent}>
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
