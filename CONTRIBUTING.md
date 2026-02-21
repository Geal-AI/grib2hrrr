# Contributing to grib2hrrr

Thank you for taking the time to contribute! This is a small, focused library
and every improvement matters — bug fixes, documentation, new GRIB2 field
support, and performance work are all welcome.

Please read this guide before opening an issue or submitting a pull request.

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md).
By participating you agree to abide by it. Please be kind.

## Before You Start

1. **Check existing issues** — your idea or bug may already be tracked.
2. **Open an issue first** for non-trivial changes — alignment on design before
   writing code saves everyone time.
3. **Small, focused PRs** are much easier to review than large ones.

## Setting Up

```bash
git clone https://github.com/Geal-AI/grib2hrrr.git
cd grib2hrrr
go test -short ./...   # verify everything passes before you start
```

No external tools are required for the core library — it has zero dependencies.

For Python validation scripts (`testdata/`):

```bash
pip install cfgrib numpy eccodes
```

## Development Workflow

### Tests first

This library can be used in safety-critical applications (weather routing,
aviation, emergency management). **Write tests before code.** If you're fixing
a bug, add a failing test that demonstrates it before the fix.

```bash
go test -short ./...      # fast unit tests — run constantly
go test ./...             # includes S3 network tests (requires internet)
make validate-python      # cross-validate vs cfgrib (requires fixture + Python deps)
```

### Making changes

1. Fork the repository and create a branch:
   ```bash
   git checkout -b feature/my-improvement
   ```

2. Make your changes, adding tests.

3. Verify:
   ```bash
   go test -short ./...
   go vet ./...
   ```

4. Push and open a Pull Request against `main`.

### Commit messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(drs53): support DRS Template 5.2
fix(lambert): correct inverse projection near poles
docs: add example for multi-field fetch
test: add fuzz corpus entry for malformed section 3
perf(bitstream): vectorise 64-bit reads
```

## What We Welcome

| Type | Notes |
|------|-------|
| Bug fixes | Please include a regression test |
| Security fixes | See [SECURITY.md](SECURITY.md) for private disclosure |
| New GRIB2 templates | DRS 5.0, 5.1, 5.2, GDT 3.0, 3.1 etc. |
| Documentation | Typos, clarifications, examples |
| Performance | With benchmarks showing the improvement |
| Test coverage | More fuzz seeds, edge cases, golden values |

## What We're Cautious About

* **New dependencies** — zero-dep is a feature. Propose in an issue first.
* **Large API changes** — stability matters for a library. Discuss first.
* **Removing safety guards** — the bounds checks and input validation exist for
  security reasons. Removing them requires a strong justification.

## Pull Request Checklist

Before requesting review, verify:

- [ ] `go test -short ./...` passes
- [ ] `go vet ./...` is clean
- [ ] New behaviour has tests
- [ ] Security-relevant changes have regression tests in `decode_security_test.go`
- [ ] `CLAUDE.md` updated if architecture changes
- [ ] Commit messages follow Conventional Commits

## Reporting Issues

Use the GitHub issue templates:

* **Bug report** — unexpected behaviour, wrong values, panics
* **Feature request** — new fields, new grid types, API improvements

For security vulnerabilities, please **do not** open a public issue. See
[SECURITY.md](SECURITY.md).

## Questions

Open a [GitHub Discussion](https://github.com/Geal-AI/grib2hrrr/discussions) —
questions are welcome and help improve the documentation for everyone.
