---
description: 新开一次运行。任意仓库的一次改动或定架构用 change
---

问清 feature 名。任意仓库的一次改动或定架构，运行 `scripts/workflow_next.py start change <feature>`。
data-service（Go 栈）实例的 L3 变更用用户选的 workflow：`feature`、`bugfix`、`hotfix`、`review-only`。
脚本在 `skills/data-workflow/scripts/workflow_next.py`，装进别的仓库后在 `.cursor/skills/data-workflow/scripts/workflow_next.py`。
开完只做 `next` 打印的那一件。
