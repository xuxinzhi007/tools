# 爆炸半径：判定细则与 grep 配方

`../../impact-assessment/SKILL.md` 的证据怎么取。**只有方法与命令模板，没有任何业务取值**；命中结果一律以当前 checkout 为准。

所有命令在 `$REPO` 下执行（`eval "$(bash "$PLUGIN_ROOT/scripts/paths.sh")"` 解析）。

---

## 通用纪律

1. **先 grep 再下结论。** 写「无调用方」之前必须有一条空结果的 grep 命令作为证据，命令本身要贴进产物。
2. **计数要可复现。** 用 `grep -rn ... | wc -l` 或 `grep -rl`，不用「大概几处」。
3. **读取方和写入方分开查。** 状态类改动的漏项 90% 在读取方。
4. **命中太多时收敛而不是放弃。** 先按目录分层（`internal/service`、`internal/biz`、`internal/data`、`api/`、`configs/`），逐层给数。

---

## 轴 1 调用方

```bash
# proto 里的字段/RPC 定义（契约事实源）
grep -rn "<字段名或RPC名>" api/ --include='*.proto'

# 生成物引用点（说明有多少 Go 代码 touching 它）
grep -rn "<GoSymbol>" --include='*.go' internal/ | wc -l

# biz 方法的调用点（含测试，测试也是调用方）
grep -rn "\.<MethodName>(" --include='*.go' .

# 共用同一 Repo 方法的调用方（改 SQL 谓词时必查）
grep -rn "<RepoMethodName>(" --include='*.go' internal/biz internal/service

# 前端可见字段名（json tag / proto 名）
grep -rn 'json:"<field>"' --include='*.go' internal/ api/
```

**判 breaking 的四类**（命中任一类，产物里必须写「谁需要跟着改」）：

| 类型 | 判据 |
|---|---|
| 删字段 / 删 method | 基线 openapi 或 proto 里有，改后没有 |
| 可选变必填 | 新增 `required` 语义、或 biz 新增缺参即整批拒绝的校验 |
| 语义变更 | 字段名不变但含义/单位/取值域变了（最隐蔽，最容易漏） |
| 错误码/HTTP 状态变更 | 同一场景返回码变了，调用方按码分支就会走错 |

契约层面的机器判定交给 `$PLUGIN_ROOT/scripts/contract_diff.sh`，不要靠肉眼比 proto。

---

## 轴 2 状态机全库追踪

新增或修改一个状态/枚举常量时，按下面六个消费面逐个 grep。**每个面都要给命中数**，命中 0 也写 0：

```bash
# ① switch / case 列举
grep -rn "switch .*<StatusName>" --include='*.go' internal/
grep -rn "case <ConstName>" --include='*.go' internal/

# ② 白名单 / 集合 / map
grep -rn "<ConstName>" --include='*.go' internal/ | grep -E "map\[|\[\]|In\(|allow|valid|支持"

# ③ SQL 过滤与计数
grep -rn "<column_name>" --include='*.go' internal/data | grep -E "Where|In|Not|Count"

# ④ 终态/派生判定函数
grep -rn "func Is[A-Z]" --include='*.go' internal/biz

# ⑤ 队列 / 调度 / 导出 / 通知的资格条件
grep -rn "<ConstName>" --include='*.go' internal/biz internal/job 2>/dev/null

# ⑥ 展示映射（响应转换层）
grep -rn "<ConstName>" --include='*.go' internal/service
```

**为什么必须全查**：漏一个 `switch` 的分支不会编译报错，会静默走 `default`；漏一个白名单不会报错，会让新状态的数据在列表里消失。这两种都是「上线后才发现」的类型。

同时查**状态族归属**：同名数字在不同族里含义不同（`../../oncall/references/state-protocol.md` 有状态族隔离五规则）。新增状态必须写清属于哪一族，否则读取方会串族。

---

## 轴 3 租户与权限

```bash
# 权限码（proto option 原文，连 description 一起贴）
grep -rn -B2 -A2 'code: *"<模块>:<资源>:<动作>"' api/ --include='*.proto'

# 数据范围口径：这次改动复用了哪个 scope 函数
grep -rn "func apply[A-Za-z]*Scope" --include='*.go' internal/

# 同一资源的两种取数口径（管理员跨租户 vs 租户内）
grep -rn "FindBy<A-Za-z>*ID\b\|FindBy<A-Za-z>*IDAndTenantId" --include='*.go' internal/data

# 管理员例外分支
grep -rn "SYS_ADMIN\|IsAdmin\|aut ==" --include='*.go' internal/ | head -30
```

产物里必须回答三句话：

1. 新入口的权限码是什么（贴 proto 原文行）。
2. 数据范围复用了哪个已有 scope 函数；如果是新写的，为什么已有的不够用。
3. 本次改动是否引入新的跨租户可达路径：是/否 + 证据。

**租户缺失时的行为**要单独写：整批拒绝还是部分成功、错误码是什么、前端会看到什么。这个口径经常和调用方预期不一致。

---

## 轴 4 回滚

按这个模板填，不许写「回滚代码即可」：

```
代码回滚
  1. revert <commit 范围>；是否依赖发布顺序：是（先 X 后 Y）/ 否
  2. 配置项 <名字> 需要改回 <值> 吗：是/否
数据回滚
  1. 新增列 <名字>：保留 / 删除（删除前需确认没有写入）
  2. 已写入的 <N> 行 / 已变更的状态：可逆向（脚本）/ 不可逆
  3. 不可逆时：备份在哪、能否重放、最坏影响是什么
半回滚
  开关 <名字>，默认 <值>，关掉后旧数据 <怎么处理>
  无开关时属于哪种允许情形：只读 / 幂等写 / 可重放
```

**阻塞判据**：数据不可逆 + 无备份 + 无重放路径 = 阻塞，回需求阶段。

---

## 维护性三查

```bash
# 副本计数：把这段逻辑的关键表达式拿出来 grep
grep -rn "<关键表达式片段>" --include='*.go' internal/ | wc -l

# 新增导出符号（对比基线）
git diff <base>...HEAD --unified=0 -- '*.go' | grep -E '^\+(func|type) [A-Z]'

# 已有 helper 是否已有同语义函数（新建 helper 包前必查）
grep -rn "func <动词>" --include='*.go' internal/pkg/ | head -20
```

| 判定 | 说明 |
|---|---|
| 第 2 次出现同语义逻辑 | 必须复用已有实现，写出复用哪个 `文件:符号` |
| 第 3 次出现 | 才允许抽象，且抽象点要指到具体函数名，不写「抽个 helper」 |
| 新增导出符号说不清唯一调用方 | 判浅模块，负债 |
| 6 个月后加一个维度要改 >3 文件 | 必须给收口点，或写进「已知代价」+ 接受理由 |

---

## UNVERIFIED 怎么写

格式：`UNVERIFIED <结论> —— 缺 <什么证据>，可以用 <什么命令/问谁> 补上`。

例：
- `UNVERIFIED 前端是否依赖该字段的旧语义 —— 缺前端仓调用点，需要前端同学确认或查网关日志`
- `UNVERIFIED 历史数据里是否存在该状态 —— 缺线上库只读查询权限，SQL: select count(*) ... （脱敏后执行）`

**UNVERIFIED 不是失败**，留空才是失败。
