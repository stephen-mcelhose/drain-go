---
name: maintain-skills
description: Audit and update the repo's own skill definitions in .agents/skills/. Use when skills may be stale after project changes, when a skill's instructions reference wrong file paths or API names, when allowed-tools need updating, or when the user says "update skills", "fix skills", "skill is wrong", or "skills are outdated".
version: "1.0.0"
allowed-tools:
  - Read
  - Write
  - Edit
  - Glob
  - Grep
  - Bash(ls:*)
  - Bash(cat:*)
  - TodoWrite
---

# maintain-skills

Audits and corrects the skill definitions living in `.agents/skills/` so they stay accurate as the project evolves.

## Skill inventory

Skills are discovered by globbing `.agents/skills/*/SKILL.md`. The registry is `.agents/skills.json`.

## Audit procedure

Track progress with TodoWrite.

### 1. Discover skills

```bash
ls .agents/skills/
```

For each skill directory found, read its `SKILL.md`.

### 2. Per-skill checks

For each skill, verify:

**Frontmatter**
- `name` matches the directory name.
- `allowed-tools` includes every tool the skill body instructs the agent to use. Add missing ones; remove tools the skill never uses.
- `version` is present.

**File path references**
- Any file path mentioned in the skill body (e.g., `README.md`, `go.mod`, `drain.go`) must exist in the repo. Use Glob or Bash to confirm. Update paths that have moved.

**API / symbol references**
- If the skill mentions exported Go names (types, functions, constants), cross-check them against the source. Grep the source if unsure.

**Accuracy of instructions**
- Step descriptions must match what the repo actually contains. For example:
  - `maintain-documentation`: the list of documentation surfaces and file names must be current.
  - `maintain-skills`: this file — if the skill directory layout changes, update the glob pattern here.

**Registry sync**
- Every skill directory in `.agents/skills/` must have an entry in `.agents/skills.json`.
- Every entry in `.agents/skills.json` must have a corresponding directory.

### 3. Registry check

Read `.agents/skills.json`. For each skill listed, verify the directory exists. For each directory found, verify the skill is listed. Fix mismatches:
- Missing from JSON → add entry.
- Directory missing → flag to user (may be intentional deletion).

### 4. Self-check

When running this skill, also check this file (`maintain-skills/SKILL.md`) itself:
- Do the glob patterns and paths in this file still reflect the actual layout?
- Are the allowed-tools still sufficient for the audit steps described?

### 5. Report findings

Summarise in a table:

| Skill                    | Issue found                              | Action taken         |
|--------------------------|------------------------------------------|----------------------|
| maintain-documentation   | Referenced `parser.go` path incorrect    | Fixed                |
| maintain-skills          | No issues found                          | —                    |

If no issues are found in a skill, state that explicitly.
