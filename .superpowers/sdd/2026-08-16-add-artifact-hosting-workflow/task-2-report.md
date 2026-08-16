Task 2 report: Validate workflow syntax and trigger semantics

Summary:
- Ran the provided YAML validation test. It initially failed because PyYAML parsed the top-level `on:` key as boolean True (a YAML 1.1 boolean alias), causing the test to KeyError on data['on'].
- Applied a minimal fix: quoted the `on` key in .github/workflows/artifact-hosting.yml to prevent boolean interpretation.
- Re-ran the validation test; it passed (WORKFLOW_REQUIREMENTS_OK).
- Committed the single-line fix.

Commands & outputs:
1) Initial test (failed):
   KeyError on missing 'on' key due to YAML parsing of bare on -> True.

2) Fix applied (git commit):
   commit: 4d87cd7 (fix(workflow): quote 'on' key to avoid YAML boolean parsing)

3) Validation test (after fix):
   WORKFLOW_REQUIREMENTS_OK

4) git status --short (workspace state):
   ?? docs/

Notes:
- The change is minimal and safe: it only quotes the YAML key 'on' to avoid PyYAML's YAML 1.1 boolean parsing. GitHub Actions itself accepts either representation; quoting ensures local YAML parsers behave predictably.

Additional actions (round 1 fixes):
- Added a short inline comment beside the quoted "on" key explaining the quoting is to avoid YAML 1.1 boolean parsing compatibility issues with some parsers.
- Implemented and ran an enhanced validation (python3) that asserts:
  * trigger keys include workflow_dispatch and pull_request
  * retention-days == 30
  * the job-level `if:` contains refs/heads/main and mentions workflow_dispatch and pull_request

Validation command (run from repo root):

python3 - <<'PY'
import re
from pathlib import Path
p = Path('.github/workflows/artifact-hosting.yml')
s = p.read_text()
# Find 'on' block and its keys
m = re.search(r'^["\']?on["\']?\s*:\s*\n((?:\s+\S.*\n)+)', s, flags=re.M)
if not m:
    raise SystemExit('MISSING_ON_BLOCK')
block = m.group(1)
if 'workflow_dispatch:' not in block or 'pull_request:' not in block:
    raise SystemExit('MISSING_TRIGGERS')
# retention-days check
if not re.search(r'^\s*retention-days\s*:\s*30\s*$', s, flags=re.M):
    raise SystemExit('BAD_RETENTION')
# job-level if check
m2 = re.search(r'^\s*if\s*:\s*(.+)$', s, flags=re.M)
if not m2:
    raise SystemExit('MISSING_JOB_IF')
if 'refs/heads/main' not in m2.group(1) or 'workflow_dispatch' not in m2.group(1) or 'pull_request' not in m2.group(1):
    raise SystemExit('JOB_IF_INCOMPLETE')
print('WORKFLOW_REQUIREMENTS_OK')
PY

Validation output: WORKFLOW_REQUIREMENTS_OK

Commit & trailer:
- Created a follow-up commit containing the inline comment and this appended report. Commit message includes Co-authored-by trailer as required.

Report produced at: .superpowers/sdd/2026-08-16-add-artifact-hosting-workflow/task-2-report.md
