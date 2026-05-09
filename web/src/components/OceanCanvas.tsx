import { useEffect, useRef, useState } from 'react';
import type { OceanEngine } from '../ocean/OceanEngine';
import { useOceanState } from '../hooks/useOceanState';

export function OceanCanvas() {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const engineRef = useRef<OceanEngine | null>(null);
  const timeOfDay = useOceanState((s) => s.timeOfDay);
  const waveIntensity = useOceanState((s) => s.waveIntensity);
  const [animationEnabled, setAnimationEnabled] = useState(true);
  const [renderFailed, setRenderFailed] = useState(false);

  useEffect(() => {
    const media = window.matchMedia('(prefers-reduced-motion: reduce)');
    const updateMotionPreference = () => setAnimationEnabled(!media.matches);
    updateMotionPreference();
    media.addEventListener('change', updateMotionPreference);

    return () => media.removeEventListener('change', updateMotionPreference);
  }, []);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || !animationEnabled) return;

    let disposed = false;

    import('../ocean/OceanEngine')
      .then(({ OceanEngine }) => {
        if (disposed) return;
        const engine = new OceanEngine(canvas);
        engineRef.current = engine;
        engine.start();
        setRenderFailed(false);
      })
      .catch((error) => {
        console.error('Failed to initialize ocean renderer', error);
        setRenderFailed(true);
      });

    return () => {
      disposed = true;
      engineRef.current?.dispose();
      engineRef.current = null;
    };
  }, [animationEnabled]);

  useEffect(() => {
    if (engineRef.current) {
      engineRef.current.setTimeOfDay(timeOfDay);
    }
  }, [timeOfDay]);

  useEffect(() => {
    if (engineRef.current) {
      engineRef.current.setWaveIntensity(waveIntensity);
    }
  }, [waveIntensity]);

  if (!animationEnabled || renderFailed) {
    return <div className="oceanCanvasFallback" aria-label="静态海洋背景" />;
  }

  return (
    <canvas
      ref={canvasRef}
      className="oceanCanvas"
      style={{
        position: 'fixed',
        inset: 0,
        zIndex: 0,
        display: 'block',
        width: '100vw',
        height: '100vh',
        pointerEvents: 'none',
      }}
      aria-label="交互式海洋场景"
    />
  );
}
