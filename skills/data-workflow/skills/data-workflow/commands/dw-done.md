---
description: 结束当前步骤，编码步骤先跑该仓库的测试入口
---

运行 data-workflow 的 `scripts/workflow_next.py done <feature>`。
feature 取当前未完成的 `specs/<feature>/run.json`。
脚本在 `skills/data-workflow/scripts/workflow_next.py`，装进别的仓库后在 `.cursor/skills/data-workflow/scripts/workflow_next.py`。
退出码不是 0 就停在这一步。
