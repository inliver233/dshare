// waveDisplacement.vert.glsl
// 输入: 平面网格顶点 (x, 0, z)
// 输出: 经Gerstner波 + 涟漪位移后的顶点 + 法线

precision highp float;

// Three.js 预定义的 attribute 和 uniform 会自动注入
// attribute vec3 position;
// attribute vec2 uv;
// uniform mat4 projectionMatrix;
// uniform mat4 modelViewMatrix;
// uniform mat3 normalMatrix;

uniform float uTime;
uniform float uWaveIntensity; // 0(平静) ~ 2(风暴)

// 8层Gerstner波参数
uniform vec2 uWaveDir[8];
uniform float uSteepness[8];
uniform float uWavelength[8];
uniform float uAmplitude[8];
uniform float uSpeed[8];

// 涟漪 - 当前为简化实现，不通过纹理而通过数学模拟点击扩散涟漪，或者如果没有涟漪传导我们可以先给默认的高度
// 为了系统完整性，这里我们按设计保留了接口
uniform sampler2D uRippleMap;
uniform float uRippleStrength;
uniform vec2 uRippleTexelSize;
uniform vec2 uWorldOffset; // 涟漪纹理在世界空间中的偏移

varying vec3 vWorldPos;
varying vec3 vNormal;
varying vec2 vUv;
varying float vWaterDepth;
varying float vFoam;

const float PI = 3.14159265359;
const float G = 9.8;

void main() {
  vec3 pos = position;
  vec3 displaced = pos;

  // 叠加Gerstner波
  for (int i = 0; i < 8; i++) {
    float k = 2.0 * PI / uWavelength[i];
    float Qi = uSteepness[i] / (k * max(uAmplitude[i] * uWaveIntensity, 0.001) * 8.0);

    float phase = k * (uWaveDir[i].x * pos.x + uWaveDir[i].y * pos.z
                       - uSpeed[i] * uTime);
    float cosP = cos(phase);
    float sinP = sin(phase);

    displaced.x += Qi * uAmplitude[i] * uWaveIntensity * uWaveDir[i].x * cosP;
    displaced.y += uAmplitude[i] * uWaveIntensity * sinP;
    displaced.z += Qi * uAmplitude[i] * uWaveIntensity * uWaveDir[i].y * cosP;
  }

  // 叠加涟漪（从纹理采样）
  // 简化: 为了稳定运行，确保纹理采样安全
  vec2 rippleUV = (pos.xz + uWorldOffset) * uRippleTexelSize * 0.1;
  // float rippleH = texture2D(uRippleMap, rippleUV).r;
  float rippleH = 0.0; // 暂时禁用涟漪纹理采样，待后续计算着色器完备
  displaced.y += rippleH * uRippleStrength;

  // 数值法线计算（采样邻近点）
  float eps = 0.1;
  vec3 dx = displaced;
  vec3 dz = displaced;
  // 对x方向微扰
  vec3 px = position + vec3(eps, 0.0, 0.0);
  for (int i = 0; i < 8; i++) {
    float k = 2.0 * PI / uWavelength[i];
    float Qi = uSteepness[i] / (k * max(uAmplitude[i] * uWaveIntensity, 0.001) * 8.0);
    float phase = k * (uWaveDir[i].x * px.x + uWaveDir[i].y * px.z - uSpeed[i] * uTime);
    px.x += Qi * uAmplitude[i] * uWaveIntensity * uWaveDir[i].x * cos(phase);
    px.y += uAmplitude[i] * uWaveIntensity * sin(phase);
    px.z += Qi * uAmplitude[i] * uWaveIntensity * uWaveDir[i].y * cos(phase);
  }
  // 对z方向微扰
  vec3 pz = position + vec3(0.0, 0.0, eps);
  for (int i = 0; i < 8; i++) {
    float k = 2.0 * PI / uWavelength[i];
    float Qi = uSteepness[i] / (k * max(uAmplitude[i] * uWaveIntensity, 0.001) * 8.0);
    float phase = k * (uWaveDir[i].x * pz.x + uWaveDir[i].y * pz.z - uSpeed[i] * uTime);
    pz.x += Qi * uAmplitude[i] * uWaveIntensity * uWaveDir[i].x * cos(phase);
    pz.y += uAmplitude[i] * uWaveIntensity * sin(phase);
    pz.z += Qi * uAmplitude[i] * uWaveIntensity * uWaveDir[i].y * cos(phase);
  }

  vec3 tangentX = normalize(px - displaced);
  vec3 tangentZ = normalize(pz - displaced);
  vec3 normal = normalize(cross(tangentZ, tangentX));

  // 泡沫检测
  float steepness = 1.0 - normal.y;
  vFoam = smoothstep(0.12, 0.28, steepness);

  vec4 worldPosV = modelMatrix * vec4(displaced, 1.0);
  vWorldPos = worldPosV.xyz;
  vNormal = normalize(normalMatrix * normal);
  vUv = uv;
  vWaterDepth = length(worldPosV.xz); // 简化：离原点越远水越深

  gl_Position = projectionMatrix * viewMatrix * worldPosV;
}
