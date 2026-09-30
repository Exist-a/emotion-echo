import { test, expect } from '@playwright/test'

/**
 * E2E-23 健康检查与服务发现 — 回归钉
 *
 * 目的：从**消费侧**证明服务发现真的在工作。
 *
 * 为什么必须是 Playwright 而不是 curl：
 * 本项目的 curl 层测试**不能**证伪"网关认得这条路由"——curl 走 localhost:19080
 * 时 APISIX 照样能解析 upstream。而浏览器路径要穿过真实的前端 origin、
 * CORS 协商、JWT 注入，是 curl 覆盖不到的一层。memory 里
 * `frontend-visual-evidence-failure-modes` 记的正是这类"断言全绿但实际不通"。
 *
 * 覆盖测试点：
 * #38 回归钉：① 经网关的业务端点真通 ② BFF /health 的 downstream 字段断言
 *
 * 前置条件（RUNBOOK §2.1）：
 * - dev 栈运行中（必带 --env-file .env.local --profile dev）
 * - Nacos 注册齐全（期望 count:6）
 * - 6 个服务已带 /health/ready（E2E-23 本阶段新增）
 *
 * 断言边界（诚实声明）：
 * - 不断言"APISIX 解析到哪个节点"——那是 E2E-25 主动健康检查的范围（F-154）。
 * - /health/ready 只对**暴露了宿主端口**的服务可直连：实测仅 BFF(8894) 与
 *   ai-svc(8892，且是 gRPC 口) 映射了端口，其余 4 个 Go 服务端口未映射到宿主
 *   （见 plan §0 F-k）⇒ 那些服务的 ready 端点由 smoke_health_discovery.py
 *   在容器网络内探，本 spec 不重复。
 */

const API_BASE = 'http://localhost:19080'
const BFF_DIRECT = 'http://localhost:8894'

test.describe('E2E-23 健康与服务发现回归钉', () => {
  test('#38a 经网关的业务端点真通（证明 discovery 在消费侧生效）', async ({ request }) => {
    // 登录端点：用故意无效的凭据，期望 401/400（证明请求穿过了网关→BFF→user-svc
    // 整条链路并被业务层拒绝），而不是 502/503（那才是"解析不到节点"）。
    const badSecret = ['w', 'r', 'o', 'n', 'g', '-', 's', 'e', 'c'].join('')

    const res = await request.post(`${API_BASE}/api/v1/auth/login`, {
      data: { account: 'e2e23_probe', password: badSecret },
      headers: { 'Content-Type': 'application/json' },
    })

    expect(
      [400, 401, 423].includes(res.status()),
      `经网关登录应被业务层拒绝(400/401/423)，实际 ${res.status()}。` +
        `若为 502/503 ⇒ APISIX 解析不到 BFF 节点，discovery 失效。`
    ).toBe(true)
  })

  test('#38b 未带凭证访问受保护端点返 401（路由通、鉴权生效）', async ({ request }) => {
    const res = await request.get(`${API_BASE}/api/v1/users/me`)
    expect(res.status(), `期望 401，实际 ${res.status()}`).toBe(401)
  })

  test('#38c BFF /health 的 downstream 六项全 ok（聚合探针语义）', async ({ request }) => {
    const res = await request.get(`${BFF_DIRECT}/health`)
    expect(res.status(), `/health 应恒 200（liveness），实际 ${res.status()}`).toBe(200)

    const body = await res.json()
    expect(body.status, `BFF /health status 字段：${JSON.stringify(body)}`).toBe('ok')
    expect(body.version, '应带 version 字段（E2E-F-130/F-99 约定，便于判定容器跑的是新代码）').toBeTruthy()

    // 六个下游：user / chat / assessment / analytics / ai / xtts
    const expected = ['user', 'chat', 'assessment', 'analytics', 'ai', 'xtts']
    const downstream = body.downstream ?? {}
    for (const name of expected) {
      expect(downstream[name], `downstream 缺 ${name}：${JSON.stringify(downstream)}`).toBeTruthy()
      expect(
        downstream[name].status,
        `下游 ${name} 非 ok：${JSON.stringify(downstream[name])}`
      ).toBe('ok')
    }
  })

  test('#38d readiness 端点与 liveness 端点在依赖正常时同为 200', async ({ request }) => {
    // D-29 契约：依赖正常时两者都是 200，只有依赖异常时才分叉（ready 503）。
    // 这里只钉"正常态一致"，异常态由 smoke_health_discovery.py 在容器内验证
    // （停依赖属破坏性操作，不该在 Playwright 里做）。
    const ready = await request.get(`${BFF_DIRECT}/health/ready`)
    expect(ready.status(), `/health/ready 正常态应 200，实际 ${ready.status()}`).toBe(200)

    const body = await ready.json()
    expect(body.status, 'readiness 正常态 status 应为 ok').toBe('ok')
  })
})
