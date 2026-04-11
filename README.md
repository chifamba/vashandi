# Vashandi Monorepo

This is the root of the **Vashandi** multi-project monorepo. It contains two independent projects that share this repository:

| Project | Folder | Description |
|---------|--------|-------------|
| [vashandi](./vashandi/) | `vashandi/` | Open-source AI-agent orchestration platform (formerly Paperclip) |
| [openbrain](./openbrain/) | `openbrain/` | *(Coming soon)* |

---

## Projects

### vashandi

> Open-source orchestration for zero-human companies.

Vashandi is a Node.js server and React UI that orchestrates a team of AI agents to run a business. Bring your own agents, assign goals, and track your agents' work and costs from one dashboard.

- **Stack:** Node.js, TypeScript, React, PostgreSQL (via Drizzle ORM), Go (backend services)
- **Dev quickstart:** see [`vashandi/README.md`](./vashandi/README.md)
- **Full docs:** [`vashandi/doc/`](./vashandi/doc/)

### openbrain

OpenBrain is a new project in this monorepo. Details and documentation will be added as the project takes shape.

- See [`openbrain/README.md`](./openbrain/README.md)

---

## Repository Layout

```
vashandi/          # Vashandi project (AI-agent orchestration platform)
  backend/         # Go backend services
  server/          # Node.js/TypeScript API server
  ui/              # React + Vite frontend
  packages/        # Shared packages (db, adapters, plugins, etc.)
  cli/             # CLI tool
  docs/            # Public documentation site
  doc/             # Internal developer documentation
  ...

openbrain/         # OpenBrain project (placeholder)
```

---

## Contributing

Please read the relevant project-level contribution guide:

- Vashandi: [`vashandi/CONTRIBUTING.md`](./vashandi/CONTRIBUTING.md)

---

## License

MIT © 2026 Vashandi
