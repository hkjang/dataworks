// Data Works Landing Page Script (i18n, Simulator & Interactive Elements)

const translations = {
  ko: {
    navFeatures: "주요 기능",
    navWorkflow: "상품화 워크플로우",
    navPublishGate: "출시 게이트",
    navFaq: "자주 묻는 질문",
    navDocs: "문서 바로가기",
    badgeText: "Enterprise Data Product Factory Platform v0.9",
    heroTitle: "내부 데이터 자산을 <br/><span class=\"gradient-text\">수익성 높은 데이터 상품과 API</span>로 전환",
    heroSubtitle: "Data Works는 기업 내 스키마, 데이터셋, 리포트를 수집하여 준비도 평가, AI Product Canvas 생성, 컴플라이언스 승인 증적(Evidence Pack), 런타임 API 배포까지 데이터 수익화를 일괄 자동화합니다.",
    btnPrimary: "상품화 워크플로우 체험",
    btnDocs: "가이드 문서 읽기",
    
    // Stats
    stat1Val: "100점",
    stat1Lbl: "자동 준비도(Readiness) 점수 산출",
    stat2Val: "409 Conflict",
    stat2Lbl: "승인 미비 시 Publish Gate 차단",
    stat3Val: "OpenAPI 3.1",
    stat3Lbl: "자동 생성 런타임 API 규격",
    stat4Val: "100%",
    stat4Lbl: "감사 가능한 Evidence Pack 보장",

    // Features
    featTitle: "Enterprise 데이터 상품화의 핵심 기능",
    featDesc: "단순 데이터 조회를 넘어 비즈니스 가치 창출과 규제 준수를 완벽히 결합한 팩토리 시스템",
    
    f1Title: "자산 수집 & 준비도 평가",
    f1Desc: "스키마, 최신성, 샘플, 결측률, 민감도 및 API 준비도를 100점 기준으로 자동 정밀 평가합니다.",
    
    f2Title: "AI Product Canvas 생성",
    f2Desc: "업종 및 고객군 니즈를 분석하여 상품 아이디어, 고객 시나리오, 가격 및 리스크 검토서를 자동 작성합니다.",
    
    f3Title: "컴플라이언스 승인 Trace",
    f3Desc: "데이터 오너, 법무, 보안 승인을 매트릭스로 연결하고 감사 가능한 JSON Evidence Pack으로 보관합니다.",
    
    f4Title: "엄격한 Publish Gate",
    f4Desc: "민감 정보(개인신용정보/가명정보) 및 risk_score >= 70 상품의 준비도/승인 미비 시 출식을 자동 차단합니다.",
    
    f5Title: "런타임 API & Entitlements",
    f5Desc: "계약별 호출 한도, 유효 기간, 허용 필드(Scope) 및 API 키를 런타임 엔드포인트에 즉시 바인딩합니다.",
    
    f6Title: "운영 & Funnel 분석",
    f6Desc: "아이디어부터 출시까지의 전환 퍼널, 팩토리 재실행(Replay) 계보 및 매출/리스크 성과를 한눈에 파악합니다.",

    // Simulator
    simTitle: "데이터 상품화 파이프라인 시뮬레이터",
    simDesc: "단계별 탭을 클릭하여 Data Works가 데이터 자산을 어떻게 파이프라인으로 처리하는지 확인해보세요.",
    
    tab1: "1. 자산 수집 & 준비도 점수",
    tab2: "2. AI Idea & Canvas 생성",
    tab3: "3. Evidence Pack & 승인",
    tab4: "4. Publish Gate & API 배포",

    // Architecture / Gate
    gateTitle: "안전한 데이터 유통을 위한 Publish Gate 기준",
    gateDesc: "고위험(risk_score >= 70) 및 민감 프로필 데이터 상품은 다음 4가지 검증을 필수로 통과해야 합니다.",
    
    g1Title: "1. Asset Readiness Score >= 70",
    g1Desc: "연결된 모든 원천 자산의 품질 및 준비도 점수가 기준점 이상이어야 합니다.",
    g2Title: "2. 필수 승인 Trace Matrix 완료",
    g2Desc: "Data Owner, Legal, Compliance 3개 주체의 승인(Approved/Waived) 증적이 유효해야 합니다.",
    g3Title: "3. Evidence Pack 생성 검증",
    g3Desc: "상품 정의, 리스크 검토 및 계약 조건이 포함된 완결된 JSON Pack이 생성되어야 합니다.",
    g4Title: "4. 승인 만료일 자동 감지",
    g4Desc: "승인 기한이 만료된 증적은 통과로 인정하지 않으며 409 Conflict 오류를 반환합니다.",

    // FAQ
    faqTitle: "자주 묻는 질문 (FAQ & AI Knowledge)",
    faqDesc: "Data Works 도입 및 운영에 관한 핵심 답변입니다.",

    q1: "Q1. Data Works는 기존 Kubernetes Hub 및 DB와 어떻게 연동되나요?",
    a1: "Data Works는 기존 게이트웨이 코어, Admin UI, PostgreSQL/SQLite 저장소, ClickHouse 분석 엔진을 그대로 재활용하면서 데이터 상품화 워크플로우에 최적화된 RESTful API 집합을 제공합니다.",

    q2: "Q2. Publish Gate가 상품 출식을 차단하는 기준은 무엇인가요?",
    a2: "risk_score가 70 이상이거나 개인신용정보(personal_credit), 가명정보(pseudonymized) 등 민감 프로필 상품인 경우, 준비도 70점 미만 또는 3대 필수 승인(오너, 법무, 컴플라이언스) 미비 시 409 Conflict 응답을 남기며 출식을 자동 차단합니다.",

    q3: "Q3. 생성되는 OpenAPI 3.1 규격과 런타임 API는 어떻게 호출하나요?",
    a3: "/admin/dataworks/products/{key}/openapi 엔드포인트를 통해 OpenAPI 규격을 발급받고, /v1/data-products/{key}/query 엔드포인트를 통해 유효한 API Key 및 Entitlement 검증 후 런타임 조회가 수행됩니다.",

    // Footer
    footerContactTitle: "데이터 상품화 및 제휴 문의",
    footerEmailText: "이메일 문의:",
    footerRights: "© 2026 Data Works Team. All rights reserved."
  },

  en: {
    navFeatures: "Features",
    navWorkflow: "Workflow",
    navPublishGate: "Publish Gate",
    navFaq: "FAQ",
    navDocs: "Documentation",
    badgeText: "Enterprise Data Product Factory Platform v0.9",
    heroTitle: "Turn Enterprise Data Assets into <br/><span class=\"gradient-text\">High-Value Products & APIs</span>",
    heroSubtitle: "Data Works automates the entire lifecycle of data monetization: asset ingestion, readiness scoring, AI Product Canvas generation, compliance evidence packs, and runtime API delivery.",
    btnPrimary: "Try Pipeline Simulator",
    btnDocs: "Read Documentation",

    // Stats
    stat1Val: "100 Pts",
    stat1Lbl: "Automated Readiness Evaluation",
    stat2Val: "409 Conflict",
    stat2Lbl: "Strict Publish Gate Block",
    stat3Val: "OpenAPI 3.1",
    stat3Lbl: "Auto-Generated Specs",
    stat4Val: "100%",
    stat4Lbl: "Auditable Evidence Guarantee",

    // Features
    featTitle: "Core Features of Data Product Factory",
    featDesc: "Combining enterprise data asset management with compliance-driven monetization.",

    f1Title: "Asset Readiness Evaluation",
    f1Desc: "Evaluates schema, freshness, null ratios, sensitivity, and API readiness on a 100-point scale.",

    f2Title: "AI Product Canvas Generation",
    f2Desc: "Generates product ideas, customer personas, pricing models, and risk checklists automatically based on market demand.",

    f3Title: "Compliance Approval Trace",
    f3Desc: "Connects Data Owner, Legal, and Compliance approvals into a trace matrix and packages auditable JSON Evidence Packs.",

    f4Title: "Strict Publish Gate",
    f4Desc: "Blocks publishing for high-risk (score >= 70) or sensitive profile assets if compliance or readiness criteria are unmet.",

    f5Title: "Runtime APIs & Scope Entitlements",
    f5Desc: "Binds rate limits, expiration dates, allowed fields, and API keys to secure runtime execution endpoints.",

    f6Title: "Funnel & Portfolio Analytics",
    f6Desc: "Tracks idea-to-market conversion funnels, AI factory run replays, and revenue-risk metrics in real-time.",

    // Simulator
    simTitle: "Data Monetization Pipeline Simulator",
    simDesc: "Click each step below to inspect how Data Works processes raw data assets into monetizable APIs.",

    tab1: "1. Asset Ingestion & Readiness",
    tab2: "2. AI Idea & Canvas",
    tab3: "3. Evidence Pack & Trace",
    tab4: "4. Publish Gate & Runtime API",

    // Architecture / Gate
    gateTitle: "Publish Gate Criteria for Secure Distribution",
    gateDesc: "High-risk (risk_score >= 70) and sensitive data products must satisfy all 4 verification gates.",

    g1Title: "1. Asset Readiness Score >= 70",
    g1Desc: "All connected source data assets must achieve a minimum readiness benchmark.",
    g2Title: "2. Mandatory Approval Traces",
    g2Desc: "Data Owner, Legal, and Compliance approvals (Approved/Waived) must be active and valid.",
    g3Title: "3. Verified Evidence Pack",
    g3Desc: "A complete JSON pack containing product specs, risk reviews, and agreement scopes must exist.",
    g4Title: "4. Expiration Auto-Detection",
    g4Desc: "Expired approvals are rejected, causing the Publish Gate to return HTTP 409 Conflict.",

    // FAQ
    faqTitle: "Frequently Asked Questions (FAQ & AI Knowledge)",
    faqDesc: "Essential insights into Data Works implementation and governance.",

    q1: "Q1. How does Data Works integrate with existing databases and infrastructures?",
    a1: "Data Works leverages existing Gateway Cores, Admin UI, PostgreSQL/SQLite storage, and ClickHouse analytics while providing dedicated Data Product Factory RESTful endpoints.",

    q2: "Q2. What causes the Publish Gate to block product releases?",
    a2: "For products with risk_score >= 70 or sensitive profiles (personal credit, pseudonymized data), any readiness score below 70 or missing approvals triggers an automatic HTTP 409 Conflict block.",

    q3: "Q3. How do I consume the generated OpenAPI 3.1 specs and runtime APIs?",
    a3: "Obtain OpenAPI specifications via /admin/dataworks/products/{key}/openapi, and execute authenticated queries via /v1/data-products/{key}/query with active entitlements.",

    // Footer
    footerContactTitle: "Inquiries & Business Partnerships",
    footerEmailText: "Email Contact:",
    footerRights: "© 2026 Data Works Team. All rights reserved."
  }
};

const simDetails = {
  tab1: {
    infoKo: {
      title: "데이터 자산 수집 & 준비도 평가 (Readiness Score)",
      desc: "내부 데이터셋/테이블/API를 수집하여 스키마 완결성, 결측률, 최신성, 민감도(Personal/Pseudonymized)를 다각도로 평가합니다.",
      badges: ["POST /admin/dataworks/assets/{key}/readiness/check", "Score: 85/100", "Sensitivity: General"]
    },
    infoEn: {
      title: "Asset Ingestion & Readiness Evaluation",
      desc: "Ingests internal datasets and APIs, evaluating schema integrity, null ratio, freshness, and sensitivity on a 100-pt scale.",
      badges: ["POST /admin/dataworks/assets/{key}/readiness/check", "Score: 85/100", "Sensitivity: General"]
    },
    code: `// Request: Check Asset Readiness
POST /admin/dataworks/assets/FIN_TRANSACTIONS_01/readiness/check

// Response:
{
  "asset_key": "FIN_TRANSACTIONS_01",
  "readiness_score": 85,
  "metrics": {
    "schema_completeness": 95,
    "freshness_watermark": "2026-08-01T09:00:00Z",
    "null_ratio": 0.02,
    "sensitivity_profile": "pseudonymized"
  },
  "api_ready": true,
  "status": "PASS"
}`
  },
  tab2: {
    infoKo: {
      title: "AI Product Canvas 및 규격 설계",
      desc: "시장 수요 및 고객 세그먼트를 매핑하여 상품 아이디어, Target Customer, Price Model, Risk Checklist를 자동 보완 및 생성합니다.",
      badges: ["POST /admin/dataworks/factory/definitions", "Market Fit: 92%", "Auto Product Canvas"]
    },
    infoEn: {
      title: "AI Product Canvas & Specification Design",
      desc: "Maps market demand and customer segments to generate product definitions, pricing models, and risk checklists.",
      badges: ["POST /admin/dataworks/factory/definitions", "Market Fit: 92%", "Auto Product Canvas"]
    },
    code: `// Request: Generate Product Canvas
POST /admin/dataworks/products/FIN_ANALYSIS_API/canvas/generate

// Response:
{
  "product_key": "FIN_ANALYSIS_API",
  "target_customers": ["Fintech", "Credit Rating Agency"],
  "pricing_model": "tier_pay_per_call",
  "risk_score": 35,
  "product_canvas": {
    "problem_statement": "Real-time credit assessment data gap",
    "unique_value_prop": "Sub-second multi-dimensional scoring"
  }
}`
  },
  tab3: {
    infoKo: {
      title: "Evidence Pack & 승인 Trace Matrix",
      desc: "데이터 오너, 법무팀, 컴플라이언스 팀의 승인 증적을 묶어 감사 불가능을 불식시키는 JSON Evidence Pack으로 저장합니다.",
      badges: ["GET/POST /admin/dataworks/products/{key}/evidence-pack", "Audit Pack Verified"]
    },
    infoEn: {
      title: "Evidence Pack & Approval Trace Matrix",
      desc: "Binds approval traces from Data Owner, Legal, and Compliance into a tamper-proof JSON Evidence Pack for auditing.",
      badges: ["GET/POST /admin/dataworks/products/{key}/evidence-pack", "Audit Pack Verified"]
    },
    code: `// Response: Product Evidence Pack
{
  "product_key": "FIN_ANALYSIS_API",
  "evidence_pack_id": "ev_pack_99481a7",
  "approval_matrix": [
    { "role": "data_owner", "status": "approved", "timestamp": "2026-07-28" },
    { "role": "legal", "status": "approved", "timestamp": "2026-07-29" },
    { "role": "compliance", "status": "approved", "timestamp": "2026-07-30" }
  ],
  "readiness_proof_score": 85,
  "hash_signature": "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
}`
  },
  tab4: {
    infoKo: {
      title: "Publish Gate 검증 및 런타임 API 배포",
      desc: "준비도 및 승인 조건을 만족하는 상품만 'published' 전환을 허용하며 OpenAPI 3.1 스펙 및 Entitlement API 키를 연결합니다.",
      badges: ["POST /v1/data-products/{key}/query", "Status: 200 OK", "Published & Active"]
    },
    infoEn: {
      title: "Publish Gate Validation & Runtime API Deployment",
      desc: "Permits 'published' status only when all gate requirements pass, linking OpenAPI 3.1 specs and API key entitlements.",
      badges: ["POST /v1/data-products/{key}/query", "Status: 200 OK", "Published & Active"]
    },
    code: `// Runtime API Call Example
POST /v1/data-products/FIN_ANALYSIS_API/query
Header: X-API-Key: dw_live_99812471...

// Response:
{
  "status": "success",
  "product_key": "FIN_ANALYSIS_API",
  "entitlement": {
    "scope": "read_financial_summary",
    "rate_limit_remaining": 4982
  },
  "data": [
    { "segment_id": "S-102", "risk_index": 0.12, "score": 820 }
  ]
}`
  }
};

let currentLang = 'ko';
let currentTab = 'tab1';

function updateContent() {
  document.querySelectorAll('[data-i18n]').forEach(el => {
    const key = el.getAttribute('data-i18n');
    if (translations[currentLang][key]) {
      el.innerHTML = translations[currentLang][key];
    }
  });
  updateSimulator();
}

function switchLanguage(lang) {
  currentLang = lang;
  document.documentElement.lang = lang;
  
  document.getElementById('btn-ko').classList.toggle('active', lang === 'ko');
  document.getElementById('btn-en').classList.toggle('active', lang === 'en');
  
  updateContent();
}

function updateSimulator() {
  const data = simDetails[currentTab];
  const info = currentLang === 'ko' ? data.infoKo : data.infoEn;
  
  document.getElementById('sim-title').innerText = info.title;
  document.getElementById('sim-desc').innerText = info.desc;
  
  const badgeContainer = document.getElementById('sim-badges');
  badgeContainer.innerHTML = info.badges.map(b => `<span class="sim-badge-item">${b}</span>`).join('');
  
  document.getElementById('sim-code').innerText = data.code;
}

// Event Listeners
document.addEventListener('DOMContentLoaded', () => {
  // Lang buttons
  document.getElementById('btn-ko').addEventListener('click', () => switchLanguage('ko'));
  document.getElementById('btn-en').addEventListener('click', () => switchLanguage('en'));

  // Simulator tabs
  const tabBtns = document.querySelectorAll('.sim-tab-btn');
  tabBtns.forEach(btn => {
    btn.addEventListener('click', () => {
      tabBtns.forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      currentTab = btn.getAttribute('data-tab');
      updateSimulator();
    });
  });

  // FAQ Accordion
  const faqItems = document.querySelectorAll('.faq-item');
  faqItems.forEach(item => {
    const qBtn = item.querySelector('.faq-question');
    qBtn.addEventListener('click', () => {
      const isActive = item.classList.contains('active');
      faqItems.forEach(i => i.classList.remove('active'));
      if (!isActive) {
        item.classList.add('active');
      }
    });
  });

  // Init
  updateContent();
});


  // Mobile Menu Toggle
  const mobileBtn = document.getElementById('mobile-menu-btn');
  const navLinks = document.querySelector('.nav-links') || document.querySelector('.nav-menu');
  if (mobileBtn && navLinks) {
    mobileBtn.addEventListener('click', () => {
      navLinks.classList.toggle('active');
    });
    navLinks.querySelectorAll('a').forEach(link => {
      link.addEventListener('click', () => navLinks.classList.remove('active'));
    });
  }
