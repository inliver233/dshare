import * as THREE from 'three';
import waveVert from './shaders/waveDisplacement.vert.glsl?raw';
import oceanFrag from './shaders/oceanSurface.frag.glsl?raw';
import { TIME_STATES, TimeStateKey } from '../theme/timeOfDay';

export type OceanEngineOptions = {
  maxPixelRatio?: number;
};

export class OceanEngine {
  canvas: HTMLCanvasElement;
  renderer: THREE.WebGLRenderer;
  scene: THREE.Scene;
  camera: THREE.PerspectiveCamera;
  clock: THREE.Clock;
  rafId: number | null = null;
  isRunning = false;

  oceanMaterial!: THREE.ShaderMaterial;
  oceanMesh!: THREE.Mesh;
  
  targetTimeOfDay: TimeStateKey = 'day';
  currentTimeOfDay: TimeStateKey = 'day';
  transitionProgress = 1;
  waveIntensity = 1.0;
  maxPixelRatio: number;

  constructor(canvas: HTMLCanvasElement, options: OceanEngineOptions = {}) {
    this.canvas = canvas;
    this.maxPixelRatio = options.maxPixelRatio ?? 2;
    this.renderer = new THREE.WebGLRenderer({ canvas, alpha: true, antialias: true, powerPreference: 'high-performance' });
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, this.maxPixelRatio));
    this.renderer.toneMapping = THREE.ACESFilmicToneMapping;

    this.scene = new THREE.Scene();
    const { width, height } = this.getViewportSize();
    this.camera = new THREE.PerspectiveCamera(55, width / height, 0.5, 1000);
    this.camera.position.set(0, 15, 30);
    this.camera.lookAt(0, 0, 0);

    this.clock = new THREE.Clock();

    this.initOcean();
    this.initSky();
    this.resizeRenderer();

    window.addEventListener('resize', this.onResize);
  }

  private initOcean() {
    const geometry = new THREE.PlaneGeometry(500, 500, 256, 256);
    geometry.rotateX(-Math.PI / 2); // 平铺

    const WAVE_LAYERS = [
      { dir: new THREE.Vector2(Math.cos(30*Math.PI/180), Math.sin(30*Math.PI/180)), Q: 0.28, L: 0.5, A: 0.02, speed: 0.5 },
      { dir: new THREE.Vector2(Math.cos(-20*Math.PI/180), Math.sin(-20*Math.PI/180)), Q: 0.22, L: 1.2, A: 0.04, speed: 0.8 },
      { dir: new THREE.Vector2(Math.cos(45*Math.PI/180), Math.sin(45*Math.PI/180)), Q: 0.18, L: 3.0, A: 0.08, speed: 1.2 },
      { dir: new THREE.Vector2(Math.cos(-60*Math.PI/180), Math.sin(-60*Math.PI/180)), Q: 0.15, L: 8.0, A: 0.20, speed: 2.0 },
      { dir: new THREE.Vector2(Math.cos(10*Math.PI/180), Math.sin(10*Math.PI/180)), Q: 0.12, L: 20.0, A: 0.40, speed: 3.0 },
      { dir: new THREE.Vector2(Math.cos(-35*Math.PI/180), Math.sin(-35*Math.PI/180)), Q: 0.10, L: 40.0, A: 0.60, speed: 4.0 },
      { dir: new THREE.Vector2(Math.cos(50*Math.PI/180), Math.sin(50*Math.PI/180)), Q: 0.08, L: 60.0, A: 0.80, speed: 5.0 },
      { dir: new THREE.Vector2(Math.cos(-15*Math.PI/180), Math.sin(-15*Math.PI/180)), Q: 0.06, L: 80.0, A: 1.00, speed: 5.5 },
    ];

    const dirs = WAVE_LAYERS.map(w => w.dir);
    const steepness = WAVE_LAYERS.map(w => w.Q);
    const wavelengths = WAVE_LAYERS.map(w => w.L);
    const amplitudes = WAVE_LAYERS.map(w => w.A);
    const speeds = WAVE_LAYERS.map(w => w.speed);

    const ts = TIME_STATES[this.currentTimeOfDay].palette;

    this.oceanMaterial = new THREE.ShaderMaterial({
      vertexShader: waveVert,
      fragmentShader: oceanFrag,
      transparent: true,
      uniforms: {
        uTime: { value: 0 },
        uWaveIntensity: { value: this.waveIntensity },
        uWaveDir: { value: dirs },
        uSteepness: { value: steepness },
        uWavelength: { value: wavelengths },
        uAmplitude: { value: amplitudes },
        uSpeed: { value: speeds },
        
        uCameraPos: { value: this.camera.position },
        uSunDirection: { value: new THREE.Vector3(1, 1, 1).normalize() },
        uSunColor: { value: new THREE.Color(ts.sun) },
        uSunIntensity: { value: 1.0 },
        uDeepColor: { value: new THREE.Color(ts.deep) },
        uShallowColor: { value: new THREE.Color(ts.shallow) },
        uFoamColor: { value: new THREE.Color(ts.foam) },
        
        uRippleStrength: { value: 0.0 },
        uRippleTexelSize: { value: new THREE.Vector2(1/256, 1/256) },
        uWorldOffset: { value: new THREE.Vector2(0, 0) },
      }
    });

    this.oceanMesh = new THREE.Mesh(geometry, this.oceanMaterial);
    this.scene.add(this.oceanMesh);
  }

  private initSky() {
    this.scene.background = new THREE.Color(TIME_STATES[this.currentTimeOfDay].palette.deep);
  }

  private getViewportSize() {
    const width = Math.max(1, this.canvas.clientWidth || window.innerWidth || 1);
    const height = Math.max(1, this.canvas.clientHeight || window.innerHeight || 1);
    return { width, height };
  }

  private resizeRenderer() {
    const { width, height } = this.getViewportSize();
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, this.maxPixelRatio));
    this.renderer.setSize(width, height, false);
    this.camera.aspect = width / height;
    this.camera.updateProjectionMatrix();
  }

  onResize = () => {
    this.resizeRenderer();
  }

  start() {
    if (this.isRunning) return;
    this.isRunning = true;
    this.clock.start();
    this.loop();
  }

  stop() {
    this.isRunning = false;
    if (this.rafId !== null) {
      cancelAnimationFrame(this.rafId);
      this.rafId = null;
    }
  }

  setTimeOfDay(tod: TimeStateKey) {
    if (this.currentTimeOfDay !== tod) {
      this.targetTimeOfDay = tod;
      this.transitionProgress = 0;
    }
  }

  setWaveIntensity(intensity: number) {
    this.waveIntensity = THREE.MathUtils.clamp(Number.isFinite(intensity) ? intensity : 1, 0, 2);
  }

  private lerpColor(c1: THREE.Color, c2: THREE.Color, t: number) {
    return new THREE.Color().copy(c1).lerp(c2, t);
  }

  loop() {
    if (!this.isRunning) return;
    const dt = Math.min(this.clock.getDelta(), 0.1);
    const time = this.clock.getElapsedTime();

    if (this.transitionProgress < 1) {
      this.transitionProgress += dt / 2.0; // 2 seconds transition
      if (this.transitionProgress > 1) this.transitionProgress = 1;
      
      // Update uniforms
      const from = TIME_STATES[this.currentTimeOfDay].palette;
      const to = TIME_STATES[this.targetTimeOfDay].palette;
      const t = this.transitionProgress;

      this.oceanMaterial.uniforms.uDeepColor.value = this.lerpColor(new THREE.Color(from.deep), new THREE.Color(to.deep), t);
      this.oceanMaterial.uniforms.uShallowColor.value = this.lerpColor(new THREE.Color(from.shallow), new THREE.Color(to.shallow), t);
      this.oceanMaterial.uniforms.uFoamColor.value = this.lerpColor(new THREE.Color(from.foam), new THREE.Color(to.foam), t);
      this.oceanMaterial.uniforms.uSunColor.value = this.lerpColor(new THREE.Color(from.sun), new THREE.Color(to.sun), t);
      this.scene.background = this.lerpColor(new THREE.Color(from.deep), new THREE.Color(to.deep), t);

      if (t === 1) {
        this.currentTimeOfDay = this.targetTimeOfDay;
      }
    }

    this.oceanMaterial.uniforms.uTime.value = time;
    this.oceanMaterial.uniforms.uWaveIntensity.value = this.waveIntensity;
    this.oceanMaterial.uniforms.uCameraPos.value.copy(this.camera.position);

    // Simple camera slight drift
    this.camera.position.y = 15 + Math.sin(time * 0.5) * 1.5;
    this.camera.lookAt(0, 0, 0);

    this.renderer.render(this.scene, this.camera);
    this.rafId = requestAnimationFrame(() => this.loop());
  }

  private disposeMaterial(material: THREE.Material | THREE.Material[]) {
    const materials = Array.isArray(material) ? material : [material];
    for (const item of materials) {
      item.dispose();
    }
  }

  dispose() {
    this.stop();
    window.removeEventListener('resize', this.onResize);
    this.scene.traverse((child) => {
      const mesh = child as THREE.Mesh;
      if (mesh.geometry) {
        mesh.geometry.dispose();
      }
      if (mesh.material) {
        this.disposeMaterial(mesh.material);
      }
    });
    this.renderer.dispose();
  }
}
