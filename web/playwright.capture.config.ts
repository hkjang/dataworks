import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './capture',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: 'line',
  outputDir: './test-results/capture',
  use: {
    baseURL: 'http://127.0.0.1:5173/dataworks/',
    colorScheme: 'light',
    locale: 'ko-KR',
    reducedMotion: 'reduce',
    screenshot: 'off',
    timezoneId: 'Asia/Seoul',
    trace: 'off',
    video: 'off',
  },
  projects: [
    {
      name: 'desktop',
      use: {
        deviceScaleFactor: 1,
        viewport: { width: 1440, height: 960 },
      },
    },
    {
      name: 'mobile',
      use: {
        deviceScaleFactor: 1,
        hasTouch: true,
        isMobile: true,
        viewport: { width: 390, height: 844 },
      },
    },
  ],
  webServer: {
    command: 'npm run dev -- --host 127.0.0.1',
    url: 'http://127.0.0.1:5173/dataworks/',
    reuseExistingServer: false,
  },
})
