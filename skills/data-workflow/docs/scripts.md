# 脚本

| 脚本 | 作用 |
|---|---|
| `skills/data-workflow/scripts/workflow_next.py` | 下一步。子命令表在 `skills/data-workflow/references/commands.md` |
| `skills/data-workflow/scripts/resolve.sh <key>` | 解析一个配置键。值走 stdout，来源走 stderr。未配置就空 |
| `skills/data-workflow/scripts/paths.sh` | 一次 `eval` 出知识源和模板变量 |
| `scripts/install.sh` | 把技能复制进已存在的工具目录。见 `docs/install.md` |
| `scripts/repo_snapshot.sh` | 记录被测仓库的 branch、commit、是否有未提交改动 |
| `scripts/knowledge_drift.sh` | 对照 `verified_commit` 与 `watches`，输出 `FRESH` / `STALE` / `SKIP` |
| `scripts/contract_diff.sh <base>` | 以 merge-base 的 proto 为基线，输出七类 breaking。恒 exit 0，裁决归技能 |
| `scripts/proto_contract.py` | 把 proto 归一成事实行，供 contract_diff 比对 |
| `scripts/svg_to_png.sh` | 把 SVG 转成 PNG |
| `scripts/find_python.sh` | 逐个实跑 `py -3` / `python3` / `python` 选可用的一个。Windows 上 `python3` 常是 Store 占位程序 |
| `scripts/layout.py` | 运行产物落到哪。一律当前工作仓库根，不回技能源仓库 |
| `scripts/check_runner.py` | 回归集。`bash scripts/verify.sh` 会调它 |

`knowledge_drift.sh` 恒 exit 0。exit 2 只表示文档无法解析。缺 frontmatter 算 STALE。知识源为空时打印 `DEGRADED`，回退 `<repo>/docs`。

被测仓库：`$DS_REPO`，然后是配置里的 `repo`，然后是当前目录所在仓库。知识源优先级写在 `config.example.yaml` 开头。不读技能仓库里的 `config.yaml`。
