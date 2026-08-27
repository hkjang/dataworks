#!/usr/bin/env bash
# 오프라인 배포용 Data Works Docker 이미지를 빌드하고 tar.gz로 패키징한다.
#
# 사용법:
#   ./scripts/release.sh [-v VERSION] [-i IMAGE] [-p PLATFORM]
#
# 예:
#   ./scripts/release.sh -v v0.1.0
#   ./scripts/release.sh -v v0.1.0 -p linux/arm64
set -euo pipefail

IMAGE="dataworks"
PLATFORM="linux/amd64"
VERSION=""

while getopts ":v:i:p:h" opt; do
    case "$opt" in
        v) VERSION="$OPTARG" ;;
        i) IMAGE="$OPTARG" ;;
        p) PLATFORM="$OPTARG" ;;
        h)
            sed -n '2,12p' "$0"
            exit 0
            ;;
        \?) echo "알 수 없는 옵션: -$OPTARG" >&2; exit 2 ;;
    esac
done

if ! command -v docker >/dev/null 2>&1; then
    echo "docker 가 PATH 에 없습니다." >&2
    exit 1
fi

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

if [[ -z "$VERSION" ]]; then
    STAMP="$(date +%Y%m%d-%H%M)"
    if SHORT_SHA="$(git rev-parse --short HEAD 2>/dev/null)"; then
        VERSION="${STAMP}-${SHORT_SHA}"
    else
        VERSION="${STAMP}-nogit"
    fi
fi

TAG="${IMAGE}:${VERSION}"
SAFE_VERSION="$(echo "$VERSION" | sed 's/[^A-Za-z0-9._-]/_/g')"
RELEASE_DIR="${REPO_ROOT}/release"
mkdir -p "$RELEASE_DIR"

TAR_PATH="${RELEASE_DIR}/${IMAGE}-${SAFE_VERSION}.tar"
GZ_PATH="${TAR_PATH}.gz"

echo "[1/3] docker build $TAG (platform=$PLATFORM)"
docker build \
    --platform "$PLATFORM" \
    --build-arg "VERSION=${VERSION}" \
    -t "$TAG" \
    -f Dockerfile \
    .

echo "[2/3] docker save -> $TAR_PATH"
docker save -o "$TAR_PATH" "$TAG"

echo "[3/3] gzip 압축 -> $GZ_PATH"
gzip -9 -f "$TAR_PATH"

echo
echo "릴리즈 완료"
echo "  이미지   : $TAG"
echo "  플랫폼   : $PLATFORM"
echo "  배포 파일: $GZ_PATH"
echo "  서비스 포트: 8080"
