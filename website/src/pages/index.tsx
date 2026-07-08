import React from 'react';
import Layout from '@theme/Layout';
import Hero from '../components/Landing/Hero';
import Stats from '../components/Landing/Stats';
import Features from '../components/Landing/Features';
import CodeExample from '../components/Landing/CodeExample';
import styles from '../css/landing.module.css';

export default function Home(): React.ReactElement {
  return (
    <Layout
      title="The ML Platform Behind Uber's AI"
      description="Open source and production-ready."
    >
      <main className={styles.landing}>
        <Hero />
        <Stats />
        <Features />
        <CodeExample />
      </main>
    </Layout>
  );
}
