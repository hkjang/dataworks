// 저장소 루트의 package.json 이 받은 검증 스크립트를 web/ 에서 그대로 실행한다.
//
// `.github/workflows/ci.yml` 의 web 잡은 `defaults.run.working-directory: web` 에 의존해
// `npm ci`·`npm run lint`·`npm test`·`npm run build` 를 돌린다. 그래서 그 명령들을 CI 에서
// 뽑아 저장소 루트에서 돌리는 러너·릴리즈 자동화에게는 스크립트가 없는 것처럼 보였고,
// npm 이 상위 디렉터리로 올라가 저장소 밖의 package.json 을 프로젝트 루트로 집어
// `Missing script: "lint"` 로 exit 1 이 됐다(저장소 밖의 남의 스크립트가 실행될 수도 있다).
//
// 이 스크립트는 web 의 스크립트를 **완화 없이 그대로** 부르는 위임 계층이다. 검증 강도는
// web/package.json 과 web/eslint.config.js 에만 있고 여기서 플래그를 더하지 않는다.
//
// npm 은 run-script 의 PATH 앞에 상위 디렉터리의 node_modules/.bin 을 전부 붙이므로
// (web/scripts/run-with-supported-node.mjs 의 주석 참고) PATH 의 node·npm 을 믿지 않고
// npm 자신이 알려 준 경로(npm_execpath)와 현재 실행 중인 node(process.execPath)를 쓴다.
import { spawnSync } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const repoRoot = fileURLToPath(new URL('..', import.meta.url))
const webRoot = path.join(repoRoot, 'web')

// 허용 목록 — 루트에서 web 의 임의 스크립트를 돌릴 수 있게 열어 두지 않는다.
const ALLOWED = new Set(['lint', 'test', 'build'])

const script = process.argv[2]
if (!script || !ALLOWED.has(script)) {
  console.error(
    `usage: node scripts/web-run.mjs <${[...ALLOWED].join('|')}>\n` +
      `받은 값: ${script ?? '(없음)'}`,
  )
  process.exit(64)
}

// 이 스크립트 자신은 루트 package.json 의 `node scripts/web-run.mjs …` 로 불리므로
// 가로채인 PATH 의 node 로 돌고 있을 수 있다(실측: process.execPath 가 Node 20.19.2 심,
// npm_node_execpath 는 nvm 의 22.23.1). 그래서 자식 npm 에는 process.execPath 를 넘기지
// 않고 npm 자신이 돌고 있는 node 를 먼저 쓴다 — 그러지 않으면 구버전 node 가 web 의
// 스크립트까지 전파되어 web/scripts/run-with-supported-node.mjs 가 engines.node 하한
// 미달로 멈춘다(= web 테스트가 돌지 못한다).
const npmExecPath = process.env.npm_execpath
const nodeExecPath = process.env.npm_node_execpath || process.execPath

// npm 밖에서 직접 호출되면 PATH 의 npm 으로 폴백한다(그때는 node 도 PATH 의 것이다).
function runInWeb(args) {
  const spawned = npmExecPath
    ? spawnSync(nodeExecPath, [npmExecPath, ...args], { cwd: webRoot, stdio: 'inherit' })
    : spawnSync('npm', args, { cwd: webRoot, stdio: 'inherit', shell: true })
  if (spawned.error) {
    console.error(`npm ${args.join(' ')} 실행 실패: ${spawned.error.message}`)
    process.exit(1)
  }
  return spawned.status ?? 1
}

// 의존성이 없을 때만 설치한다. 매번 돌리면 검증이 몇 분씩 길어지고 lock 문제를 가린다.
// 설치 실패는 삼키지 않고 그 exit code 로 즉시 끝낸다 — CI 의 Install step 과 같은 명령이다.
if (!fs.existsSync(path.join(webRoot, 'node_modules'))) {
  const installStatus = runInWeb(['ci'])
  if (installStatus !== 0) process.exit(installStatus)
}

// cwd 를 web 으로 둔 채 web 의 스크립트를 부른다. 루트 cwd 에서 같은 이름을 다시 부르면
// 무한 재귀가 된다.
process.exit(runInWeb(['run', script]))
