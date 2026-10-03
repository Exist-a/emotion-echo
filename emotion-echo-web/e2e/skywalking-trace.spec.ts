/**
 * E2E-26 回归钉（测试点 #14/#15 的自动化形态）：
 * SkyWalking UI 跨进程 trace 查询与可视化。
 *
 * 运行方式（BASE_URL 指向 UI，跳过 webServer）：
 *   BASE_URL=http://127.0.0.1:18080 pnpm playwright test e2e/skywalking-trace.spec.ts
 *
 * 自包含：测试内先经 APISIX 造一笔真实跨进程流量（login → /users/me → ai/stream），
 * 再在 UI 按服务过滤查询 trace，断言 3 服务 chips 可见并落盘截图
 * （docs/e2e-roadmap/stages/e2e-26-tracing-skywalking/screenshots/）。
 *
 * 背景（E2E-26 F-184）：IAB 截图存在渲染帧与 DOM 不同步的失真，
 * [V] 视觉证据改由本 spec 的独立 Chromium 渲染栈产出。
 */
import { test, expect, request } from '@playwright/test';
import * as path from 'path';
import * as fs from 'fs';
import { fileURLToPath } from 'url';

const UI_BASE = process.env.SW_UI_URL ?? 'http://127.0.0.1:18080';
const API_BASE = 'http://localhost:19080';
const SHOTS_DIR = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  '../../docs/e2e-roadmap/stages/e2e-26-tracing-skywalking/screenshots',
);

async function seedTraffic(): Promise<void> {
  const api = await request.newContext({ baseURL: API_BASE });
  const login = await api.post('/api/v1/auth/login', {
    data: { username: 'echo', password: 'echo123' },
    timeout: 15000,
  });
  expect(login.status(), 'login must succeed').toBe(200);
  const token = (await login.json()).data.accessToken as string;

  const me = await api.get('/api/v1/users/me', {
    headers: { Authorization: `Bearer ${token}` },
    timeout: 15000,
  });
  expect(me.status(), 'GET /users/me must succeed').toBe(200);

  // 聊天流：产生 BFF→chat-svc→ai-svc 跨进程 trace（3 服务）
  const chat = await api.post('/api/v1/ai/stream', {
    headers: { Authorization: `Bearer ${token}` },
    data: {
      message: 'E2E-26 回归钉:跨进程 trace 造数',
      emotion: '平静',
      conversationId: '1001',
    },
    timeout: 60000,
  });
  expect(chat.status(), 'ai/stream must succeed').toBe(200);
  await api.dispose();
}

test.describe('E2E-26 SkyWalking trace UI 回归钉', () => {
  test.setTimeout(180_000);

  test('造数: 经 APISIX 产生跨进程流量 (login + users/me + ai/stream)', async () => {
    await seedTraffic();
    // 等 OAP 收录（reporter 上报 + 段落入库有秒级延迟）
    await new Promise((r) => setTimeout(r, 12_000));
  });

  test('Trace 页可查询跨进程 trace 并渲染 3 服务树 (#14)', async ({ page }) => {
    await page.goto(UI_BASE + '/', { waitUntil: 'domcontentloaded' });

    // 服务列表页先出现（OAP listServices 有数据）
    await expect(
      page.getByText('emotion-echo-chat-svc').first(),
    ).toBeVisible({ timeout: 20000 });

    // 切到 Trace tab（顶部4个 tab 是 input.tab-name，顺序 Service/Topology/Trace/Log）
    await page.locator('input.tab-name').nth(2).click({ force: true });
    await expect(page.getByText('追踪ID:')).toBeVisible({ timeout: 10000 });

    // 服务过滤 → emotion-echo-web-bff（el-select 交互）
    await page.getByPlaceholder('Select a service').click();
    await page
      .getByRole('listitem')
      .filter({ hasText: 'emotion-echo-web-bff' })
      .first()
      .click();

    // 直接搜索（默认时间窗=最近30分钟；造数刚发生）
    await page.getByRole('button', { name: '搜索' }).click();

    // 状态钉子：服务过滤必须是 web-bff（防截图瞬间状态漂移/旧帧）
    await expect(page.getByPlaceholder('Select a service')).toHaveValue('emotion-echo-web-bff', { timeout: 10000 });

    // 结果：Trace Segments 里出现聊天入口 trace
    const segRow = page.getByText('/api/v1/ai/stream').first();
    await expect(segRow).toBeVisible({ timeout: 20000 });
    await segRow.click();

    // 详情面板：跨度标签（exact 避开 popover 提示歧义）+ 3 服务 chips（DOM 顺序详情在 popper 前）
    await expect(page.getByText('跨度', { exact: true })).toBeVisible({ timeout: 15000 });
    await expect(page.getByText('emotion-echo-ai-svc', { exact: true }).first()).toBeVisible({ timeout: 15000 });
    await expect(page.getByText('emotion-echo-chat-svc', { exact: true }).first()).toBeVisible({ timeout: 5000 });

    // 树结构视图
    const treeBtn = page.getByRole('button', { name: '树结构' });
    if (await treeBtn.isVisible().catch(() => false)) {
      await treeBtn.click();
    }
    await page.waitForTimeout(1000);

    // 截图前终钉：详情活着且列表非空
    await expect(page.getByText('跨度', { exact: true })).toBeVisible();
    await expect(page.getByText('数据为空')).toHaveCount(0);

    fs.mkdirSync(SHOTS_DIR, { recursive: true });
    await page.screenshot({
      path: path.join(SHOTS_DIR, '26-14-trace-tree-cross-service.png'),
    });
    // eslint-disable-next-line no-console
    console.log('[shot] 26-14 written, url =', page.url());
  });

  test('Topology 页渲染服务拓扑 (#15)', async ({ page }) => {
    await page.goto(UI_BASE + '/', { waitUntil: 'domcontentloaded' });
    await expect(
      page.getByText('emotion-echo-chat-svc').first(),
    ).toBeVisible({ timeout: 20000 });

    await page.locator('input.tab-name').nth(1).click({ force: true });

    // 拓扑画布/服务节点出现（渲染层证据由截图承载）
    await expect(
      page.getByText('emotion-echo').first(),
    ).toBeVisible({ timeout: 20000 });
    await page.waitForTimeout(2500); // 等图布局收敛

    fs.mkdirSync(SHOTS_DIR, { recursive: true });
    await page.screenshot({
      path: path.join(SHOTS_DIR, '26-15-service-topology.png'),
    });
  });
});
