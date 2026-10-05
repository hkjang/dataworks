// @vitest-environment node
import { spawnSync } from 'node:child_process'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, describe, expect, it } from 'vitest'

// npm 은 run-script 의 PATH 앞에 상위 디렉터리의 node_modules/.bin 을 전부 붙인다
// (프로젝트 → … → /home/<user> → /). 그 중 하나에 `node` 심링크가 있으면 그 node 가
// nvm·CI 의 node 를 가리고, 테스트 러너가 package.json 의 engines 보다 낮은 런타임에서
// 기동된다. 그러면 jsdom 이 끌어오는 undici 가 로드 시점에 죽어
// (`webidl.util.markAsUncloneable is not a function`) jsdom 환경 테스트 파일이 전부
// 시작조차 못 하고 `npm test` 가 exit 1 이 된다.
//
// 그래서 여기서는 PATH 맨 앞에 `node` 라는 이름의 probe 를 끼워 넣고 — probe 는 실제
// node 로 그대로 넘기면서 자기가 무엇을 실행했는지만 기록한다 — package.json 의 test
// 스크립트를 그대로 돌려, 러너가 PATH 의 node 로 기동되지 않는지 확인한다.
const webRoot = fileURLToPath(new URL('../..', import.meta.url))

let tmpDir = ''

afterEach(() => {
  if (tmpDir) fs.rmSync(tmpDir, { recursive: true, force: true })
  tmpDir = ''
})

it(
  'starts the test runner with npm own node, not whatever node comes first on PATH',
  () => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'dataworks-node-probe-'))
    const log = path.join(tmpDir, 'invocations.log')
    const probeDir = path.join(tmpDir, 'bin')
    fs.mkdirSync(probeDir)
    fs.writeFileSync(
      path.join(probeDir, 'node'),
      `#!/bin/sh\nprintf '%s\\n' "$*" >> ${JSON.stringify(log)}\nexec ${JSON.stringify(process.execPath)} "$@"\n`,
      { mode: 0o755 },
    )

    // 실제 릴리즈 검증이 돌리는 그 스크립트 문자열을 읽어서 쓴다. 중첩 실행이 이
    // 파일을 다시 집어 재귀하지 않도록 jsdom 환경 파일 하나로 필터를 건다
    // (labels.ko 는 순수 문자열 테스트라서 빠르고, 환경은 jsdom 이라 실패를 재현한다).
    const pkg = JSON.parse(fs.readFileSync(path.join(webRoot, 'package.json'), 'utf8')) as {
      scripts: Record<string, string>
    }
    const run = spawnSync('sh', ['-c', `${pkg.scripts.test} src/lib/labels.ko.test.ts`], {
      cwd: webRoot,
      encoding: 'utf8',
      env: {
        ...process.env,
        PATH: [probeDir, path.join(webRoot, 'node_modules', '.bin'), process.env.PATH].join(
          path.delimiter,
        ),
        npm_node_execpath: process.execPath,
      },
    })

    const invocations = fs.existsSync(log)
      ? fs.readFileSync(log, 'utf8').split('\n').filter(Boolean)
      : []
    // 각 줄의 첫 토큰이 probe 가 실행한 스크립트다. 러너(vitest)가 그 중에 있으면
    // PATH 의 node 로 기동된 것이다.
    const launchedByPathNode = invocations.map((line) => line.split(' ')[0] ?? '')
    expect(launchedByPathNode.filter((script) => script.includes('vitest'))).toEqual([])
    expect(run.status, `${run.stdout}\n${run.stderr}`).toBe(0)
  },
  120_000,
)

// 위 테스트는 "이미 설치된 워크트리" 를 다룬다. 릴리즈 러너는 그보다 앞선 두 상태에서
// 같은 명령을 돌린다: (1) web/node_modules 가 아직 없는 새 워크트리, (2) npm 자신이
// 가로채인 구버전 node 로 떠 있는 셸. 두 경우 모두 런처가 "조용히 구버전에서 돌리기" 도
// "스택 트레이스를 내며 죽기" 도 하면 안 된다.
//
// 아래 사례는 실제 저장소를 건드리지 않도록 tmp 에 가짜 web 루트를 만들고 런처 파일만
// 그대로 복사해 돌린다(런처는 자기 파일 위치로 webRoot 를 정한다). node 후보는 버전만
// 보고하고 나머지는 진짜 node 로 넘기는 래퍼라 실행 경로까지 실제로 지나간다.
describe('run-with-supported-node launcher', () => {
  const launcher = path.join(webRoot, 'scripts', 'run-with-supported-node.mjs')
  const currentMajor = Number(process.versions.node.split('.')[0])

  function makeWebRoot(engines: string) {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'dataworks-launcher-'))
    const fakeWeb = path.join(tmpDir, 'web')
    fs.mkdirSync(path.join(fakeWeb, 'scripts'), { recursive: true })
    fs.copyFileSync(launcher, path.join(fakeWeb, 'scripts', 'run-with-supported-node.mjs'))
    fs.writeFileSync(
      path.join(fakeWeb, 'package.json'),
      JSON.stringify({ name: 'fake-web', private: true, engines: { node: engines } }),
    )
    return fakeWeb
  }

  // `npm ci` 대역 — 진짜 설치 대신 로컬 bin 하나를 만들고, 호출된 사실을 로그에 남긴다.
  function writeStubNpm(file: string, exitCode: number) {
    fs.writeFileSync(
      file,
      [
        "import fs from 'node:fs'",
        "import path from 'node:path'",
        "fs.appendFileSync(process.env.STUB_NPM_LOG, process.argv.slice(2).join(' ') + '\\n')",
        `if (${exitCode} !== 0) process.exit(${exitCode})`,
        "const dir = path.join(process.cwd(), 'node_modules', 'fakebin')",
        'fs.mkdirSync(dir, { recursive: true })',
        "fs.writeFileSync(path.join(dir, 'package.json'), JSON.stringify({ name: 'fakebin', bin: { fakebin: 'cli.mjs' } }))",
        "fs.writeFileSync(path.join(dir, 'cli.mjs'), \"console.log('fakebin ran on ' + process.version)\\n\")",
      ].join('\n'),
    )
  }

  function installFakeBin(fakeWeb: string) {
    const dir = path.join(fakeWeb, 'node_modules', 'fakebin')
    fs.mkdirSync(dir, { recursive: true })
    fs.writeFileSync(
      path.join(dir, 'package.json'),
      JSON.stringify({ name: 'fakebin', bin: { fakebin: 'cli.mjs' } }),
    )
    fs.writeFileSync(path.join(dir, 'cli.mjs'), "console.log('fakebin ran on ' + process.version)\n")
  }

  // 버전만 거짓으로 보고하고 실행은 진짜 node 에 넘기는 래퍼. 자기가 bin 을 띄웠다는
  // 사실을 로그에 남겨, 런처가 실제로 어느 후보로 돌렸는지를 실행으로 확인할 수 있게 한다.
  function writeNodeShim(file: string, reportedVersion: string, log: string) {
    fs.mkdirSync(path.dirname(file), { recursive: true })
    fs.writeFileSync(
      file,
      `#!/bin/sh\nif [ "$1" = "--version" ]; then echo "${reportedVersion}"; exit 0; fi\n` +
        `printf '%s\\n' "${reportedVersion}" >> ${JSON.stringify(log)}\n` +
        `exec ${JSON.stringify(process.execPath)} "$@"\n`,
      { mode: 0o755 },
    )
  }

  function runLauncher(fakeWeb: string, env: NodeJS.ProcessEnv) {
    return spawnSync(
      process.execPath,
      [path.join(fakeWeb, 'scripts', 'run-with-supported-node.mjs'), 'fakebin'],
      { cwd: fakeWeb, encoding: 'utf8', env: { ...process.env, ...env }, timeout: 120_000 },
    )
  }

  it('installs dependencies instead of crashing when the local bin is missing', () => {
    const fakeWeb = makeWebRoot(`>=${currentMajor}.0.0`)
    const stubNpm = path.join(tmpDir, 'stub-npm.mjs')
    const log = path.join(tmpDir, 'npm.log')
    fs.writeFileSync(log, '')
    writeStubNpm(stubNpm, 0)

    const run = runLauncher(fakeWeb, { npm_execpath: stubNpm, STUB_NPM_LOG: log })

    expect(run.stderr).not.toContain('ENOENT')
    expect(run.status, `${run.stdout}\n${run.stderr}`).toBe(0)
    expect(fs.readFileSync(log, 'utf8').trim().split('\n')).toEqual(['ci'])
    expect(run.stdout).toContain('fakebin ran on')
  })

  it('exits with the install status when the dependency install fails', () => {
    const fakeWeb = makeWebRoot(`>=${currentMajor}.0.0`)
    const stubNpm = path.join(tmpDir, 'stub-npm.mjs')
    const log = path.join(tmpDir, 'npm.log')
    fs.writeFileSync(log, '')
    writeStubNpm(stubNpm, 7)

    const run = runLauncher(fakeWeb, { npm_execpath: stubNpm, STUB_NPM_LOG: log })

    // 설치 실패를 삼켜 통과시키지도, 1 로 뭉개지도 않는다.
    expect(run.status, `${run.stdout}\n${run.stderr}`).toBe(7)
    expect(run.stdout).not.toContain('fakebin ran on')
  })

  it('does not reinstall when the local bin is already present', () => {
    const fakeWeb = makeWebRoot(`>=${currentMajor}.0.0`)
    const stubNpm = path.join(tmpDir, 'stub-npm.mjs')
    const log = path.join(tmpDir, 'npm.log')
    fs.writeFileSync(log, '')
    writeStubNpm(stubNpm, 0)
    installFakeBin(fakeWeb)

    const run = runLauncher(fakeWeb, { npm_execpath: stubNpm, STUB_NPM_LOG: log })

    expect(run.status, `${run.stdout}\n${run.stderr}`).toBe(0)
    expect(fs.readFileSync(log, 'utf8')).toBe('')
    expect(run.stdout).toContain('fakebin ran on')
  })

  it('falls back to an installed node when npm itself runs on an unsupported one', () => {
    // 하한을 현재 node 보다 높게 잡아 npm_node_execpath·process.execPath 두 후보를 모두
    // 떨어뜨린다 = 러너의 npm 까지 구버전 node 에 가로채인 상태.
    const fakeWeb = makeWebRoot(`>=${currentMajor + 1}.0.0`)
    installFakeBin(fakeWeb)
    const shimLog = path.join(tmpDir, 'shims.log')
    fs.writeFileSync(shimLog, '')
    const nvmDir = path.join(tmpDir, 'nvm')
    const nodeDir = path.join(nvmDir, 'versions', 'node')
    writeNodeShim(
      path.join(nodeDir, `v${currentMajor + 1}.0.0`, 'bin', 'node'),
      `v${currentMajor + 1}.0.0`,
      shimLog,
    )
    writeNodeShim(
      path.join(nodeDir, `v${currentMajor + 2}.0.0`, 'bin', 'node'),
      `v${currentMajor + 2}.0.0`,
      shimLog,
    )
    const oldNode = path.join(tmpDir, 'old-node')
    writeNodeShim(oldNode, 'v20.19.2', shimLog)

    const run = runLauncher(fakeWeb, { npm_node_execpath: oldNode, NVM_DIR: nvmDir })

    expect(run.status, `${run.stdout}\n${run.stderr}`).toBe(0)
    expect(run.stdout).toContain('fakebin ran on')
    // 하한을 넘기는 **가장 낮은** 후보로 bin 을 띄운다. 최신을 집으면 이 저장소가
    // 검증해 본 적 없는 런타임으로 넘어간다(실측: 이 머신의 Node 25 에서는
    // silent-sso 테스트가 깨진다). 구버전 후보는 --version 질의만 받고 bin 은 못 돌린다.
    expect(fs.readFileSync(shimLog, 'utf8').trim().split('\n')).toEqual([
      `v${currentMajor + 1}.0.0`,
    ])
  })

  it('refuses to run on an unsupported node instead of silently downgrading', () => {
    const fakeWeb = makeWebRoot('>=999.0.0')
    installFakeBin(fakeWeb)
    const oldNode = path.join(tmpDir, 'old-node')
    writeNodeShim(oldNode, 'v20.19.2', path.join(tmpDir, 'shims.log'))

    const run = runLauncher(fakeWeb, {
      npm_node_execpath: oldNode,
      NVM_DIR: path.join(tmpDir, 'nvm'),
    })

    expect(run.status).toBe(1)
    expect(run.stdout).not.toContain('fakebin ran on')
    expect(run.stderr).toContain('>=999.0.0')
    expect(run.stderr).toContain('node_modules/.bin')
  })
})
