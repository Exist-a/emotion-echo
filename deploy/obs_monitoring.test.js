// E2E-22 监控告警 —— 配置面契约测试 (Node.js, 无依赖)
// =============================================================================
// 目的: 把 E2E-22 实测抓到的静默失效钉死, 防止回退.
//
// 本文件的存在理由 (每条都有实测证据, 不是假想):
//   ① kafka-exporter 的 depends_on 指向 prometheus(与它毫无关系)而非 kafka,
//      启动时 broker 9092 未监听 → "Error Init Kafka Client" → 进程 exit 255 且无
//      restart 策略 → target 永久 DOWN (2026-09-29 实测复现两次)
//   ② emotion-llm-service 暴露 /metrics 但 prometheus.yml 无 target (dev 采不到),
//      而 k8s chart 却有 prometheus.io/scrape annotation (双栈不一致)
//   ③ prometheus.yml 5 个 job 中没有 alertmanager —— 收告警的东西自己无指标
//   ④ k8s prometheus chart 声明 rule_files 指向 /etc/prometheus/rules/*.yml, 但该
//      ConfigMap 的 data 只有 prometheus.yml 一个 key, deployment 也没有对应
//      volume 挂载 → 空 glob → 0 条告警规则且零报错
//   ⑤ dev alertmanager 只有无集成段的 dev-ui receiver (E2E-F-11)
//   ⑥ 文档 4 处漂移 (验收误导源)
//
// 运行: node deploy/obs_monitoring.test.js

const fs = require('fs');
const path = require('path');

const ROOT = path.join(__dirname, '..');
const p = (...s) => path.join(ROOT, ...s);

function fail(msg) { console.error('  ✗ ' + msg); process.exitCode = 1; }
function pass(msg) { console.log('  ✓ ' + msg); }

function read(rel) {
  const abs = p(rel);
  if (!fs.existsSync(abs)) return null;
  return fs.readFileSync(abs, 'utf8');
}

// 抽取 compose 中某个服务块 (从 "  <name>:" 到下一个同缩进的服务名)
function serviceBlock(yml, name) {
  const re = new RegExp(`^  ${name}:$([\\s\\S]*?)(?=^  [a-z][a-z0-9-]*:$|^[a-z])`, 'm');
  const m = yml.match(re);
  return m ? m[1] : null;
}

const infra = read('deploy/docker-compose.infra.yml');
const promYml = read('deploy/prometheus/prometheus.yml');
const amYml = read('deploy/alertmanager/alertmanager.yml');
const selfRules = read('deploy/prometheus/rules/observability-self.yml');
const k8sCmRules = read('charts/emotion-echo/charts/prometheus/templates/configmap-rules.yaml');
const k8sDeploy = read('charts/emotion-echo/charts/prometheus/templates/deployment.yaml');
const obsRunbook = read('docs/deployment/runbook/observability-compose.md');
const deployConfig = read('deploy/configuration.md');

if (!infra || !promYml || !amYml) {
  console.error('缺少基础配置文件, 无法执行契约测试');
  process.exit(1);
}

const groups = [];
function group(name, fn) { groups.push([name, fn]); }

// -----------------------------------------------------------------------------
group('① kafka-exporter 启动依赖 (2026-09-29 实测 exit 255 根因)', () => {
  const block = serviceBlock(infra, 'kafka-exporter');
  if (!block) return fail('kafka-exporter 服务块未找到');
  pass('kafka-exporter 服务块存在');

  // 依赖方向: 必须依赖 kafka, 而不是 prometheus
  const dependsOnKafka = /depends_on:[\s\S]*?\bkafka\b/.test(block);
  dependsOnKafka
    ? pass('kafka-exporter depends_on 含 kafka')
    : fail('kafka-exporter depends_on 缺少 kafka —— broker 未就绪时启动必 exit 255');

  // 旧错误依赖: depends_on prometheus 与本服务无因果关系
  if (/depends_on:[\s\S]*?prometheus/.test(block)) {
    fail('kafka-exporter depends_on 仍含 prometheus —— 该依赖与 consumer lag 采集无因果关系, 且掩盖了对 kafka 的真实依赖');
  } else {
    pass('kafka-exporter 不再错误依赖 prometheus');
  }

  // 必须有 restart 策略: kafka-exporter 一次性初始化失败即退出, 无 restart 则永久 DOWN
  /restart:\s*\S/.test(block)
    ? pass('kafka-exporter 有 restart 策略 (失败可自愈)')
    : fail('kafka-exporter 缺 restart 策略 —— 一次启动失败即永久 DOWN, 无任何自愈');

  // 锁住既有正确配置, 防回退
  /--kafka\.server=emotion-echo-kafka:9092/.test(block)
    ? pass('kafka-exporter command 指向 emotion-echo-kafka:9092')
    : fail('kafka-exporter command 的 --kafka.server 被改动');
});

// -----------------------------------------------------------------------------
group('② Prometheus 抓取面完整性', () => {
  // llm-service 暴露了 /metrics 却无 target (双栈不一致)
  /emotion-llm-service:8000/.test(promYml)
    ? pass('prometheus.yml 含 emotion-llm-service:8000 target')
    : fail('prometheus.yml 缺 emotion-llm-service:8000 —— 该服务暴露 /metrics 但 dev 采不到');

  // alertmanager 自身无指标
  /job_name:\s*["']?alertmanager["']?/.test(promYml)
    ? pass('prometheus.yml 含 alertmanager job (收告警的东西自己可观测)')
    : fail('prometheus.yml 缺 alertmanager job —— Alertmanager 自身无指标, 挂掉时无人知道');

  // 锁住既有接线
  /rule_files:/.test(promYml)
    ? pass('prometheus.yml 声明 rule_files')
    : fail('prometheus.yml 的 rule_files 丢失');

  /^alerting:/m.test(promYml)
    ? pass('prometheus.yml 保留 alerting 段接线')
    : fail('prometheus.yml 的 alerting 段丢失');

  /apisix\/prometheus\/metrics/.test(promYml)
    ? pass('prometheus.yml 保留 APISIX 自监控路径')
    : fail('APISIX 自监控 metrics_path 被改动');
});

// -----------------------------------------------------------------------------
group('③ 观测面自身告警规则', () => {
  if (!selfRules) {
    fail('deploy/prometheus/rules/observability-self.yml 不存在 —— 观测面 target DOWN 时无任何规则会响');
    return;
  }
  pass('observability-self.yml 存在');

  /alert:\s*PrometheusTargetDown/.test(selfRules)
    ? pass('含 PrometheusTargetDown 规则 (up == 0)')
    : fail('缺 PrometheusTargetDown 规则');

  /alert:\s*AlertmanagerNotificationFailing/.test(selfRules)
    ? pass('含 AlertmanagerNotificationFailing 规则 (通知发不出去要能发现)')
    : fail('缺 AlertmanagerNotificationFailing 规则');

  /severity:\s*critical/.test(selfRules)
    ? pass('规则标注 severity')
    : fail('规则缺 severity 标注 —— Alertmanager 无法按 severity 分流');
});

// -----------------------------------------------------------------------------
group('④ Alertmanager 通知渠道 (E2E-F-11)', () => {
  /webhook_configs:/.test(amYml)
    ? pass('alertmanager.yml 含 webhook_configs receiver (通知可被机器断言)')
    : fail('alertmanager.yml 仍只有空 receiver —— 通知发不出去且无法自动验证 (E2E-F-11 未闭环)');

  /send_resolved:\s*true/.test(amYml)
    ? pass('alertmanager.yml 开启 send_resolved (resolved 链路可验)')
    : fail('alertmanager.yml 未开启 send_resolved —— 告警解除无法验证');

  // 既有 dev-ui receiver 必须保留 (不破坏"UI 可见全部告警"语义)
  /name:\s*dev-ui/.test(amYml)
    ? pass('dev-ui receiver 保留 (Stage 86 决策不被推翻)')
    : fail('dev-ui receiver 丢失 —— 告警将不再在 :9093 UI 可见');

  // 期望链路: receiver 服务可达
  const block = serviceBlock(infra, 'obs-mock-receiver');
  if (!block) {
    fail('compose 缺 obs-mock-receiver 服务 —— webhook receiver 无投递目标, 等于空接线');
  } else {
    pass('obs-mock-receiver 服务存在');
    /profiles:\s*\[\s*["']obs["']\s*\]/.test(block)
      ? pass('obs-mock-receiver 归属 obs profile (不污染默认 dev 启动)')
      : fail('obs-mock-receiver 未标 obs profile');
  }
});

// -----------------------------------------------------------------------------
group('⑤ k8s 侧规则挂载 (rule_files 空 glob 静默失效)', () => {
  if (!k8sCmRules) {
    fail('charts/.../prometheus/templates/configmap-rules.yaml 不存在 —— k8s 侧 0 条告警规则且零报错');
  } else {
    pass('configmap-rules.yaml 存在');
    /PrometheusTargetDown/.test(k8sCmRules) || /OutboxEventsDead/.test(k8sCmRules)
      ? pass('configmap-rules.yaml 内联了告警规则内容')
      : fail('configmap-rules.yaml 未内联任何告警规则 —— 会再次变成空 glob');
  }

  if (!k8sDeploy) return fail('prometheus deployment.yaml 未找到');
  /mountPath:\s*\/etc\/prometheus\/rules/.test(k8sDeploy)
    ? pass('deployment 挂载 /etc/prometheus/rules')
    : fail('deployment 未挂载 /etc/prometheus/rules —— rule_files 指向空目录, 规则永不加载');
  /name:\s*rules/.test(k8sDeploy)
    ? pass('deployment 声明 rules volume')
    : fail('deployment 未声明 rules volume');

  // 防漂移: dev 侧规则文件与 k8s 内联副本的 alert 名集合必须**双向相等**。
  // 改一侧忘另一侧 = 一侧静默加载 0 条规则, 正是本组要防的失效模式。
  const rulesDir = p('deploy/prometheus/rules');
  const devAlerts = new Set();
  for (const f of fs.readdirSync(rulesDir)) {
    if (!f.endsWith('.yml')) continue;
    for (const m of read(`deploy/prometheus/rules/${f}`).matchAll(/^\s*-?\s*alert:\s*(\S+)/gm)) {
      devAlerts.add(m[1]);
    }
  }
  const k8sAlerts = new Set();
  for (const m of k8sCmRules.matchAll(/^\s*- alert:\s*(\S+)/gm)) k8sAlerts.add(m[1]);

  const missingInK8s = [...devAlerts].filter((a) => !k8sAlerts.has(a));
  const extraInK8s = [...k8sAlerts].filter((a) => !devAlerts.has(a));

  missingInK8s.length === 0
    ? pass(`k8s 内联规则覆盖 dev 全部 ${devAlerts.size} 条告警`)
    : fail(`k8s 侧缺规则: ${missingInK8s.join(', ')} —— k8s 环境这些告警不会响`);
  extraInK8s.length === 0
    ? pass('k8s 侧无 dev 不存在的规则')
    : fail(`k8s 侧多出规则: ${extraInK8s.join(', ')} —— 与 dev 不一致`);
  if (devAlerts.size === 0) fail('dev 侧未解析到任何告警规则 —— 解析器或规则目录失效');
});

// -----------------------------------------------------------------------------
group('⑥ 文档漂移 (验收误导源)', () => {
  if (!obsRunbook) return fail('obs runbook 未找到');
  /dev 未启用/.test(obsRunbook)
    ? fail('obs runbook 仍写 "Alertmanager dev 未启用" —— 与 compose/prometheus.yml 实际矛盾')
    : pass('obs runbook 不再声称 Alertmanager 未启用');

  /未设 SW_TELEMETRY/.test(obsRunbook)
    ? fail('obs runbook 仍写 "sw-oap 未设 SW_TELEMETRY" —— compose 已设')
    : pass('obs runbook 不再声称 sw-oap 未设 SW_TELEMETRY');

  /11 项 PASS/.test(obsRunbook)
    ? fail('obs runbook 仍写 "11 项 PASS" —— smoke 已扩展, 数字过期')
    : pass('obs runbook 的 smoke 项数表述已更新');

  if (!deployConfig) return fail('deploy/configuration.md 未找到');
  /smoke_observability\.sh/.test(deployConfig)
    ? fail('configuration.md 引用 smoke_observability.sh —— 实际文件是 .py')
    : pass('configuration.md 引用 .py');
});

// -----------------------------------------------------------------------------
let failed = 0;
for (const [name, fn] of groups) {
  console.log('\n' + name);
  const before = process.exitCode;
  fn();
  if (process.exitCode !== before) failed++;
  else if (process.exitCode === undefined) process.exitCode = 0;
}

console.log('');
if (process.exitCode) {
  console.error(`契约测试失败: ${failed} 组断言未通过`);
  process.exit(1);
}
console.log('契约测试全部通过');
