// oceanSurface.frag.glsl
precision highp float;

varying vec3 vWorldPos;
varying vec3 vNormal;
varying vec2 vUv;
varying float vWaterDepth;
varying float vFoam;

uniform vec3 uCameraPos;
uniform vec3 uSunDirection;
uniform vec3 uSunColor;
uniform float uSunIntensity;
uniform vec3 uDeepColor;       // 深水色
uniform vec3 uShallowColor;    // 浅水色
uniform vec3 uFoamColor;       // 泡沫色
// uniform samplerCube uEnvMap;   // 环境反射贴图
// uniform sampler2D uCausticsMap;
uniform float uTime;

float fresnelSchlick(float cosTheta, float f0) {
  return f0 + (1.0 - f0) * pow(1.0 - cosTheta, 5.0);
}

// 简单的程序化天空反射，替代 uEnvMap 避免异步加载依赖
vec3 getSkyColor(vec3 viewDir) {
    float t = clamp(viewDir.y * 0.5 + 0.5, 0.0, 1.0);
    vec3 skyBlue = vec3(0.5, 0.7, 0.9);
    vec3 horizon = vec3(0.8, 0.9, 1.0);
    return mix(horizon, skyBlue, t);
}

void main() {
  vec3 N = normalize(vNormal);
  vec3 V = normalize(uCameraPos - vWorldPos);
  vec3 L = normalize(uSunDirection);
  vec3 H = normalize(L + V);

  float NdotV = max(dot(N, V), 0.0);
  float NdotL = max(dot(N, L), 0.0);
  float NdotH = max(dot(N, H), 0.0);

  // 反射
  vec3 reflectDir = reflect(-V, N);
  // vec3 reflection = textureCube(uEnvMap, reflectDir).rgb;
  vec3 reflection = getSkyColor(reflectDir);

  // 折射（程序化）
  float depthFactor = smoothstep(0.0, 30.0, vWaterDepth);
  vec3 refraction = mix(uShallowColor, uDeepColor, depthFactor);

  // 焦散
  // float caustic = texture2D(uCausticsMap, vWorldPos.xz * 0.3 + uTime * 0.05).r;
  float caustic = 0.0;
  refraction += caustic * uSunColor * 0.15;

  // 菲涅尔混合
  float fresnel = fresnelSchlick(NdotV, 0.02);
  vec3 color = mix(refraction, reflection, fresnel);

  // 镜面高光
  float specular = pow(NdotH, 512.0);
  color += uSunColor * specular * 0.6 * fresnel * uSunIntensity;

  // 次表面散射近似（仅薄角度可见）
  float sss = pow(1.0 - NdotV, 4.0) * 0.1;
  color += uShallowColor * sss;

  // 泡沫叠加
  color = mix(color, uFoamColor, vFoam * 0.75);

  // 泡沫边缘变暗（模拟泡沫厚度）
  color *= 1.0 - vFoam * 0.15;

  // Reinhard色调映射
  color = color / (1.0 + color);

  // 微弱的青色色调（海洋氛围）
  color = mix(color, color * vec3(0.9, 1.0, 1.1), 0.2);

  gl_FragColor = vec4(color, 0.94);
}
