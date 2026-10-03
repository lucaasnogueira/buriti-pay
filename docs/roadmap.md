# Implementation Roadmap

Buriti Pay is built progressively in testable, independently deliverable phases.

| Phase | Title | Milestone Deliverable |
|---|---|---|
| **0** | **Preparation** | Repository setup, Go module, structure, ADRs, build tooling. |
| **1** | **Foundation & Infrastructure** | Docker Compose, configuration, migrations, DB/Redis/RabbitMQ clients, Account API. |
| **2** | **Transactional Core** | Synchronous payment logic, state machine, ACID double-entry transfer, idempotency, invariant tests. |
| **3** | **Concurrency & Worker Pool** | Async 202 Accepted, bounded worker channel, backpressure 429, graceful shutdown, reaper. |
| **4** | **Distributed Lock (Redis)** | Locker interface, SET NX PX + Lua unlock, watchdog TTL renewal, multi-replica tests. |
| **5** | **Messaging & Outbox** | Transactional outbox table, outbox relay with publisher confirms, AMQP consumer with DLQ. |
| **6** | **Performance & Observability** | Prometheus metrics, Grafana dashboards, k6 load testing, pprof heap/CPU profiling, sync.Pool optimization. |
| **7** | **Quality & CI** | GitHub Actions pipeline, test coverage badges, container packaging, security audit. |
| **8** | **Showcase & Delivery** | Interactive web demo, documentation polish, portfolio technical breakdown. |
