# 💻 Development Implementation Blueprint

## 1. Multi-Container Local Sandbox Configuration (`docker-compose.yml`)
Developers launch this orchestration footprint locally to simulate all microservices, cache states, and database boundary engines safely inside a single isolated workstation environment.

```yaml
version: '3.8'

services:
  # Central SaaS Control API Engine
  saas-backend:
    image: golang:1.23-alpine
    container_name: saas_backend_dev
    volumes:
      - ./backend:/app
    working_dir: /app
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=postgres://saas_admin:AdminSecurePass123@saas-db:5443/saas_dev?sslmode=disable
      - REDIS_URL=redis://saas-cache:6379/0
      - MAGIC_HUB_BROKER_URL=nats://mq-broker:4222
    depends_on:
      - saas-db
      - saas-cache
      - mq-broker

  # Multi-Tenant Data Vault
  saas-db:
    image: postgres:16-alpine
    container_name: saas_database_dev
    ports:
      - "5443:5432"
    environment:
      - POSTGRES_USER=saas_admin
      - POSTGRES_PASSWORD=AdminSecurePass123
      - POSTGRES_DB=saas_dev
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./database/init.sql:/docker-entrypoint-initdb.d/init.sql

  # Real-Time Telemetry & Online Status Cache
  saas-cache:
    image: redis:7-alpine
    container_name: saas_cache_dev
    ports:
      - "6379:6379"

  # Asynchronous Message Bus Broker (Handles API-to-Magic Hub Comm)
  mq-broker:
    image: nats:2-alpine
    container_name: mq_broker_dev
    ports:
      - "4222:4222"

  # Magic Hub Edge Transport Bridge (Local Proxy Container Mock)
  magic-hub-bridge:
    image: linuxserver/wireguard:latest
    container_name: magichub_bridge_dev
    cap_add:
      - NET_ADMIN
      - SYS_MODULE
    ports:
      - "51820:51820/udp"
    environment:
      - PUID=1000
      - PGID=1000
      - TZ=UTC
      - SERVERURL=127.0.0.1
      - PEERS=1
    volumes:
      - ./magichub/config:/config

volumes:
  postgres_data:
```

## 2. Automated Relational Data Seed Scripts (`init.sql`)
This initial structure injects strict test data profiles directly into the relational engine to let programmers evaluate feature gates and multi-tenant resource locks automatically.

```sql
-- Initial Schema Initialization Seed Data
-- Creates Tier-Pass Verification Accounts for Testing

-- 1. Insert Core Business Entities
INSERT INTO businesses (business_id, company_name, owner_email, access_tier, is_active, device_quota_limit) VALUES
('b0000000-0000-0000-0000-000000000001', 'SaaS Landlord Global Admin', 'you@platformowner.com', 'ENTERPRISE_ISP', true, 99999),
('b0000000-0000-0000-0000-000000000002', 'Alpha Neighborhood WISP', 'owner@alphawisp.net', 'SMALL_WISP', true, 10),
('b0000000-0000-0000-0000-000000000003', 'Metro Carrier Networks', 'ceo@metrocarrier.com', 'ENTERPRISE_ISP', true, 100);

-- 2. Insert Metered Add-on Allowances
INSERT INTO business_metered_quotas (business_id, purchased_extra_slots, webfig_pass_expires_at, ai_token_balance) VALUES
('b0000000-0000-0000-0000-000000000002', 0, NULL, 50000),
('b0000000-0000-0000-0000-000000000003', 25, NOW() + INTERVAL '3 hours', 2000000);

-- 3. Insert Multi-Tier RBAC Testing Profiles (Passwords are pre-hashed for testing)
INSERT INTO users (user_id, business_id, full_name, email, password_secure_hash, user_role) VALUES
-- WISP Owner Profile
('u0000000-0000-0000-0000-000000000021', 'b0000000-0000-0000-0000-000000000002', 'Alex Wisp Owner', 'owner@alphawisp.net', '$2a$12$SecureHashForLocalTestingPass123', 'OWNER'),
-- Restricted Front Desk Operator
('u0000000-0000-0000-0000-000000000022', 'b0000000-0000-0000-0000-000000000002', 'Sarah Front Desk', 'sarah@alphawisp.net', '$2a$12$SecureHashForLocalTestingPass123', 'OPERATOR'),
-- Enterprise Field Engineer
('u0000000-0000-0000-0000-000000000031', 'b0000000-0000-0000-0000-000000000003', 'John Core Tech', 'tech@metrocarrier.com', '$2a$12$SecureHashForLocalTestingPass123', 'TECHNICIAN');

-- 4. Seed Mock Device Fleet Configurations
INSERT INTO devices (device_id, business_id, parent_gateway_id, hardware_name, device_model, deployment_layer, mac_hardware_address, local_management_ip, is_online) VALUES
-- Main Gateway Router for WISP
('d0000000-0000-0000-0000-000000000101', 'b0000000-0000-0000-0000-000000000002', NULL, 'Main Gate Neighborhood Node', 'RB5009', 'MAIN_GATEWAY', '00:1A:4F:11:22:33', '192.168.99.1', true),
-- Downstream Relayed Switch sitting behind Main Gateway
('d0000000-0000-0000-0000-000000000102', 'b0000000-0000-0000-0000-000000000002', 'd0000000-0000-0000-0000-000000000101', 'Distribution Switch Tier 2', 'CRS326', 'DOWNSTREAM_SWITCH', '00:1A:4F:44:55:66', '192.168.99.5', true);
```

## 3. Virtual Hardware Sandboxing & CHR Execution Guide
To allow your software engineers to test real RouterOS API behaviors safely without burning physical boxes, your engineering group can configure a free **RouterOS v7 Cloud Hosted Router (CHR)** virtual machine directly inside their local workspace environments.

1. **Grab the Raw Disk Image:** Download the standard virtual machine disk image (`.vmdk` or `.qcow2`) for **RouterOS v7 CHR** from MikroTik's official software library archive.
2. **Launch Virtual Sandbox Instance:** Run the virtual machine inside an emulator program like Oracle VirtualBox or native QEMU. Set up two networking adapter pathways:
   * *Adapter 1 (WAN Boundary Mapping):* Set network properties to Host-Only or NAT mode. This allows your local dockerized software core backend (`saas_backend_dev`) to route queries directly into the VM via a fixed network IP loop.
   * *Adapter 2 (Intranet Client Mapping):* Virtual internal segment used to mock downstream endpoints.
3. **Inject Provisioning Code:** Open the active CLI screen of your running RouterOS instance and paste the **Pre-Deployment Edge Integration Script** into the terminal. This links the virtual hardware's command listening parameters to respond instantly to instructions pushed from the local development containers.
