// waveDisplacement.vert.glsl
precision highp float;

uniform float uTime;
uniform float uWaveIntensity; // 0(平静) ~ 2(风暴)

// 降级为 4 层宏观 Gerstner 波（减少顶点计算，细节由法线贴图负责）
uniform vec2 uWaveDir[4];
uniform float uSteepness[4];
uniform float uWavelength[4];
uniform float uAmplitude[4];
uniform float uSpeed[4];

varying vec3 vWorldPos;
varying vec3 vNormal;
varying vec2 vUv;
varying float vWaterDepth;
varying float vFoam;

const float PI = 3.14159265359;

void main() {
  vec3 pos = position;
  vec3 displaced = pos;
  vec3 tangentX = vec3(1.0, 0.0, 0.0);
  vec3 tangentZ = vec3(0.0, 0.0, 1.0);

  for (int i = 0; i < 4; i++) {
    float k = 2.0 * PI / uWavelength[i];
    float amplitude = uAmplitude[i] * uWaveIntensity;
    float Qi = uSteepness[i] / (k * max(amplitude, 0.001) * 4.0);
    float phase = k * (uWaveDir[i].x * pos.x + uWaveDir[i].y * pos.z - uSpeed[i] * uTime);
    float cosP = cos(phase);
    float sinP = sin(phase);

    displaced.x += Qi * amplitude * uWaveDir[i].x * cosP;
    displaced.y += amplitude * sinP;
    displaced.z += Qi * amplitude * uWaveDir[i].y * cosP;

    float horizontalSlope = Qi * amplitude * k * sinP;
    float verticalSlope = amplitude * k * cosP;
    vec2 dir = uWaveDir[i];

    tangentX.x -= horizontalSlope * dir.x * dir.x;
    tangentX.y += verticalSlope * dir.x;
    tangentX.z -= horizontalSlope * dir.x * dir.y;

    tangentZ.x -= horizontalSlope * dir.x * dir.y;
    tangentZ.y += verticalSlope * dir.y;
    tangentZ.z -= horizontalSlope * dir.y * dir.y;
  }

  vec3 normal = normalize(cross(tangentZ, tangentX));
  float steepness = 1.0 - normal.y;
  vFoam = smoothstep(0.05, 0.25, steepness);

  vec4 worldPosV = modelMatrix * vec4(displaced, 1.0);
  vWorldPos = worldPosV.xyz;
  vNormal = normalize(mat3(modelMatrix) * normal);
  vUv = uv;
  vWaterDepth = length(worldPosV.xz);

  gl_Position = projectionMatrix * viewMatrix * worldPosV;
}
