# Notices

Reames Agent is based on DeepSeek Reasonix, which is licensed under the MIT
License. The upstream project is available at:

https://github.com/esengine/DeepSeek-Reasonix

The repository also studies behavior and product patterns from the reference
projects documented in `docs/REFERENCE_GOVERNANCE.md`. Those reference
repositories are not vendored wholesale into this repository; copied code or
assets must keep their original license notices.

An inherited Hermes/Python repository snapshot was present during the initial
migration and was removed from the current tree on 2026-07-17 after its useful
mechanisms had either been reimplemented behind the Go control boundary or kept
in the external reference repository. Historical commits retain the original
snapshot and its attribution; no Hermes runtime, Electron shell, Python package,
or Cloudflare worker is part of the current product checkout.

## The Update Framework Go client

Reames Agent includes `github.com/theupdateframework/go-tuf/v2`, licensed under
the Apache License 2.0:

Copyright 2024 The Update Framework Authors

The dependency's Apache license and upstream NOTICE are preserved in
`third_party/go-tuf/LICENSE` and `third_party/go-tuf/NOTICE`.
