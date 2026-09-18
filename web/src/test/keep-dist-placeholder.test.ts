// @vitest-environment node
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { build } from 'vite'
import { afterEach, describe, expect, it } from 'vitest'

// 실제 vite.config.ts 를 그대로 읽어 실제 `vite build` 를 돌린다. 설정 객체를
// 들여다보는 대신 emptyOutDir 이 outDir 를 비운 뒤에도 placeholder 가 다시
// 생기는지, 그리고 stale 산출물은 여전히 지워지는지를 파일로 확인한다.
const configFile = fileURLToPath(new URL('../../vite.config.ts', import.meta.url))

let workDir = ''

afterEach(() => {
  if (workDir) fs.rmSync(workDir, { recursive: true, force: true })
  workDir = ''
})

async function buildFixture(placeholderBody: string | null) {
  workDir = fs.mkdtempSync(path.join(os.tmpdir(), 'dataworks-vite-'))
  const outDir = path.join(workDir, 'dist')
  fs.mkdirSync(outDir)
  fs.writeFileSync(path.join(outDir, 'stale-chunk.js'), 'console.log("stale")\n')
  if (placeholderBody !== null) {
    fs.writeFileSync(path.join(outDir, '.gitkeep'), placeholderBody)
  }
  fs.writeFileSync(
    path.join(workDir, 'index.html'),
    '<!doctype html><html><body><p>fixture</p></body></html>\n',
  )
  await build({
    configFile,
    root: workDir,
    logLevel: 'silent',
    build: { outDir, emptyOutDir: true },
  })
  return outDir
}

describe('keepDistPlaceholder', () => {
  it('recreates .gitkeep byte-for-byte after emptyOutDir removed it', async () => {
    const body = '# 원본 placeholder 내용\n'
    const outDir = await buildFixture(body)

    expect(fs.readFileSync(path.join(outDir, '.gitkeep'), 'utf8')).toBe(body)
    expect(fs.existsSync(path.join(outDir, 'index.html'))).toBe(true)
    // emptyOutDir 은 그대로 살아 있어야 한다 — stale 청크가 임베드되면 안 된다.
    expect(fs.existsSync(path.join(outDir, 'stale-chunk.js'))).toBe(false)
  })

  it('creates a default .gitkeep when the outDir had none', async () => {
    const outDir = await buildFixture(null)

    const placeholder = path.join(outDir, '.gitkeep')
    expect(fs.existsSync(placeholder)).toBe(true)
    expect(fs.readFileSync(placeholder, 'utf8')).toContain('Vite')
    expect(fs.existsSync(path.join(outDir, 'stale-chunk.js'))).toBe(false)
  })
})
