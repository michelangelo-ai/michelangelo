import React, { useEffect } from 'react';
import Layout from '@theme/Layout';
import GradientBackground from '../components/Landing/GradientBackground';
import Hero from '../components/Landing/Hero';
import Stats from '../components/Landing/Stats';
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
      title="The ML Platform Behind Uber's AI"
      description="Open source and production-ready."
    >
      <main className={styles.landing}>
        <GradientBackground />
        <Hero />
        <Stats />
        <Features />
        <CodeExample />
      </main>
    </Layout>
  );
}
