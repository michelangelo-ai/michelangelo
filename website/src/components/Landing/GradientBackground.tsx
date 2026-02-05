import React, {useEffect, useState} from 'react';
import styles from '../../css/landing.module.css';

export default function GradientBackground(): React.ReactElement {
  const [mousePosition, setMousePosition] = useState({x: 0, y: 0});

  useEffect(() => {
    const prefersReducedMotion = window.matchMedia(
      '(prefers-reduced-motion: reduce)',
    ).matches;

    if (prefersReducedMotion) {
      return;
    }

    const handleMouseMove = (e: MouseEvent) => {
      // Normalize to -1 to 1 range centered on viewport
      const x = (e.clientX / window.innerWidth - 0.5) * 2;
      const y = (e.clientY / window.innerHeight - 0.5) * 2;
      setMousePosition({x, y});
    };

    window.addEventListener('mousemove', handleMouseMove);
    return () => window.removeEventListener('mousemove', handleMouseMove);
  }, []);

  // Each blob moves at different speeds for parallax effect
  const blob1Style = {
    transform: `translate(${mousePosition.x * 100}px, ${mousePosition.y * 80}px)`,
  };
  const blob2Style = {
    transform: `translate(${mousePosition.x * -80}px, ${mousePosition.y * 100}px)`,
  };
  const blob3Style = {
    transform: `translate(${mousePosition.x * 60}px, ${mousePosition.y * -70}px)`,
  };

  return (
    <div className={styles.gradientBackground} aria-hidden="true">
      <div className={styles.gradientBlob1} style={blob1Style} />
      <div className={styles.gradientBlob2} style={blob2Style} />
      <div className={styles.gradientBlob3} style={blob3Style} />
    </div>
  );
}
