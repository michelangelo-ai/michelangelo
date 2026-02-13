import React, { useEffect } from 'react';
import Layout from '@theme/Layout';
import GradientBackground from '../components/Landing/GradientBackground';
import Hero from '../components/Landing/Hero';
import Features from '../components/Landing/Features';
import CodeExample from '../components/Landing/CodeExample';
import styles from '../css/landing.module.css';

export default function Home(): React.ReactElement {
  useEffect(() => {
    document.documentElement.setAttribute('data-landing-page', 'true');
    return () => {
      document.documentElement.removeAttribute('data-landing-page');
    };
  }, []);

  return (
    <Layout
      title="ML at Scale. Open Source."
      description="The end-to-end platform for building, deploying, and monitoring ML models"
    >
      <main className={styles.landing}>
        <GradientBackground />
        <Hero />
        <Features />
        <CodeExample />
      </main>
    </Layout>
  );
}
