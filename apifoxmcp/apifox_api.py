"""Apifox API 客户端。只包含已对 api.apifox.com 验证过的调用。"""

import json
import secrets
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

API = "https://api.apifox.com/api/v1"
API_VERSION = "2026-05-28"
CLI_VERSION = "2.2.11"
DEFAULT_PROJECT = 8013009
CONFIG = Path(__file__).with_name("config.yaml")

AUTH_HEADER_NAMES = (
    "x-xr-auth-type",
    "x-xr-auth-uid",
    "x-xr-auth-sub",
    "x-xr-auth-tid",
    "x-xr-auth-cid",
    "x-xr-auth-aut",
    "x-xr-auth-eml",
)
SECURITY_SCHEME = {
    "use": {
        "configs": {"1038302": {"authConfigs": {"x-apifox": {"token": "{{access_token}}"}}}},
        "id": "ugEk19m6apjl8Pii2mPMC",
    },
    "scopes": {},
    "schemeGroups": [{"id": "ugEk19m6apjl8Pii2mPMC", "schemeIds": [1038302]}],
    "required": True,
}


def load_token():
    for line in CONFIG.read_text().splitlines():
        line = line.split("#", 1)[0].strip()
        if line.startswith("api_fox_tokens:"):
            return line.split(":", 1)[1].strip().strip("\"'")
    raise RuntimeError("config.yaml missing api_fox_tokens")


def api(method, path, body=None, project_id=None, branch_id=None):
    data = None if body is None else json.dumps(body, ensure_ascii=False).encode()
    headers = {
        "Authorization": "Bearer " + load_token(),
        "X-Apifox-Api-Version": API_VERSION,
        "X-Apifox-Cli-Version": CLI_VERSION,
        "Accept": "application/json",
        "User-Agent": "apifox-mcp/" + CLI_VERSION,
        "Content-Type": "application/json",
    }
    if project_id:
        headers["X-Project-Id"] = str(project_id)
    if branch_id:
        headers["X-Branch-Id"] = str(branch_id)
    req = urllib.request.Request(API + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            return json.load(resp)
    except urllib.error.HTTPError as exc:
        raw = exc.read().decode()[:800]
        try:
            payload = json.loads(raw)
        except json.JSONDecodeError:
            payload = {"errorMessage": raw}
        raise RuntimeError("HTTP %s %s" % (exc.code, payload.get("errorMessage") or payload))


def ok(payload):
    if isinstance(payload, dict) and payload.get("success") is False:
        raise RuntimeError(payload.get("errorMessage") or "apifox error")
    return payload.get("data") if isinstance(payload, dict) and "data" in payload else payload


def whoami():
    user = ok(api("GET", "/current-user"))
    return {"id": user.get("id"), "name": user.get("name"), "username": user.get("username")}


def list_projects():
    rows = ok(api("GET", "/user-projects"))
    return [{"id": p.get("id"), "name": p.get("name"), "teamId": p.get("teamId"), "roleType": p.get("roleType")} for p in rows]


def list_teams():
    rows = ok(api("GET", "/user-teams"))
    return [{"id": t.get("id"), "name": t.get("name"), "roleType": t.get("roleType"), "roleName": t.get("roleName")} for t in rows]


def list_branches(project_id):
    rows = ok(api("GET", "/projects/%s/sprint-branches" % project_id, project_id=project_id))
    out = []
    for b in rows:
        out.append({
            "id": b.get("id"),
            "name": b.get("name"),
            "isMain": b.get("isMain"),
            "forkFromBranchId": b.get("forkFromBranchId"),
            "editableCallerType": b.get("editableCallerType"),
        })
    return out


def create_automation_branch(project_id, name, fork_from_branch_id):
    data = ok(api("POST", "/projects/%s/sprint-branches" % project_id, {
        "name": name,
        "forkFromBranchId": int(fork_from_branch_id),
        "createAutomation": True,
    }, project_id=project_id))
    return {"id": data.get("branchId") or data.get("id"), "name": data.get("name"), "forkFromBranchId": data.get("forkFromBranchId")}


def search_endpoints(project_id, keyword, branch_id=None):
    query = urllib.parse.urlencode({"keyword": keyword})
    rows = ok(api("GET", "/projects/%s/http-apis?%s" % (project_id, query), project_id=project_id, branch_id=branch_id))
    needle = keyword.lower()
    hits = []
    for item in rows:
        path = item.get("path") or ""
        name = item.get("name") or ""
        if needle in path.lower() or needle in name.lower():
            hits.append({"id": item.get("id"), "method": item.get("method"), "path": path, "name": name})
        if len(hits) >= 30:
            break
    return hits


def get_endpoint(project_id, endpoint_id, branch_id=None):
    item = ok(api("GET", "/projects/%s/http-apis/%s" % (project_id, endpoint_id), project_id=project_id, branch_id=branch_id))
    return {"id": item.get("id"), "method": item.get("method"), "path": item.get("path"), "name": item.get("name")}


def pick_endpoints(project_id, source_branch_id, target_branch_id, endpoint_ids):
    ok(api(
        "POST",
        "/projects/%s/sprint-branches/%s/pick-to" % (project_id, source_branch_id),
        {"targetBranchId": int(target_branch_id), "pickIds": {"endpointIds": [int(i) for i in endpoint_ids]}},
        project_id=project_id,
        branch_id=source_branch_id,
    ))
    return {"sourceBranchId": int(source_branch_id), "targetBranchId": int(target_branch_id), "endpointIds": [int(i) for i in endpoint_ids]}


def list_scenarios(project_id, branch_id):
    data = ok(api("GET", "/projects/%s/test-scenario/tree-list" % project_id, project_id=project_id, branch_id=branch_id))
    return [{"id": s.get("id"), "name": s.get("name"), "priority": s.get("priority"), "folderId": s.get("folderId")} for s in data.get("testScenarios") or []]


def scenario_summary(scenario):
    steps = []
    for step in scenario.get("steps") or []:
        case = step.get("httpApiCase") or {}
        body = ((case.get("requestBody") or {}).get("data") or "").replace("\n", " ")
        steps.append({
            "number": step.get("number"),
            "type": step.get("type"),
            "name": step.get("name"),
            "method": case.get("method"),
            "path": case.get("path"),
            "bindId": step.get("bindId"),
            "processors": [p.get("type") for p in (case.get("postProcessors") or [])],
            "body": body[:180],
        })
    return {"id": scenario.get("id"), "name": scenario.get("name"), "description": scenario.get("description"), "steps": steps}


def get_scenario(project_id, branch_id, scenario_id):
    data = ok(api("GET", "/api-test/cases/%s/steps?withCaseDetail=true" % scenario_id, project_id=project_id, branch_id=branch_id))
    return scenario_summary(data)


def _assertion(index, item):
    return {
        "id": "postProcessors.%s.assertion" % index,
        "type": "assertion",
        "defaultEnable": True,
        "enable": True,
        "data": {
            "name": item.get("name") or "断言",
            "subject": item["subject"],
            "comparison": item["comparison"],
            "path": item.get("path") or "",
            "value": item.get("value") or "",
        },
    }


def _http_step(scenario_id, number, step):
    headers = [{
        "id": secrets.token_hex(5),
        "relatedName": name,
        "name": name,
        "value": "{{%s}}" % name,
        "type": "string",
        "description": "",
        "enable": True,
        "isDelete": False,
    } for name in AUTH_HEADER_NAMES]
    processors = [_assertion(i, item) for i, item in enumerate(step.get("assertions") or [])]
    script = step.get("script")
    if script:
        processors.append({
            "id": "postProcessors.%s.customScript" % len(processors),
            "type": "customScript",
            "defaultEnable": True,
            "enable": True,
            "data": script,
        })
    endpoint_id = int(step["endpoint_id"])
    return {
        "id": secrets.token_hex(16),
        "type": "http",
        "name": step["name"],
        "number": number,
        "disable": False,
        "parameters": {},
        "bind": False,
        "bindId": endpoint_id,
        "bindType": "API",
        "syncMode": "MANUAL",
        "httpApiCase": {
            "path": step["path"],
            "name": step["name"],
            "type": "http",
            "method": (step.get("method") or "post").lower(),
            "testSuiteId": scenario_id,
            "parameters": {"query": [], "header": headers, "cookie": [], "path": []},
            "commonParameters": {"query": [], "body": [], "header": [], "cookie": []},
            "requestBody": {
                "parameters": [],
                "type": "application/json",
                "data": step.get("body") or "{}\n",
                "generateMode": "normal",
            },
            "apiDetailId": endpoint_id,
            "projectId": int(step.get("project_id") or DEFAULT_PROJECT),
            "preProcessors": [{"type": "placeholder", "renderType": "dynamicValueDivider", "defaultEnable": False, "enable": False}],
            "postProcessors": processors,
            "inheritPreProcessors": {},
            "inheritPostProcessors": {},
            "auth": {"type": "securityscheme"},
            "advancedSettings": {"disabledSystemHeaders": {}, "isDefaultUrlEncoding": 2, "disableUrlEncoding": False},
            "options": {},
            "securityScheme": SECURITY_SCHEME,
        },
    }


def _write_steps(project_id, branch_id, scenario_id, name, description, steps, priority, folder_id):
    built = []
    for index, step in enumerate(steps, start=1):
        step = dict(step)
        step["project_id"] = project_id
        built.append(_http_step(scenario_id, index, step))
    ok(api("PUT", "/api-test/cases/%s" % scenario_id, {
        "id": scenario_id,
        "name": name,
        "description": description or "",
        "priority": int(priority),
        "folderId": int(folder_id),
        "steps": built,
    }, project_id=project_id, branch_id=branch_id))
    return get_scenario(project_id, branch_id, scenario_id)


def create_scenario(project_id, branch_id, name, description, steps, priority=2, folder_id=0, reuse_existing=True):
    if reuse_existing:
        for item in list_scenarios(project_id, branch_id):
            if item.get("name") == name:
                return {"created": False, "scenario": get_scenario(project_id, branch_id, item["id"])}
    created = ok(api("POST", "/api-test/cases", {
        "name": name,
        "description": description or "",
        "folderId": int(folder_id),
        "priority": int(priority),
    }, project_id=project_id, branch_id=branch_id))
    scenario = _write_steps(project_id, branch_id, created["id"], name, description, steps, priority, folder_id)
    return {"created": True, "scenario": scenario}


def update_scenario(project_id, branch_id, scenario_id, name, description, steps, priority=2, folder_id=0):
    scenario = _write_steps(project_id, branch_id, int(scenario_id), name, description, steps, priority, folder_id)
    return {"updated": True, "scenario": scenario}


