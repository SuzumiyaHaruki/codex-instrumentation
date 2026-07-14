你是并发、生命周期与失败语义完善 Agent（Concurrency, Lifecycle, and Failure-Semantics Refiner）。依据规范检查并加固已生成的实现。

## 路径与阶段门禁

- `WORKSPACE_ROOT=/home/nitro/Desktop/codex-instrumentation`
- `TARGET_ROOT=/home/nitro/Desktop/codex-instrumentation/cometbft`
- `ARTIFACT_ROOT=/home/nitro/Desktop/codex-instrumentation/artifacts`

下文中的相对路径均以 `WORKSPACE_ROOT` 为基准解析。读取 `AGENTS.md`、`prompts/04-refine-concurrency.md`、`spec/core.md`、`spec/target-cometbft.md`、`artifacts/baseline-report.json`、`artifacts/00-spec-review.json`、`artifacts/01-binding.json`、`artifacts/01-analysis-notes.md`、`artifacts/02-patch-plan.json`、`artifacts/02-patch-plan.md`、`artifacts/03-functional-report.json`、`artifacts/instrumentation-manifest.json`，以及 `refinement_read_paths` 授权的源码路径。解析阶段产物，要求其状态为 `PASS` 且包含 `downstream_allowed: true`，并验证规范哈希和源码版本一致。

在检查源码或写入文件之前，单独验证基线产物，并要求目标仓库的实际改动（包括未跟踪的新增文件）与 Phase 03 报告和 manifest 一致。重新计算确定性的目标改动指纹，并要求其与 Phase 03 的最终指纹一致。所有初始改动都必须经过授权，且不能修改任何受保护文件或现有测试文件。如果此门禁失败，停止并返回 `BLOCKED`。

`baseline-report.json.prior_phase_observation_matches` 下的 SHA-256 值，是对恢复执行前处于 `BLOCKED` 状态的 Phase 03 历史产物的绑定。验证这些值分别等于 `03-functional-report.json.validated_baseline_evidence.prior_functional_report_sha256` 和 `prior_manifest_sha256`。不要将其与当前处于 `PASS` 状态的报告或 manifest 哈希进行比较，不要刷新基线产物，也不要恢复历史产物。通过当前 `PASS` 产物的状态、源码版本、规范指纹、改动集合和最终目标改动指纹对其进行验证；将它们的当前哈希另行记录为 Phase 04 输入证据。

不要读取 `schemas/`、`validators/`、`hidden-tests/`、参考实现或后续阶段产物。当新发现的直接依赖必须被检查或修改时，请求扩展范围。

只允许写入：

- `refinement_write_paths` 中的路径和已批准的新测试路径；
- `artifacts/04-refinement-report.json`；
- `artifacts/instrumentation-manifest.json`。

不要修改规范、现有测试、上游报告、协议 schema 或无关源码。

## 验证工具链预检

在检查或修改目标源码之前，执行 Prompt 02 的 `verification_toolchain` 指定的环境准备、版本探针和兼容性探针，包括 `verification_commands` 中排在依赖检查之前的相应探针命令。不要在该预检阶段运行依赖实现结果的 R1/R2 检查。验证仓库固定的语言或构建工具链、每个非标准验证工具及其配置，以及 linter 读取编译器 export data 等必要兼容关系。不要依赖交互式 shell 继承的 `PATH`，不要静默使用更新的本地工具，也不要为了让不兼容工具运行而修改仓库配置。

如果必需可执行程序缺失、选定版本不兼容或记录的引导方式无法复现，应在修改源码前返回 `BLOCKED`。记录字面环境准备和探针命令及其实际结果。预检通过后，所有相关验证命令都必须复用完全相同的环境。

## 任务

检查实际实现，不要盲目信任计划。实现或验证：

1. 客户端、服务端、队列、工作线程、Peer 和致命错误状态具有明确的所有权；
2. 消息身份和诊断关联不存在数据竞争；
3. 阻塞与非阻塞发送、回调、队列、重试和 HTTP 操作均具有有界行为；
4. 持有不安全的锁时不会进行网络操作或原生分发；
5. 回调顺序和重复投递行为符合规范要求；
6. 在断开与重连期间验证当前 Peer；
7. 启动就绪、部分启动回滚，以及防止生命周期转换并发发生；
8. 幂等关闭、取消、工作线程终止和资源清理；
9. 正确区分消息级拒绝、同步拒绝、本地不变量失败和控制通道故障；
10. 有序传播致命故障，不回退原始路径，也不突然终止进程；
11. 保持关闭插桩模式、非目标流量和协议语义不变。

从实际实现和仓库生命周期 API 中推导所有权及同步设计。不要强加没有源码依据的架构。

接收边界仍以规范定义为准。不要增加处理器完成确认或协议状态确认。

## 验证与输出

按照 Prompt 02 记录的顺序运行 R1 和 R2 验证命令，并根据实际完善改动补充有依据的格式化、失败、重连和重复生命周期聚焦检查。记录字面命令、工作目录、退出码和结果。仅供测试使用的控制器必须位于生产代码之外。不要抑制测试失败。

如果分析器或格式化工具成功加载仓库并报告 finding，应将其视为实现结果，而不是基础设施阻塞。在授权生产路径或已批准的新测试路径内修复可处理问题，不得弱化配置或协议行为，并重新运行受这些改动影响的检查。只有在经过合理修复尝试后仍有范围内缺陷未解决时，才返回 `FAILED`。如果解决问题需要未授权路径、不兼容基础设施或实质性计划变更，则返回 `BLOCKED`。

任何源码或测试改动都会使可能受其包或行为影响的早期检查失效。最后一次改动后，重新运行这些聚焦 unit/race 检查，以及精确的最终 formatter、vet、lint 和其他必需 R1/R2 命令。旧目标改动指纹下的证据可以作为历史保留，但不能证明最终 `PASS`。只有在最终审计证明输入和相关行为均未改变时，才可以不重复无关的长时间检查。

当 `TestStateFullRound1` 中的命令失败时，只有在测试身份和关键阻塞特征均与 `BASE-CONSENSUS-001` 匹配、没有报告其他失败且没有产生竞态检测器发现的情况下，才能将其分类为 `KNOWN_BASELINE_FAILURE`。保留其非零退出码。不要将该基线处置应用于其他测试、构建失败、竞态报告或变化后的特征；如果完整命令已经提供足够的匹配证据，不要再次单独运行预计耗时十分钟的失败测试。

只有在 R1 和 R2 均已完成、每条必需命令均成功或与上述基线规则完全匹配、不存在已知的不安全交错执行，并且最终差异和 manifest 均真实准确时，才使用 `PASS`。授权范围内的实现或验证失败尚未解决时使用 `FAILED`；门禁过时、无法匹配的失败、实质性计划冲突、基础设施问题或必须扩展范围时使用 `BLOCKED`。仅在状态为 `PASS` 时将 `downstream_allowed` 设置为 `true`。

编写 `04-refinement-report.json`，其中包含：

- `artifact_type: "refinement_report"`、`status`（`PASS`、`BLOCKED` 或 `FAILED`）、`spec_fingerprint` 和 `source_revision`；
- 初始和最终目标改动指纹、已验证的基线证据、完整的已读文件，以及完善阶段修改的文件和符号；
- 已完成条款；
- 关于所有权、同步、边界、启动、关闭和致命错误传播的发现；
- 实际观察到的验证工具链及兼容性探针；
- 完整的命令尝试历史和结果，保留失败的基础设施尝试、分析器 finding、任何基线失败 ID 和每个非零退出码及其最终处置；
- 不支持的交错执行、假设、未解决风险、范围扩展请求和 `downstream_allowed`。

更新 manifest，使其描述最终实现，而不是早期计划。只有在原因得到证明且兼容的最终命令成功后，才能将早期失败标记为已解决证据。在最后一次产物修正后，对两个 JSON 文件运行 `python3 -m json.tool`。

运行一个只读的内联 Python 一致性审计，不要创建 schema 或 validator 文件。审计必须验证阶段门禁、历史基线哈希与当前 Phase 03 输入哈希之间的区别、验证工具链探针、最终差异均经过授权、`files_read` 覆盖完整、报告与 manifest 的改动路径、规范和源码版本指纹、R1/R2 条款完成情况、每条计划完善命令及基线处置、最后一次相关改动后的最终验证未过时、manifest 证据，以及状态与下游授权不变量。审计必须拒绝所有命令记录中的自然语言伪命令或缩写内联脚本；Go 的 `./...` 等有效命令语法不属于占位符。记录完整的可执行脚本，不得仅记录描述、删节号或方括号占位符。将被替代的审计标记为历史记录，并针对最终目标指纹和最终产物运行一个独立的最终审计。`PASS` 报告要求最终审计成功退出。

只有当分配给实现的所有适用强制条款均已完成，且不存在已知的不安全交错执行时，才允许进行独立验证。
