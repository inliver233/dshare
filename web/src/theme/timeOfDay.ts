export const TIME_STATES = {
  dawn: {
    name: '黎明',
    hours: [5, 7],
    sunElevation: 5,
    waveAmplitudeScale: 0.5,
    windDirection: [30, 20],
    cloudCoverage: 0.25,
    atmosphereHue: 12,
    bioluminescence: 0,
    starVisibility: 0,
    transitionDuration: 120,
    palette: {
      deep: '#0B3D4E', mid: '#146C7C', shallow: '#2A9D8F', foam: '#E9C46A',
      sun: '#FF6B35'
    }
  },
  day: {
    name: '白昼',
    hours: [7, 17],
    sunElevation: 55,
    waveAmplitudeScale: 1.0,
    windDirection: [45, 30],
    cloudCoverage: 0.35,
    atmosphereHue: 0,
    bioluminescence: 0,
    starVisibility: 0,
    transitionDuration: 180,
    palette: {
      deep: '#023E8A', mid: '#0077B6', shallow: '#48CAE4', foam: '#FFFFFF',
      sun: '#FFFDE7'
    }
  },
  dusk: {
    name: '黄昏',
    hours: [17, 19],
    sunElevation: 5,
    waveAmplitudeScale: 0.7,
    windDirection: [-20, 15],
    cloudCoverage: 0.30,
    atmosphereHue: 340,
    bioluminescence: 0.2,
    starVisibility: 0.3,
    transitionDuration: 120,
    palette: {
      deep: '#1A1A40', mid: '#3A0CA3', shallow: '#7209B7', foam: '#F72585',
      sun: '#FF6B35'
    }
  },
  night: {
    name: '星夜',
    hours: [19, 5],
    sunElevation: -30,
    waveAmplitudeScale: 0.4,
    windDirection: [-10, 10],
    cloudCoverage: 0.15,
    atmosphereHue: 260,
    bioluminescence: 1.0,
    starVisibility: 1.0,
    transitionDuration: 180,
    palette: {
      deep: '#000814', mid: '#001D3D', shallow: '#003566', foam: '#00F5D4',
      sun: '#E0E0E0'
    }
  },
};

export type TimeStateKey = keyof typeof TIME_STATES;
