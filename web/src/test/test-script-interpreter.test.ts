// @vitest-environment node
import { spawnSync } from 'node:child_process'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { afterEach, expect, it } from 'vitest'

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
