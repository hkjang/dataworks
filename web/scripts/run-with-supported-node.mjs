// npm 은 run-script 의 PATH 앞에 상위 디렉터리의 node_modules/.bin 을 전부 붙인다
// (프로젝트 → … → /home/<user> → /). 그 중 한 곳에 `node` 심링크가 있으면 그 node 가
// nvm·CI 의 node 를 가린 채 로컬 bin 의 `#!/usr/bin/env node` 를 받아, 툴체인이
// package.json 의 engines 보다 낮은 런타임에서 돈다. 실제로 그 상태에서는 jsdom 이
// 끌어오는 undici 가 로드 중 `webidl.util.markAsUncloneable is not a function` 으로
// 죽어서(Node 22.10 에서 추가된 worker_threads.markAsUncloneable 이 없다) jsdom 환경
// 테스트 파일이 전부 시작조차 못 하고 `npm test` 가 exit 1 이 됐다.
//
// 그래서 PATH 의 node 를 믿지 않고, npm 이 자기 자신을 돌리고 있는
// node(npm_node_execpath)로 로컬 bin 을 실행한다. npm 밖에서 직접 호출되면 이
// 스크립트를 띄운 node 를 쓴다. 어느 후보도 engines.node 하한을 넘기지 못하면
// 구버전에서 조용히 도는 대신 원인을 밝히고 멈춘다.
import { spawnSync } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const webRoot = fileURLToPath(new URL('..', import.meta.url))

function readJson(file) {
  return JSON.parse(fs.readFileSync(file, 'utf8'))
}

function parseVersion(text) {
  const match = /(\d+)\.(\d+)\.(\d+)/.exec(String(text ?? ''))
  return match ? [Number(match[1]), Number(match[2]), Number(match[3])] : null
}

function meetsFloor(version, floor) {
  if (!floor) return true
  if (!version) return false
  for (let i = 0; i < 3; i += 1) {
    if (version[i] !== floor[i]) return version[i] > floor[i]
  }
  return true
}

function nodeVersionOf(execPath) {
  if (execPath === process.execPath) return parseVersion(process.versions.node)
  const probe = spawnSync(execPath, ['--version'], { encoding: 'utf8' })
  if (probe.error || probe.status !== 0) return null
  return parseVersion(probe.stdout)
}

function resolveLocalBin(name) {
  const packageDir = path.join(webRoot, 'node_modules', name)
  const meta = readJson(path.join(packageDir, 'package.json'))
  const entry = typeof meta.bin === 'string' ? meta.bin : meta.bin?.[name]
  if (!entry) throw new Error(`${name} 패키지에 ${name} bin 항목이 없습니다`)
  return path.join(packageDir, entry)
}

const [binName, ...args] = process.argv.slice(2)
if (!binName) {
  console.error('usage: node scripts/run-with-supported-node.mjs <local-bin> [args...]')
  process.exit(64)
}

const pkg = readJson(path.join(webRoot, 'package.json'))
const floor = parseVersion(pkg.engines?.node)

// npm 이 쓰는 node 를 먼저 보고, 없거나 하한 미달이면 이 스크립트를 띄운 node 를 본다.
const candidates = [...new Set([process.env.npm_node_execpath, process.execPath].filter(Boolean))]
const seen = []
let interpreter = ''
for (const candidate of candidates) {
  const version = nodeVersionOf(candidate)
  seen.push(`${candidate} → ${version ? version.join('.') : '확인 불가'}`)
  if (meetsFloor(version, floor)) {
    interpreter = candidate
    break
  }
}

if (!interpreter) {
  console.error(
    `${binName} 을 실행할 수 있는 node 를 찾지 못했습니다. ` +
      `package.json 의 engines.node 는 ${pkg.engines?.node} 입니다.\n` +
      `확인한 후보:\n  ${seen.join('\n  ')}\n` +
      'npm 은 상위 디렉터리의 node_modules/.bin 을 PATH 앞에 붙이므로 ' +
      '거기에 설치된 구버전 node 가 PATH 를 가로챌 수 있습니다.',
  )
  process.exit(1)
}

const result = spawnSync(interpreter, [resolveLocalBin(binName), ...args], {
  cwd: process.cwd(),
  stdio: 'inherit',
})
if (result.error) {
  console.error(`${binName} 실행 실패: ${result.error.message}`)
  process.exit(1)
}
process.exit(result.status ?? 1)
