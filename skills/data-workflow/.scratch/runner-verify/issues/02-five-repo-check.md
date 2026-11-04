# 02: 一条命令证明编码结束时的五种仓库

**What to build:** 一条可重复运行的命令。它用临时仓库证明：没有检查入口时停住；package.json 的 test 和 Makefile 的 test 能通过并前进；只有 go.mod 时停住；显式 verify 优先于 package.json，失败的命令停住。

**Blocked by:** None (can start immediately).

**Status:** resolved

- [x] 空仓库和只有 go.mod 的仓库在编码步骤 done 后仍停在该步骤，输出含 RED
- [x] package.json 的 test 与 Makefile 的 test 在退出码为 0 时前进到下一步
- [x] 配置了失败的 verify 时，即使 package.json 有 test，也停住，日志里的命令是 verify

## Comments

`python3 skills/data-workflow/scripts/check_runner.py` 的前五项已通过：none、npm test、make test、go.mod、显式 `verify: "false"`。
