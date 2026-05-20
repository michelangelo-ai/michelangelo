import React from 'react';
import styles from '../../css/landing.module.css';

export default function GradientBackground(): React.ReactElement {
  return (
    <div className={styles.gradientBackground} aria-hidden="true">
      <div className={styles.gradientBlob1} />
    </div>
  );
}
