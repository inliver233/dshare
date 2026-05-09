import { create } from 'zustand'
import { TimeStateKey } from '../theme/timeOfDay'

interface OceanState {
  timeOfDay: TimeStateKey;
  waveIntensity: number;
  setTimeOfDay: (t: TimeStateKey) => void;
  setWaveIntensity: (w: number) => void;
  rippleClicks: { x: number, y: number, id: number }[];
  addRipple: (x: number, y: number) => void;
}

export const useOceanState = create<OceanState>((set) => ({
  timeOfDay: 'day',
  waveIntensity: 1.0,
  setTimeOfDay: (t) => set({ timeOfDay: t }),
  setWaveIntensity: (w) => set({ waveIntensity: w }),
  rippleClicks: [],
  addRipple: (x, y) => set((state) => ({ 
    rippleClicks: [...state.rippleClicks, { x, y, id: Date.now() }]
  }))
}))
