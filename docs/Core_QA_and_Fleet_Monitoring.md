# 🧪 Core QA & Fleet Monitoring

## 1. Infrastructure Monitoring Metric Specifications (Global Admin Panel)
The platform owner's central dashboard monitors operational telemetry indices pushed continuously from the cloud layers and edge devices to resolve performance bottlenecks.

### Magic Hub Transport Tunnel Telemetric Objects:
*   **Active Concurrency Envelope:** Total persistent WireGuard peer handshakes terminated at the Cloud Node vs. Available Regional Memory Capacities. (Alert Threshold: Trigger amber flag if concurrent pipelines exceed 80% capacity).
*   **Keep-Alive Packet Verification (Heartbeats):** Lightweight UDP telemetry check executed every 25 seconds down each edge pathway. (Fail-Safe Event: If 3 consecutive keep-alives time out, flag device node as 🔴 DOWN, append entry to the tenant's fleet ledger, and push a webhook notification).
*   **Transport Jitter & Packet Drop Metric:** Tracks control-payload packet drop percentages. (Optimization Rule: If control packet drops exceed 5% on a regional routing node, automatically drop dynamic WebFig reverse-proxy allocations to protect primary command pipelines).

### SaaS Core Engine Runtime Objects:
*   **Redis Active Telemetry Key Saturation:** Number of active hardware network statuses cached concurrently inside the system memory.
*   **PostgreSQL Pool Connection Load:** Real-time API query saturation levels checking multi-tenant license tier authorizations and active in-app purchase passes.
*   **AI RAG Compute Registry:** Tally of context-parsing tasks running concurrently through your vector mapping layers.

### On-Device Hardware Snapshot Telemetry (Pushed via Main Gateway REST API Every 60s):
*   **CPU & Core Thermal Saturation:** Local hardware processing usage index. (Guard Trigger: If a subscriber's edge router spikes past 95% CPU load for 5 minutes, the SaaS interface dynamically drops non-essential visual telemetry updates and issues an warning banner: "Edge Gateway overloading. Suspending non-essential telemetry tracking.").
*   **Flash Memory / Memory Write Boundaries:** Monitored to prevent local router log overflows.

## 2. Core QA Testing Strategy & Automated Test Cases
This testing matrix outlines formal validation scripts used by QA engineers to guarantee feature isolation boundaries before production rollouts.

### Test Profile Alpha: Magic Hub Connection Handshake & Quota Isolation Boundaries
*   **Objective:** Confirm that the system automatically enforces resource constraints at the application layer before executing configuration pushes.
*   **Pre-conditions:** Test account `u0000000-0000-0000-0000-000000000021` (Small WISP Owner) logged into the dashboard, currently holding an active base limit allocation count of 10 routers.
*   **QA Step Execution Pathway:**
    1. Direct the browser user interface to navigation element `/fleet/register-new-device`.
    2. Populate input parameter form values with mock entries, using a unique testing hardware MAC signature.
    3. Programmatically force the seeding database to increment this business’s registered device count to 10 active devices, hitting their ceiling profile limit.
    4. Click the `[Register Device Connection]` interface trigger button.
*   **Strict Expected Outcome Evaluation Criteria:** The WebUI must block the operation immediately. The dashboard view layout displays an amber message box: *"Device registration failed. Active hardware profile count has reached your account allocation limit. Purchase additional infrastructure slots to expand."* The database remains un-mutated.

### Test Profile Beta: 1-Click Advanced Configuration Generation & Injection Execution
*   **Objective:** Validate that interactive front-end UI buttons convert seamlessly into syntax-accurate RouterOS v7 REST configurations and execute via the control pipe.
*   **Pre-conditions:** Mock RouterOS VM up and responsive via virtual loopback address. Test account `u0000000-0000-0000-0000-000000000031` (Enterprise Engineer) logged in.
*   **QA Step Execution Pathway:**
    1. Navigate to `/traffic-management/queue-trees`.
    2. Use the graphical UI sliders to set *Corporate Guaranteed Bandwidth* to `100M`.
    3. Click the `[Optimize Corporate Traffic Profile]` action button.
    4. Capture the asynchronous payload sent through the message bus (`mq_broker_dev`) down to the Magic Hub bridge proxy simulator.
*   **Strict Expected Outcome Evaluation Criteria:** The API must intercept the action, confirm an Enterprise tier token flag is valid, and compile the correct JSON parameter request block: `{"path": "/queue/tree", "action": "add", "name": "Corporate_Parent", "max-limit": "100M"}`. The mock hardware VM processing cache must instantly accept this stream and inject the rule without dropping connections.

### Test Profile Gamma: Downstream Relayed Device Operations Verification Loop
*   **Objective:** Verify that commands targeting sub-devices (switches, APs) route through the primary gateway via internal management paths.
*   **Pre-conditions:** Main gateway and a secondary managed distribution switch properly mapped in the database structure with Parent-Child relationships active.
*   **QA Step Execution Pathway:**
    1. Access the web interface path `/network-map/canvas-workspace`.
    2. Click the specific node element representing the downstream **CRS326 Switch** component.
    3. In the context inspector panel, select the 1-click friendly button `[🔄 Force Reboot via Relay]`.
    4. Monitor the outbound communications logs on the central API engine console.
*   **Strict Expected Outcome Evaluation Criteria:** The platform backend must dispatch an execution command to the *Main Gateway's* unique token profile. The payload must instruct the Main Gateway to trigger a local script targeting the downstream unit's internal address (`192.168.99.5`). The system dashboard node element for the sub-switch must cycle gracefully from Green to Red, then return to Green once the heartbeat telemetry confirms the loop is complete.

### Test Profile Delta: Tactical Active Session Override Controls (Kick, Ban, Remove)
*   **Objective:** Confirm that emergency security flags sever malicious unauthenticated connections at the layer-2 hardware interface.
*   **Pre-conditions:** A live emulated captive portal user session registered under active leases on the test router instance.
*   **QA Step Execution Pathway:**
    1. Open the front-end layout screen tracking customer guest data (`/subscribers/active-leases`).
    2. Locate the row entry displaying suspect target MAC `CC:3A:91:XX:XX:XX`.
    3. Click the high-priority action switch option `[🚨 BAN HARD]`.
    4. Verify both the local cloud data array state and query the emulated router terminal ruleset.
*   **Strict Expected Outcome Evaluation Criteria:** The lease session row must immediately disappear from the web view list layout. The edge router instance must execute an un-bind trigger to dump the lease connection. Simultaneously, the router's internal firewall register must append `src-mac-address=CC:3A:91:XX:XX:XX action=drop` to its raw rules, locking out the attacker at the edge boundary.
