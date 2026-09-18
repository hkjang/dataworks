import fs from 'node:fs'
import path from 'node:path'
import tailwindcss from '@tailwindcss/vite'
import react from '@vitejs/plugin-react'
import { defineConfig, type Plugin } from 'vite'

// 소스 체크아웃에는 web/dist/.gitkeep 이 추적되어 있고 web/embed.go 의
// `//go:embed all:dist` 가 그 파일 하나로 컴파일된다. Vite 는 빌드마다
// outDir 를 통째로 비우므로(emptyOutDir) 빌드 뒤 이 파일이 삭제 상태로
// 남아 작업 트리가 더러워지고, 복원을 잊으면 dist 가 빈 디렉터리가 되어
// `go build` 가 실패한다. emptyOutDir 을 끄면 stale 청크가 바이너리에
// 임베드되므로 비우는 동작은 그대로 두고, 산출물을 다 쓴 뒤 placeholder 만
// 다시 만든다. 내용은 빌드 전에 읽어 둔 원본 그대로 써서 git 이 변경으로
// 보지 않게 한다.
const DIST_PLACEHOLDER = '.gitkeep'
const DIST_PLACEHOLDER_DEFAULT = '# Vite 빌드 출력 디렉터리를 소스 체크아웃에 유지합니다.\n'

export function keepDistPlaceholder(): Plugin {
  let placeholderPath = ''
  let placeholderBody = DIST_PLACEHOLDER_DEFAULT
  return {
    name: 'dataworks:keep-dist-placeholder',
    apply: 'build',
    configResolved(config) {
      placeholderPath = path.join(path.resolve(config.root, config.build.outDir), DIST_PLACEHOLDER)
      try {
        placeholderBody = fs.readFileSync(placeholderPath, 'utf8')
      } catch {
        // 아직 없으면(예: Docker 의 빈 outDir) 기본 문구로 만든다.
      }
    },
    closeBundle() {
      fs.mkdirSync(path.dirname(placeholderPath), { recursive: true })
      fs.writeFileSync(placeholderPath, placeholderBody)
    },
  }
}

export default defineConfig({
  base: '/dataworks/',
  plugins: [react(), tailwindcss(), keepDistPlaceholder()],
  resolve: {
    alias: {
      '@': path.resolve(import.meta.dirname, './src'),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/admin': 'http://localhost:8080',
      '/auth': 'http://localhost:8080',
      '/me': 'http://localhost:8080',
    },
  },
})
