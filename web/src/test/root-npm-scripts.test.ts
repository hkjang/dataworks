// @vitest-environment node
import fs from 'node:fs'
import path from 'node:path'
import { spawnSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

// npm 은 package.json 을 찾을 때 cwd 에서 위로 올라간다. 저장소 루트에 package.json 이
// 없으면 npm 은 저장소 밖(홈 디렉터리 등)의 package.json 을 프로젝트 루트로 집는다.
// 그러면 CI 의 web 잡에서 뽑아 온 `npm run lint` 를 저장소 루트에서 돌리는 러너·
// 릴리즈 자동화에게는 `Missing script: "lint"` 로 영구히 exit 1 이고, 동시에 저장소
// 밖의 남의 스크립트가 실행될 수 있다. 문자열 검사가 아니라 실제로 실행해서 확인한다.
const repoRoot = fileURLToPath(new URL('../../..', import.meta.url))

// npm 이 PATH 앞에 상위 node_modules/.bin 을 붙이므로(web/scripts/run-with-supported-node.mjs
// 의 주석 참고) PATH 의 npm 을 믿지 않고 npm 자신이 알려 준 경로를 쓴다.
//
// 릴리즈 게이트는 이 스위트를 `npm test --silent` 로 돌린다. 그러면 바깥 npm 이
// npm_config_loglevel=silent 를 환경에 넣고, 여기서 띄우는 자식 npm 이 그것을 물려받아
// `> dataworks-web@0.1.0 lint` / `> eslint .` 배너를 통째로 삼킨다 — eslint 가 깨끗하면
// 출력이 빈 문자열이 되어 "eslint 가 실제로 돌았다" 는 단정이 깨졌다(= 러너가 보던
// `cd web && npm test --silent` exit 1). 단정을 약하게 만드는 대신, 관찰 대상인 자식의
// 로그 수준을 테스트가 직접 고정한다 — 바깥에서 어떻게 불렸든 같은 것을 본다.
function runNpm(args: string[]) {
  const execpath = process.env.npm_execpath
  const env = { ...process.env }
  delete env.npm_config_loglevel
  const argv = [...args, '--loglevel=notice']
  const result = execpath
    ? spawnSync(process.execPath, [execpath, ...argv], {
        cwd: repoRoot,
        encoding: 'utf8',
        env,
        timeout: 180_000,
      })
    : spawnSync('npm', argv, {
        cwd: repoRoot,
        encoding: 'utf8',
        env,
        shell: true,
        timeout: 180_000,
      })
  return {
    status: result.status,
    output: `${result.stdout ?? ''}${result.stderr ?? ''}`,
  }
}

describe('repository root npm scripts', () => {
  it('resolves the project root to this repository instead of escaping to an ancestor', () => {
    const { status, output } = runNpm(['prefix'])

    expect(status).toBe(0)
    expect(fs.realpathSync(output.trim())).toBe(fs.realpathSync(repoRoot))
  })

  it('runs the web eslint from the repository root', () => {
    const { status, output } = runNpm(['run', 'lint'])

    expect(output).not.toContain('Missing script')
    expect(status).toBe(0)
    // web 의 lint 스크립트가 실제로 돌았다는 증거 — 루트가 자체 린터를 흉내내면 안 된다.
    expect(output).toContain('eslint')
  })

  it('delegates lint to web without loosening it', () => {
    const rootPkg = JSON.parse(fs.readFileSync(path.join(repoRoot, 'package.json'), 'utf8'))
    const webPkg = JSON.parse(fs.readFileSync(path.join(repoRoot, 'web', 'package.json'), 'utf8'))

    // 루트는 위임만 한다: web 의 스크립트 본문을 복제하거나 완화 플래그를 끼워 넣으면
    // 루트와 web 의 검증 강도가 갈라진다.
    for (const name of ['lint', 'test', 'build']) {
      expect(webPkg.scripts[name]).toBeTruthy()
      expect(rootPkg.scripts[name]).toBeTruthy()
      expect(rootPkg.scripts[name]).not.toMatch(
        /\|\|\s*true|--no-error-on-unmatched-pattern|--max-warnings|exit 0|--passWithNoTests/,
      )
    }
    // workspaces 선언은 node_modules 호이스팅과 CI 의 cache-dependency-path 전제를 깬다.
    expect(rootPkg.workspaces).toBeUndefined()
  })
})
