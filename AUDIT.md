# AUDIT.md — ZJDDNS 审计手册

本文档定义 ZJDDNS 的端到端审计与修复流程，方法论对标 ZJDNS（`docs/AUDIT-METHODOLOGY.md`），按本项目规模适配：包更少、链路更短，但维度、Sprint 纪律、门禁与存档要求不减。每次审计照此执行：**多维度并行审查 → 按严重程度分 Sprint 修复 → 质量门禁 → Round C 修复面复查 → 存档回填**。

按轮审计，直到一轮不再产生新发现为止。每轮详细发现与修复计划存档于 `docs/audit/`；新教训回填本手册与 AGENTS.md，同一错误不审计两次。

### 历史记录

| 轮次 | 时间 | 主题 | 报告 | 状态 |
|------|------|------|------|------|
| 1 | 2026-11 | 首轮全量审计：3 包级（foundation / domain+ddns / provider+cmd）+ 7 交叉维度，重点：手写 DNS 字节层的解析防御、Cloudflare URL/JSON 边界、日志纪律、文档漂移（CLAUDE.md→AGENTS.md 迁移同期执行） | `docs/audit/2026-11-round1/` | 见存档 |

---

## 一、审计维度

每个文件、每个包在以下维度接受审查（标注 ✂ 的维度按本项目形态裁剪，但结论要显式记录，不留空白）：

| 维度 | 关注点 |
|------|--------|
| **代码质量** | 死代码、冗余代码、重复代码、低效代码；`Foo`/`FooWithX` 冗余函数对 |
| **内存安全** | 切片别名共享底层数组、无界增长（本任务形态：单次运行/低频 ticker，重点在配置数组与响应解析缓冲） |
| **锁正确性** | `internal/log` 的 atomic level + RWMutex filter 的正确配对；新增并发时的 race 面（基线：项目顺序执行，无 goroutine） |
| **耦合度** | 导入分层 DAG 违规（AGENTS.md §Architecture）；provider 细节泄漏进 `ddns` / `cmd` |
| **架构设计** | 消费侧 Provider 接口契约、段数组语义（一段一实例）、常量唯一归属（`config/defaults.go`） |
| **性能** | 每 tick 的 API 调用数（content-compare 免 no-op）、检测次数（union-type 合并）、重复编译 regex / 重复构造 client |
| **Panic 检测** | nil 解引用、切片越界（手写 DNS 解析每一步 offset 推进必须有界）、裸类型断言、空 map 写入 |
| **错误处理** | `%w` 包装链路完整（sentinel `ErrZoneNotFound`/`ErrRecordNotFound` 可被 `errors.Is` 穿透）；错误信息可定位（带 record/zone/server）；`_ =` 丢弃必须有行内注释说明原因 |
| **参数校验** | 配置值进入网络调用前的合法性（族校验、TTL 白名单）；用户配置的字符串拼进 URL/查询串前是否转义 |
| **Context 传播** ✂ | 本项目无 ctx 参数——超时全部经 `SetDeadline` / `http.Client.Timeout`；此为登记的设计取舍，新增阻塞 I/O 必须同样有界，不留无限等待 |
| **Goroutine 生命周期** ✂ | 基线：零 goroutine（顺序调度循环）。任何新 goroutine 必须有 owner、取消路径与 panic 防线 |
| **资源生命周期** | resp.Body 关闭（defer + 忽略注释）、idle connection 清理（`CloseIdleConnections`）、Close 幂等 |
| **日志质量** | 规范前缀（DDNS/CLOUDFLARE/IPDETECT/CONFIG）；级别语义（Error=组件失败、Warn=回退、Info=仅状态变化、Debug=例行）；Info 带 record 名 + IP 溯源；默认 info 安静是特性 |
| **文档质量** | README / AGENTS.md / docs/providers/* 与代码交叉验证（符号存在、行为一致）；注释引用符号名而非行号；无日期/审计编号/变更史叙述 |
| **常量提取** | 魔法数字抽为命名常量；同一常量唯一归属；常量值与 RFC/供应商规范一致（Cloudflare TTL 档位） |
| **RFC 一致性** | 手写 DNS 层对照 RFC 1035（报文格式、压缩指针、TXT character-string）、RFC 7766（TCP 2 字节长度前缀）；偏离处注释说明理由 |
| **注释风格** | godoc、English only（含测试）、导出符号注释以其名开头、完整句号结尾；`//nolint:NAME // reason` 全部带理由 |
| **函数排序** | `type → const → var → func`（decorder 强制）；`New*` 紧跟 var 块；同接收者方法聚合 |
| **Go 版本特性** | go.mod 1.27.0 的现代器是否应用（`go fix`）；手写模式可否被 `slices`/`new(expr)`/`strings` 新惯语替代 |
| **测试质量** | 表驱动 + `t.Run`；断言不可恒真；错误路径有用例；覆盖缺口显式登记（网络路径经注入 baseURL / 手工字节覆盖） |

## 二、审计架构

按规模适配为 3 包级 + 7 交叉维度，可由并行 agent 承担；结论汇总到综合报告。

```
Phase 1: 包级审计（3 并行）
├── Foundation: internal/log + internal/ipdetect（含手写 DNS 字节层逐行）
├── Domain:     config + ddns（校验链、Runner 编排、静态 IP 解析）
└── Edge:       providers + providers/cloudflare + cmd/zjddns（API 客户端、装配、循环）

Phase 2: 交叉维度（7 并行，grep 驱动）
├── CrossCut Panic/Validation:  越界、nil、`_ =` 丢弃清单、URL 拼接转义
├── CrossCut Error:             %w 链路、sentinel 可达性、错误上下文充分性
├── CrossCut Logging:           每个日志调用点的前缀/级别/信息量
├── CrossCut Docs:              全部 .md × 代码符号交叉验证
├── CrossCut Constants:         数字字面量扫描、TTL 档位 vs 文档
├── CrossCut Style:             注释语言/句法、decorder、Go 1.27 惯语
└── CrossCut Infra:             CI workflows、scripts（POSIX/PS 孪生）、Dockerfile、config.example.json

Phase 3: 综合
└── Synthesis: 排序 → 主题分析 → 修复计划（00-plan.md）
```

### 审计报告格式

每个发现包含：**文件路径 + 行号**、**严重程度**（CRITICAL/HIGH/MEDIUM/LOW）、**类别标签**、**问题描述**、**风险说明**、**修复建议**。

## 三、修复流程

| Sprint | 范围 | 标准 |
|--------|------|------|
| Sprint 1 | CRITICAL | 立即修复 — 数据损坏、崩溃、panic、安全绕过 |
| Sprint 2 | HIGH | 下个发布周期 — 资源泄漏、竞态、静默失败 |
| Sprint 3 | MEDIUM + LOW | 一致性 — 校验、文档、注释、微优化 |

同一 Sprint 内按序：单行修复 → 模式化修复 → 逻辑重写 → 死代码删除。每类修复单独 commit（规范见 AGENTS.md §Git Discipline）；修复必须带影响面回归（对应包测试 + 全量 lint）。

## 四、质量门禁

### 每次提交前

```bash
go build ./...                    # 零编译错误
go fix ./...                      # 自动现代器
golangci-lint run                 # 零警告
golangci-lint fmt                 # 格式化
```

### 每个 Sprint 后

```bash
go test ./...                     # 全部通过
go test -race ./...               # race 全绿（基线：顺序执行，理应零竞争）
```

### 性能回归

本项目无 benchmark 基线（低频 ticker 场景，微基准噪声大于收益）。性能维度的审计结论以**结构判据**代替：每 tick 的 API 调用数、每类型的检测次数、热路径无重复构造。若未来引入热路径（如高频调度），先建 benchmark 基线再谈回归。

## 五、常见根因模式（ZJDDNS 语料）

历轮审计沉淀，新发现按此格式追加：

| 模式 | 根因 | 预防措施 |
|------|------|----------|
| **URL 拼接未转义** | `fmt.Sprintf` 把用户配置（zone_name/record_name）直接插进 query string——含空格/保留字符的名字静默查不到 | 统一 `url.Values.Encode()`；测试含非常规字符用例 |
| **`_` 丢弃无注释** | 错误返回值被 `_` 吃掉且无理由注释——签名变更时静默丢语义 | 每处 `_ =` 带行内原因（引用拥有日志的层级） |
| **文档漂移** | 行为删除后 AGENTS.md/README 残留旧描述；引用文件名失效（CLAUDE.md 已退役并入 AGENTS.md） | 每次行为变更 grep 文档；审计期逐条核对 docs 中的行为断言与文件引用 |
| **注释语言混入** | 测试文件中文注释违反 English-only 约定 | 审计 grep 非 ASCII 字符 |
| **解析偏移无界推进** | 手写字节层 offset 推进后未查界就切切片 | 每步推进后 `offset+N > len` 判定；用例覆盖截断形态 |
| **多记录同名取首** | `records[0]` 无分页/去重语义说明——同名多记录时只更新第一条 | 文档化语义；若 API 可返回多条需显式策略 |
| **API 分页缺失** | 列表端点未带 per_page 分页——zone/record 超单页上限时静默漏 | 记录为已知边界，超限时显式报错优于静默截断 |

## 六、审计报告存档

```
docs/audit/<YYYY-MM>-round<N>/
├── 00-plan.md          ← 综合报告（排序、主题分析、逐项修复计划 + 全覆盖清单）
├── 01-foundation.md    ← internal/* 包审计
├── 02-domain.md        ← config + ddns 审计
├── 03-edge.md          ← providers/* + cmd 审计
└── 04-crosscut.md      ← 交叉维度审计（7 维度合卷）
```

每轮结束后更新 §历史记录 表；教训回填 §五 与 AGENTS.md。
