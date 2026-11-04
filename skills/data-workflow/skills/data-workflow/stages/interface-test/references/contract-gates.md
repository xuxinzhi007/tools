# 契约门：判据、命令与已知环境事实

`../../interface-test/SKILL.md` 的三级门在这里给具体命令与判据。

**下面所有「本仓事实」（文件路径、Makefile 目标、环境变量名）都会随 commit 变化**：用之前先核一眼，不要照抄。核对命令附在每条后面。

---

## 门 ① 进程内契约测试（硬门）

### 为什么它是硬门

`api/**` 下已有一批 `*_contract_test.go`：用 kratos 的 `http.Server` + `httptest` 在**进程内**起服务打真实路由，不需要 DB、Redis、Kafka 或已部署的服务。所以它在任何机器上都能跑，也就没有跳过的理由。

```bash
ls api/*/*/*_contract_test.go        # 核对：现有哪些契约测试
go test ./api/... 2>&1 | tail -30    # 门 ①
```

失败时贴：包名、用例名、断言行。不要只说「测试失败」。

### 什么时候要补一条

改动碰到了某个端点，而 `api/**` 里没有对应契约测试 → 照最近的一份同类文件补最小用例（一个请求体 + 断言 HTTP 状态与关键字段）。补测的位置在 `api/<模块>/<版本>/`，**不要放进 `tests/api/`**（那里需要真实服务）。

```bash
grep -rln "httptest" api/ | head -3   # 核对：既有写法参考哪份
```

---

## 门 ② 契约破坏性 diff（硬门）

### 为什么比 proto 不比 openapi.yaml

`openapi.yaml` 在本仓是**生成物且被 gitignore**（`git check-ignore -v openapi.yaml` 可核对），拿不到历史基线；`api/**/*.proto` 是 tracked 的事实源。生成命令是 `make api`（`Makefile` 的 `api` 目标），`make api-one` 需要 `PROTO=`，裸跑会失败。

```bash
git check-ignore -v openapi.yaml     # 核对：确实是忽略的生成物
bash "$PLUGIN_ROOT/scripts/contract_diff.sh" main
```

脚本用 `scripts/proto_contract.py` 把两侧 proto 归一化成事实行（RPC / REQ / RES / FIELD / ENUMVAL），基线取 `merge-base(base, HEAD)`，对比面取**工作区**（含未提交改动），输出七类：

| 输出段 | 含义 | 默认判定 |
|---|---|---|
| 删除的 rpc | 调用方直接 404 / Unimplemented | breaking |
| 端点变更 | 同一 rpc 的 method 或 path 变了 | breaking |
| 删除的字段 | 请求体字段消失 = breaking；响应体字段消失 = 前端读不到 | breaking |
| 字段号复用/改名/改类型 | 同一 `message+number` 换了名字或类型 | breaking（线上不兼容，最隐蔽） |
| 删除的枚举值 | 旧客户端还会发这个值 | breaking |
| 枚举号复用 | 同一 `enum+number` 换了名字 | breaking |
| 删除的 proto 文件 | 整个模块下线 | breaking |

`ADDED` 段只记录，不判 breaking。

### 裁决怎么写

脚本恒 exit 0，**判定权在人**。每行 BREAKING 必须写：

```
<类别> <对象> → 谁需要跟着改：<前端/网关/其他服务/无> ；已通知：<是|否|不适用> ；可接受理由：<...>
```

机器测不出的那一类要单独点名：**字段名不变、含义变了**（单位、取值域、口径）。这种 diff 是干净的，只有人能判。发现就写进 CONCERNS。

`UNPARSEABLE` 的常见原因是新分支首次引入 proto、或 base ref 写错 —— 裁 WAIVED 并写明。

---

## 门 ③ 场景回放（条件门）

### 依赖

`tests/api/` 下的用例打真实 HTTP：需要**已运行的服务**（默认 `http://localhost:8000`，可用 `TEST_BASE_URL` 覆盖）+ 可达的 DB。鉴权靠网关注入的 `x-xr-auth-*` 头（`uid` / `sub` / `tid` / `aut` / `type`），测试里用固定的假身份。

```bash
ls tests/api/                                        # 核对：现有场景用例
grep -rn 'TEST_BASE_URL\|x-xr-auth-' tests/api/*.go | head   # 核对：base url 与鉴权头
sed -n '1,40p' scripts/test-api.sh                   # 核对：仓库自带的跑法
```

### 探活再决定跑不跑

```bash
curl -sS -o /dev/null -w '%{http_code}\n' --max-time 3 "${TEST_BASE_URL:-http://localhost:8000}/"
```

- 非 000 → 服务在，跑回放：`bash scripts/test-api.sh` 或 `go test ./tests/api/... -run <相关用例>`。
- 000 / 超时 → **裁 WAIVED**，写清三件事：缺什么（服务未起 / DB 不可达 / 缺测试身份）、哪些端点因此没被覆盖、谁在什么条件下补跑。

### 红线

- 不为了跑门 ③ 去改配置连线上库，不往真实环境写数据。
- 本地起服务需要云上 RDS/Redis/gRPC 时，起不来就是起不来，如实 WAIVED；不要伪造输出。
- 产物里的鉴权头一律写占位符（`x-xr-auth-uid: <test-uid>`），不贴真实值。

---

## 与其它门的边界

| 想做的事 | 该去哪 |
|---|---|
| 构建、单测覆盖率、`make test` | `../../quality-gate/SKILL.md` |
| 调用方清单、状态机消费点、权限与回滚 | `../../impact-assessment/SKILL.md` |
| 线上真实症状与运行时证据 | `../../oncall/SKILL.md` |
| 逐行代码规范 | `../../change-review/SKILL.md` |

接口测试只回答「契约与端点行为有没有被证据覆盖」，不重复上面四件事。

---

## e2e 的边界

`tests/e2e/` 需要显式开关（形如 `E2E_RUN=1`）才跑，属于更重的验证。**默认不跑**；用户点名要跑，或改动跨了多个服务边界时才跑，且同样先探活、同样如实记 WAIVED。

```bash
grep -rn 'E2E_RUN' tests/e2e/*.go | head -3   # 核对：开关名与默认行为
```
