# 📖 User Manual & Portal Consent

## 1. Onboarding & Pre-Deployment Provisioning Guide
To pair an on-premise MikroTik RouterBoard gateway with the SaaS management app, the WISP or Enterprise operator must apply the following baseline provisioning script. 

### Step-by-Step Hardware Pairing Execution:
1. Access your raw MikroTik router via standard WinBox or native CLI.
2. Open a clean `Terminal` screen.
3. Copy and paste the configuration code block below, ensure you replace the global variables (`BusinessID`, `GatewayToken`, and `MagicHubIP`) with your unique dashboard credentials, and press **Enter**.

```routeros
# ==============================================================================
# MIKROTIK PROVISIONING & EDGE INTEGRATION SCRIPT v1.0
# Core Function: Establish Magic Hub Control Plane & Hardware Hardening
# ==============================================================================

:global BusinessID "biz_prod_99342"
:global GatewayToken "mghub_tok_771a2b3c4d5e"
:global MagicHubIP "138.197.42.11"
:global MagicHubPort "51820"
:global ClientTunnelIP "10.255.0.2/30"

:log info "Initializing Edge Isolation and Magic Hub Command Tunnel..."

# 1. WIREGUARD COMMAND TUNNEL INITIALIZATION
/interface wireguard
add comment="Magic Hub Control Plane Tunnel" listen-port=13231 name=wg-magichub

/interface wireguard peers
add allowed-address=10.255.0.1/32,192.168.0.0/16 comment="SaaS Backend API Routing" \
    endpoint-address=$MagicHubIP endpoint-port=$MagicHubPort \
    interface=wg-magichub public-key="Base64KeyFromYourCloudServerEngine=" \
    persistent-keepalive=25s

/ip address
add address=$ClientTunnelIP comment="Magic Hub Tunnel IP" interface=wg-magichub

# 2. MANAGEMENT VLAN 99 & RELAYED PATHWAY CREATION
/interface vlan
add comment="Isolated Out-of-Band Management (OOBM)" interface=ether1 name=VLAN99_Mgmt vlan-id=99

/ip address
add address=192.168.99.1/24 comment="Isolated Management Interface Network" interface=VLAN99_Mgmt

# 3. ROUTEROS SYSTEM SECURITY HARDENING & API ACTIVATION
/ip service
set telnet disabled=yes
set ftp disabled=yes
set www disabled=yes
set ssh disabled=yes
set api disabled=yes
set winbox disabled=yes

# Configure secure REST API over SSL for SaaS backend communications
set api-ssl address=10.255.0.1/32,192.168.99.0/24 disabled=no port=8729 certificate=none

# Configure localized user account for SaaS Engine API processing
/user group
add name=saas_orchestrator policy=read,write,api,test,!sensitive,!password
/user
add comment="Automated Engine Identity" group=saas_orchestrator name="saas_worker" password="SuperSecureGeneratedPasswordString"

# 4. HARDENED CAPTIVE PORTAL FIREWALL GUARDS (ANTI-BYPASS & ANTI-SPOOF)
/ip firewall filter
add action=drop chain=input comment="CAPTIVE PORTAL GUARD: Drop Brute-Force Attacking MACs" \
    src-address-list=portal_bruteforce

add action=drop chain=forward comment="CAPTIVE PORTAL GUARD: Block Unauthenticated DNS Tunneling" \
    dst-port=53 forward-hotspot=!auth protocol=udp src-address=!192.168.99.0/24
add action=drop chain=forward comment="CAPTIVE PORTAL GUARD: Block Unauthenticated ICMP Bypass" \
    forward-hotspot=!auth protocol=icmp

:log info "Edge Integration Success. Gateway is now bound to the Cloud Control Plane."
```

## 2. Interactive Dude-Style Topology Map Operations
The **Topology Workspace** utilizes a live canvas mesh rendering all mapped assets automatically. Clicking on any network node pops out the **Relayed Management inspector Sidebar**:
*   🟢 **Green State:** Node is responsive, secure, and processing normal heartbeat loops.
*   🟡 **Yellow State:** An anomaly or port loop alert is occurring. The **RSTP/MSTP Spanning Tree protection engine** has isolated a loop port. Click `[🚫 Clear Port Loop State]` to dynamically force script resets down the line.
*   🔴 **Red State:** Downstream signal connectivity loss. The Main Gateway is active, but the local wire link to that specific Switch or AP has collapsed. Click `[📄 Pull Local Logs]` to run an out-of-band management query over **VLAN 99** to diagnose the hardware port locally.

## 3. Active Session Management Manual (Kick, Ban, Remove)
Operators can monitor active guest hotspot subscribers in real-time from the **Active Fleet Registry Ledger**. If a connection shows suspicious traffic usage or looks like a bypass exploit attempt, the operator can execute three tactical fallback actions instantly:
*   **Kick:** Terminates the current active dynamic DHCP and hotspot login session lease. The user is kicked out and forced back to the primary Captive Portal landing screen to re-authenticate.
*   **Ban:** Pushes an absolute firewall blocking instruction down the control pipeline. The edge gateway router appends the target hardware device's unique physical MAC identity directly to its Layer-2 filter drop lists permanently.
*   **Remove:** Completely revokes the active intranet lease configuration and obliterates the valid printed voucher key string inside the local User Manager database, invalidating the card forever.

## 4. Captive Portal User Consent Module (HTML Framework)
This responsive legal component layer is deployed out-of-the-box onto the on-premise hardware storage space, capturing verified guest authorization checkboxes locally before provisioning internet streams:

```html
<!-- Privacy Policy Interface Consent Component Layer -->
<div class="privacy-container" style="font-family: sans-serif; max-width: 480px; margin: 0 auto; padding: 15px; border: 1px solid #e2e8f0; border-radius: 8px; background-color: #ffffff;">
    <h3 style="margin-top: 0; color: #1e293b; font-size: 16px;">📶 Network Connection & Privacy Terms</h3>
    
    <div class="privacy-body" style="height: 120px; overflow-y: scroll; border: 1px solid #cbd5e1; padding: 10px; border-radius: 4px; font-size: 12px; color: #475569; line-height: 1.5; margin-bottom: 12px;">
        <p><strong>1. Data Minimization & Privacy Protection:</strong> This network operates under an edge-first network management design. Your browsing history, data packet contents, personal search logs, and web destinations are completely private, are never tracked, and are never logged by this platform or stored on our cloud infrastructure.</p>
        
        <p><strong>2. Information Collected for Authentication:</strong> To grant authorized internet access and monitor your purchased voucher limits, the local hardware router registers the following ephemeral infrastructure telemetry data:</p>
        <ul>
            <li>Your device's hardware identity (MAC Address)</li>
            <li>Dynamically allocated local network tracking token (IP Address)</li>
            <li>The specific Voucher Code entered during authentication</li>
            <li>Real-time traffic throughput metrics (Bytes Used & Time Session Elapsed)</li>
        </ul>
        
        <p><strong>3. Data Retention Framework (Right to be Forgotten):</strong> In compliance with international data privacy guidelines, your technical access log footprint is automatically deleted and entirely scrubbed from the edge hardware router data logs within 24 hours of your voucher code expiring.</p>
        
        <p><strong>4. Absolute Fraud & Abuse Prevention:</strong> If malicious access maneuvers, rogue bypass scripts, or rapid authorization brute-force combinations are initiated from your hardware identity, your access will be instantly terminated and blacklisted at the hardware boundary layer.</p>
    </div>

    <label class="consent-checkbox-wrapper" style="display: flex; align-items: flex-start; cursor: pointer; font-size: 13px; color: #0f172a; line-height: 1.4;">
        <input type="checkbox" id="privacy-consent-toggle" name="privacy_consent" required style="margin-top: 3px; margin-right: 8px;">
        <span>I explicitly agree to the network connection parameters and data optimization policies stated above.</span>
    </label>
</div>
```
