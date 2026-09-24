# 🏢 Business Profile & Compliance Framework

## 1. Executive Summary & Vision
The application functions as a highly scalable, multi-tenant Software-as-a-Service (SaaS) platform built to manage decentralized internet service providers (WISPs) and Enterprise ISP carrier-grade networks from a single, zero-CLI web dashboard. 

*   **Strategic Vision:** To redefine network management for decentralized internet service providers by building an edge-first orchestration platform that combines enterprise-grade carrier capabilities with simple, one-click control interfaces.
*   **Mission Statement:** To empower neighborhood WISPs and enterprise network operators with a resilient, multi-tier remote control infrastructure. We achieve this by turning local edge hardware into an autonomous network server, utilizing a secure, lightweight cloud bridge for command transport, and eliminating complex command-line interaction through intuitive, automated visual design engines.

## 2. Multi-Tenant Operational Hierarchy
The platform uses a relational architecture partitioned securely via a uniform `business_id` system parameter. Data visibility and feature capabilities are split across distinct business roles:

*   **Global Platform Admin (You):** Top-down oversight over the entire SaaS architecture. Monitors absolute Platform Monthly Recurring Revenue (MRR), total multi-tenant subscription accounts active, in-app micro-transaction logs, global Magic Hub transport cluster workloads, and global feature-flag authorization toggles.
*   **Small ISP / WISP Owner + Local Staff:** Neighborhood tier pass. Pays platform license fees to use simplified hotspot configuration tools, design localized paper voucher templates, manage up to 10 gateway devices, and control local staff permission scopes (Operators, Technicians).
*   **Enterprise ISP Owner + Core Operators:** Carrier tier pass. Pays advanced operational fees to unlock high-capacity core network routing panels (BGP, OSPF, MPLS/VPLS, hierarchical Queue Trees), interactive node-link map canvasses, extensive device quota pools, and priority AI script generation engines.
*   **Subscriber Guests:** Ultimate residential end-users accessing local hotspot deployments. They load a default, hardware-hosted Captive Portal screen and submit a paper voucher code to log in.

## 3. Global Security & Compliance Frameworks
Because our architecture separates the cloud control plane from the edge data plane, the platform remains highly compliant across international jurisdictions with drastically reduced operational overhead.

### Data Privacy Regulations (GDPR & CCPA Alignment)
*   **The Zero-Content-Logging Mandate:** The application operates purely as a remote control system. End-user web browsing history, traffic packet contents, destination URLs, and raw metadata are entirely private, never tracked, and never centralized into our cloud repositories.
*   **Data Minimization Protocol:** Customer identification strings (MAC addresses, IP leases, voucher activation tokens) remain stored locally inside the physical on-premise hardware edge server.
*   **Right to be Forgotten Automation:** When an end-user's voucher code hits its expiration parameter, localized background routines automatically scrub and purge the target device's MAC logging footprint and associated IP lease from the system cache within 24 hours.

### Corporate System Integrity (SOC 2 Type II Alignment)
*   **Cryptographic Key Isolation:** Administrative MikroTik gateway router access configurations and internal peer-to-peer connection keys must never be saved in open plain text inside the database vault. All system identities are encrypted at rest utilizing **AES-256-GCM**, with keys rotated automatically via a centralized KMS.
*   **Immutable Command Audit Logging:** The Magic Hub engine generates a cryptographically signed, read-only operational timeline tracking every automated script push event. Every 1-click execution records `[Timestamp] | [User_ID] | [Action] | [Target_Device_ID]` permanently to pass corporate data integrity and availability audits.
*   **API Transport Layer Security:** All transactional control payloads transmitted across the backend, dashboard, and Magic Hub proxy loops operate strictly over TLS 1.3 tunnels.
