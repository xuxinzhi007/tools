@echo off
rem gate-edit —— 门禁入口（原生 Windows）。
rem 与 POSIX 的 gate-edit 等价：找到能跑的 python 再交给 gate_edit.py；
rem 找不到就输出合法 JSON 放行，不制造「坏门」。
setlocal
set "HERE=%~dp0"
set "PY="
py -3 -c "import sys" >nul 2>nul && set "PY=py -3"
if not defined PY ( python3 -c "import sys" >nul 2>nul && set "PY=python3" )
if not defined PY ( python -c "import sys" >nul 2>nul && set "PY=python" )
if not defined PY (
  echo {"permission": "allow", "agent_message": "data-workflow: no working python (py -3 / python3 / python), write gate skipped"}
  exit /b 0
)
%PY% "%HERE%gate_edit.py"
