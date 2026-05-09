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
uniform vec3 uDeepColor;
uniform vec3 uShallowColor;
uniform vec3 uFoamColor;
uniform float uTime;

uniform sampler2D uNormalMap;
uniform samplerCube uEnvMap;
uniform float uEnvMapReady;

float fresnelSchlick(float cosTheta, float f0) {
  return f0 + (1.0 - f0) * pow(1.0 - cosTheta, 5.0);
}

vec3 getFallbackSkyColor(vec3 viewDir) {
  float t = clamp(viewDir.y * 0.5 + 0.5, 0.0, 1.0);
  vec3 horizon = mix(uShallowColor, uSunColor, 0.18);
  vec3 zenith = uDeepColor;
  return mix(horizon, zenith, t);
}

vec3 blendNormals(vec3 a, vec3 b) {
  return normalize(vec3(a.xy + b.xy, max(a.z * b.z, 0.05)));
}

vec3 readWaterNormal(vec2 uv) {
  vec3 normalColor = texture2D(uNormalMap, uv).rgb * 2.0 - 1.0;
  return normalize(normalColor.xzy);
}

vec3 tangentToWorld(vec3 tangentNormal, vec3 baseNormal) {
  vec3 reference = abs(baseNormal.x) > 0.85 ? vec3(0.0, 0.0, 1.0) : vec3(1.0, 0.0, 0.0);
  vec3 tangent = normalize(reference - baseNormal * dot(baseNormal, reference));
  vec3 bitangent = normalize(cross(baseNormal, tangent));
  return normalize(
    tangent * tangentNormal.x +
    bitangent * tangentNormal.y +
    baseNormal * tangentNormal.z
  );
}

void main() {
  vec3 baseNormal = normalize(vNormal);

  vec2 uv0 = vUv * 40.0 + vec2(uTime * 0.015, uTime * 0.01);
  vec2 uv1 = vUv * 40.0 + vec2(-uTime * 0.01, uTime * 0.015);

  vec3 normal0 = readWaterNormal(uv0);
  vec3 normal1 = readWaterNormal(uv1);
  vec3 detailNormal = blendNormals(normal0, normal1);
  vec3 detailWorldNormal = tangentToWorld(detailNormal, baseNormal);
  vec3 N = normalize(mix(baseNormal, detailWorldNormal, 0.42));

  vec3 V = normalize(uCameraPos - vWorldPos);
  vec3 L = normalize(uSunDirection);
  vec3 H = normalize(L + V);

  float NdotV = max(dot(N, V), 0.0);
  float NdotH = max(dot(N, H), 0.0);

  vec3 reflectDir = reflect(-V, N);
  vec3 reflection = getFallbackSkyColor(reflectDir);
  if (uEnvMapReady > 0.5) {
    reflection = textureCube(uEnvMap, reflectDir).rgb;
  }
  reflection = mix(reflection, reflection * uSunColor, 0.4);

  float distortion = detailNormal.x * 12.0;
  float depthFactor = smoothstep(0.0, 40.0, vWaterDepth + distortion);
  vec3 baseColor = mix(uShallowColor, uDeepColor, depthFactor);

  float fresnel = fresnelSchlick(NdotV, 0.02);
  vec3 color = mix(baseColor, reflection, fresnel);

  float specular = pow(NdotH, 350.0);
  color += uSunColor * specular * 1.5 * fresnel * uSunIntensity;

  float foamNoise = texture2D(uNormalMap, vUv * 20.0 + uTime * 0.01).r;
  float foamMask = smoothstep(0.4, 0.8, vFoam * foamNoise * 2.0);
  color = mix(color, uFoamColor, foamMask);

  color = color / (1.0 + color);

  gl_FragColor = vec4(color, 0.96);
}
