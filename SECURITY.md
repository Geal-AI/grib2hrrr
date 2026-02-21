# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| latest (`main`) | Yes |
| older tags | Best-effort |

We recommend always using the latest release.

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub Issues.**

If you discover a security vulnerability, please disclose it responsibly by
emailing **security@geal.ai** with:

* A description of the vulnerability and its potential impact
* Steps to reproduce (proof-of-concept input if applicable)
* Any suggested fixes you have in mind

You should receive an acknowledgement within **48 hours** and a fuller response
within **5 business days** outlining next steps.

We ask that you:

* Give us reasonable time to investigate and fix the issue before any public
  disclosure
* Avoid accessing or modifying data that isn't yours
* Act in good faith — we commit to doing the same

## Scope

Security issues of particular interest for this library:

* **Memory exhaustion (DoS)** — crafted GRIB2 bytes that cause unbounded
  allocation or infinite loops
* **Panics on untrusted input** — any code path that panics rather than
  returning an error
* **Integer overflow** — arithmetic on wire values that wraps or overflows
* **Wrong decoded values** — silent corruption of weather data (especially
  relevant for safety-critical consumers)

## Security Design Notes

This library is designed to decode untrusted GRIB2 bytes safely:

* All allocation sizes are bounded by constants (`maxNG`, `maxTotal`, `maxGridDim`)
* All HTTP response bodies are limited with `io.LimitReader`
* No `panic` calls exist in library code — only `error` returns
* Section lengths are validated with `uint64` arithmetic before use
* Bit widths are capped at 64 to prevent shift overflow

Known mitigations are documented in source comments with `// Issue #N` tags.
