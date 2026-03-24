# VirtueStack Comprehensive Audit Prompt

Perform a comprehensive audit of the entire VirtueStack repository. Analyze every file across all directories for the following categories:

## Audit Categories

### 1. Bugs & Logic Errors
Identify runtime errors, incorrect logic, off-by-one errors, null/undefined handling, race conditions, type mismatches, and unhandled edge cases.

### 2. Security Vulnerabilities & Threat Modeling
- Injection flaws (SQL, XSS, command injection)
- Authentication/authorization gaps (missing auth checks, privilege escalation paths)
- Secrets/credentials hardcoded or leaked in code
- Insecure dependencies (outdated packages with known CVEs)
- OWASP Top 10 violations
- Attack surface mapping: identify entry points, trust boundaries, and data flow risks

### 3. Gaps & Missing Implementation
Functions stubbed out, incomplete features, missing validation, absent error handling, uncovered API routes, and missing test coverage for critical paths.

### 4. TODO / FIXME / Not Implemented
Surface all TODO, FIXME, HACK, XXX, NOSONAR, and "not implemented" markers with surrounding context.

### 5. Dead Code
Unreachable code blocks, unused imports/variables/functions/components, deprecated modules still present, and orphaned files with no references.

### 6. Unoptimized Code
N+1 queries, redundant re-renders, unnecessary re-computations, missing memoization, inefficient algorithms (O(n²) where O(n) is possible), large bundle sizes, and missing lazy loading.

### 7. Documentation Drift
README, API docs, inline comments, and config examples that are outdated, misleading, or contradict the current codebase behavior.

## Output Requirements

Write all findings to `tasking.md` in the repository root, formatted as follows:

- Group findings under H2 headers matching each audit category above.
- Each finding is an unticked task: `- [ ] **[FILE:LINE]** Description of the issue`
- Below each task, include:
  - **Severity:** Critical / High / Medium / Low
  - **Why it matters:** One-line impact explanation
  - **Suggested fix:** Specific, actionable solution (with code snippet if applicable)
- Sort findings within each category by severity (Critical first).
- End with a `## Summary` section containing counts per category and an overall risk assessment.

## Example Entry

```
- [ ] **[src/api/auth.ts:47]** JWT token is verified without checking expiration claim
  - **Severity:** Critical
  - **Why it matters:** Expired tokens remain valid, enabling session hijacking
  - **Suggested fix:** Add `exp` claim validation: `jwt.verify(token, secret, { algorithms: ['HS256'], clockTolerance: 30 })`
```
