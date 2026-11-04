# Interface Test（接口测试与契约门）

回答一个问题：**接口层面的改动，有没有可执行的证据证明它没破坏调用方。**

> Go 栈阶段（data-service 实例）：本文件的 Go 门（`go test ./api/...`、`contract_diff.sh`）仅当项目声明 `stack: go` 时适用。未声明 Go 栈的项目走通用质量门，用该项目声明的测试与契约入口。

判据与命令的完整清单在 [`references/contract-gates.md`](references/contract-gates.md)。本文件只给顺序与裁决规则。

所有命令在 `$REPO` 下跑（`eval "$(bash "$PLUGIN_ROOT/scripts/paths.sh")"`）。**每条门的结论必须贴命令与输出摘录**，不接受「测过了」。

## 三级门

| 级别 | 门 | 依赖 | 性质 |
|---|---|---|---|
| ① | 进程内契约测试：`go test ./api/...`（kratos http.Server + httptest，进程内起服务） | 只需 Go 工具链 | **硬门**，不可跳过 |
| ② | 契约破坏性 diff：`bash "$PLUGIN_ROOT/scripts/contract_diff.sh" <base>` | 只需 git + python3 | **硬门**，不可跳过 |
| ③ | 场景回放：`bash scripts/test-api.sh` 或 `go test ./tests/api/...` | 需要**已运行**的服务 + DB | **条件门**，探活失败即裁 WAIVED |

①② 任何机器上都能跑，所以没有借口。③ 依赖环境，跑不了就如实记 WAIVED，**不许谎报 PASS**，也不许因为跑不了就把整条接口测试判为「无风险」。

## 执行顺序

1. **确定 base**：默认 `main`；用户指定分支/commit 时用指定值。记下来，产物里要写。
2. **跑门 ①**：`go test ./api/...`。失败 → 直接 FAIL，贴失败用例名与断言行，不继续往下（后面的门无法解释这个失败）。
3. **跑门 ②**：`bash "$PLUGIN_ROOT/scripts/contract_diff.sh" <base>`。
   - 输出 `OK` → 门 ② 通过。
   - 输出 BREAKING / NEEDS-RULING → **停下**，逐行裁决：谁需要跟着改、是否已通知、是否可接受。裁决结果写进产物；未裁决 = FAIL。
   - 输出 UNPARSEABLE → 门 ② 裁 WAIVED，写明原因（例如新分支首次引入 proto）。
4. **探活门 ③**：`curl -sS -o /dev/null -w '%{http_code}' "${TEST_BASE_URL:-http://localhost:8000}/<任一已知健康或轻量端点>"`。
   - 通 → 跑场景回放，按输出裁决。
   - 不通 → 裁 WAIVED，写清「需要什么才能跑」（服务 + DB + 鉴权头），并列出**没被覆盖的端点**。
5. **补测缺口**：本次改动碰到的端点，如果 ①③ 都没覆盖到，写一条最小 httptest 契约测试（照 `api/**/*_contract_test.go` 的既有写法），或明确记 UNVERIFIED。

## 四态裁决

| 状态 | 判据 |
|---|---|
| **PASS** | ①② 通过，且改动碰到的端点被 ① 或 ③ 覆盖 |
| **CONCERNS** | ①② 通过，但有端点无覆盖、或契约 diff 出的是「语义变更」这类机器测不出的项 |
| **FAIL** | ① 失败、或 ② 有未裁决的 BREAKING |
| **WAIVED** | 某一门因环境缺失未执行（只能用于门 ③，或 ② 的 UNPARSEABLE）；必须写清缺什么、影响哪些端点 |

WAIVED 是**记账**，不是免罪：产物里必须留下「谁在什么条件下补跑」。

## 禁止

- 把「编译通过」「单测通过」当成接口测试通过。
- 门 ② 报 BREAKING 却只写「兼容」不给理由。
- 为了跑门 ③ 去改配置连线上库、或往真实环境写数据。**只连本地或测试环境**，不确定就 WAIVED。
- 在产物里贴 token、Cookie、真实租户 ID、客户数据（鉴权头写占位符）。

## 产物

落 `specs/<feature>/interface-test.md`（模板 `$PLUGIN_ROOT/templates/plan-template.md` 的验证段，或单独成文）。`review-only` 审别人的工作树时，改落到该工作流的 `artifacts_root`，不写进对方仓库。一屏摘要：

```
接口测试 <feature>  base=<ref>
门① go test ./api/...      PASS|FAIL   <用例数 / 失败用例名>
门② contract_diff.sh       PASS|WAIVED|NEEDS-RULING   breaking ×N（逐行裁决）
门③ 场景回放               PASS|WAIVED   <探活结果 / 覆盖端点>
覆盖缺口                   <端点> ×N（补测 or UNVERIFIED）
裁决                       PASS | CONCERNS | FAIL | WAIVED
补跑条件                   <谁、在什么环境下、跑哪条命令>
```

## 自查（输出前）

- 三级门是否都有命令与输出摘录，没有「应该没问题」？
- BREAKING 清单是否逐行有裁决，而不是整体一句「兼容」？
- WAIVED 是否写清了缺什么、影响哪些端点、谁补跑？
- 有没有为了过门去动环境或数据？
