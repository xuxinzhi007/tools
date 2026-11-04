#!/usr/bin/env bash
# install.sh —— 把 skills/data-workflow 复制进各 agent 工具的原生技能目录。
#
# 复制，不是软链。装完后项目里的技能和技能仓库互不依赖，和旁边那些单独的技能一样。
# 再跑一次会更新副本，但不覆盖已经写过的 config.yaml。
# 多个工具目录的技能正文各有一份；config.yaml 只留一份实体，其余指过去。
#
# 用法:
#   install.sh                 # 技能装到 $HOME，项目里只放 / 命令（默认，不进版本库）
#   install.sh --project       # 技能副本也装进项目 .<工具>/skills/（团队共享一份）
#   install.sh --with-hooks    # 额外写 Cursor 强制门禁（要求技能副本在项目里）
#   install.sh --global        # 只装到 $HOME（同默认，但不动项目）
#   install.sh <target_dir>    # 显式指定目标根
#   install.sh --unlink [...]  # 撤掉本脚本写过的东西，不动其他东西
#
# 三种落点，能力不同，别混着理解：
#   默认（$HOME）      技能 100%，无强制门禁。项目 diff 只有 4 个命令文件。
#   --project          技能 100%，副本进项目。要提交才有效，升级时 diff 是整包。
#   --project --with-hooks
#                      技能 100% + 真门禁。hooks.json 指向项目内副本，
#                      所以副本必须提交 —— 否则 hook 文件缺失，写入不再被拦（fail-open）。
set -uo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# 解释器不信任 `python3`：Windows 上它常是 Microsoft Store 的占位程序（exit 49）。
# shellcheck source=../skills/data-workflow/scripts/find_python.sh
. "$repo_root/skills/data-workflow/scripts/find_python.sh"
PY="$(dw_python 2>/dev/null || true)"
global=0
unlink=0
project=0
with_hooks=0
target=""
for a in "$@"; do
  case "$a" in
    --global) global=1 ;;
    --project) project=1 ;;
    --with-hooks) with_hooks=1 ;;
    --unlink) unlink=1 ;;
    --help|-h) sed -n '2,30p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) target="$a" ;;
  esac
done

# --with-hooks 要往 hooks.json 写相对项目根的命令，脚本必须真的在项目里。
# 默认模式的冲突检查要看最终是否真的走 $HOME。
if [ "$with_hooks" = 1 ] && [ "$global" = 1 ]; then
  printf 'CONFLICT --with-hooks 不能和 --global 一起用。\n'
  printf '         用户级 hook 会拦到所有仓库，且脚本不在各仓库的 .cursor 里。\n'
  exit 2
fi

# 默认装 $HOME：技能能力完整，项目里只留命令。
# 只有显式 --project / --with-hooks / target_dir 才把副本写进目标仓库。
# 这一段必须在算 base 之前跑，否则 base 已经钉成 pwd()。
# 例外：当前目录就是技能仓库自己（dogfood），不转 $HOME，按原地处理。
in_skill_repo=0
if [ -z "$target" ]; then
  _cwd_real="$(pwd -P)"
  [ "$_cwd_real" = "$(cd "$repo_root" && pwd -P)" ] && in_skill_repo=1
fi

if [ "$global" = 0 ] && [ "$project" = 0 ] && [ "$with_hooks" = 0 ] \
   && [ -z "$target" ] && [ "$in_skill_repo" = 0 ]; then
  global=1
fi

# 命令文件总是往调用者当前目录写（如果有 .cursor），否则项目里拿不到 /dw-next。
project_root="$target"
[ -z "$project_root" ] && project_root="$(pwd)"

base="$target"
[ -z "$base" ] && base="$(pwd)"
[ "$global" = 1 ] && base="$HOME"

dogfood=0
if [ "$global" != 1 ]; then
  base_real="$(cd "$base" && pwd -P)"
  repo_real="$(cd "$repo_root" && pwd -P)"
  if [ "$base_real" = "$repo_real" ]; then
    dogfood=1
    mkdir -p "$base/.cursor"
  fi
fi

dirs=()
for d in .claude .cursor .qoder .codex .agents; do
  [ -d "$base/$d" ] && dirs+=("$base/$d/skills")
done

src="$repo_root/skills/data-workflow"

# 默认落点（$HOME）不需要事先有工具目录：没有就建一个 Cursor 的。
# 同理：显式 --project / --with-hooks 时，目标仓库还没有工具目录也不能直接拒——
# 新项目本来就可能连 .cursor 都还没建，拒了就只能让用户自己去 mkdir。
if [ "${#dirs[@]}" -eq 0 ]; then
  if [ "$unlink" = 0 ] && { [ "$global" = 1 ] || [ "$project" = 1 ] || [ "$with_hooks" = 1 ]; }; then
    mkdir -p "$base/.cursor/skills"
    dirs+=("$base/.cursor/skills")
    printf 'CREATE %s （目标仓库还没有工具目录，建了一个 Cursor 的）\n' "$base/.cursor/skills"
  else
    printf 'NOTHING %s 下没有 .claude/.cursor/.qoder/.codex/.agents 任何一个。\n' "$base"
    printf '        先初始化目标工具，或显式传 --project / target_dir。\n'
    exit 0
  fi
fi

src="$repo_root/skills/data-workflow"

# 项目级副本要进版本库，但 config.yaml 是本机路径（含个人 Obsidian 绝对路径），
# 不该跟着提交。写一个带标记的块，--unlink 时整块撤掉，不碰用户原文。
GITIGNORE_BEGIN="# >>> data-workflow >>>"
GITIGNORE_END="# <<< data-workflow <<<"

strip_gitignore_block() {
  local file="$1"
  [ -f "$file" ] || return 0
  # 没有标记块就不动文件：不忙改别人的行尾、不制造无意义的 diff。
  grep -qxF "$GITIGNORE_BEGIN" "$file" 2>/dev/null || return 0
  if [ -z "$PY" ]; then
    printf 'SKIP  %s 没有可用的 python，无法摘掉 data-workflow 的 gitignore 块\n' "$file"
    return 1
  fi
  "$PY" - "$file" "$GITIGNORE_BEGIN" "$GITIGNORE_END" << 'PY'
import sys
path, begin, end = sys.argv[1], sys.argv[2], sys.argv[3]
with open(path, encoding="utf-8") as fh:
    lines = fh.readlines()
out, skip = [], False
for line in lines:
    if line.rstrip("\n") == begin:
        skip = True
        continue
    if line.rstrip("\n") == end:
        skip = False
        continue
    if not skip:
        out.append(line)
while out and not out[-1].strip():
    out.pop()
text = "".join(out)
if text and not text.endswith("\n"):
    text += "\n"
if text:
    with open(path, "w", encoding="utf-8") as fh:
        fh.write(text)
else:
    import os
    os.remove(path)
PY
}

write_gitignore_block() {
  local file="$project_root/.gitignore"
  if [ -z "$PY" ]; then
    printf 'SKIP  %s 没有可用的 python（py -3 / python3 / python），没写 .gitignore 块\n' "$file"
    return 1
  fi
  strip_gitignore_block "$file"
  "$PY" - "$file" "$GITIGNORE_BEGIN" "$GITIGNORE_END" "${1:-0}" << 'PY'
import sys
path, begin, end, with_hooks = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4] == "1"
import os
block = [begin, "# data-workflow 产物。运行记录是本机工作记忆，不进版本库。"]
block.append(".*/skills/data-workflow/config.yaml")
block.append(".*/skills/data-workflow/origin")
block.append("")
block.append("# 一次运行的产物（run.json / decision.md / check.md / verify.log）。")
block.append("# 想让团队看到进度，就删掉下面这一行。")
block.append("specs/")
if not with_hooks:
    block += ["# 没有门禁时不要把 hooks.json 提交给团队：它指向的脚本可能不在同级仓库里。"]
block.append(end)
text = ""
if os.path.isfile(path):
    with open(path, encoding="utf-8") as fh:
        text = fh.read()
    if text and not text.endswith("\n"):
        text += "\n"
with open(path, "w", encoding="utf-8") as fh:
    fh.write(text + "\n".join(block) + "\n")
PY
  printf 'IGNORE %s\n' "$file"
}

# Cursor 项目钩子。--global 不写：用户级 hook 会拦到所有仓库。
merge_cursor_hooks() {
  local mode="$1"
  local hooks_file="$base/.cursor/hooks.json"
  local session_cmd gate_cmd
  [ "$global" = 1 ] && return 0
  [ -d "$base/.cursor" ] || return 0
  if [ "$mode" != "remove" ] && [ -z "$PY" ]; then
    printf 'SKIP  %s 没有可用的 python，没写 Cursor 钩子。\n' "$hooks_file"
    return 1
  fi
  if [ "$dogfood" = 1 ]; then
    session_cmd="skills/data-workflow/hooks/session-start"
    gate_cmd="skills/data-workflow/hooks/gate-edit"
  else
    session_cmd=".cursor/skills/data-workflow/hooks/session-start"
    gate_cmd=".cursor/skills/data-workflow/hooks/gate-edit"
  fi
  if [ "$mode" = "remove" ] && [ -z "$PY" ]; then
    printf 'SKIP  %s 没有可用的 python，没改 Cursor 钩子。\n' "$hooks_file"
    return 1
  fi
  "$PY" - "$hooks_file" "$mode" "$session_cmd" "$gate_cmd" << 'PY'
import json, os, sys
path, mode, session, gate = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]

def load():
    if not os.path.isfile(path):
        return {"version": 1, "hooks": {}}
    with open(path, encoding="utf-8") as fh:
        try:
            data = json.load(fh)
        except json.JSONDecodeError:
            return {"version": 1, "hooks": {}}
    if not isinstance(data, dict):
        return {"version": 1, "hooks": {}}
    data.setdefault("version", 1)
    if not isinstance(data.get("hooks"), dict):
        data["hooks"] = {}
    return data

data = load()
hooks = data["hooks"]

def entries(event):
    raw = hooks.get(event, [])
    return [e for e in raw if isinstance(e, dict)] if isinstance(raw, list) else []

def is_ours(entry):
    cmd = entry.get("command") or ""
    return cmd.endswith("data-workflow/hooks/session-start") or cmd.endswith("data-workflow/hooks/gate-edit")

if mode == "remove":
    for event in list(hooks):
        kept = [e for e in entries(event) if not is_ours(e)]
        if kept:
            hooks[event] = kept
        else:
            del hooks[event]
    if not hooks and set(data) <= {"version", "hooks"}:
        if os.path.isfile(path):
            os.remove(path)
        print("HOOKS removed %s" % path)
        raise SystemExit(0)
else:
    # 命令带 bash 前缀：Cursor 直接执行无扩展名文件靠 shebang，Windows 上不可靠。
    # failClosed 关掉：门禁的严格度由 config.yaml 的 gate_strict 决定，
    # 不再让「hook 自己跑了但没输出」变成「拦住一切」。
    session_hooks = [e for e in entries("sessionStart") if not is_ours(e)]
    session_hooks.append({"command": "bash " + session})
    hooks["sessionStart"] = session_hooks
    for event in ("preToolUse", "beforeShellExecution"):
        event_hooks = [e for e in entries(event) if not is_ours(e)]
        event_hooks.append({"command": "bash " + gate, "failClosed": False})
        hooks[event] = event_hooks

os.makedirs(os.path.dirname(path), exist_ok=True)
with open(path, "w", encoding="utf-8") as fh:
    json.dump(data, fh, indent=2, ensure_ascii=False)
    fh.write("\n")
print("HOOKS %s %s" % (mode, path))
PY
}

install_commands() {

  local root="$project_root"
  [ -d "$root/.cursor" ] || return 0
  local dest="$root/.cursor/commands"
  local name
  if [ "$unlink" = 1 ]; then
    # dogfood：本仓库的 .cursor/commands 是分发源，不是安装产物，不删。
    if [ "$dogfood" = 1 ]; then
      return 0
    fi
    for name in dw-next.md dw-done.md dw-wait.md dw-start.md; do
      rm -f "$dest/$name"
    done
    printf 'COMMANDS removed %s\n' "$dest"
    return 0
  fi
  mkdir -p "$dest"
  for name in dw-next.md dw-done.md dw-wait.md dw-start.md; do
    cp "$src/commands/$name" "$dest/$name"
    printf 'COMMAND %s\n' "$dest/$name"
  done
}

if [ "$unlink" = 1 ]; then
  # dogfood：本仓库的 hooks.json 是分发源，不撤。
  [ "$dogfood" = 1 ] || merge_cursor_hooks remove
  install_commands
  if [ "$base" != "$HOME" ] && [ "$dogfood" = 0 ] && [ -d "$base" ]; then
    strip_gitignore_block "$base/.gitignore"
    if [ -f "$base/.gitignore" ] && grep -q "data-workflow" "$base/.gitignore" 2>/dev/null; then
      printf 'IGNORE removed %s\n' "$base/.gitignore"
    fi
  fi
fi

# 拷贝技能正文。没有 rsync 时走 tar 管道（Windows 的 git-bash 通常没有 rsync）。
# config.yaml 是本机配置，不是技能正文，两边都不动它。
copy_skill() {
  local from="$1" to="$2"
  mkdir -p "$to"
  find "$to" -mindepth 1 -maxdepth 1 ! -name config.yaml -exec rm -rf {} + 2>/dev/null || true
  if command -v rsync >/dev/null 2>&1; then
    rsync -a --exclude config.yaml "$from/" "$to/"
  else
    ( cd "$from" && tar cf - --exclude=config.yaml . ) | ( cd "$to" && tar xf - )
  fi
}

for sd in "${dirs[@]}"; do
  if [ "$dogfood" = 1 ] && [ "$unlink" != 1 ]; then
    printf 'SOURCE %s\n' "$src"
    continue
  fi
  mkdir -p "$sd"
  for link in "$sd"/*; do
    [ -L "$link" ] || continue
    target_path="$(readlink "$link")"
    case "$target_path" in
      "$repo_root/skills"/*)
        if [ "$target_path" != "$src" ]; then
          rm "$link"
          printf 'UNLINK %s\n' "$link"
        fi
        ;;
    esac
  done
  dest="$sd/data-workflow"
  if [ "$unlink" = 1 ]; then
    if [ -L "$dest" ] && [ "$(readlink "$dest")" = "$src" ]; then
      rm "$dest"
      printf 'UNLINK %s\n' "$dest"
    elif [ -d "$dest" ] && [ ! -L "$dest" ]; then
      find "$dest" -mindepth 1 -maxdepth 1 ! -name config.yaml -exec rm -rf {} +
      rmdir "$dest" 2>/dev/null || true
      printf 'UNLINK %s\n' "$dest"
    fi
    continue
  fi
  if [ -L "$dest" ]; then
    rm "$dest"
  fi
  mkdir -p "$dest"
  copy_skill "$src" "$dest"
  printf '%s\n' "$repo_root" > "$dest/origin"
  printf 'COPY   %s\n' "$dest"
done

if [ "$unlink" = 0 ]; then
  legacy="$base/.data-workflow/config.yaml"
  primary=""
  for sd in "${dirs[@]}"; do
    dest="$sd/data-workflow"
    [ -d "$dest" ] && [ ! -L "$dest" ] || continue
    if [ -z "$primary" ]; then
      primary="$dest/config.yaml"
      if [ -L "$primary" ]; then
        rm "$primary"
      fi
      if [ ! -f "$primary" ]; then
        if [ -f "$legacy" ]; then
          mv "$legacy" "$primary"
          rmdir "$base/.data-workflow" 2>/dev/null || true
          printf 'CONFIG %s\n' "$primary"
        else
          cp "$repo_root/config.example.yaml" "$primary"
          printf 'CONFIG %s\n' "$primary"
        fi
      else
        printf 'CONFIG %s 已存在，不覆盖\n' "$primary"
      fi
    elif [ ! -e "$dest/config.yaml" ] || [ -L "$dest/config.yaml" ]; then
      ln -sfn "$primary" "$dest/config.yaml"
      printf 'CONFIG %s\n' "$dest/config.yaml"
    fi
  done
  if [ -n "$primary" ] && [ -f "$legacy" ]; then
    rm -f "$legacy"
    rmdir "$base/.data-workflow" 2>/dev/null || true
  fi
  if [ "$dogfood" = 1 ]; then
    # 本仓库自己：hooks.json 是分发源，始终指向 skills/data-workflow/hooks/。
    merge_cursor_hooks add
  elif [ "$global" = 1 ]; then
    printf 'HOOKS 未写。--global 装的是技能本体，强制门禁属项目级。\n'
    printf '      要 Cursor 写入拦截：install.sh --project --with-hooks\n'
  elif [ "$with_hooks" = 1 ]; then
    merge_cursor_hooks add
  else
    merge_cursor_hooks remove
    printf 'HOOKS 未写。要强制门禁：install.sh --with-hooks（需副本已提交）\n'
  fi
  install_commands
  # specs/ 现在落在当前工作仓库里，所以每个模式都要盖住它，不只在 --project 时。
  if [ "$dogfood" = 0 ] && [ "$project_root" != "$HOME" ] && [ -d "$project_root/.git" ]; then
    if [ "$with_hooks" = 1 ]; then
      write_gitignore_block 1
    else
      write_gitignore_block 0
    fi
  fi
  if [ "$dogfood" = 1 ]; then
    printf '\nDONE 本仓库不另拷技能。会话钩子指向 %s/hooks/。\n' "$src"
    printf '     运行记录在仓库根的 specs/。配置仍是仓库根的 config.yaml。\n'
  else
    printf '\nDONE 技能副本在 %s 个工具目录。配置在 %s。\n' "${#dirs[@]}" "$primary"
    printf '     要更新技能正文，再跑一次本脚本，config.yaml 不会被盖掉。\n'
    if [ "$global" = 1 ] && [ "$(cd "$base" 2>/dev/null && pwd -P)" != "$HOME" ]; then
      printf '     Cursor 命令在 %s/.cursor/commands/，可提交给团队共享入口。\n' "$base"
    fi
  fi
fi
