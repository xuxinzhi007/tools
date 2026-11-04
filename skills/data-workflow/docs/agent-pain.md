# agent 开发里值得这个仓库管的痛

查过的材料：Gloaguen 等，[Evaluating AGENTS.md](https://arxiv.org/html/2602.11988v1)（2026-02）；dos Santos 等对 100 个 AGENTS.md 的气味统计，转述见 [Configuration Smells](https://agentpatterns.ai/patterns/anti-patterns/configuration-smells-agents-md/)（arxiv 2606.15828）；[Hacker News：Writing a good Claude.md](https://news.ycombinator.com/item?id=46098838)；[Hacker News：Claude Code swarms](https://news.ycombinator.com/item?id=48407998)；Reddit 上反复出现的是「说修好了但没有证据」和上下文散掉（[r/ClaudeAI](https://www.reddit.com/r/ClaudeAI/comments/1to8l0j/building_the_harness_around_our_coding_agents/)、[r/AI_Agents](https://www.reddit.com/r/AI_Agents/comments/1w1ecg0/coding_agents_dont_always_need_a_smarter_model/)）。

结论有三条，和这个仓库对得上的只有第一条。

1. 每次会话都塞进去的长说明，模型会照做，于是多读文件、多解释、多花步骤。LLM 生成的上下文文件平均把任务成功率打低大约 3%，推理成本升高超过 20%。91% 的流行 AGENTS.md 至少有一种气味：把技能正文、lint 规则、很少用到的说明放进始终加载的文件。
2. 模型说做完了，但没有红灯或命令输出。这是验证问题。data-service 的质量门已经写在阶段文件里，不在每次会话的开场白里再讲一遍。
3. 长任务忘掉先前的决定。未做完的 `specs/<feature>/run.json` 就是给这件事留的。做完的运行不必每场聊天再念一次。

因此会话钩子在有未完成运行时注入一条 NOTICE，不注入 `next` 的操作指令。没有未完成运行时不注入。AGENTS.md 写明 NOTICE 不代替当前需求。

开发过程里的对应问题是：模型说改完了，但没有一条命令的退出码。编码阶段的 `done` 跑该仓库已经声明的测试入口。显式命令写在 `verify`。没写时依次看 `package.json` 的 `test`、`Makefile` 的 `test`、pytest、`cargo test`。都没有，或退出码不是 0，就停在 `RED`。不替某个服务选一套测试。结果在 `specs/<feature>/verify.log`。重复检查是 `bash scripts/verify.sh`（它自己探测可用的 python；不要直接执行 `.py`）。它覆盖五种检查入口、pnpm、yarn、pytest、cargo、拍板回复，读写区分与 `gate_strict`，无解释器时 fail-open，UTF-8 输出，以及一次按别的仓库安装：副本自己的 runner 和钩子、空知识源、默认写入门禁、旁边的其他技能还在。
