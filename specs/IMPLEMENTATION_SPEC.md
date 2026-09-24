# Mega Magic Project - Implementation Specification

**Generated:** 2026-09-23  
**Source Analysis:** Current MikroTik Magic (`/opt/mikromagic-app`) → Mega Magic Project Spec (`docs/*.md`)

---

## 🗺️ Current → New Mapping

### **Database Schema Mapping**

| Current Table | New Table | Changes |
|---------------|-----------|---------|
| `profiles` | `businesses` | Rename, add `access_tier`, `device_quota_limit` |
| `router_connections` | `devices` | Add `parent_gateway_id`, `deployment_layer` |
| `managed_devices` | `devices` | Merge, add vendor/category/transport |
| `user_roles` | `users` | Add `business_id`, `user_role` enum |
| `portal_plans` | `business_metered_quotas` | Split: plans + quotas + AI tokens |
| `voucher_codes` | `vouchers` | Add `business_id`, `plan_key`, status enum |
| `device_health_samples` | `telemetry_samples` | Add `tenant_id`, structured metrics |
| `alert_rules` | `alert_policies` | Add `business_id`, SLA thresholds |
| `incidents` | `incidents` | Add `business_id`, severity escalation |
| `connectors` | `connectors` | Add `business_id`, NATS subject prefix |

### **New Tables Required (From Mega Magic Spec)**

```sql
-- Core tenant isolation
CREATE TABLE businesses (
  business_id uuid PRIMARY KEY,
  company_name text,
  owner_email text,
  access_tier text CHECK (access_tier IN ('SMALL_WISP','ENTERPRISE_ISP','GLOBAL_ADMIN')),
  is_active boolean DEFAULT true,
  device_quota_limit integer DEFAULT 10,
  created_at timestamptz DEFAULT now()
);

-- Metered quotas (WebFig passes, AI tokens, extra slots)
CREATE TABLE business_metered_quotas (
  business_id uuid REFERENCES businesses(business_id),
  purchased_extra_slots integer DEFAULT 0,
  webfig_pass_expires_at timestamptz,
  ai_token_balance bigint DEFAULT 0,
  updated_at timestamptz DEFAULT now()
);

-- Users with RBAC
CREATE TABLE users (
  user_id uuid PRIMARY KEY,
  business_id uuid REFERENCES businesses(business_id),
  full_name text,
  email text,
  password_hash text,
  user_role text CHECK (user_role IN ('OWNER','ADMIN','OPERATOR','TECHNICIAN','VIEWER','SUPPORT')),
  created_at timestamptz DEFAULT now()
);

-- Devices with hierarchy
CREATE TABLE devices (
  device_id uuid PRIMARY KEY,
  business_id uuid REFERENCES businesses(business_id),
  parent_gateway_id uuid REFERENCES devices(device_id),
  hardware_name text,
  device_model text,
  deployment_layer text CHECK (deployment_layer IN ('MAIN_GATEWAY','DOWNSTREAM_SWITCH','ACCESS_POINT','CONTROLLER')),
  vendor text CHECK (vendor IN ('mikrotik','ruijie','cisco','tplink','ubiquiti','generic')),
  category text CHECK (category IN ('router','gateway','switch','ap','controller')),
  transport text CHECK (transport IN ('direct','connector','tunnel','controller')),
  mac_address text,
  management_ip inet,
  is_online boolean DEFAULT false,
  created_at timestamptz DEFAULT now()
);

-- Telemetry with tenant isolation
CREATE TABLE telemetry_samples (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  business_id uuid REFERENCES businesses(business_id),
  device_id uuid REFERENCES devices(device_id),
  subject_kind text CHECK (subject_kind IN ('site','router','device','connector')),
  subject_id text,
  cpu_pct numeric(5,2),
  memory_pct numeric(5,2),
  latency_ms integer,
  uptime_seconds bigint,
  wan_state text CHECK (wan_state IN ('up','degraded','down','unknown')),
  connector_state text CHECK (connector_state IN ('online','stale','offline','none')),
  tunnel_state text CHECK (tunnel_state IN ('up','down','none')),
  observed_at timestamptz DEFAULT now()
);

-- NATS subject prefixes per tenant
CREATE TABLE nats_subjects (
  business_id uuid REFERENCES businesses(business_id),
  subject_prefix text UNIQUE,  -- e.g., "tenant.biz_123."
  created_at timestamptz DEFAULT now()
);
```

---

## 🔄 Service Mapping (TypeScript → Go)

### **Current lib/ → New Go Services**

| Current File | Current Responsibility | New Go Service | NATS Subjects |
|--------------|----------------------|----------------|---------------|
| `fleet.functions.ts` | Fleet CRUD, AI scans | `fleet-service` | `fleet.{register,health,scan,config}` |
| `health.functions.ts` | Health history, sweeps, incidents | `monitoring-service` | `telemetry.{ingest,query}`, `incidents.{create,resolve}` |
| `health.server.ts` | Router probing, cloud peers | `probe-service` | `probe.{execute,result}`, `peer.{status,handshake}` |
| `fleet-probe.server.ts` | RouterOS data collection | `collector-service` | `collect.{request,response}` |
| `routers.functions.ts` | Router operations | `router-service` | `router.{create,update,delete,operate}` |
| `topology.functions.ts` | Topology, relay ops | `topology-service` | `topology.{map,relay,reboot}` |
| `voucher-*.ts` | Voucher lifecycle | `voucher-service` | `voucher.{create,redeem,expire,print}` |
| `portal.functions.ts` | Captive portal, consent | `portal-service` | `portal.{config,consent,session}` |
| `business.functions.ts` | Revenue, snapshots | `billing-service` | `billing.{snapshot,quota,upgrade}` |
| `monetization.functions.ts` | Magic Coins, tiers | `monetization-service` | `monetization.{coins,tiers,pricing}` |
| `ai-scan-quota.ts` | AI scan limits | `ai-service` | `ai.{scan,quota,generate}` |
| `gateway-bootstrap/*.ts` | Provisioning planner | `provisioner-service` | `provision.{plan,execute,verify}` |
| `cloud-vps.server.ts` | VPS API | `vps-client` | `vps.{peer,config,health}` |
| `guards.server.ts` | Auth, ownership | `auth-middleware` | (HTTP middleware) |

---

## 🚌 NATS Subject Design

```
# Tenant-scoped subjects (prefix per business_id)
{tenant_prefix}fleet.register          # Register new device
{tenant_prefix}fleet.health            # Health check request
{tenant_prefix}fleet.scan              # AI scan request
{tenant_prefix}fleet.config            # Config push
{tenant_prefix}telemetry.ingest        # 60s hardware telemetry
{tenant_prefix}telemetry.heartbeat     # 25s WireGuard keep-alive
{tenant_prefix}probe.execute           # RouterOS probe request
{tenant_prefix}probe.result            # Probe response
{tenant_prefix}peer.status             # WireGuard peer status
{tenant_prefix}peer.handshake          # Handshake timestamp
{tenant_prefix}router.operate          # Router operation (raw/whitelist)
{tenant_prefix}topology.map            # Topology discovery
{tenant_prefix}topology.relay          # Downstream relay command
{tenant_prefix}voucher.create          # Generate voucher codes
{tenant_prefix}voucher.redeem          # Voucher redemption
{tenant_prefix}billing.quota           # Quota check/enforce
{tenant_prefix}billing.upgrade         # Plan upgrade
{tenant_prefix}ai.scan                 # AI script generation
{tenant_prefix}ai.quota                # AI token balance
{tenant_prefix}provision.plan          # Provisioning plan
{tenant_prefix}provision.execute       # Execute provisioning
{tenant_prefix}provision.verify        # Verify result

# Global admin subjects (no tenant prefix)
global.admin.mrr                       # Platform MRR
global.admin.tenants                   # Tenant management
global.admin.cluster.health            # Magic Hub cluster health
global.admin.feature.flags             # Feature flag toggles
global.alerts.platform                 # Platform-level alerts
```

---

## 🔐 RBAC Mapping

| Current Role | New Role | Permissions |
|--------------|----------|-------------|
| `primary` | `OWNER` | Full tenant access, billing, users |
| `admin` | `ADMIN` | Manage devices, config, operators |
| `site_manager` | `OPERATOR` | Device ops, vouchers, sessions |
| `read_only` | `VIEWER` | Read-only dashboard access |
| `agent` | `TECHNICIAN` | Field ops, relay commands |
| `dev` | `SUPPORT` | Debug access, logs |
| `pending` | (transient) | Onboarding state |
| `expired` | (transient) | Suspended state |
| `client` | (subscriber) | Portal access only |

---

## 📊 Telemetry Pipeline Spec

### **Ingestion (Every 60s per device)**
```json
{
  "business_id": "uuid",
  "device_id": "uuid",
  "subject_kind": "router",
  "subject_id": "router_id",
  "cpu_pct": 45.2,
  "memory_pct": 62.1,
  "latency_ms": 12,
  "uptime_seconds": 86400,
  "wan_state": "up",
  "connector_state": "online",
  "tunnel_state": "up",
  "observed_at": "2026-09-23T02:15:00Z"
}
```

### **Heartbeat (Every 25s per tunnel)**
```json
{
  "business_id": "uuid",
  "peer_id": "wireguard_peer_id",
  "handshake_at": "2026-09-23T02:15:00Z",
  "state": "connected",
  "rx_bytes": 1024000,
  "tx_bytes": 512000
}
```

### **Alerting Rules (From Spec)**
- **Concurrency > 80%:** Amber flag
- **3 missed heartbeats:** 🔴 DOWN + webhook
- **Packet drop > 5%:** Drop WebFig proxy allocations
- **CPU > 95% for 5min:** Suspend non-essential telemetry
- **Handshake age > threshold:** SLA violation alert

---

## 🧪 QA Test Profiles (From Spec)

### **Test Profile Alpha: Quota Enforcement**
- **Given:** Small WISP at 10/10 device limit
- **When:** Register 11th device
- **Then:** Block with "purchase additional slots" message

### **Test Profile Beta: 1-Click Config Generation**
- **Given:** Enterprise tier, queue tree UI
- **When:** Set 100M corporate bandwidth, click optimize
- **Then:** NATS payload `{"path":"/queue/tree","action":"add","name":"Corporate_Parent","max-limit":"100M"}`

### **Test Profile Gamma: Relayed Device Ops**
- **Given:** Main gateway + CRS326 switch mapped
- **When:** Click "Force Reboot via Relay" on switch
- **Then:** Command routed via main gateway to 192.168.99.5

### **Test Profile Delta: Session Override**
- **Given:** Active captive portal session
- **When:** Click "BAN HARD" on MAC CC:3A:91:XX:XX:XX
- **Then:** Session removed + firewall drop rule added

---

## 🐳 Docker Compose (From Spec)

```yaml
version: '3.8'
services:
  saas-backend:
    image: golang:1.23-alpine
    ports: ["8080:8080"]
    env: DATABASE_URL, REDIS_URL, MAGIC_HUB_BROKER_URL
  
  saas-db:
    image: postgres:16-alpine
    ports: ["5443:5432"]
    volumes: [postgres_data, ./database/init.sql]
  
  saas-cache:
    image: redis:7-alpine
    ports: ["6379:6379"]
  
  mq-broker:
    image: nats:2-alpine
    ports: ["4222:4222"]
  
  magic-hub-bridge:
    image: linuxserver/wireguard:latest
    cap_add: [NET_ADMIN, SYS_MODULE]
    ports: ["51820:51820/udp"]
    volumes: [./magichub/config]
```

---

## 🚀 CI/CD Pipeline (From Spec)

1. **Pre-deploy:** Spin up sandbox → run QA profiles against CHR VMs
2. **Blue/Green:** Deploy to Pod A → health check → switch traffic
3. **Rolling Proxy:** Update regional WireGuard bridges sequentially (2s each)
4. **Edge Patching:** Optional updates → scheduled maintenance windows

---

## 📋 Implementation Checklist

### **Week 1-2: Foundation**
- [ ] Go module init with dependencies
- [ ] PostgreSQL schema from mapping above
- [ ] NATS connection + subject registration
- [ ] Redis client + cache patterns
- [ ] Docker Compose for local dev

### **Week 3-4: Core Services**
- [ ] Auth middleware (JWT + tenant context)
- [ ] Fleet service (CRUD + quota check)
- [ ] Probe service (RouterOS + cloud peer)
- [ ] Telemetry ingestion (60s + 25s)
- [ ] Provisioning service (v1.1 script → API)

### **Week 5-8: Business Logic**
- [ ] Voucher service (create/redeem/expire)
- [ ] Portal service (config + consent)
- [ ] Billing service (tiers + metered quotas)
- [ ] Topology service (map + relay)
- [ ] AI service (scan + quota)

### **Week 9-12: Enterprise**
- [ ] Advanced routing panels
- [ ] Queue tree designer
- [ ] Interactive canvas
- [ ] Global admin dashboard
- [ ] Feature flags

### **Week 13-16: Production Ready**
- [ ] AES-256-GCM key management
- [ ] Immutable audit log
- [ ] Blue/green deploy
- [ ] Rolling proxy updates
- [ ] SOC2/GDPR audit prep

---

## 🔗 Reference Files in This Workspace

- `migrations/*.sql` → Current schema (58 files)
- `lib/*.ts` → Business logic reference (30 files)
- `provisioning/MagicHub_Provisioning_v1.1.rsc` → Edge script
- `docs/*.md` → Mega Magic Project spec (6 files)
- `ui-reference/*.tsx` → UI patterns (13 routes)
- `shared/types/types.ts` → Supabase types (reference)

---

**Use this spec as the implementation guide. All new code in `~/mega-magic-project/`.**