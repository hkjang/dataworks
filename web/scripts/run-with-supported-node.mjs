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
//
// 그 두 후보만으로는 부족한 경우가 둘 있고, 아래에서 각각 막는다.
//  (1) npm 자신이 이미 가로채인 구버전 node 로 떠 있으면 두 후보가 같은 구버전이라
//      하한을 넘기지 못한다(실측: `npx vitest` 로 들어오면 process.execPath 와
//      npm_node_execpath 가 모두 홈 디렉터리의 Node 20.19.2 심이 된다). 그래서
//      시스템에 설치된 node 들(nvm 버전 디렉터리 → /usr/local/bin → /usr/bin)까지
//      후보로 넓힌다. 상위 node_modules/.bin 은 문제의 근원이므로 후보에 넣지 않는다.
//  (2) 새 워크트리에는 web/node_modules 가 아예 없다. 예전에는 로컬 bin 의
//      package.json 을 그대로 readFileSync 해서 ENOENT 가 스택 트레이스와 함께
//      터지며 exit 1 로 죽었다(= 릴리즈 러너가 보던 그 exit 1). 이제 bin 이 없으면
//      고른 interpreter 로 `npm ci` 를 한 번 돌리고, 설치 실패는 그 종료 코드로
//      그대로 드러낸다. 이미 설치돼 있으면 아무 것도 설치하지 않는다.
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

// 설치돼 있지 않으면 빈 문자열 — 호출부가 설치를 돌린 뒤 다시 묻는다.
function resolveLocalBin(name) {
  const packageDir = path.join(webRoot, 'node_modules', name)
  const metaFile = path.join(packageDir, 'package.json')
  if (!fs.existsSync(metaFile)) return ''
  const meta = readJson(metaFile)
  const entry = typeof meta.bin === 'string' ? meta.bin : meta.bin?.[name]
  if (!entry) throw new Error(`${name} 패키지에 ${name} bin 항목이 없습니다`)
  return path.join(packageDir, entry)
}

function executable(file) {
  try {
    fs.accessSync(file, fs.constants.X_OK)
    return fs.statSync(file).isFile()
  } catch {
    return false
  }
}

// 시스템에 설치된 node 후보들. nvm 의 버전 디렉터리 + 흔한 설치 경로.
// 상위 node_modules/.bin 은 넣지 않는다 — 그게 애초에 이 스크립트가 있는 이유다.
function systemNodeCandidates() {
  const found = []
  const nvmDir = process.env.NVM_DIR
  if (nvmDir) {
    const base = path.join(nvmDir, 'versions', 'node')
    let entries = []
    try {
      entries = fs.readdirSync(base)
    } catch {
      entries = []
    }
    for (const name of entries) {
      if (parseVersion(name)) found.push(path.join(base, name, 'bin', 'node'))
    }
  }
  found.push('/usr/local/bin/node', '/usr/bin/node')
  return found.filter(executable)
}

function compareVersions(a, b) {
  for (let i = 0; i < 3; i += 1) {
    if (a[i] !== b[i]) return a[i] - b[i]
  }
  return 0
}

const [binName, ...args] = process.argv.slice(2)
if (!binName) {
  console.error('usage: node scripts/run-with-supported-node.mjs <local-bin> [args...]')
  process.exit(64)
}

const pkg = readJson(path.join(webRoot, 'package.json'))
const floor = parseVersion(pkg.engines?.node)

const seen = []
function probe(candidate) {
  const version = nodeVersionOf(candidate)
  seen.push(`${candidate} → ${version ? version.join('.') : '확인 불가'}`)
  return version
}

// 1) 운영자가 고른 node 를 먼저 존중한다 — npm 이 자기를 돌리는 node, 그 다음 이
//    스크립트를 띄운 node. 하한만 넘기면 그대로 쓴다.
let interpreter = ''
const preferred = [...new Set([process.env.npm_node_execpath, process.execPath].filter(Boolean))]
for (const candidate of preferred) {
  if (meetsFloor(probe(candidate), floor)) {
    interpreter = candidate
    break
  }
}

// 2) 둘 다 하한 미달이면(= npm 자신까지 구버전 node 에 가로채인 상태) 시스템에 설치된
//    node 중에서 고른다. 이때는 **하한을 넘기는 가장 낮은 버전**을 쓴다. 최신을 집으면
//    이 저장소가 한 번도 돌아 본 적 없는 런타임으로 검증이 넘어간다 — 실측으로 이
//    머신의 Node 25(25.0.0·25.9.0)에서는 src/features/auth/silent-sso.test.ts 가
//    깨지고 22.23.1·23.11.1 에서는 통과한다. engines.node 는 하한 선언이지 "최신을
//    쓰라" 는 뜻이 아니므로, 강요당해 고를 때는 가장 보수적인 쪽을 고른다.
if (!interpreter) {
  const usable = []
  for (const candidate of systemNodeCandidates()) {
    if (preferred.includes(candidate)) continue
    const version = probe(candidate)
    if (meetsFloor(version, floor)) usable.push({ candidate, version })
  }
  usable.sort((a, b) => compareVersions(a.version, b.version))
  interpreter = usable[0]?.candidate ?? ''
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

// 로컬 bin 이 없으면 — 러너가 막 만든 워크트리다 — 고른 interpreter 로 한 번 설치한다.
let binPath = resolveLocalBin(binName)
if (!binPath) {
  console.error(
    `web/node_modules 에 ${binName} 이 없어 먼저 npm ci 를 돌립니다 (${path.join(webRoot, 'node_modules')}).`,
  )
  const npmExecPath = process.env.npm_execpath
  // npm 밖에서 직접 호출되면 PATH 의 npm 으로 폴백한다.
  const install = npmExecPath
    ? spawnSync(interpreter, [npmExecPath, 'ci'], { cwd: webRoot, stdio: 'inherit' })
    : spawnSync('npm', ['ci'], { cwd: webRoot, stdio: 'inherit', shell: true })
  if (install.error) {
    console.error(`npm ci 실행 실패: ${install.error.message}`)
    process.exit(1)
  }
  // 설치 실패를 삼키지 않는다 — 네트워크·캐시 문제를 그 종료 코드로 그대로 드러낸다.
  if (install.status !== 0) process.exit(install.status ?? 1)

  binPath = resolveLocalBin(binName)
  if (!binPath) {
    console.error(
      `npm ci 는 성공했지만 ${binName} 이 여전히 없습니다. ` +
        `web/package.json 의 의존성에 ${binName} 이 있는지, ` +
        'npm ci 가 devDependencies 를 건너뛰도록 설정돼 있지 않은지(--omit=dev, NODE_ENV=production) 확인하세요.',
    )
    process.exit(1)
  }
}

const result = spawnSync(interpreter, [binPath, ...args], {
  cwd: process.cwd(),
  stdio: 'inherit',
})
if (result.error) {
  console.error(`${binName} 실행 실패: ${result.error.message}`)
  process.exit(1)
}
process.exit(result.status ?? 1)
