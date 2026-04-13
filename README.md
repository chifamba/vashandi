# Vashandi Monorepo

This is the root of the **Vashandi** multi-project monorepo. It contains two independent projects that share this repository:

| Project | Folder | Description |
|---------|--------|-------------|
| [vashandi](./vashandi/) | `vashandi/` | Open-source AI-agent orchestration platform (Production Go Backend) |
| [openbrain](./openbrain/) | `openbrain/` | Multi-tier memory service with autonomous curation |

---

## Projects

Vashandi is a high-performance orchestration platform fueled by a Go backend and a React dashboard. It manages a team of AI agents through a secure control plane with atomic task checkout and hard budget enforcement.

- **Stack:** Go (Core Backend), Node.js (Admin/UI-Proxy), React, PostgreSQL (pgvector)
- **Dev quickstart:** see [`vashandi/README.md`](./vashandi/README.md)
- **Full docs:** [`vashandi/doc/`](./vashandi/doc/)

### openbrain

OpenBrain is a new project in this monorepo. Details and documentation will be added as the project takes shape.

- See [`openbrain/README.md`](./openbrain/README.md)

---

## Docker Quickstart (Maturity Phase I)

The easiest way to run the full hardened stack (Vashandi + OpenBrain + mTLS) is via the root-level Docker Compose.

1. **Bootstrap CA**:
   ```bash
   docker compose up ca
   ```
   Capture the `Fingerprint` shown in the logs.

2. **Configure**:
   ```bash
   cp .env.example .env
   # Edit .env and paste the STEP_CA_FINGERPRINT
   ```

3. **Launch**:
   ```bash
   docker compose up -d
   ```

Vashandi will be available at `http://localhost:3100` and OpenBrain at `http://localhost:3101`. Communications between them are secured via mTLS auto-rotated by Step-CA.

---

## Repository Layout

```
vashandi/          # Vashandi project (AI-agent orchestration platform)
  backend/         # Go core backend (Production Parity 100% - Hardened)
  server/          # Node.js legacy baseline and board proxy
  ui/              # React + Vite board UI
  doc/             # Internal developer documentation (Standardized)
  docs/            # Public documentation site (Mintlify)

openbrain/         # OpenBrain project (Memory & Context service)
  doc/             # Internal developer documentation
```

---

## Documentation Standard

This monorepo follows a systematic documentation layout:
- **Root README**: High-level overview and cross-project navigation.
- **Project README**: Quickstart and setup for that specific project.
- **Project `doc/`**: Canonical home for all internal specifications and plans.
  - `doc/specs/`: Verified technical specifications.
  - `doc/plans/`: Dated/versioned implementation roadmaps.

---

## Contributing

Please read the relevant project-level contribution guide:

- Vashandi: [`vashandi/CONTRIBUTING.md`](./vashandi/CONTRIBUTING.md)

---

## License

MIT © 2026 Vashandi
