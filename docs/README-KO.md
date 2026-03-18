# springlog

> Spring Boot 로그 분석 CLI — 여러 프로젝트의 로그 파일을 필터링, 검색, 통계 분석합니다.

[![CI](https://github.com/bdgom73/springlog/actions/workflows/ci.yml/badge.svg)](https://github.com/bdgom73/springlog/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](../LICENSE)
[![Release](https://img.shields.io/github/v/release/bdgom73/springlog)](https://github.com/bdgom73/springlog/releases/latest)

**English documentation:** [README.md](../README.md)

---

## 주요 기능

- **로그 형식 자동 감지** — 확장자가 아닌 파일 내용으로 text(log4j/slf4j) / JSON 자동 판별
- **멀티 프로젝트 지원** — `--all-projects` 옵션으로 여러 프로젝트 동시 분석
- **다양한 필터** — 로그 레벨, 시간 범위, 키워드/정규식
- **통계 분석** — 레벨별 분포, 예외 그룹, 에러 스파이크 감지, 응답시간 백분위수(p50/p95/p99)
- **예외 분석** — 예외 타입별 그룹화, 스택 트레이스 미리보기
- **트레이스 추적** — Trace ID로 MSA 전체 요청 흐름 조회
- **실시간 감시** — `tail` 명령, 로그 로테이션 자동 처리
- **인터랙티브 대시보드** — TUI 기반 실시간 필터링 대시보드
- **Go 설치 불필요** — 단일 바이너리, 런타임 없이 바로 실행

---

## 설치 방법

### 바이너리 다운로드 (권장)

[Releases 페이지](https://github.com/bdgom73/springlog/releases/latest)에서 운영체제에 맞는 파일을 다운로드합니다.

| 운영체제 | 다운로드 파일 |
|----------|--------------|
| Windows (64bit) | `springlog-vX.X.X-windows-amd64.exe` |
| macOS Intel | `springlog-vX.X.X-darwin-amd64` |
| macOS Apple Silicon (M1·M2·M3) | `springlog-vX.X.X-darwin-arm64` |
| Linux (64bit) | `springlog-vX.X.X-linux-amd64` |
| Linux (ARM) | `springlog-vX.X.X-linux-arm64` |

---

### Windows 실행 방법

> ⚠️ **`.exe` 파일을 더블클릭하면 창이 바로 닫힙니다.**
> 이 도구는 CLI(명령줄) 도구이기 때문에 반드시 **PowerShell** 또는 **명령 프롬프트(CMD)** 에서 실행해야 합니다.

**Step 1.** 파일을 다운로드하고 편한 경로에 저장합니다. 파일명을 `springlog.exe`로 변경하면 편합니다.

```
예) C:\tools\springlog.exe
```

**Step 2.** PowerShell을 실행합니다.

```
시작 버튼 → "PowerShell" 검색 → 실행
```

**Step 3.** 파일이 있는 폴더로 이동해서 실행합니다.

```powershell
cd C:\tools
.\springlog.exe --help
```

**Step 4 (선택).** 어디서든 실행하려면 PATH에 등록합니다.

```powershell
# 영구 등록 (PowerShell을 관리자 권한으로 실행 후)
[Environment]::SetEnvironmentVariable("PATH", $env:PATH + ";C:\tools", "Machine")

# 이후 새 PowerShell 창에서 어디서든 실행 가능
springlog --help
```

> **Windows SmartScreen 경고가 뜨는 경우**
> 팝업에서 **"추가 정보"** → **"실행"** 을 클릭하면 됩니다.
> 이 바이너리는 코드 서명이 없는 오픈소스 소프트웨어이기 때문에 경고가 표시될 수 있습니다.

---

### macOS 실행 방법

**Step 1.** 다운로드 후 터미널을 열고 실행 권한을 부여합니다.

```bash
# Apple Silicon (M1/M2/M3) 예시
chmod +x springlog-v0.1.0-darwin-arm64

# 파일명을 springlog로 변경
mv springlog-v0.1.0-darwin-arm64 springlog
```

**Step 2.** 실행합니다.

```bash
./springlog --help
```

**Step 3 (선택).** 어디서든 실행하려면 PATH에 추가합니다.

```bash
sudo mv springlog /usr/local/bin/springlog
springlog --help
```

> **"개발자를 확인할 수 없음" 경고가 뜨는 경우**
> ```bash
> xattr -d com.apple.quarantine springlog
> ```
> 위 명령을 실행한 뒤 다시 시도하세요.

---

### Linux 실행 방법

```bash
chmod +x springlog-v0.1.0-linux-amd64
sudo mv springlog-v0.1.0-linux-amd64 /usr/local/bin/springlog
springlog --help
```

---

### 소스코드로 직접 빌드

Go가 설치되어 있다면 직접 빌드할 수 있습니다.

```bash
git clone https://github.com/bdgom73/springlog.git
cd springlog
go build -o springlog ./cmd/springlog

# Windows
go build -o springlog.exe ./cmd/springlog
```

---

## 로그 파일 구조

springlog는 아래와 같이 **프로젝트별 디렉토리**로 분리된 구조를 기대합니다.

```
logs/
├── project-a/
│   ├── app.log               ← 현재 로그
│   ├── app.log.2024-01-14    ← 로테이션된 로그 (자동 포함)
│   └── app.log.2024-01-15
├── project-b/
│   └── app.log
└── project-c/
    └── app.json              ← JSON 형식도 지원
```

> 파일 확장자(`.log`, `.json` 등)는 무관합니다. 파일 내용을 분석해서 형식을 자동 감지합니다.

---

## 사용법

### analyze — 로그 조회 및 필터링

```bash
# 특정 프로젝트에서 ERROR 이상 조회
springlog analyze ./logs/project-a/ -l ERROR

# 모든 프로젝트 동시 분석
springlog analyze ./logs/ --all-projects -l ERROR

# 최근 24시간, 키워드 검색
springlog analyze ./logs/project-a/ --from -24h -s "NullPointerException"

# 최근 7일, 특정 프로젝트, JSON 출력
springlog analyze ./logs/ --all-projects -p project-a --from -7d -o json | jq .

# 정규식 검색
springlog analyze ./logs/ -s "timeout after \d+ms"

# 특정 시간대 조회
springlog analyze ./logs/ --from "2024-01-15 09:00:00" --to "2024-01-15 10:00:00"
```

### stats — 통계 요약 리포트

```bash
# 단일 프로젝트 통계
springlog stats ./logs/project-a/

# 전체 프로젝트, 상위 20개 에러 그룹
springlog stats ./logs/ --all-projects --top-errors 20

# 최근 7일 에러 통계, 6시간 단위 히스토그램
springlog stats ./logs/ -l ERROR --from -7d --bucket-size 6h
```

### tail — 실시간 로그 감시

```bash
# 파일 실시간 감시 (새 로그가 추가될 때마다 출력)
springlog tail ./logs/project-a/app.log

# WARN 이상만 필터링
springlog tail ./logs/project-a/app.log -l WARN

# 특정 키워드 포함 로그만 표시
springlog tail ./logs/project-a/app.log -s "Exception"
```

### exceptions — 예외 분석

```bash
# 예외 유형별 발생 통계
springlog exceptions ./logs/project-a/

# 전체 스택 트레이스 출력
springlog exceptions ./logs/project-a/ --show-stack

# 5회 이상 발생한 예외만 표시
springlog exceptions ./logs/ --all-projects --min-count 5
```

### trace — 트레이스 ID로 요청 추적

Micrometer / Spring Cloud Sleuth로 생성된 traceId를 기준으로 특정 요청의 전체 흐름을 조회합니다.

```bash
springlog trace ./logs/ --trace-id 4bf92f3577b34da6a3ce929d0e0e4736
```

### dashboard — 인터랙티브 대시보드

터미널 안에서 GUI처럼 동작하는 대시보드입니다. 키보드로 필터를 조작하면 즉시 결과가 갱신됩니다.

```bash
# 단일 프로젝트 대시보드
springlog dashboard ./logs/project-a/

# 모든 프로젝트
springlog dashboard ./logs/ --all-projects
```

**키보드 조작:**

| 키 | 동작 |
|----|------|
| `/` | 키워드 검색 입력 |
| `L` | 로그 레벨 필터 순환 (ALL → ERROR → WARN → ...) |
| `T` | 시간 범위 필터 순환 (ALL → 1h → 24h → 7d → ...) |
| `P` | 프로젝트 필터 순환 |
| `Esc` | 필터 전체 초기화 |
| `Tab` / `←` `→` | 패널 이동 (Summary / Exceptions / Latency / Errors) |
| `↑` `↓` | 현재 패널 스크롤 |
| `R` | 디스크에서 로그 새로고침 |
| `Q` | 대시보드 종료 |

---

## 전역 옵션

| 옵션 | 단축키 | 기본값 | 설명 |
|------|--------|--------|------|
| `--output` | `-o` | `table` | 출력 형식: `table` \| `json` \| `text` |
| `--level` | `-l` | — | 최소 레벨: `TRACE` `DEBUG` `INFO` `WARN` `ERROR` `FATAL` |
| `--from` | | — | 시작 시간: `-1h`, `-30m`, `-7d`, `yesterday`, `today`, `2024-01-15` |
| `--to` | | — | 종료 시간 (동일 형식) |
| `--search` | `-s` | — | 메시지 키워드 또는 정규식 |
| `--search-fields` | | `message` | 검색 대상 필드: `message`, `logger`, `thread`, `raw` |
| `--project` | `-p` | — | 프로젝트 이름 필터 (여러 개: `-p a -p b`) |
| `--trace-id` | | — | Trace ID 필터 |
| `--mdc` | | — | MDC 필드 필터 (예: `--mdc userId=1234`) |
| `--no-color` | | `false` | 컬러 출력 비활성화 |

---

## 지원 로그 형식

### Spring Boot 텍스트 형식 (log4j / slf4j)

```
2024-01-15 10:23:45.123 ERROR 12345 --- [http-nio-8080-exec-1] c.example.MyClass : Something failed
java.lang.NullPointerException: null
    at c.example.MyClass.method(MyClass.java:42)
    ... 10 more
```

traceId/spanId가 포함된 형식도 지원합니다.

```
2024-01-15 09:10:01.001 INFO 12345 --- [exec-1] [traceId=4bf92f357 spanId=00f067aa] c.example.OrderController : POST /api/orders
```

### JSON 형식 (Logback structured logging)

```json
{
  "@timestamp": "2024-01-15T10:23:45.123Z",
  "level": "ERROR",
  "logger": "c.example.MyClass",
  "message": "Something failed",
  "traceId": "4bf92f3577b34da6a3ce929d0e0e4736"
}
```

---

## 기여하기

1. 먼저 [이슈](https://github.com/bdgom73/springlog/issues)를 등록해 논의합니다
2. 저장소를 Fork합니다
3. 기능 브랜치 생성: `git checkout -b feat/my-feature`
4. 커밋 메시지는 Conventional Commits 형식을 따릅니다

   ```
   feat: 새로운 기능 추가
   fix: 버그 수정
   docs: 문서 수정
   test: 테스트 추가
   chore: 빌드/설정 변경
   ```

5. PR을 올립니다 — PR 제목도 동일한 형식이어야 합니다

자세한 내용은 [CONTRIBUTING.md](../CONTRIBUTING.md)를 참고하세요.

---

## 라이선스

[MIT](../LICENSE) © 2026 BDGOM73
