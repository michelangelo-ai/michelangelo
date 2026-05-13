import React from 'react';
import styles from '../../css/landing.module.css';

const stats = [
  {value: '400+', label: 'Active ML projects'},
  {value: '20K+', label: 'Model training jobs / month'},
  {value: '5K+', label: 'Models in production'},
  {value: '45M+', label: 'Real-time predictions / sec (peak)'},
];

export default function Stats(): React.ReactElement {
  return (
    <section className={styles.statsBar}>
      {stats.map(({value, label}) => (
        <div key={label} className={styles.statItem}>
          <span className={styles.statValue}>{value}</span>
          <span className={styles.statLabel}>{label}</span>
        </div>
      ))}
    </section>
  );
}
