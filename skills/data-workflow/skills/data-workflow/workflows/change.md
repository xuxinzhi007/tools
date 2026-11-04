# 一次改动

这条用于 L1：已有边界内的局部改动。分级在 `references/scope.md`。把任务叫成小改动，不能改受保护路径。改动落在该项目 `config.yaml` 声明的 `protected_paths`（data-service 实例是 `api/`、proto、pb.go、`internal/biz/status.go`），或涉及状态机、租户权限、回滚、持久化迁移时，不用这条，用 `feature` 或 `hotfix`。

`decision.md` 回答 `references/scope.md` 里的那几问，简单任务用几句。第一句是用户真正要解决的问题，不是他随口说的实现。接着写怎样算满足、复用哪个 `文件:符号`、最小充分方案、相关的债、这次明确不做的事。无关的债写在「不做」里，不进 diff。

定架构时，同一份 `decision.md` 写四句：现有边界各管什么、这次不新增什么、下一刀落在哪个边界、还没有出处的词。词的含义走知识层，空则待确认。不套 feature 的九步。项目把 `protected_paths` 和 `waiver_globs` 写成自己的路径；两个键都不出现时默认没有受保护路径（不拦），data-service 实例在其 `config.yaml` 里注入门禁路径。只定架构、还没有该仓库的测试入口时，`done` 会停在 `RED`，不要写成通过。

1. 按 `templates/decision-template.md` 写 `specs/<feature>/decision.md`。第一屏：**原话（一字不改） / 我理解你要的是 / 可能理解错的地方 / 本质 / 怎样算满足 / 未知 / 这次不做**。原话就是用户那句，不改写——后面所有工件引用它。`workflow_next` 在 `原话` / `我理解你要的是` / `可能理解错` / `本质` / `未知` 上做字面量检查，缺一行就打 `GATE` 并停；这是内容门，不是语义门，别把它当“已想清楚”的证明。还剩一个选择时，文件里写上「待拍板」，然后运行 `workflow_next.py wait`。`reply` 是对方能原样粘贴回来的一句。写完停止，不要接着改代码。

   **明白时说清楚，含糊时才问。** 「我理解你要的是」是把用户的话换成自己的话写一遍，供用户一眼校验。「可能理解错的地方」写过真歧义，也写「无歧义」——但只有在真的没有歧义时才这么写。歧义真的影响方案，就 `wait`，不要猜一个看起来合理的。不影响方案的细节，写明假设直接往下走。
2. 对方回了那一句，运行 `answer`。然后只按 decision.md 改文件。改完运行 `done`。`done` 先跑 `gate_check`（`change` 的编码阶段检查工作树真的动过：一个文件都没变就拦住），再跑这个仓库自己的测试入口（`verify`，或 package.json / Makefile / pytest / cargo 里已经写明的那条）。退出码不是 0 就停，输出在 `specs/<feature>/verify.log`。

   要放行 `protected_paths` 时用 `workflow_next.py approve <feature> design|waiver <前缀>`，不要手写 `specs/passes.log`；没有已答复的拍板时它会拒绝。
3. 写 `specs/<feature>/check.md`：先写 verify.log 的 exit 那一行，再写验收条件是否达到、diff 有没有超出 `decision.md`。`check.md` 缺 `exit` 或 `verify.log` 时 `next` / `done` 打 `GATE` 并停。没有配置 verify 时，`done` 停在 RED，不要自己写成通过。退出码 0 只说明命令通过。

保护哪些路径看该项目 `config.yaml` 的 `protected_paths` 和 `waiver_globs`。这两个键都没写时，默认没有受保护路径，`specs/` 之外直接放行（见 `references/edit-gate.md`）。

## 这套东西不替代你的判断

门只检查「有没有写」，不检查「写得好不好」。这是故意的：

- 字面量门能拦住「没想就往下走」，拦不住「写了句废话」。后者靠的就是模型自己的判断力，不该让格式要求去均它。
- 写「无歧义」是合法答案。门不会因为你没写歧义就卡你，也不会因为你写了就认为你真的想过。
- 不清楚时最优动作是 `wait`（问人），不是先编一个完整方案。但清楚的、小的、可逆的改动，也应该一口气做完——加门不是把每件事都变成问答。
