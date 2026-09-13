// Package buildinfo 는 빌드를 식별하는 값들을 담는다. 링크할 때 주입된다.
// 떠 있는 프로세스에서 "무엇이 돌고 있는가" 를 답할 수 있어야 장애를
// 쫓을 수 있다.
//
//	go build -ldflags "-X dataworks/internal/buildinfo.Version=v0.9.53 \
//	                   -X dataworks/internal/buildinfo.Commit=<sha> \
//	                   -X dataworks/internal/buildinfo.BuildTime=<RFC3339>"
package buildinfo

// 값이 들어오지 않은 빌드는 그렇다고 말한다. 조용히 빈 문자열을 내보내면
// 로그를 보는 쪽이 "버전이 없다" 와 "버전을 못 읽었다" 를 구분할 수 없다.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)
