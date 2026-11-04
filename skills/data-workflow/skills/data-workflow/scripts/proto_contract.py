#!/usr/bin/env python3
"""proto_contract.py —— 把一批 .proto 归一化成可 diff 的契约事实集合。

用法: proto_contract.py <proto 根目录>
输出（stdout，排序去重，每行一条）:
    RPC  <package>.<Service>.<rpc> <METHOD> <http_path>     # 无 google.api.http 时后两列为 -
    REQ  <package>.<Service>.<rpc> <RequestMessage>
    RES  <package>.<Service>.<rpc> <ResponseMessage>
    FIELD <package>.<Message[.Nested]> <field_number> <field_name> <type> <repeated|optional|->
    ENUMVAL <package>.<Enum[.Nested]> <number> <name>
    FILE <相对路径>

为什么不用 openapi.yaml: 它在本仓是 gitignore 的生成物，取不到基线。
proto 是 tracked 的事实源，`git show <base>:<file>` 就能拿基线。
"""
import os
import re
import sys

RE_MESSAGE = re.compile(r'^\s*(message|enum)\s+(\w+)\s*\{')
RE_SERVICE = re.compile(r'^\s*service\s+(\w+)\s*\{')
RE_RPC = re.compile(r'^\s*rpc\s+(\w+)\s*\(\s*([\w.]+)\s*\)\s*returns\s*\(\s*([\w.]+)\s*\)')
RE_FIELD = re.compile(
    r'^\s*(repeated\s+|optional\s+)?([\w.<>, ]+?)\s+(\w+)\s*=\s*(\d+)\s*(\[[^\]]*\])?\s*[,;]')
RE_ENUMVAL = re.compile(r'^\s*([A-Z][A-Z0-9_]*)\s*=\s*(-?\d+)\s*(\[[^\]]*\])?\s*[,;]')
RE_HTTP = re.compile(r'\b(get|post|put|delete|patch)\s*:\s*"([^"]*)"')
RE_BLOCK_COMMENT = re.compile(r'/\*.*?\*/', re.S)
RE_PACKAGE = re.compile(r'^\s*package\s+([\w.]+)\s*;', re.M)


def strip_comments(text: str) -> str:
    text = RE_BLOCK_COMMENT.sub(' ', text)
    return '\n'.join(ln.split('//')[0] for ln in text.split('\n'))


def parse(path: str, root: str):
    try:
        raw = open(path, encoding='utf-8', errors='replace').read()
    except OSError:
        return []
    text = strip_comments(raw)
    # 一行里塞多条语句的写法（message Req { string a = 1; } ）拆成一行一条，
    # 否则按行解析会漏掉单行块里的字段与枚举值。
    text = text.replace('{', '{\n').replace('}', '\n}\n').replace(';', ';\n')
    # 多行签名归一：rpc Foo(Req)\n    returns (Res); → 单行，否则 RE_RPC 抓不到
    text = re.sub(r'\)\s*\n(\s*)returns\b', r') returns', text)
    m = RE_PACKAGE.search(text)
    package = m.group(1) if m else ''

    out = []
    depth = 0
    stack = []          # [(kind, name, depth_inside_block)]
    pending = None      # {'svc','rpc','req','res','buf','start'}

    def msg_path():
        return '.'.join(n for k, n, _ in stack if k == 'message')

    def enum_path():
        return '.'.join(n for k, n, _ in stack if k in ('message', 'enum'))

    def service_name():
        for k, n, _ in reversed(stack):
            if k == 'service':
                return n
        return '?'

    def flush_rpc(p):
        body = '\n'.join(p['buf'])
        prefix = '%s.%s.%s' % (package, p['svc'], p['rpc'])
        lines = ['REQ %s %s' % (prefix, p['req']), 'RES %s %s' % (prefix, p['res'])]
        http = RE_HTTP.findall(body)
        if http:
            lines += ['RPC %s %s %s' % (prefix, v.upper(), pth) for v, pth in http]
        else:
            lines.append('RPC %s - -' % prefix)
        out.extend(lines)

    for line in text.split('\n'):
        if pending is None:
            ms = RE_SERVICE.match(line)
            if ms:
                stack.append(('service', ms.group(1), depth + 1))
            mm = RE_MESSAGE.match(line)
            if mm:
                stack.append((mm.group(1), mm.group(2), depth + 1))

        innermost = stack[-1][0] if stack else None

        mr = RE_RPC.match(line)
        if mr and pending is None:
            pending = {'svc': service_name(), 'rpc': mr.group(1),
                       'req': mr.group(2), 'res': mr.group(3),
                       'buf': [line], 'start': depth}

        if pending is not None and not mr:
            pending['buf'].append(line)

        if innermost == 'message':
            fm = RE_FIELD.match(line)
            if fm and not line.strip().startswith(('option', 'reserved', 'rpc')):
                label = (fm.group(1) or '-').strip()
                out.append('FIELD %s.%s %s %s %s %s' % (
                    package, msg_path(), fm.group(4), fm.group(3), fm.group(2), label))
        elif innermost == 'enum':
            em = RE_ENUMVAL.match(line)
            if em and not line.strip().startswith(('option', 'reserved')):
                out.append('ENUMVAL %s.%s %s %s' % (package, enum_path(), em.group(2), em.group(1)))

        depth += line.count('{') - line.count('}')

        while stack and stack[-1][2] > depth:
            stack.pop()

        if pending is not None and depth <= pending['start'] and ';' in '\n'.join(pending['buf']):
            flush_rpc(pending)
            pending = None

    if pending is not None:
        flush_rpc(pending)
    if out:
        out.append('FILE %s' % os.path.relpath(path, root))
    return out


def main():
    if len(sys.argv) != 2:
        print(__doc__, file=sys.stderr)
        return 2
    root = sys.argv[1]
    facts = []
    for dirpath, _dirs, files in os.walk(root):
        for f in sorted(files):
            if f.endswith('.proto'):
                facts += parse(os.path.join(dirpath, f), root)
    for ln in sorted(set(facts)):
        print(ln)
    return 0


if __name__ == '__main__':
    sys.exit(main())
