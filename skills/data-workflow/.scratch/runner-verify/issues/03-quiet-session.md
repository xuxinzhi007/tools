# 03: 没有未完成的运行就不注入会话

**What to build:** 会话钩子在没有运行、或只有已完成运行时，additional_context 为空。有未完成运行时，只注入下一步的输出。

**Blocked by:** None (can start immediately).

**Status:** resolved

- [x] 没有运行时，钩子输出的 additional_context 为空
- [x] 只有已完成运行时，钩子输出的 additional_context 为空
- [x] 有未完成运行时，additional_context 含下一步标记，不含用法说明

## Comments

同一条 `python3 skills/data-workflow/scripts/check_runner.py` 的后三项已通过：无运行、已完成运行、未完成运行。
