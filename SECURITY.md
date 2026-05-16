# Security Policy

## Supported Versions

Security fixes are handled on the default branch until versioned releases are introduced.

## Reporting A Vulnerability

Please report security issues privately by opening a GitHub security advisory for the repository, or by contacting the maintainers through the organization contact listed on GitHub.

Do not open public issues for vulnerabilities until a fix or mitigation is available.

## Scope Notes

gopm's IPC listener is designed for loopback/local use. Do not expose the daemon port directly to untrusted networks.

