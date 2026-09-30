# Mega — Mega Magic Project

**Created:** 2026-09-23  
**Source:** `/opt/mikromagic-app` (live production - DO NOT MODIFY)  
**Purpose:** Parallel development of enterprise-grade MikroTik Magic SaaS platform

---

## 📁 Project Structure

```
mega-magic-project/
├── docs/                    # Mega Magic Project specification documents (6 files)
├── migrations/              # All Supabase SQL migrations (schema reference)
├── provisioning/            # RouterOS provisioning script v1.1
├── lib/                     # Core business logic (TypeScript reference for Go port)
├── shared/                  # Shared types & utilities
├── ui-reference/            # Key React UI components (reference for new frontend)
├── specs/                   # New specifications (to be created)
├── scripts/                 # Build/deploy scripts (to be created)
├── package.json             # Reference only - new project uses Go
├── tsconfig.json            # Reference only
├── vite.config.ts           # Reference only
└── bunfig.toml              # Reference only
```

---

## 📋 What Was Copied (Reference Only)

### **Migrations (58 files)**
Complete Supabase schema evolution - reference for new PostgreSQL schema design

### **Core Business Logic (lib/ - 30 files)**
| File | Purpose | Port Target |
|------|---------|-------------|
| `fleet-health.ts` | Health calculations, formatting | Go telemetry service |
| `fleet.functions.ts` | Fleet operations, AI scans | Go fleet service |
| `health.functions.ts` | Health history, incidents, sweeps | Go monitoring service |
| `health.server.ts` | Router probing, cloud peer status | Go probe service |
| `fleet-probe.server.ts` | RouterOS data collection | Go collector |
| `cloud-peer-status.ts` | WireGuard handshake analysis | Go tunnel monitor |
| `guards.server.ts` | Auth, expiry, ownership checks | Go auth middleware |
| `cloud-vps.server.ts` | VPS API communication | Go VPS client |
| `routers.functions.ts` | Router CRUD, operations | Go router service |
| `topology.functions.ts` | Network topology, relay ops | Go topology service |
| `voucher-*.ts` | Voucher lifecycle, printing | Go voucher service |
| `portal.functions.ts` | Captive portal, consent | Go portal service |
| `business.functions.ts` | Business snapshots, revenue | Go billing service |
| `monetization.functions.ts` | Magic Coins, tiers, pricing | Go monetization |
| `ai-scan-quota.ts` / `ai-quota.server.ts` | AI scan limits | Go AI service |
| `gateway-bootstrap/*.ts` | Provisioning planner, CIDR | Go provisioner |
| `devices/` | Vendor drivers, port mapping | Go device abstraction |

### **UI Reference (ui-reference/ - 13 routes + 3 components)**
Key user-facing pages for frontend redesign reference

### **Provisioning Script**
`MagicHub_Provisioning_v1.1.rsc` - RouterOS v7 edge integration script

### **Documentation (docs/ - 6 files)**
Complete Mega Magic Project specification

---

## 🚀 New Project Stack (Per Mega Magic Spec)

| Layer | Technology | Status |
|-------|------------|--------|
| **API Backend** | Go 1.23 + NATS | 🟢 To build |
| **Database** | PostgreSQL 16 + RLS | 🟢 Schema from migrations |
| **Cache** | Redis 7 | 🟢 To configure |
| **Message Bus** | NATS 2 | 🟢 To configure |
| **Tunnel** | WireGuard (stateless) | 🟢 Provisioning script exists |
| **Frontend** | React + TanStack (or new) | 🟢 UI reference exists |
| **Testing** | RouterOS CHR VMs | 🟢 Spec defined |
| **CI/CD** | Blue/Green + Rolling | 🟢 Spec defined |

---

## 🎯 Development Phases

### **Phase 1: Infrastructure (Weeks 1-4)**
- [ ] Go project setup with NATS, PostgreSQL, Redis clients
- [ ] Database schema from migrations → new PostgreSQL design
- [ ] NATS subject design for async command bus
- [ ] WireGuard hub container deployment
- [ ] Docker Compose orchestration (from spec)

### **Phase 2: Core Services (Weeks 5-8)**
- [ ] Tenant isolation (`business_id` + RLS)
- [ ] Fleet management (register, health, config)
- [ ] Provisioning automation (v1.1 script → API)
- [ ] Telemetry ingestion (60s + 25s heartbeats)
- [ ] Quota enforcement (Test Profile Alpha)

### **Phase 3: Enterprise Features (Weeks 9-12)**
- [ ] Tiered access (Small WISP / Enterprise ISP)
- [ ] Metered quotas (devices, WebFig passes, AI tokens)
- [ ] Advanced routing panels (BGP, OSPF, MPLS)
- [ ] Queue tree designer
- [ ] Interactive topology canvas

### **Phase 4: Compliance & Scale (Weeks 13-16)**
- [ ] AES-256-GCM key management
- [ ] Immutable audit logging
- [ ] SOC2/GDPR audit readiness
- [ ] Blue/green deployment pipeline
- [ ] Rolling proxy updates

---

## ⚠️ Rules

1. **NEVER modify `/opt/mikromagic-app`** - It's live production
2. **All new code goes in `~/mega-magic-project/`**
3. **Reference only** - Copy patterns, don't copy-paste TypeScript to Go
4. **Test against CHR VMs** - Use virtual RouterOS for validation
5. **Deploy to staging first** - Never directly to production VPS

---

## 🔗 Key References

- **Mega Magic Specs:** `docs/*.md` (6 documents)
- **Current Schema:** `migrations/*.sql` (58 files)
- **Business Logic:** `lib/*.ts` (30 files)
- **Provisioning:** `provisioning/MagicHub_Provisioning_v1.1.rsc`
- **UI Patterns:** `ui-reference/*.tsx`

---

## 📝 Next Steps

1. **Initialize Go module** in `~/mega-magic-project/`
2. **Design PostgreSQL schema** from migrations reference
3. **Define NATS subjects** for command/telemetry topics
4. **Create docker-compose.yml** from spec
5. **Start Phase 1 implementation**

---

**This workspace is isolated from production. Develop freely here.**