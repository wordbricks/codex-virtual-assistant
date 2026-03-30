# PRD: WTL GAN Policy 기반 웹 업무용 개인 가상 비서

## 1. 문서 정보

- 프로젝트명: Codex Virtual Assistant
- 문서 버전: v0.2
- 작성일: 2026-03-27
- 대상 레포지토리: `/Users/dev/git/CodexVirtualAssistant`

## 2. 제품 개요

이 프로젝트의 목표는 비개발자인 CEO, PM, 운영 담당자 같은 사용자가 웹에서 반복적으로 하는 일을 자연어로 맡기면, 가능한 한 끝까지 수행하는 개인 가상 비서를 만드는 것이다.

이 비서는 OpenAI의 Codex app server를 실행 기반 런타임으로 사용하고, 기본 모델은 `gpt-5.4`를 사용한다.

핵심 차별점은 한 번 응답하고 끝나는 챗봇이 아니라, WhatTheLoop(WTL)의 GAN policy 구조를 사용해 결과를 반복 개선하는 실행형 비서라는 점이다.

- `Generator`: 실제 업무를 수행하고 산출물을 만든다.
- `Evaluator`: 결과가 사용자의 요청을 정말 충족했는지 검사한다.
- `Loop`: evaluator가 미흡하다고 판단하면 critique를 generator에 다시 전달해 작업을 이어서 수행한다.

제품의 핵심 가치는 "설명"이 아니라 "완료"다.

## 3. 대표 사용자

### 3.1 핵심 사용자

- CEO
- PM
- 운영 담당자
- 사업개발 담당자
- 고객 성공 담당자

### 3.2 사용자 특성

- 코드를 거의 또는 전혀 모른다.
- 웹 브라우저에서 여러 SaaS를 오가며 반복 업무를 수행한다.
- 상세한 구현 방식보다 결과가 끝났는지에 관심이 많다.
- 자료 수집, 정리, 입력, 확인, 요약, 보고 초안 작성 같은 업무 비중이 높다.

### 3.3 대표 업무 유형

- 여러 사이트를 확인해 정보 수집 후 요약하기
- 경쟁사 정보, 가격, 기능 비교표 만들기
- Notion, Slack, Google Docs, Sheets 사이의 반복 정리
- 대시보드 숫자를 모아 보고서 초안 만들기
- CRM 상태 확인 후 업데이트하기
- 반복적인 웹 입력과 상태 점검 수행하기

## 4. 문제 정의

기존 AI 비서나 챗봇은 다음 한계를 가진다.

- 사용자의 요청을 충분히 구조화하지 않은 채 바로 작업한다.
- 여러 웹 서비스와 단계를 오가는 반복 업무를 끝까지 수행하지 못한다.
- 결과가 미완성이어도 스스로 완료라고 선언한다.
- 실패 이유와 부족한 점을 명시적으로 점검하지 않는다.
- 사용자 승인, 로그인, 추가 정보 입력 같은 현실적 제약을 안정적으로 다루지 못한다.

이 프로젝트는 다음 방식으로 해결한다.

- 요청을 `TaskSpec`으로 구조화한다.
- generator와 evaluator를 분리한다.
- 완료 판정을 evaluator가 내리게 한다.
- 실패 시 critique 기반 재시도를 한다.
- wait, resume, approval 같은 현실적 흐름을 상태 머신 안에 포함한다.

## 5. 제품 목표

### 5.1 제품 목표

- 사용자가 자연어로 준 웹 업무 요청을 가능한 한 자율적으로 수행한다.
- 결과물이 완료 기준을 만족할 때까지 generator/evaluator 루프를 돈다.
- 작업 도중 필요한 브라우저 조작, SaaS 도구 사용, 문서 작성, 데이터 정리를 수행한다.
- 각 실행(run)의 상태, 시도, 산출물, 평가 결과를 추적 가능하게 저장한다.
- 비개발자도 이해할 수 있는 웹 UI로 진행 상황을 보여준다.

### 5.2 기술 목표

- OpenAI Codex app server 기반의 실행 런타임을 사용한다.
- 기본 모델을 `gpt-5.4`로 고정한다.
- WTL의 GAN policy를 구현한다.
- phase/thread/attempt/artifact/evaluation 기록을 영속화한다.
- wait/resume/cancel이 가능한 run 구조를 만든다.
- `agent-browser` CLI 기반의 실제 브라우저 조작을 우선 지원한다.

## 6. 비목표

- 범용 개인 일정 관리 앱 전체를 이번 버전에서 완성하는 것
- 모바일 앱 동시 개발
- 멀티 유저/멀티 테넌시 완전 지원
- 외부 결제/구독 시스템 구축
- 모든 외부 SaaS 연동을 1차 버전에 포함하는 것
- 범용 소프트웨어 개발 에이전트를 1차 목표로 삼는 것

## 7. 대표 시나리오

사용자는 다음과 같이 지시한다.

- "경쟁사 5곳 가격 정책을 조사해서 표로 정리해."
- "지난주 고객 문의를 분류해서 요약해."
- "Notion 문서와 Slack 메시지를 바탕으로 월간 보고 초안을 만들어."
- "채용 사이트에서 지원자 상태를 확인하고 스프레드시트 업데이트해."
- "이번 주 미팅 준비에 필요한 링크와 자료를 모아줘."

비서는 다음 흐름으로 동작한다.

1. 요청을 분석해 `TaskSpec`을 만든다.
2. Planner 단계에서 작업 계획과 완료 기준을 명문화한다.
3. Generator가 실제 업무를 수행한다.
4. Evaluator가 결과를 검증한다.
5. 미완료라면 critique를 generator에 다시 전달한다.
6. 완료 기준 충족 시 run을 종료한다.
7. 외부 입력이 필요하면 wait 상태로 전환한다.

## 8. 핵심 제품 요구사항

### 8.1 Run 관리

- 사용자는 새 작업(run)을 시작할 수 있어야 한다.
- 각 run은 고유 ID를 가져야 한다.
- run은 상태를 가져야 한다.
- 상태 예시:
  - `queued`
  - `planning`
  - `generating`
  - `evaluating`
  - `waiting`
  - `completed`
  - `failed`
  - `exhausted`
  - `cancelled`

### 8.2 TaskSpec 생성

시스템은 사용자의 자유형 요청을 최소한 아래 구조로 정리해야 한다.

- `goal`
- `user_request_raw`
- `deliverables`
- `constraints`
- `tools_allowed`
- `tools_required`
- `done_definition`
- `evidence_required`
- `risk_flags`
- `max_generation_attempts`

### 8.3 Generator

- generator는 실제 업무 수행 주체다.
- 조사, 요약, 문서 작성, 브라우저 조작, SaaS 도구 호출, 데이터 정리 등을 수행한다.
- 현재 상태와 evaluator critique를 반영해 후속 작업을 이어가야 한다.
- 산출물뿐 아니라 작업 근거도 남겨야 한다.

### 8.4 Evaluator

- evaluator는 generator와 독립된 평가 역할을 수행한다.
- 평가 결과는 구조화된 형태로 반환해야 한다.
- 최소 필드:
  - `passed: boolean`
  - `score: number`
  - `summary: string`
  - `missing_requirements: string[]`
  - `incorrect_claims: string[]`
  - `evidence_checked: string[]`
  - `next_action_for_generator: string`

평가 원칙:

- generator의 자기신고를 그대로 믿지 않는다.
- 실제 수집한 페이지 정보, 도구 호출 결과, 생성 문서, 최종 산출물을 기준으로 판단한다.
- 완료 정의(`done_definition`)를 기준으로만 통과 판정한다.

### 8.5 재시도 루프

- evaluator가 실패 판정 시 critique를 generator에 주입한다.
- generator는 critique를 반영해 다음 시도를 수행한다.
- 시도 횟수는 설정 가능해야 한다.
- 최대 시도 횟수 초과 시 `exhausted` 또는 `failed`로 종료한다.

### 8.6 사용자 개입

아래 상황에서는 사용자 입력이 필요할 수 있다.

- 비밀값, 인증 정보, 계정 연결
- 요구사항 모호성 해소
- 위험한 작업 승인
- 외부 서비스 로그인

이 경우 run은 `waiting` 상태가 되어야 하며, 입력이 들어오면 재개 가능해야 한다.

### 8.7 관찰성과 감사 가능성

모든 run은 아래 내용을 기록해야 한다.

- 사용자 원문 요청
- 정규화된 TaskSpec
- phase 전이
- 각 attempt별 prompt/response 요약
- tool 호출 목록
- 방문/조작한 웹 단계 요약
- evaluator 결과
- 최종 산출물

## 9. UX 원칙

- 사용자는 코드를 모른다는 전제에서 설명한다.
- 내부 phase나 모델 동작보다 "지금 무엇을 하고 있는지"를 먼저 보여준다.
- 기술적 세부사항은 숨기고, 필요한 경우에만 펼쳐서 보여준다.
- 위험한 작업은 쉬운 언어로 승인을 요청한다.
- 결과 화면은 "무엇을 했는지", "남은 것이 있는지", "사용자에게 필요한 다음 행동"을 분명히 보여줘야 한다.

## 10. 아키텍처

### 10.1 상위 구조

권장 구조는 다음과 같다.

- `web`: 사용자 UI
- `cmd/assistantd`: 서버 엔트리포인트
- `internal/assistant`: 애플리케이션 레이어
- `internal/wtl`: WTL engine/policy/runtime 구현
- `internal/store`: SQLite 영속화
- `internal/api`: HTTP/SSE API
- `internal/policy/gan`: GAN policy 구현
- `internal/prompting`: planner/generator/evaluator 프롬프트 빌더

주의:

- 현재 `specs/what-the-loop`는 참조 스펙/벤더링 자산으로 취급한다.
- 실제 제품 코드는 `specs/` 바깥 루트 계층에 새로 작성한다.

### 10.2 WTL 역할 분리

#### Engine

책임:

- turn 시작/종료
- iteration/retry 제한
- phase 전환 처리
- wait/compact/complete/fail/exhaust 처리
- observer 이벤트 방출

#### Policy

책임:

- 현재 phase 판단
- 다음 directive 결정
- generator/evaluator/planner용 prompt 생성 규칙 선택
- 완료 가능 여부 판단

#### Observer

책임:

- 로그
- UI 스트리밍
- 메트릭
- 감사 추적

observer는 실행 제어권을 갖지 않는다.

### 10.3 GAN Policy 상태 기계

1차 구현에서 따를 상태 흐름:

- `idle`
- `planning`
- `generating`
- `evaluating`
- `completed`
- `failed`
- `waiting`

기본 전이:

1. `idle -> planning`
2. `planning -> generating`
3. `generating -> evaluating`
4. `evaluating -> completed` if passed
5. `evaluating -> generating` if failed and attempts remain
6. `evaluating -> failed|exhausted` if attempts exhausted
7. any phase -> `waiting` when external input required

### 10.4 Thread 전략

WTL GAN spec에 맞춰 phase별 thread 전략을 분리한다.

- planner: 새 thread
- generator: attempt별 새 thread 권장
- evaluator: attempt별 새 thread

이유:

- generator/evaluator 간 컨텍스트 오염 방지
- 이전 실패 시도에 덜 끌려가게 함
- 평가 독립성 강화

단, 같은 attempt 안의 재시도는 같은 thread 재사용이 가능해야 한다.

## 11. OpenAI 및 실행 런타임 요구사항

### 11.1 모델

- 기본 모델: `gpt-5.4`

선정 이유:

- OpenAI 공식 문서 기준으로 Codex와 agentic workflow에서 최신 GPT-5 계열, 특히 `gpt-5.4` 사용이 권장된다.

### 11.2 API/런타임

1차 구현은 Codex app server 기반 런타임을 사용한다.

- 브라우저 조작, 로컬 명령 실행, 파일 생성은 Codex app server 기반 실행 계층이 담당한다.
- 장기 실행 작업은 background 처리 가능성을 열어둔다.
- phase 정보는 장기적으로 Responses API `phase`와 정합되게 설계한다.

### 11.3 Browser Execution Layer

외부 서비스 연동의 1차 우선순위는 `agent-browser` CLI를 이용한 실제 브라우저 조작이다.

우선순위:

1. `agent-browser` CLI로 사용자의 브라우저 또는 자동화 세션을 조작한다.
2. 브라우저로 처리하기 비효율적인 경우에만 직접 API 연동을 검토한다.

이 선택의 이유:

- 대표 사용자의 업무가 대부분 브라우저에서 일어난다.
- 비개발자 업무는 웹 UI를 따라가는 방식이 더 범용적이다.
- 서비스별 공식 API가 없거나 접근 권한이 제한된 경우가 많다.
- 사람이 하던 클릭, 입력, 조회, 복사, 정리 흐름을 그대로 자동화하기 쉽다.

초기 지원 후보 업무:

- 웹 리서치 및 정보 수집
- 대시보드/관리자 화면 조회
- 폼 입력 및 상태 업데이트
- 여러 탭을 오가며 정보 취합
- 웹 기반 문서/CRM/운영도구 업데이트

### 11.4 agent-browser 사용 원칙

`agent-browser`는 다음 흐름을 기본 패턴으로 사용한다.

1. 페이지 열기
2. snapshot으로 조작 가능한 요소 확인
3. ref 기반 클릭/입력/선택
4. 페이지 변화 후 재-snapshot
5. 결과 텍스트, URL, 스크린샷, 다운로드 파일을 수집

비서는 다음 기능을 우선 활용해야 한다.

- `open`, `snapshot -i`, `click`, `fill`, `select`, `press`
- `wait --load networkidle`, `wait --url`
- `get text`, `get url`, `get title`
- `screenshot`, `pdf`
- `auth save/login`
- `state save/load`
- `--session-name` 기반 세션 유지

로그인과 세션 처리 원칙:

- 가능한 경우 `agent-browser auth` 또는 session persistence를 사용한다.
- 비밀번호나 민감정보는 모델 프롬프트에 직접 남기지 않는다.
- 로그인 만료 후 재인증이 필요하면 `waiting` 상태로 전환한다.

증거 수집 원칙:

- evaluator가 판정할 수 있도록 브라우저 단계별 텍스트, URL, 캡처, 다운로드 결과를 남긴다.
- "입력했다"보다 "입력 후 어떤 화면이 보였는지"를 기록한다.
- 최종 성공 여부는 화면 상태나 생성된 아티팩트로 검증한다.

## 12. 현재 코드베이스 갭 분석

현재 레포에는 완성된 제품 코드가 없고, WTL 관련 참조 구현이 일부 존재한다.

### 12.1 이미 있는 것

- WTL 스펙 문서
- 간단한 Go engine/runtime
- Codex app server와 통신하는 기본 런타임
- 단순 marker 기반 `SimpleLoopPolicy`

### 12.2 부족한 것

- `advance_phase` directive
- `fail` terminal state
- GAN policy 구현
- phase별 prompt builder
- 구조화된 evaluator 출력
- run persistence
- API 서버
- 웹 UI
- 실시간 이벤트 스트리밍
- 재개/취소
- approval policy 분리
- `agent-browser` 중심 업무 실행 플로우
- 브라우저 상태/세션 저장

### 12.3 주의점

현재 참조 구현은 승인 요청을 자동 수락하는 형태라, 제품화 시 위험하다. 작업 유형에 따라 승인 정책을 분리해야 한다.

## 13. 기능 요구사항 상세

### 13.1 API

초기 API 제안:

- `POST /runs`
  - 새 작업 생성
- `GET /runs/:id`
  - 상태 조회
- `GET /runs/:id/events`
  - SSE 기반 이벤트 스트림
- `POST /runs/:id/input`
  - waiting 상태에 추가 입력 전달
- `POST /runs/:id/cancel`
  - 실행 취소
- `POST /runs/:id/resume`
  - 중단된 실행 재개

### 13.2 데이터 저장

SQLite 기반 저장을 권장한다.

테이블 초안:

- `runs`
- `run_events`
- `attempts`
- `artifacts`
- `evaluations`
- `tool_calls`
- `wait_requests`

### 13.3 아티팩트 관리

각 시도에서 생성된 결과를 아티팩트로 저장한다.

예시:

- 생성 문서
- 수집한 링크 목록
- 표/리포트/요약 결과
- 브라우저 작업 결과 요약
- 평가 리포트
- 최종 결과 패키지
- 필요한 경우 단계별 스크린샷

### 13.4 UI 요구사항

- 첫 화면에서 사용자는 자연어로 일을 입력할 수 있어야 한다.
- 시스템은 요청을 자동으로 구조화하되, 필요하면 쉬운 문장으로 확인 질문을 해야 한다.
- 진행 화면에서는 현재 상태, 최근 수행 작업, 대기 사유, 중간 산출물을 보여줘야 한다.
- 비개발자 기준으로 이해 가능한 표현을 사용해야 한다.
- `failed`, `waiting`, `approval_required` 상태를 명확히 구분해 보여줘야 한다.

## 14. 프롬프트 설계

### 14.1 Planner Prompt

목적:

- 사용자 요청을 실행 가능한 `TaskSpec`으로 정규화
- deliverables와 done definition 확정

출력:

- 엄격한 JSON

### 14.2 Generator Prompt

입력:

- TaskSpec
- 현재 저장된 artifacts
- evaluator critique
- 남은 시도 횟수

목적:

- 실제 작업 수행
- 중간 결과와 최종 결과 생성

출력:

- 작업 결과 요약
- 증거
- 다음 phase로 넘길 artifact reference

### 14.3 Evaluator Prompt

입력:

- TaskSpec
- generator 산출물
- 실제 페이지 내용, 도구 호출 결과, 생성 문서, 사용자에게 제시될 결과 근거

목적:

- 완료 여부 판단
- 부족한 점을 구체적으로 나열

출력:

- 구조화된 JSON

## 15. 성공 지표

### 15.1 정량 지표

- evaluator 통과율
- 평균 시도 횟수
- waiting 진입률
- 사용자 후속 수정 요청률
- 최종 완료까지 걸린 시간
- 실패 사유 분포
- 사람이 수동으로 하던 웹 반복 작업 시간 절감량

### 15.2 정성 지표

- 사용자가 "끝까지 해냈다"고 느끼는가
- 결과가 실제로 바로 활용 가능한가
- 평가 리포트가 설득력 있고 감사 가능한가
- 비개발자도 진행 상황을 이해할 수 있는가

## 16. 리스크와 대응

### 16.1 무한 루프

리스크:

- evaluator와 generator가 같은 문제를 반복할 수 있음

대응:

- 최대 attempt 제한
- critique 중복 감지
- 동일 실패 반복 시 강제 종료

### 16.2 evaluator 오판

리스크:

- 잘못 통과시키거나 과도하게 실패 판정할 수 있음

대응:

- 구조화된 done_definition 사용
- evidence 기반 평가
- 필요 시 rule-based post-check 추가

### 16.3 위험한 툴 실행

리스크:

- destructive 작업을 자동 승인할 수 있음

대응:

- approval policy 계층 추가
- 위험 작업은 user confirmation 요구
- tool allowlist/denylist 도입

### 16.4 컨텍스트 오염

리스크:

- generator와 evaluator가 서로의 문맥에 끌려갈 수 있음

대응:

- phase별 신규 thread 사용
- attempt 단위 artifact 전달
- 평가용 thread 분리

### 16.5 웹 UI 변화와 예외 상황

리스크:

- 웹사이트 구조 변경, 로그인 만료, 예상치 못한 팝업으로 자동화가 깨질 수 있음

대응:

- 브라우저 작업을 단계 단위로 기록
- 실패 시 현재 화면 상태와 에러 원인을 evaluator에 전달
- 사용자 개입 후 재개 가능한 wait 설계
- 자주 쓰는 서비스는 사이트별 실행 가이드와 셀렉터 전략 분리

## 17. 마일스톤

### M1. WTL 코어 확장

- 제품 코드 디렉터리 초기화
- engine directive 확장
- phase/thread mode 지원
- failed/exhausted 처리
- 기본 테스트 추가

### M2. GAN Policy 구현

- Planner/Generator/Evaluator prompt builder
- GANPolicy 상태 기계 구현
- evaluator JSON schema 정의
- critique 기반 재시도 루프

### M3. Persistence + API

- SQLite schema
- run/attempt/evaluation 저장
- REST API
- SSE 이벤트 스트림

### M4. UI

- 작업 생성 화면
- run 진행 상태 타임라인
- 현재 phase/attempt 표시
- evaluator 리포트 표시
- waiting 상태 입력 UI

### M5. 외부 도구 및 안정화

- approval policy
- `agent-browser` 실행 계층 통합
- 재개/취소 안정화
- 메트릭/로그 정리
- 브라우저 자동화 안정화

## 18. 1차 구현 우선순위

반드시 먼저 할 것:

1. 제품 코드 구조 생성
2. WTL 코어 확장
3. GANPolicy 구현
4. evaluator JSON 계약 확정
5. SQLite 저장
6. 최소 API + SSE
7. 브라우저 기반 대표 업무 2~3개에 대한 실행 플로우 구현

나중에 해도 되는 것:

- 복잡한 외부 서비스 연동
- 세련된 UI
- 멀티 유저 지원
- 고급 메트릭 대시보드

## 19. 권장 초기 기술 스택

- 언어: Go
- 런타임: Codex app server
- 모델: `gpt-5.4`
- 저장소: SQLite
- API: Go HTTP server
- 프론트엔드: 이후 선택
- 실시간 업데이트: SSE

Go를 권장하는 이유는 현재 레포의 WTL 참조 구현이 Go 기반이기 때문이다. 초기 단계에서는 언어를 섞지 말고 Go로 수직 통합하는 것이 가장 빠르다.

## 20. 오픈 이슈

- planner를 1차 버전에 둘지, generator 첫 턴에 흡수할지
- evaluator를 항상 LLM으로만 둘지, 일부 rule-based check를 섞을지
- Codex app server thread lifecycle을 제품 레벨에서 어디까지 제어할지
- background execution을 첫 버전부터 넣을지
- approval policy를 사용자별 설정으로 노출할지
- 브라우저 자동화를 범용으로 갈지, 서비스별 템플릿으로 시작할지
- `agent-browser` 세션을 사용자별로 어떻게 매핑하고 보호할지

## 21. 최종 권고

이 프로젝트는 "채팅형 AI"가 아니라 "비개발자를 위한 웹 업무 실행 루프"로 설계해야 한다.

따라서 1차 목표는 화려한 UI보다 다음 네 가지를 먼저 만드는 것이다.

- 명확한 `TaskSpec`
- 엄격한 `Evaluator`
- 재개 가능한 `Run` 상태 머신
- 브라우저 기반 대표 업무 플로우

이 네 가지가 안정화되면, 이후 외부 도구 연결과 UX는 비교적 쉽게 확장할 수 있다.
