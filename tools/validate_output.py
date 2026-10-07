"""Validate built agent-plugin output against the output contract.

Usage: python tools/validate_output.py <dir> [<dir> ...]

Each directory is searched for emitted files (a build target, a directory of
targets, or a directory of fixture outputs). Every JSON file the agent-plugin
target emits is checked against conformance/schemas/, and every Markdown
component's frontmatter is checked against the fields its host reads. See
docs/output-contract.md.
"""

import hashlib
import json
import pathlib
import re
import sys

import yaml
from jsonschema import Draft202012Validator
from jsonschema.validators import validator_for

ROOT = pathlib.Path(__file__).resolve().parent.parent
SCHEMAS = ROOT / "conformance" / "schemas"
NAME = re.compile(r"^[a-z0-9]+(-[a-z0-9]+)*$")

SKILL_FIELDS = {
    "name", "description", "license", "compatibility", "metadata", "allowed-tools",
    "argument-hint", "user-invocable", "disable-model-invocation",
}
AGENT_FIELDS = {"name", "description", "model", "tools", "user-invocable", "disable-model-invocation", "mcp-servers"}
COMMAND_FIELDS = {"description", "argument-hint", "allowed-tools", "disable-model-invocation"}
RULE_FIELDS = {"paths", "description"}
AGENT_SERVER = {
    "stdio": ({"type", "command"}, {"type", "command", "args", "env", "cwd", "tools"}),
    "http": ({"type", "url"}, {"type", "url", "headers", "tools"}),
}


def load_schema(rel):
    schema = json.loads((SCHEMAS / rel).read_text(encoding="utf-8"))
    Draft202012Validator.check_schema(schema)
    return Draft202012Validator(schema)


VALIDATORS = {
    "plugin": load_schema("agent-plugins-1.0.0/plugin.schema.json"),
    "mcp": load_schema("agent-plugins-1.0.0/mcp.schema.json"),
    "bundle": load_schema("typeference/bundle.schema.json"),
    "build": load_schema("typeference/build.schema.json"),
    "compatibility": load_schema("typeference/compatibility.schema.json"),
    "marketplace": load_schema("copilot/marketplace.schema.json"),
    "hooks": load_schema("copilot/hooks.schema.json"),
    "lsp": load_schema("copilot/lsp.schema.json"),
}

errors = []
counts = {}


def fail(path, message):
    errors.append(f"{path}: {message}")


def count(kind):
    counts[kind] = counts.get(kind, 0) + 1


def read_json(path):
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        fail(path, f"not valid UTF-8 JSON: {exc}")
        return None


def check_json(path, kind):
    value = read_json(path)
    if value is None:
        return None
    for error in sorted(VALIDATORS[kind].iter_errors(value), key=lambda e: list(e.absolute_path)):
        where = "/".join(str(p) for p in error.absolute_path) or "(root)"
        fail(path, f"{kind} schema: {where}: {error.message}")
    count(kind)
    return value


def frontmatter(path, required):
    text = path.read_text(encoding="utf-8")
    if not text.startswith("---\n"):
        if required:
            fail(path, "missing frontmatter")
        return None, text
    end = text.find("\n---\n", 4)
    if end < 0:
        fail(path, "unterminated frontmatter")
        return None, text
    try:
        data = yaml.safe_load(text[4:end + 1])
    except yaml.YAMLError as exc:
        fail(path, f"frontmatter is not YAML: {exc}")
        return None, text
    if not isinstance(data, dict):
        fail(path, "frontmatter is not a mapping")
        return None, text
    return data, text[end + 5:]


def check_fields(path, data, allowed):
    for key in data:
        if key not in allowed:
            fail(path, f"frontmatter field '{key}' is not one the host reads")


def check_string(path, data, key, limit=None, required=True):
    if key not in data:
        if required:
            fail(path, f"frontmatter '{key}' is required")
        return
    value = data[key]
    if not isinstance(value, str) or not value:
        fail(path, f"frontmatter '{key}' must be a non-empty string")
    elif limit and len(value) > limit:
        fail(path, f"frontmatter '{key}' exceeds {limit} characters")


def check_bools_and_lists(path, data):
    for key in ("user-invocable", "disable-model-invocation"):
        if key in data and not isinstance(data[key], bool):
            fail(path, f"frontmatter '{key}' must be a boolean")
    for key in ("tools", "allowed-tools"):
        if key in data and not (isinstance(data[key], list) and all(isinstance(v, str) for v in data[key])):
            fail(path, f"frontmatter '{key}' must be a list of strings")
    if "argument-hint" in data and not isinstance(data["argument-hint"], str):
        fail(path, "frontmatter 'argument-hint' must be a string")


def check_skill(path):
    data, body = frontmatter(path, True)
    if data is None:
        return
    check_fields(path, data, SKILL_FIELDS)
    check_string(path, data, "name", 64)
    check_string(path, data, "description", 1024)
    name = data.get("name")
    if isinstance(name, str):
        if not NAME.match(name):
            fail(path, f"skill name '{name}' must be lowercase words joined by single hyphens")
        if name != path.parent.name:
            fail(path, f"skill name '{name}' must match its directory '{path.parent.name}'")
    check_string(path, data, "compatibility", 500, required=False)
    if "metadata" in data and not (isinstance(data["metadata"], dict) and all(isinstance(v, str) for v in data["metadata"].values())):
        fail(path, "frontmatter 'metadata' must map strings to strings")
    check_bools_and_lists(path, data)
    if not body.strip():
        fail(path, "skill has no instructions")
    count("SKILL.md")


def check_agent(path):
    data, _ = frontmatter(path, True)
    if data is None:
        return
    check_fields(path, data, AGENT_FIELDS)
    check_string(path, data, "name")
    check_string(path, data, "description")
    expected = path.name[: -len(".agent.md")]
    if data.get("name") != expected:
        fail(path, f"agent name '{data.get('name')}' must match its file name '{expected}'")
    check_string(path, data, "model", required=False)
    check_bools_and_lists(path, data)
    servers = data.get("mcp-servers")
    if servers is not None:
        if not isinstance(servers, dict) or not servers:
            fail(path, "frontmatter 'mcp-servers' must be a non-empty mapping")
        else:
            for name, server in servers.items():
                where = f"mcp-servers.{name}"
                if not isinstance(server, dict) or server.get("type") not in AGENT_SERVER:
                    fail(path, f"{where} must have type stdio or http")
                    continue
                required, allowed = AGENT_SERVER[server["type"]]
                for key in required - set(server):
                    fail(path, f"{where} is missing '{key}'")
                for key in set(server) - allowed:
                    fail(path, f"{where} field '{key}' is not one the host reads")
                if "${PLUGIN_DATA}" in json.dumps(server):
                    fail(path, f"{where} uses ${{PLUGIN_DATA}}, which agent-scoped servers do not expand")
    count("agent")


def check_command(path):
    data, body = frontmatter(path, True)
    if data is None:
        return
    check_fields(path, data, COMMAND_FIELDS)
    check_string(path, data, "description")
    check_bools_and_lists(path, data)
    if not body.strip():
        fail(path, "command has no prompt")
    count("command")


def check_rule(path):
    data, body = frontmatter(path, False)
    if data is not None:
        check_fields(path, data, RULE_FIELDS)
        check_string(path, data, "paths", required=False)
        check_string(path, data, "description", required=False)
    if not body.strip():
        fail(path, "rule has no guidance")
    count("rule")


def check_schema_file(path):
    value = read_json(path)
    if value is None:
        return
    try:
        validator_for(value, default=Draft202012Validator).check_schema(value)
    except Exception as exc:  # jsonschema.SchemaError and friends
        fail(path, f"not a valid JSON Schema: {exc}")
    count("contract schema")


def check_plugin_dir(plugin_dir):
    manifest = check_json(plugin_dir / "plugin.json", "plugin")
    if manifest and manifest.get("name") != plugin_dir.name:
        fail(plugin_dir / "plugin.json", f"plugin name '{manifest.get('name')}' must match its directory '{plugin_dir.name}'")
    for path in sorted(plugin_dir.rglob("*")):
        if not path.is_file():
            continue
        rel = path.relative_to(plugin_dir).as_posix()
        if rel == "mcp.json":
            check_json(path, "mcp")
        elif rel == ".typeference/bundle.json":
            bundle = check_json(path, "bundle")
            if bundle and bundle.get("name") != plugin_dir.name:
                fail(path, "bundle name must match its plugin directory")
        elif re.fullmatch(r"skills/[^/]+/SKILL\.md", rel):
            check_skill(path)
        elif re.fullmatch(r"skills/[^/]+/references/(input|output)\.schema\.json", rel):
            check_schema_file(path)
        elif re.fullmatch(r"com\.github\.copilot/agents/[^/]+\.agent\.md", rel):
            check_agent(path)
        elif re.fullmatch(r"com\.github\.copilot/commands/[^/]+\.md", rel):
            check_command(path)
        elif re.fullmatch(r"com\.github\.copilot/rules/[^/]+\.md", rel):
            check_rule(path)
        elif rel == "com.github.copilot/hooks/hooks.json":
            check_json(path, "hooks")
        elif rel == "com.github.copilot/lsp.json":
            check_json(path, "lsp")
        elif rel.startswith("com.github.copilot/"):
            fail(path, "TypeFerence does not emit this Copilot component")


def check_target(target):
    build = check_json(target / ".typeference" / "build.json", "build")
    check_json(target / ".typeference" / "compatibility.json", "compatibility")
    marketplace = target / ".github" / "plugin" / "marketplace.json"
    if marketplace.exists():
        check_json(marketplace, "marketplace")
    paths = set()
    if build:
        paths = {artifact["path"] for artifact in build.get("artifacts", [])}
    for child in sorted(target.iterdir()):
        if child.is_dir() and (child / "plugin.json").exists():
            check_plugin_dir(child)
            if build and child.name not in paths:
                fail(child, "plugin directory is not listed in build.json")
    for missing in sorted(paths - {c.name for c in target.iterdir() if c.is_dir()}):
        fail(target / missing, "build.json lists a plugin directory that was not emitted")
    check_root_files(target, build, paths)
    count("target")


INDEX_FILES = {".typeference/build.json", ".typeference/compatibility.json", ".github/plugin/marketplace.json"}


def file_digest(path):
    """The SHA-256 of a file's content as typeference-directory-v1 reads it."""
    raw = path.read_bytes()
    try:
        text = raw.decode("utf-8")
    except UnicodeDecodeError:
        return "sha256:" + hashlib.sha256(raw).hexdigest()
    text = text.removeprefix("\ufeff").replace("\r\n", "\n")
    return "sha256:" + hashlib.sha256(text.encode("utf-8")).hexdigest()


def check_root_files(target, build, artifact_dirs):
    """Every file outside the artifact directories is an index file or a
    marketplace file that build.json lists with a matching digest (ADR-0004)."""
    if not build:
        return
    listed = {entry["path"]: entry["digest"] for entry in build.get("files", [])}
    for path in sorted(target.rglob("*")):
        if not path.is_file():
            continue
        rel = path.relative_to(target).as_posix()
        if rel.split("/", 1)[0] in artifact_dirs or rel in INDEX_FILES:
            continue
        if rel not in listed:
            fail(path, "file is neither an artifact, an index file, nor a marketplace file listed in build.json")
        elif listed[rel] != file_digest(path):
            fail(path, "file does not match its digest in build.json")
        else:
            count("marketplace file")
    for rel in sorted(listed):
        if not (target / rel).is_file():
            fail(target / rel, "build.json lists a marketplace file that was not emitted")


def main(argv):
    if not argv:
        print(__doc__)
        return 2
    targets = set()
    for arg in argv:
        root = pathlib.Path(arg)
        if not root.is_dir():
            fail(root, "not a directory")
            continue
        for build in root.rglob("build.json"):
            if build.parent.name == ".typeference":
                targets.add(build.parent.parent)
    if not targets and not errors:
        fail(" ".join(argv), "no agent-plugin build output found")
    for target in sorted(targets):
        check_target(target)
    for error in errors:
        print(error)
    summary = ", ".join(f"{n} {kind}" for kind, n in sorted(counts.items()))
    print(f"validated {summary or 'nothing'}; {len(errors)} problem(s)")
    return 1 if errors else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
