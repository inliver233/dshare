import * as THREE from 'three';
import waveVert from './shaders/waveDisplacement.vert.glsl?raw';
import oceanFrag from './shaders/oceanSurface.frag.glsl?raw';
import { TIME_STATES, TimeStateKey } from '../theme/timeOfDay';

export type OceanEngineOptions = {
  maxPixelRatio?: number;
  normalMapUrl?: string;
  envMapUrls?: string[];
};

type OceanTransitionColors = {
  deep: THREE.Color;
  shallow: THREE.Color;
  foam: THREE.Color;
  sun: THREE.Color;
};

export class OceanEngine {
  canvas: HTMLCanvasElement;
  renderer: THREE.WebGLRenderer;
  scene: THREE.Scene;
  camera: THREE.PerspectiveCamera;
  startTime = 0;
  lastFrameTime = 0;
  rafId: number | null = null;
  isRunning = false;

  oceanMaterial!: THREE.ShaderMaterial;
  oceanMesh!: THREE.Mesh;
  
  targetTimeOfDay: TimeStateKey = 'day';
  currentTimeOfDay: TimeStateKey = 'day';
  transitionProgress = 1;
  transitionFromColors!: OceanTransitionColors;
  transitionToColors!: OceanTransitionColors;
  waveIntensity = 1.0;
  maxPixelRatio: number;
  exposureFrom = 1.0;
  exposureTo = 1.0;
  normalMap?: THREE.Texture;
  envMap?: THREE.CubeTexture;
  fallbackEnvMap?: THREE.CubeTexture<THREE.DataTexture>;
  fallbackEnvFaces: THREE.DataTexture[] = [];
  isDisposed = false;
  options: Required<OceanEngineOptions>;

  constructor(canvas: HTMLCanvasElement, options: OceanEngineOptions = {}) {
    this.canvas = canvas;
    this.maxPixelRatio = options.maxPixelRatio ?? 2;
    this.options = {
      maxPixelRatio: this.maxPixelRatio,
      normalMapUrl: options.normalMapUrl ?? '/textures/waternormals.jpg',
      envMapUrls: options.envMapUrls ?? [
        '/textures/skybox/px.jpg',
        '/textures/skybox/nx.jpg',
        '/textures/skybox/py.jpg',
        '/textures/skybox/ny.jpg',
        '/textures/skybox/pz.jpg',
        '/textures/skybox/nz.jpg',
      ],
    };

    this.renderer = new THREE.WebGLRenderer({ canvas, alpha: true, antialias: true, powerPreference: 'high-performance' });
    this.renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, this.maxPixelRatio));
    this.renderer.toneMapping = THREE.ACESFilmicToneMapping;
    this.renderer.outputColorSpace = THREE.SRGBColorSpace;
    this.renderer.toneMappingExposure = this.exposureForTimeOfDay(this.currentTimeOfDay);

    this.scene = new THREE.Scene();
    const { width, height } = this.getViewportSize();
    this.camera = new THREE.PerspectiveCamera(55, width / height, 0.5, 1000);
    this.camera.position.set(0, 15, 30);
    this.camera.lookAt(0, 0, 0);

    this.initOcean();
    this.resizeRenderer();

    window.addEventListener('resize', this.onResize);
  }

  private initOcean() {
    // Keep macro geometry modest; fine ripples come from the fragment normal maps.
    const geometry = new THREE.PlaneGeometry(500, 500, 128, 128);
    geometry.rotateX(-Math.PI / 2);

    const WAVE_LAYERS = [
      { dir: new THREE.Vector2(Math.cos(45*Math.PI/180), Math.sin(45*Math.PI/180)), Q: 0.18, L: 8.0, A: 0.20, speed: 2.0 },
      { dir: new THREE.Vector2(Math.cos(-35*Math.PI/180), Math.sin(-35*Math.PI/180)), Q: 0.12, L: 20.0, A: 0.40, speed: 3.0 },
      { dir: new THREE.Vector2(Math.cos(50*Math.PI/180), Math.sin(50*Math.PI/180)), Q: 0.08, L: 60.0, A: 0.80, speed: 5.0 },
      { dir: new THREE.Vector2(Math.cos(-15*Math.PI/180), Math.sin(-15*Math.PI/180)), Q: 0.06, L: 80.0, A: 1.00, speed: 5.5 },
    ];

    const dirs = WAVE_LAYERS.map(w => w.dir);
    const steepness = WAVE_LAYERS.map(w => w.Q);
    const wavelengths = WAVE_LAYERS.map(w => w.L);
    const amplitudes = WAVE_LAYERS.map(w => w.A);
    const speeds = WAVE_LAYERS.map(w => w.speed);

    const ts = TIME_STATES[this.currentTimeOfDay].palette;
    const initialColors = this.paletteColors(this.currentTimeOfDay);
    this.transitionFromColors = this.clonePaletteColors(initialColors);
    this.transitionToColors = this.clonePaletteColors(initialColors);

    this.normalMap = this.createFallbackNormalMap();
    this.fallbackEnvMap = this.createFallbackEnvMap();
    this.scene.background = new THREE.Color(ts.deep);

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
        uSunColor: { value: initialColors.sun },
        uSunIntensity: { value: 1.0 },
        uDeepColor: { value: initialColors.deep },
        uShallowColor: { value: initialColors.shallow },
        uFoamColor: { value: initialColors.foam },
        
        uNormalMap: { value: this.normalMap },
        uEnvMap: { value: this.fallbackEnvMap },
        uEnvMapReady: { value: 0.0 },
      }
    });

    this.oceanMesh = new THREE.Mesh(geometry, this.oceanMaterial);
    this.scene.add(this.oceanMesh);

    this.loadNormalMap();
    this.loadEnvMap();
  }

  private createFallbackNormalMap() {
    const data = new Uint8Array([128, 128, 255, 255]);
    const texture = new THREE.DataTexture(data, 1, 1, THREE.RGBAFormat);
    texture.wrapS = texture.wrapT = THREE.RepeatWrapping;
    texture.colorSpace = THREE.NoColorSpace;
    texture.needsUpdate = true;
    return texture;
  }

  private configureNormalMap(texture: THREE.Texture) {
    texture.wrapS = texture.wrapT = THREE.RepeatWrapping;
    texture.colorSpace = THREE.NoColorSpace;
    texture.anisotropy = Math.min(8, this.renderer.capabilities.getMaxAnisotropy());
  }

  private createFallbackEnvMap() {
    const faceColor = new Uint8Array([2, 62, 138, 255]);
    this.fallbackEnvFaces = Array.from({ length: 6 }, () => {
      const face = new THREE.DataTexture(faceColor, 1, 1, THREE.RGBAFormat);
      face.colorSpace = THREE.SRGBColorSpace;
      face.needsUpdate = true;
      return face;
    });

    const texture = new THREE.CubeTexture(this.fallbackEnvFaces);
    texture.colorSpace = THREE.SRGBColorSpace;
    texture.needsUpdate = true;
    return texture;
  }

  private loadNormalMap() {
    new THREE.TextureLoader().load(
      this.options.normalMapUrl,
      (texture) => {
        this.configureNormalMap(texture);
        if (this.isDisposed) {
          texture.dispose();
          return;
        }
        this.normalMap?.dispose();
        this.normalMap = texture;
        this.oceanMaterial.uniforms.uNormalMap.value = texture;
      },
      undefined,
      () => {
        if (this.isDisposed) return;
        this.oceanMaterial.uniforms.uNormalMap.value = this.normalMap;
      },
    );
  }

  private loadEnvMap() {
    new THREE.CubeTextureLoader().load(
      this.options.envMapUrls,
      (texture) => {
        texture.colorSpace = THREE.SRGBColorSpace;
        if (this.isDisposed) {
          texture.dispose();
          return;
        }
        this.envMap?.dispose();
        this.envMap = texture;
        this.scene.background = texture;
        this.oceanMaterial.uniforms.uEnvMap.value = texture;
        this.oceanMaterial.uniforms.uEnvMapReady.value = 1.0;
        this.fallbackEnvMap?.dispose();
        this.fallbackEnvMap = undefined;
        this.disposeFallbackEnvFaces();
      },
      undefined,
      () => {
        if (this.isDisposed) return;
        this.scene.background = new THREE.Color(TIME_STATES[this.currentTimeOfDay].palette.deep);
        this.oceanMaterial.uniforms.uEnvMap.value = this.fallbackEnvMap;
        this.oceanMaterial.uniforms.uEnvMapReady.value = 0.0;
      },
    );
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

  private exposureForTimeOfDay(tod: TimeStateKey) {
    if (tod === 'night') return 0.45;
    if (tod === 'dawn' || tod === 'dusk') return 0.72;
    return 1.0;
  }

  private paletteColors(tod: TimeStateKey): OceanTransitionColors {
    const palette = TIME_STATES[tod].palette;
    return {
      deep: new THREE.Color(palette.deep),
      shallow: new THREE.Color(palette.shallow),
      foam: new THREE.Color(palette.foam),
      sun: new THREE.Color(palette.sun),
    };
  }

  private clonePaletteColors(colors: OceanTransitionColors): OceanTransitionColors {
    return {
      deep: colors.deep.clone(),
      shallow: colors.shallow.clone(),
      foam: colors.foam.clone(),
      sun: colors.sun.clone(),
    };
  }

  private currentUniformColors(): OceanTransitionColors {
    const uniforms = this.oceanMaterial.uniforms;
    return {
      deep: (uniforms.uDeepColor.value as THREE.Color).clone(),
      shallow: (uniforms.uShallowColor.value as THREE.Color).clone(),
      foam: (uniforms.uFoamColor.value as THREE.Color).clone(),
      sun: (uniforms.uSunColor.value as THREE.Color).clone(),
    };
  }

  private applyTransitionColors(t: number) {
    const uniforms = this.oceanMaterial.uniforms;
    (uniforms.uDeepColor.value as THREE.Color).copy(this.transitionFromColors.deep).lerp(this.transitionToColors.deep, t);
    (uniforms.uShallowColor.value as THREE.Color).copy(this.transitionFromColors.shallow).lerp(this.transitionToColors.shallow, t);
    (uniforms.uFoamColor.value as THREE.Color).copy(this.transitionFromColors.foam).lerp(this.transitionToColors.foam, t);
    (uniforms.uSunColor.value as THREE.Color).copy(this.transitionFromColors.sun).lerp(this.transitionToColors.sun, t);

    if (!this.envMap) {
      if (!(this.scene.background instanceof THREE.Color)) {
        this.scene.background = new THREE.Color();
      }
      this.scene.background.copy(this.transitionFromColors.deep).lerp(this.transitionToColors.deep, t);
    }
  }

  onResize = () => {
    this.resizeRenderer();
  }

  start() {
    if (this.isRunning) return;
    this.isRunning = true;
    const now = performance.now() * 0.001;
    this.startTime = now;
    this.lastFrameTime = now;
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
    if (this.targetTimeOfDay === tod && this.transitionProgress === 1) return;

    this.targetTimeOfDay = tod;
    this.transitionProgress = 0;
    this.transitionFromColors = this.currentUniformColors();
    this.transitionToColors = this.paletteColors(tod);
    this.exposureFrom = this.renderer.toneMappingExposure;
    this.exposureTo = this.exposureForTimeOfDay(tod);
  }

  setWaveIntensity(intensity: number) {
    this.waveIntensity = THREE.MathUtils.clamp(Number.isFinite(intensity) ? intensity : 1, 0, 2);
  }

  loop() {
    if (!this.isRunning) return;
    const now = performance.now() * 0.001;
    const dt = Math.min(now - this.lastFrameTime, 0.1);
    const time = now - this.startTime;
    this.lastFrameTime = now;

    if (this.transitionProgress < 1) {
      this.transitionProgress += dt / 2.0; // 2 seconds transition
      if (this.transitionProgress > 1) this.transitionProgress = 1;
      
      const t = this.transitionProgress;

      this.applyTransitionColors(t);
      this.renderer.toneMappingExposure = THREE.MathUtils.lerp(this.exposureFrom, this.exposureTo, t);

      if (t === 1) {
        this.currentTimeOfDay = this.targetTimeOfDay;
      }
    }

    this.oceanMaterial.uniforms.uTime.value = time;
    this.oceanMaterial.uniforms.uWaveIntensity.value = this.waveIntensity;
    this.oceanMaterial.uniforms.uCameraPos.value.copy(this.camera.position);

    // 动态相机的微弱漂移，增加沉浸感
    this.camera.position.y = 15 + Math.sin(time * 0.5) * 1.5;
    this.camera.position.x = Math.cos(time * 0.2) * 2.0;
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

  private disposeFallbackEnvFaces() {
    for (const face of this.fallbackEnvFaces) {
      face.dispose();
    }
    this.fallbackEnvFaces = [];
  }

  dispose() {
    this.isDisposed = true;
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
    this.normalMap?.dispose();
    this.envMap?.dispose();
    this.fallbackEnvMap?.dispose();
    this.disposeFallbackEnvFaces();
    this.scene.background = null;
    this.renderer.renderLists.dispose();
    this.renderer.dispose();
  }
}
