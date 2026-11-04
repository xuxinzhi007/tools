# AGENTS.md

拦住写错和假完成。一个词在业务里指什么，从知识层读，不写进技能。流程在 `skills/data-workflow/SKILL.md`，点名再读。

- 知识源走 `skills/data-workflow/scripts/resolve.sh`。空值就问，不猜路径。
- 梳理一个词、字段或状态：读 `skills/data-workflow/stages/oncall/references/routing.md`。关键结论标已证实、推断、待确认或存在冲突。不编口径。
- 动手前读 `skills/data-workflow/references/scope.md`。**能自己查的事实先查完再给方案**，不把查证当成问题反问用户。L1 走 `change`。无关的债不进这次 diff。受保护路径不能靠口头说「小改动」绕过。
- 定架构走 `workflows/change.json`。形状写在 `decision.md`，不套 feature 九步。
- 未做完的运行在产物目录的 `specs/<feature>/run.json`。产物目录一律是**当前工作仓库的仓库根**——在哪个仓库 install、在哪干活，记录就落在哪里，不回到技能源仓库。`install.sh` 会把 `specs/` 写进该仓库的 `.gitignore`。会话里的 NOTICE 只说明它还在。用户要继续那条时，`workflow_next.py next` 打出 NEED、WAIT、DO、OPEN 才只做那一件。新需求先做新需求，不把旧运行标成完成或放弃。

## Agent skills

### Issue tracker

Issues 和 spec 记在仓库里的 `.scratch/<feature>/`。见 `docs/agents/issue-tracker.md`。

### Triage labels

五个默认角色，标签名与角色名相同。见 `docs/agents/triage-labels.md`。

### Domain docs

单仓库：根目录 `GLOSSARY.md`，决策在 `docs/adr/`。见 `docs/agents/domain.md`。
