# system_tags.json 可视化编辑

改质检标签的 `device_scope`（任务类型 / 本体 / 设备型号），不用手改大 JSON。

## 用法

```bash
cd /Users/xuxinzhi/Documents/gowork/tools/system_tags_editor
python3 serve.py
# 局域网访问（手机/同事机器）：
python3 serve.py --host 0.0.0.0   # 启动时会打印 LAN 地址
```

浏览器打开提示的地址（默认 http://127.0.0.1:8765）。

- **从仓库加载 / 写回仓库**：读写  
  `../../data-service/configs/tag/system_tags.json`
- **预览**：写回前弹窗展示紧凑文本与仓库当前内容的行级 diff，确认后才落盘
- 也可本地选文件导入、导出下载

## 写回格式（compact serializer）

写回与导出共用 `format_tags.py`，按仓库手写紧凑风格序列化，不会把整文件 `indent=2` 铺开：

- 叶子节点 1~2 行：`{ "code": ..., "name": ...,` + `"device_scope": { ... } }`
- 带 children 的节点保持展开式；键顺序与原文件一致
- 首次写回会顺带规范化手写不一致处（如 `[8,128]` → `[8, 128]`），后续 diff 只含语义变更
- 可独立验证：`python3 format_tags.py [json路径]`（含 round-trip 校验）

## 规则提示

- `job_types` **不勾 = 通用**（全任务类型）
- 勾选了 = **白名单**，仅这些类型可见
- 任务类型：1 自由 / 2 定制 / 4 生成 / 8 恢复 / 16 纯推理 / 32 接管 / 64 评测 / 128 强化学习
