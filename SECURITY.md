# Security Policy

## Reporting a vulnerability

Please do **not** open a public GitHub issue for sensitive security reports.

Instead, use GitHub private vulnerability reporting from this repository's Security tab.

When reporting a vulnerability, please include:
- affected command(s)
- reproduction steps
- impact assessment
- any suggested mitigation if available

## Scope notes

Current known security tradeoff:
- local tokens are stored in plaintext files with restrictive permissions (`0600`) unless environment-based auth is used

This is documented behavior for the current preview release and may be improved in future versions.
