#!/usr/bin/env python3
"""Validate the Customer V4 monitoring definitions (B21).

  deploy/monitoring/customer-v4/alerts.rules.yml   Prometheus rule groups
  deploy/monitoring/customer-v4/dashboard.json     Grafana dashboard

Checks, all offline:
  * rule-file structure (groups, alert, expr, for, labels, annotations);
  * every expression parses under a strict PromQL subset (operator
    precedence, aggregations with by/without, typed range-vector functions,
    matchers, offset). Anything outside the subset is an error, never a pass;
  * every metric exists in the §27 catalogue DERIVED FROM THE GO SOURCE
    (gauge queries, counters, durations, journeyshare counters), and every
    label used on it is one the code actually emits;
  * labels are bounded vocabularies: no id/ref/token/customer dimension;
  * each rule declares threshold_status=initial_default and says in its
    description that the value is pending SRE tuning (no SLO is claimed);
  * dashboard panel ids are unique and every target parses and resolves.

When `promtool` is on PATH (CI installs it) the rule file is also checked
with `promtool check rules`; the result says which validators ran.
"""
import json
import re
import shutil
import subprocess
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
MONITORING = ROOT / "deploy" / "monitoring" / "customer-v4"
BOUNDED_LABELS = {"authority", "outcome", "family", "service", "channel", "status", "state", "purpose", "wallet_status"}
RULE_LABELS = {"severity", "component", "threshold_status"}
SEVERITIES = {"info", "warning", "critical"}
UNBOUNDED = re.compile(r"(^id$|_id$|uuid|ref|token|customer|booking|source|share|provider|email|phone|key|hash|correlation|request)")


# --------------------------------------------------------------------------
# metric catalogue derived from source
# --------------------------------------------------------------------------
def catalogue():
    metrics = {"up": set()}  # name -> emitted label keys (None = unknown)
    sources = [p for p in list(ROOT.glob("services/*/**/*.go")) + list(ROOT.glob("shared/go/**/*.go")) if not p.name.endswith("_test.go")]
    prefixes = {}
    for path in sources:
        text = path.read_text(encoding="utf-8", errors="replace")
        for name, labels in re.findall(r'Name:\s*"(karry_[a-z0-9_]+)"\s*,\s*(?:Labels:\s*\[\]string\{([^}]*)\})?', text):
            metrics.setdefault(name, set()).update(re.findall(r'"([a-z_]+)"', labels))
        for name, labels in re.findall(r'IncCounter\("(karry_[a-z0-9_]+)",\s*map\[string\]string\{([^}]*)\}', text):
            metrics.setdefault(name, set()).update(re.findall(r'"([a-z_]+)"\s*:', labels))
        for name in re.findall(r'IncCounter\("(karry_[a-z0-9_]+)",\s*(?:nil|labels)', text):
            metrics.setdefault(name, None if name not in metrics else metrics[name])
        for name, labels in re.findall(r'SetGauge\("(karry_[a-z0-9_]+)",\s*(nil|map\[string\]string\{[^}]*\})', text):
            metrics.setdefault(name, set()).update(re.findall(r'"([a-z_]+)"\s*:', labels))
        for name, labels in re.findall(r'ObserveDuration\("(karry_[a-z0-9_]+)",\s*map\[string\]string\{([^}]*)\}', text):
            for suffix in ("_seconds_sum", "_seconds_count"):
                metrics.setdefault(name + suffix, set()).update(re.findall(r'"([a-z_]+)"\s*:', labels))
        for name in re.findall(r'Observe\("([a-z_]+)"\s*,', text):
            for suffix in ("_total", "_duration_seconds_sum"):
                metrics.setdefault(f"karry_journeyshare_{name}{suffix}", set())
        for prefix in re.findall(r'Observe\("([a-z_]+)"\s*\+', text):
            prefixes[prefix] = path
    # Dynamic journeyshare names ("taxi_location_"+state): expand with the
    # state vocabulary declared in the same file, never a wildcard.
    for prefix, path in prefixes.items():
        states = set(re.findall(r'State:\s*"([A-Z_]+)"', path.read_text(encoding="utf-8"))) | set(re.findall(r'state\s*:?=\s*"([A-Z_]+)"', path.read_text(encoding="utf-8")))
        for state in states:
            for suffix in ("_total", "_duration_seconds_sum"):
                metrics.setdefault(f"karry_journeyshare_{prefix}{state}{suffix}", set())
    return metrics


# --------------------------------------------------------------------------
# strict PromQL subset parser
# --------------------------------------------------------------------------
TOKEN = re.compile(r"""\s*(?:(?P<num>\d+(?:\.\d+)?(?:e[+-]?\d+)?)|(?P<dur>\[\d+[smhdw]\])|(?P<str>"(?:[^"\\]|\\.)*")|
    (?P<op>=~|!~|!=|==|>=|<=|[-+*/%(){},=<>])|(?P<ident>[A-Za-z_:][A-Za-z0-9_:]*))""", re.X)
AGGREGATIONS = {"sum", "max", "min", "avg", "count"}
FUNCTIONS = {  # name: argument types ("v" instant vector, "m" range vector, "s" scalar)
    "rate": "m", "increase": "m", "delta": "m", "max_over_time": "m", "min_over_time": "m", "avg_over_time": "m",
    "clamp_min": "vs", "clamp_max": "vs", "vector": "s", "abs": "v", "absent": "v",
}
DURATION = re.compile(r"^\d+[smhdw]$")


class ParseError(Exception):
    pass


class Parser:
    def __init__(self, text):
        self.tokens, pos, text = [], 0, text.strip()
        while pos < len(text):
            m = TOKEN.match(text, pos)
            if not m or m.end() == pos:
                raise ParseError(f"unexpected input at {text[pos:pos + 20]!r}")
            kind = m.lastgroup
            self.tokens.append((kind, m.group(kind)))
            pos = m.end()
            while pos < len(text) and text[pos].isspace():
                pos += 1
        self.i, self.metrics = 0, []

    def peek(self, value=None):
        if self.i >= len(self.tokens):
            return None
        tok = self.tokens[self.i]
        return tok if value is None or tok[1] == value else None

    def take(self, value=None):
        tok = self.peek()
        if tok is None or (value is not None and tok[1] != value):
            raise ParseError(f"expected {value or 'token'}, found {tok[1] if tok else 'end'}")
        self.i += 1
        return tok

    def parse(self):
        kind = self.binary(0)
        if self.i != len(self.tokens):
            raise ParseError(f"trailing input at {self.tokens[self.i][1]!r}")
        return kind

    LEVELS = [("or",), ("and", "unless"), ("==", "!=", ">", "<", ">=", "<="), ("+", "-"), ("*", "/", "%")]

    def binary(self, level):
        if level == len(self.LEVELS):
            return self.unary()
        left = self.binary(level + 1)
        while self.peek() and self.peek()[1] in self.LEVELS[level]:
            op = self.take()[1]
            if level == 2 and self.peek("bool"):
                self.take()
            right = self.binary(level + 1)
            if op in ("or", "and", "unless") and ("s" in (left, right)):
                raise ParseError(f"'{op}' needs vector operands")
            left = "v" if "v" in (left, right) else "s"
        return left

    def unary(self):
        if self.peek("-"):
            self.take()
        return self.primary()

    def labels_clause(self):
        self.take("(")
        names = []
        while not self.peek(")"):
            names.append(self.take()[1])
            if self.peek(","):
                self.take()
        self.take(")")
        return names

    def primary(self):
        tok = self.peek()
        if tok is None:
            raise ParseError("unexpected end of expression")
        if tok[0] == "num":
            self.take()
            return "s"
        if tok[1] == "(":
            self.take()
            kind = self.binary(0)
            self.take(")")
            return kind
        if tok[0] != "ident":
            raise ParseError(f"unexpected {tok[1]!r}")
        name = tok[1]
        nxt = self.tokens[self.i + 1][1] if self.i + 1 < len(self.tokens) else None
        if name in AGGREGATIONS and nxt in ("(", "by", "without"):
            self.take()
            grouping = []
            if self.peek("by") or self.peek("without"):
                self.take()
                grouping = self.labels_clause()
            self.take("(")
            if self.binary(0) != "v":
                raise ParseError(f"{name}() needs an instant vector")
            self.take(")")
            if self.peek("by") or self.peek("without"):
                self.take()
                grouping = self.labels_clause()
            self.grouping_labels.extend(grouping)
            return "v"
        if name in FUNCTIONS and nxt == "(":
            self.take()
            self.take("(")
            signature, args = FUNCTIONS[name], []
            while not self.peek(")"):
                args.append(self.range_or_expr())
                if self.peek(","):
                    self.take()
            self.take(")")
            if len(args) != len(signature):
                raise ParseError(f"{name}() takes {len(signature)} argument(s)")
            for want, got in zip(signature, args):
                if want != got:
                    raise ParseError(f"{name}() argument must be {'a range vector' if want == 'm' else 'an instant vector' if want == 'v' else 'a scalar'}")
            return "v"
        return self.selector(allow_range=False)

    def range_or_expr(self):
        start = self.i
        tok = self.peek()
        if tok and tok[0] == "ident" and tok[1] not in AGGREGATIONS and tok[1] not in FUNCTIONS:
            try:
                return self.selector(allow_range=True)
            except ParseError:
                self.i = start
        return self.binary(0)

    def selector(self, allow_range):
        name = self.take()[1]
        if name in AGGREGATIONS or name in FUNCTIONS or name in ("by", "without", "and", "or", "unless", "offset", "bool"):
            raise ParseError(f"misplaced keyword {name!r}")
        matchers = []
        if self.peek("{"):
            self.take()
            while not self.peek("}"):
                label = self.take()[1]
                op = self.take()[1]
                if op not in ("=", "!=", "=~", "!~"):
                    raise ParseError(f"bad matcher operator {op}")
                value = self.take()
                if value[0] != "str":
                    raise ParseError("matcher value must be a quoted string")
                matchers.append(label)
                if self.peek(","):
                    self.take()
            self.take("}")
        kind = "v"
        if self.peek() and self.peek()[0] == "dur":
            if not allow_range:
                raise ParseError(f"range selector {name}{self.peek()[1]} outside a range function")
            self.take()
            kind = "m"
        if self.peek("offset"):
            self.take()
            duration = self.take()[1]
            if not DURATION.match(duration) and not re.match(r"^\d+$", duration):
                raise ParseError("offset needs a duration")
            if re.match(r"^\d+$", duration):
                unit = self.take()[1]
                if unit not in ("s", "m", "h", "d", "w"):
                    raise ParseError("offset needs a duration")
        self.metrics.append((name, matchers))
        return kind


def parse(expr):
    parser = Parser(expr)
    parser.grouping_labels = []
    kind = parser.parse()
    return kind, parser.metrics, parser.grouping_labels


def check_expr(expr, metrics, where, problems):
    try:
        kind, used, grouping = parse(expr)
    except ParseError as exc:
        problems.append(f"{where}: PromQL parse error: {exc}")
        return
    if kind != "v":
        problems.append(f"{where}: expression is not a vector")
    for name, labels in used:
        if name not in metrics:
            problems.append(f"{where}: metric {name} is not emitted by any service")
            continue
        emitted = metrics[name]
        for label in labels:
            if label not in BOUNDED_LABELS or UNBOUNDED.search(label):
                problems.append(f"{where}: label {label} is not a bounded vocabulary")
            elif emitted is not None and name != "up" and label not in emitted:
                problems.append(f"{where}: {name} carries no label {label}")
    for label in grouping:
        if label not in BOUNDED_LABELS or UNBOUNDED.search(label):
            problems.append(f"{where}: grouping label {label} is not a bounded vocabulary")


def main():
    import yaml

    metrics, problems, report = catalogue(), [], {}
    rules = yaml.safe_load((MONITORING / "alerts.rules.yml").read_text(encoding="utf-8"))
    names, count = set(), 0
    for group in rules.get("groups") or []:
        if not group.get("name") or not group.get("rules"):
            problems.append("rule group without name or rules")
            continue
        for rule in group["rules"]:
            count += 1
            alert = rule.get("alert")
            where = f"rule {alert}"
            if not alert or not re.fullmatch(r"[A-Z][A-Za-z0-9]+", alert):
                problems.append(f"{where}: alert must be a CamelCase name")
            if alert in names:
                problems.append(f"{where}: duplicate alert name")
            names.add(alert)
            if not re.fullmatch(r"\d+[smh]", str(rule.get("for", ""))):
                problems.append(f"{where}: 'for' must be a duration")
            labels = rule.get("labels") or {}
            if set(labels) - RULE_LABELS:
                problems.append(f"{where}: unexpected rule labels {sorted(set(labels) - RULE_LABELS)}")
            if labels.get("severity") not in SEVERITIES:
                problems.append(f"{where}: severity must be one of {sorted(SEVERITIES)}")
            if labels.get("threshold_status") != "initial_default":
                problems.append(f"{where}: threshold_status must be initial_default (no SLO is claimed)")
            annotations = rule.get("annotations") or {}
            if not annotations.get("summary") or "pending SRE tuning" not in annotations.get("description", ""):
                problems.append(f"{where}: needs a summary and a description stating the default is pending SRE tuning")
            check_expr(rule.get("expr", ""), metrics, where, problems)
    report["rules"] = count

    dashboard = json.loads((MONITORING / "dashboard.json").read_text(encoding="utf-8"))
    ids, targets = set(), 0
    for panel in dashboard.get("panels") or []:
        if panel["id"] in ids:
            problems.append(f"dashboard: duplicate panel id {panel['id']}")
        ids.add(panel["id"])
        for target in panel.get("targets") or []:
            targets += 1
            check_expr(target["expr"], metrics, f"panel '{panel['title']}' {target['refId']}", problems)
    report["dashboard_panels"], report["dashboard_targets"] = len(ids), targets

    promtool = shutil.which("promtool")
    if promtool:
        run = subprocess.run([promtool, "check", "rules", str(MONITORING / "alerts.rules.yml")], capture_output=True, text=True)
        report["promtool"] = "pass" if run.returncode == 0 else "fail"
        if run.returncode:
            problems.append("promtool check rules failed: " + (run.stdout + run.stderr)[-1500:])
    else:
        report["promtool"] = "not installed (strict subset parser only)"
    report["catalogue_series"] = len(metrics)
    report["problems"] = problems
    print(json.dumps(report, indent=1))
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main())
