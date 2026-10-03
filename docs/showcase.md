# Showcase and Portfolio Strategy Guide

This guide contains materials for presenting **Buriti Pay** across professional channels (LinkedIn, technical blogs, portfolio website, and resume).

---

## 1. Portfolio Project Card

- **Title:** Buriti Pay — High-Concurrency Payment Processing Microservice in Go
- **Summary:** Microserviço financeiro de alta concorrência construído em Go, processando pagamentos de forma assíncrona com latência de ingestão p99 < 50ms, mutual exclusion distribuído via Redis + Lua script e garantia matemática de integridade (zero double-spending) via PostgreSQL e Transactional Outbox com RabbitMQ.
- **Tags:** `Go`, `PostgreSQL`, `Redis`, `RabbitMQ`, `Docker`, `Distributed Systems`, `Concurrency`.
- **Links:**
  - Repositório: `https://github.com/lucaasnogueira/buriti-pay`
  - Showcase / Live Simulator: `docs/index.html` (deploy no GitHub Pages ou Cloudflare Pages)

---

## 2. LinkedIn Post Draft

> **Título do Post:** Como evitei Double-Spending e Race Conditions em um sistema de pagamentos de alta concorrência com Go, Redis e Postgres 🌴🚀
>
> Processar movimentação de dinheiro parece simples... até milhares de clientes tentarem movimentar a mesma conta simultaneamente.
>
> Três problemas clássicos de sistemas financeiros:
> 1. **Race Conditions:** Duas transações leem o mesmo saldo e ambas são aprovadas (double-spending).
> 2. **Cobrança duplicada:** Retentativas de clientes ou mensagens duplicadas gerando débitos extras.
> 3. **Exaustão de recursos:** Goroutines infinitas consumindo memória e derrubando pools de conexões.
>
> Para explorar isso na prática, desenvolvi o **Buriti Pay**, um microserviço em Go focado em concorrência extrema e consistência transacional:
>
> 🔹 **Ingestão Não-Bloqueante com Backpressure:** O endpoint responde `202 Accepted` de imediato com chave de idempotência. A fila é delimitada por canais bufferizados do Go; se o sistema saturar, responde `429 Too Many Requests` com `Retry-After`.
> 🔹 **Lock Distribuído + Fencing Otimista:** Coordenação entre réplicas via Redis (`SET NX PX` com script Lua atômico e renovação automática por watchdog). Ordenação lexicográfica de UUIDs para eliminar deadlocks entre transferências cruzadas (A->B e B->A).
> 🔹 **Postgres como Fonte da Verdade:** Caso um lock no Redis expire por GC pause ou rede, o versionamento otimista no Postgres (`WHERE version = $expected`) aborta escritas obsoletas atomicamente.
> 🔹 **Transactional Outbox com RabbitMQ:** Eventos de confirmação são gravados na mesma transação atômica das contas e publicados com Publisher Confirms, eliminando eventos perdidos ou fantasmas.
>
> 📊 **Resultados aferidos:**
> • $\ge 2.000$ pagamentos/segundo em ambiente local
> • Latência de aceitação p99 $< 50$ ms
> • Divergência de saldo: **ZERO absoluto**
>
> O código fonte, a documentação completa, os 6 ADRs (Architecture Decision Records) e o simulador interativo estão disponíveis no GitHub:
> 👉 https://github.com/lucaasnogueira/buriti-pay
>
> #golang #distributedsystems #backend #softwareengineering #fintech #microservices #redis #rabbitmq

---

## 3. Resume Bullets (CV)

- **Inglês:**
  * *Architected and implemented a high-concurrency payment microservice in Go, processing 2,000+ tx/s with sub-50ms p99 ingest latency utilizing bounded worker pools and backpressure (HTTP 429).*
  * *Guaranteed zero double-spending and ledger consistency through Redis distributed locks with Lua safe-release scripts, deterministic deadlock-free account ordering, and PostgreSQL optimistic version fencing.*
  * *Implemented the Transactional Outbox Pattern with RabbitMQ publisher confirms and idempotent event consumers, eliminating lost or phantom financial notifications.*

- **Português:**
  * *Arquitetei e implementei um microserviço de pagamentos de alta concorrência em Go, alcançando mais de 2.000 tx/s com latência p99 de ingestão inferior a 50ms utilizando worker pools delimitados e backpressure.*
  * *Eliminei qualquer possibilidade de double-spending e divergência de saldo combinando locks distribuídos no Redis com scripts Lua, ordenação determinística de contas livre de deadlocks e versionamento otimista no PostgreSQL.*
  * *Implementei o Transactional Outbox Pattern com RabbitMQ publisher confirms e consumidores idempotentes, garantindo entrega at-least-once sem duplicações.*
