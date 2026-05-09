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
  skyZenith: THREE.Color;
  skyHorizon: THREE.Color;
  skyLower: THREE.Color;
  starIntensity: number;
  envBlend: number;
  sunIntensity: number;
};

const SKY_VERT = `
varying vec3 vWorldDir;

void main() {
  vec4 worldPosition = modelMatrix * vec4(position, 1.0);
  vWorldDir = normalize(worldPosition.xyz - cameraPosition);
  gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
}
`;

const SKY_FRAG = `
precision highp float;

varying vec3 vWorldDir;

uniform vec3 uSkyZenith;
uniform vec3 uSkyHorizon;
uniform vec3 uSkyLower;
uniform float uStarIntensity;
uniform float uTime;

float hash(vec2 p) {
  p = fract(p * vec2(123.34, 456.21));
  p += dot(p, p + 45.32);
  return fract(p.x * p.y);
}

void main() {
  vec3 dir = normalize(vWorldDir);
  float y = dir.y;
  float horizon = 1.0 - smoothstep(0.0, 0.48, abs(y));

  vec3 upper = mix(uSkyHorizon, uSkyZenith, smoothstep(0.0, 0.92, y));
  vec3 lower = mix(uSkyLower, uSkyHorizon, smoothstep(-0.34, 0.08, y));
  vec3 color = mix(lower, upper, smoothstep(-0.02, 0.12, y));
  color += uSkyHorizon * horizon * 0.32;

  vec2 starUv = dir.xz / max(0.18, y + 1.15) * 90.0;
  vec2 cell = floor(starUv);
  vec2 local = fract(starUv) - 0.5;
  float starSeed = hash(cell);
  float starCore = smoothstep(0.055, 0.0, length(local));
  float twinkle = 0.65 + 0.35 * sin(uTime * 0.8 + starSeed * 37.0);
  float star = step(0.992, starSeed) * starCore * twinkle * smoothstep(0.04, 0.42, y);
  color += vec3(0.68, 0.82, 1.0) * star * uStarIntensity;

  gl_FragColor = vec4(color, 1.0);
}
`;

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
  skyMaterial!: THREE.ShaderMaterial;
  skyMesh!: THREE.Mesh;
  
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
    this.camera.position.set(0, 14, 34);
    this.camera.lookAt(0, 2.5, 0);

    this.initSky();
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

    const initialColors = this.paletteColors(this.currentTimeOfDay);
    this.transitionFromColors = this.clonePaletteColors(initialColors);
    this.transitionToColors = this.clonePaletteColors(initialColors);

    this.normalMap = this.createFallbackNormalMap();
    this.fallbackEnvMap = this.createFallbackEnvMap();
    this.scene.background = null;

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
        uSunIntensity: { value: initialColors.sunIntensity },
        uDeepColor: { value: initialColors.deep },
        uShallowColor: { value: initialColors.shallow },
        uFoamColor: { value: initialColors.foam },
        
        uNormalMap: { value: this.normalMap },
        uEnvMap: { value: this.fallbackEnvMap },
        uEnvMapReady: { value: 0.0 },
        uEnvBlend: { value: initialColors.envBlend },
      }
    });

    this.oceanMesh = new THREE.Mesh(geometry, this.oceanMaterial);
    this.scene.add(this.oceanMesh);

    this.loadNormalMap();
    this.loadEnvMap();
  }

  private initSky() {
    const colors = this.paletteColors(this.currentTimeOfDay);

    this.skyMaterial = new THREE.ShaderMaterial({
      vertexShader: SKY_VERT,
      fragmentShader: SKY_FRAG,
      depthWrite: false,
      depthTest: false,
      side: THREE.BackSide,
      uniforms: {
        uSkyZenith: { value: colors.skyZenith },
        uSkyHorizon: { value: colors.skyHorizon },
        uSkyLower: { value: colors.skyLower },
        uStarIntensity: { value: colors.starIntensity },
        uTime: { value: 0 },
      },
    });

    this.skyMesh = new THREE.Mesh(new THREE.SphereGeometry(700, 48, 24), this.skyMaterial);
    this.skyMesh.frustumCulled = false;
    this.skyMesh.renderOrder = -100;
    this.scene.add(this.skyMesh);
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
        this.oceanMaterial.uniforms.uEnvMap.value = texture;
        this.oceanMaterial.uniforms.uEnvMapReady.value = 1.0;
        this.fallbackEnvMap?.dispose();
        this.fallbackEnvMap = undefined;
        this.disposeFallbackEnvFaces();
      },
      undefined,
      () => {
        if (this.isDisposed) return;
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
    const skyByTime: Record<TimeStateKey, {
      zenith: string;
      horizon: string;
      lower: string;
      starIntensity: number;
      envBlend: number;
      sunIntensity: number;
    }> = {
      dawn: {
        zenith: '#123B62',
        horizon: '#F08A4B',
        lower: '#075C68',
        starIntensity: 0.08,
        envBlend: 0.22,
        sunIntensity: 1.15,
      },
      day: {
        zenith: '#51B5E6',
        horizon: '#C8F4FF',
        lower: '#0877A6',
        starIntensity: 0.0,
        envBlend: 0.12,
        sunIntensity: 1.35,
      },
      dusk: {
        zenith: '#25154F',
        horizon: '#F35B75',
        lower: '#24185F',
        starIntensity: 0.16,
        envBlend: 0.28,
        sunIntensity: 1.05,
      },
      night: {
        zenith: '#020617',
        horizon: '#14315F',
        lower: '#00162B',
        starIntensity: 0.72,
        envBlend: 0.45,
        sunIntensity: 0.68,
      },
    };
    const sky = skyByTime[tod];

    return {
      deep: new THREE.Color(palette.deep),
      shallow: new THREE.Color(palette.shallow),
      foam: new THREE.Color(palette.foam),
      sun: new THREE.Color(palette.sun),
      skyZenith: new THREE.Color(sky.zenith),
      skyHorizon: new THREE.Color(sky.horizon),
      skyLower: new THREE.Color(sky.lower),
      starIntensity: sky.starIntensity,
      envBlend: sky.envBlend,
      sunIntensity: sky.sunIntensity,
    };
  }

  private clonePaletteColors(colors: OceanTransitionColors): OceanTransitionColors {
    return {
      deep: colors.deep.clone(),
      shallow: colors.shallow.clone(),
      foam: colors.foam.clone(),
      sun: colors.sun.clone(),
      skyZenith: colors.skyZenith.clone(),
      skyHorizon: colors.skyHorizon.clone(),
      skyLower: colors.skyLower.clone(),
      starIntensity: colors.starIntensity,
      envBlend: colors.envBlend,
      sunIntensity: colors.sunIntensity,
    };
  }

  private currentUniformColors(): OceanTransitionColors {
    const uniforms = this.oceanMaterial.uniforms;
    const skyUniforms = this.skyMaterial.uniforms;
    return {
      deep: (uniforms.uDeepColor.value as THREE.Color).clone(),
      shallow: (uniforms.uShallowColor.value as THREE.Color).clone(),
      foam: (uniforms.uFoamColor.value as THREE.Color).clone(),
      sun: (uniforms.uSunColor.value as THREE.Color).clone(),
      skyZenith: (skyUniforms.uSkyZenith.value as THREE.Color).clone(),
      skyHorizon: (skyUniforms.uSkyHorizon.value as THREE.Color).clone(),
      skyLower: (skyUniforms.uSkyLower.value as THREE.Color).clone(),
      starIntensity: Number(skyUniforms.uStarIntensity.value),
      envBlend: Number(uniforms.uEnvBlend.value),
      sunIntensity: Number(uniforms.uSunIntensity.value),
    };
  }

  private applyTransitionColors(t: number) {
    const uniforms = this.oceanMaterial.uniforms;
    const skyUniforms = this.skyMaterial.uniforms;
    (uniforms.uDeepColor.value as THREE.Color).copy(this.transitionFromColors.deep).lerp(this.transitionToColors.deep, t);
    (uniforms.uShallowColor.value as THREE.Color).copy(this.transitionFromColors.shallow).lerp(this.transitionToColors.shallow, t);
    (uniforms.uFoamColor.value as THREE.Color).copy(this.transitionFromColors.foam).lerp(this.transitionToColors.foam, t);
    (uniforms.uSunColor.value as THREE.Color).copy(this.transitionFromColors.sun).lerp(this.transitionToColors.sun, t);
    uniforms.uEnvBlend.value = THREE.MathUtils.lerp(this.transitionFromColors.envBlend, this.transitionToColors.envBlend, t);
    uniforms.uSunIntensity.value = THREE.MathUtils.lerp(this.transitionFromColors.sunIntensity, this.transitionToColors.sunIntensity, t);

    (skyUniforms.uSkyZenith.value as THREE.Color).copy(this.transitionFromColors.skyZenith).lerp(this.transitionToColors.skyZenith, t);
    (skyUniforms.uSkyHorizon.value as THREE.Color).copy(this.transitionFromColors.skyHorizon).lerp(this.transitionToColors.skyHorizon, t);
    (skyUniforms.uSkyLower.value as THREE.Color).copy(this.transitionFromColors.skyLower).lerp(this.transitionToColors.skyLower, t);
    skyUniforms.uStarIntensity.value = THREE.MathUtils.lerp(this.transitionFromColors.starIntensity, this.transitionToColors.starIntensity, t);
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
    this.skyMaterial.uniforms.uTime.value = time;

    // 动态相机的微弱漂移，增加沉浸感
    this.camera.position.y = 14 + Math.sin(time * 0.5) * 1.1;
    this.camera.position.x = Math.cos(time * 0.2) * 1.6;
    this.camera.lookAt(0, 2.8, 0);

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
