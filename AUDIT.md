# AUDIT.md — ZJDDNS audit playbook

ZJDDNS 全量审计的可复用手册。下一轮审计照此执行：**门禁基线 → 并行审计 → Sprint 修复 → Round C 复查 → 文档回填**。

目标设定：**按轮审计，直到一轮审计不再产生新发现为止**。每个修复都要重跑其影响面的回归 + lint 门禁；每轮结束把新教训以现在时约束落进 AGENTS.md 与本文件对应节（不写轮次战报，不设报告存档——commit 历史即记录），同一错误不审计两次。

## 1. 环境准备（门禁基线）

```sh
go build ./...                    # 零编译错误
go test ./... -count=1            # 全部通过（-count=1 防缓存假绿）
go test -race ./...               # race 全绿（项目顺序执行，理应零竞争）
go fix ./... && golangci-lint fmt && golangci-lint run   # 零警告（与 pre-commit hook 同序）
```

- 基线必须在审计开始前全绿——审计发现要跟基线 diff，不跟噪声 diff。
- pre-commit hook（`scripts/pre-commit`）每次提交自动跑 fix → fmt → 部分暂存安全重加 → run；hook 的部分暂存契约：**只重加格式化器真正改过的文件**（worktree blob hash 前后对比）。
- 本项目无 benchmark 基线（低频 ticker 场景，微基准噪声大于收益）。性能维度的审计结论用**结构判据**：每 tick 的 API 调用数（content-compare 免 no-op）、每类型检测次数（union-type 合并）、热路径无重复构造。若未来引入高频调度，先建基线再谈回归。

## 2. 审计维度（扫查清单）

并行派 Explore 只读代理 + 自查，覆盖：

0. **正确性/健壮性**：panic 面（手写 DNS 解析每步 offset 推进必须有界）、吞错误（`_ =` 清单逐条核理由）、lookup 失败与「记录缺失」的判别（§5 第一条模式）、配置值进网络调用前的转义与族校验。
1. **一致性**：注释 English-only（grep 非 ASCII）、godoc 句法、decorder 顺序（`type → const → var → func`）、Go 1.27 惯语（`new(expr)`、`min/max`、`range-over-int`、`slices`）。
2. **文档漂移**：全部 .md × 代码交叉验证——符号存在性、行为断言、引用文件名有效性；幻影 API 引用与「并发」这类被顺口写上的假断言都属此类。「文档与实现不一致按 bug 处理」。
3. **日志纪律**：每个调用点逐个核前缀（DDNS/CLOUDFLARE/IPDETECT/CONFIG）、级别语义（Error=组件失败、Warn=回退、Info=仅状态变化、Debug=例行）、状态变化 Info 行带 record 名 + IP 溯源。
4. **常量**：数字字面量扫描（`\d{3,}` 与 wire 层裸数字）、同一常量唯一归属、Cloudflare TTL 档位与文档一致。
5. **测试质量**：断言不可恒真、错误路径有用例、覆盖缺口显式登记（`go test -cover ./...` 快照）；纯函数必须可测且测到。
6. **工程配套**：CI workflow（测试门禁存在性、失败是否被吞、action 版本）、dependabot 生态覆盖、scripts POSIX/PS 孪生一致性、Dockerfile、config.example.json 同步门禁。

## 3. 执行方式

按规模适配为 3 包级 + 7 交叉维度，可由并行 agent 承担（只读），主审综合排序：

```
Phase 1: 包级（3 并行）
├── Foundation: internal/log + internal/ipdetect（手写 DNS 字节层逐行）
├── Domain:     config + ddns（校验链、Runner 编排、静态 IP）
└── Edge:       providers/* + cmd/zjddns（API 客户端、装配、循环）

Phase 2: 交叉维度（7 并行，grep 驱动）→ §2 的 0-6
Phase 3: Synthesis → 排序 → 主题分析 → 修复计划（直接进 commit，不设报告存档）
```

每个发现记：文件 + 行号、严重度（CRITICAL/HIGH/MEDIUM/LOW）、类别、问题、风险、修复方向。报告假阳性的代价由「逐条核代码再报」纪律控制。

## 4. Sprint 修复策略

| Sprint | 范围 | 标准 |
|--------|------|------|
| 1 | CRITICAL | 立即修复 — 数据损坏、崩溃、panic、安全绕过 |
| 2 | HIGH | 静默失败、竞态、资源耗尽、门禁缺失 |
| 3 | MEDIUM + LOW | 一致性 — 校验、文档、注释、微优化 |

- 提交纪律见 AGENTS.md §Git Discipline：**一类修复一个 commit**，主题行描述修复内容本身（非审计阶段），可带编号引用（`(H1)`）。
- 修复优先级：单行修复 → 模式化修复 → 逻辑重写 → 死代码删除。
- **Round C 复查**：修复完逐 commit 读 diff 抓修复自身引入的回归，全量门禁复跑；连续两轮无新发现才收尾。
- `go run ./cmd/zjddns -generate-config | diff - config.example.json` 是行为兼容的快速回归。

## 5. 根因模式（ZJDDNS 语料）

反复出现即成模式；新发现按此格式追加：

| 模式 | 根因 | 预防措施 |
|------|------|----------|
| **吞错误 = 状态误判** | `recordID, _ := c.RecordID(...)` 把 API 失败与「记录不存在」混为一谈——upsert 变重复创建、delete 变静默成功 | lookup 失败必须传播；「空值 = 不存在」只在无错误时成立 |
| **URL 拼接未转义** | `fmt.Sprintf` 把用户配置（zone_name/record_name）直插 query string | 统一 `url.Values.Encode()`；测试含保留字符用例 |
| **CI 无质量门禁** | workflow 只有 build+push，测试/lint 全靠本地自觉；`continue-on-error` 把平台失败吞成绿色 | 发布前必有 test/lint job 依赖链；禁用 continue-on-error 于发布链 |
| **文档漂移** | 行为删除后文档残留旧描述、引用文件名失效、包注释与实现矛盾 | 每次行为变更 grep 文档；审计期逐条核对行为断言 |
| **注释语言混入** | 测试文件中文注释违反 English-only | 审计 grep CJK 区段 |
| **`_` 丢弃无注释** | 契约只在接口 godoc，调用点裸 `_ =` | 每处行内理由（idiomatic defer-Close 除外） |
| **脚本静默失败** | `grep '\s'` macOS 不识别 → 解析空 → sed 无匹配 → 假装成功；部分暂存文件被 hook 整体重加 | 解析结果先验 semver 形态、替换后 grep 落地校验、hook 用 blob hash 判「格式化器真的改过」 |
| **退出码语义** | fatal 配置错误 `log.Errorf` 后 `return` → exit 0，Docker 重启策略失明 | 启动失败统一走 `fatalStartupError`（exit 1） |
| **注释先于事实** | 文档承诺「打印 🌐」「并发运行」——代码里从未存在或已删除 | 行为断言逐条可 grep；文档照现状写，不照意图写 |

## 6. 已知取舍与设计内行为（勿当 bug 报）

- **Dockerfile 构建期 wget CA bundle**（curl.se）：owner 刻意设计（AGENTS.md §Behavior preservation 登记），勿「简化」成构建镜像内拷贝、勿加校验和。
- **无 context.Context**：超时全部经 `SetDeadline` / `http.Client.Timeout`；此为登记的设计取舍——新增阻塞 I/O 必须同样有界。
- **零 goroutine**：顺序调度循环是基线；`internal/log` 的 writer 无锁依赖这一点。引入并发前先补 writer 锁与 race 测试。
- **API 列表端点不分页**（zone/record 取首页）：DDNS 自用 token 场景数量可控；显式报错优于静默截断，超限时上层 API 错误可感知。
- **parseDNSResponse 不校验 QDCOUNT**：ID/QR/rcode 三重门在前，TXT 内容还要过族校验；错位解析必然报错，不构成路径。
- **非 root 容器未启用**：scratch 无 /etc/passwd，且挂载的 root:600 config.json 会读不了——保持 root 是部署兼容决策。
- **默认级别字符串两处定义**（`log.DefaultLevel` 与 `config.DefaultLogLevel`）：两包同为零依赖 foundation，互不导入；`cmd` 经 `ParseLevelFilter` 已有默认，config 的常量服务 example 输出。
- **`Runner.Run` 的 invalid-mode Error 分支不可达**：config.Validate 已把关；保留为编程错误防线。
- **`staticIP` 夹在 `*Runner` 方法之间**：idiomatic Go，decorder 不约束；接收者分组是视觉约定非硬规则。

## 7. 已登记的测试缺口（下轮审计起步项）

- **`Runner.detector` 不可注入**：auto 检测路径与检测失败路径无法离线测试——接口化 detector 后补 auto/DNS/HTTP/失败四类用例。
- **本地假 DNS server 测试**：`net.Listen("udp"/"tcp")` 回环假服务器覆盖 TC→TCP 重试（dnsquery.go 的传输层，当前覆盖率主缺口）；回环不违反「无外网」测试纪律。
- **版本化发布**：CI 目前只推 `latest`；tag 触发的版本镜像 + GitHub Release 仍是缺口（新增项，需 owner 拍板发布流程）。
- **parseDNSResponse 截断形态用例**：question/answer/TXT 各损坏分支（:130/:140/:147/:155）无手工构造用例。
- **`internal/log` 补测**：`sanitizeLogMessage`（含 NUL/DEL/多字节）、`Level.String`、SetLevel 钳位、Warn 恒通过规则。

## 8. 收尾闭环

1. 修复 → 全量门禁（§1）→ 影响面回归（对应包 `-count=1`）。
2. Round C 复查（§4）连续两轮无新发现即收尾。
3. 每轮把新教训以现在时约束落文档：AGENTS.md（约定 + 一句话教训）、本文件（§5 根因模式 / §6 取舍清单 / §7 缺口追加）。
