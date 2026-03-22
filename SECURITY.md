# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability, please report it privately via
[GitHub Security Advisories](https://github.com/justinstimatze/adit-code/security/advisories/new).

Do not open a public issue for security vulnerabilities.

## Scope

adit-code is a static analysis tool that reads source files and produces
reports. It does not execute analyzed code, make network requests, or modify
files. The attack surface is limited to:

- Maliciously crafted source files that could cause parser crashes (mitigated
  by tree-sitter's memory-safe parsing)
- Path traversal in file scanning (mitigated by restricting to specified paths)
- TOML config parsing (handled by well-tested BurntSushi/toml library)

## Supported Versions

Only the latest release is supported with security updates.
