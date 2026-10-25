import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
export default defineConfig({ plugins: [react()], test: { environment: 'jsdom', setupFiles: ['./src/test/setup.tsx'], include: ['src/**/*.test.{ts,tsx}'], restoreMocks: true, clearMocks: true } });
