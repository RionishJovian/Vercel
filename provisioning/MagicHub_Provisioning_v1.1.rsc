# ==============================================================================
# MIKROTIK PROVISIONING & EDGE INTEGRATION SCRIPT v1.1
# Core Function: Establish Magic Hub Control Plane & Hardware Hardening
# CHANGES v1.0 -> v1.1
#   [FIX-1] Serve JSON REST on www-ssl:443 /rest. NOT api-ssl:8729.
#   [FIX-2] Create and sign a self-signed cert BEFORE enabling TLS.
#   [FIX-3] Add 'rest-api' user policy (separate from 'api' on ROS 7.15+).
#   [FIX-4] Narrow allowed-address to the hub /32 only.
#   [FIX-5] Add output MSS clamp (change-mss 1160).
#   [FIX-6] Wire up the 'portal_bruteforce' address-list.
#   [FIX-7] Restrict www-ssl / REST bind to tunnel + mgmt VLAN only.

:global BusinessID "biz_prod_99342"
:global GatewayToken "mghub_tok_771a2b3c4d5e"
 :global MagicHubIP "138.197.42.11"
 :global MagicHubPort "51820"
 :global ClientTunnelIP "10.255.0.2/30"

 :log info "Initializing Edge Isolation and Magic Hub Command Tunnel..."


 # 1. WIREGUARD COMMAND TUNNEL INITIALIZATION

 /interface wireguard
add comment="Magic Hub Control Plane Tunnel" listen-port=13231 name=wg-magichub


 # [FIX-4] Hub peer /32 only. Removed 192.168.0.0/16.
 /interface wireguard peers
add allowed-address=10.255.0.1/32 comment="SaaS Backend API Routing (hub peer only)" \
    endpoint-address=$MagicHubIP endpoint-port=$MagicHubPort \
    interface=wg-magichub public-key="<HUB_PUBLIC_KEY_BASE64>" \
    persistent-keepalive=25s

 /ip address
add address=$ClientTunnelIP comment="Magic Hub Tunnel IP" interface=wg-magichub


 # 2. MANAGEMENT VLAN 99 & RELAYED PATHWAY CREATION
 #    [FIX-NOTE] ether1 must be your TRUSTED LAN port, NOT the WAN uplink..


 #    If ether1 is the uplink, move VLAN99 to a LAN-facing bridge/port..
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

 # [FIX-1] Serve JSON REST via www-ssl. The SaaS engine dials /rest here..
 # [FIX-7] Bind REST to tunnel + mgmt VLAN only..
 set www-ssl address=10.255.0.1/32,192.168.99.0/24 disabled=no port=443


 # [FIX-2] Self-signed cert, created and signed before enabling TLS..


 :if ([:len [/certificate find name="magic-hub-https"]] = 0) do={
   /certificate add name=magic-hub-https common-name=magic-hub days-valid=3650
   /certificate sign magic-hub-https
 }
 /ip service set [find name="www-ssl"] certificate=magic-hub-https


 # Configure localized service account for SaaS Engine API processing
 # [FIX-3] Added 'rest-api' to policy. Kept least privilege (!sensitive,!password).
 /user group
add name=saas_orchestrator policy=read,write,api,rest-api,test,!sensitive,!password
 /user
add comment="Automated Engine Identity" group=saas_orchestrator name="saas_worker" password="SuperSecureGeneratedPasswordString"


 # 4. HARDENED CAPTIVE PORTAL FIREWALL GUARDS (ANTI-BYPASS & ANTI-SPOOF)
 # [FIX-6] Address list is now maintained by a scan-rate feed. No longer dead config..


 /ip firewall raw
add action=add-src-to-address-list place-before=0 \
    chain=prerouting \
    dst-port=80,8000,8080 protocol=tcp connection-state=new \
    connection-rate=5,60 \
    address-list=portal_bruteforce address-list-timeout=1h \
    comment="Portal brute-force feed -- adjust rate/src to your hotspot"

 /ip firewall filter
add action=drop chain=input comment="CAPTIVE PORTAL GUARD: Drop Brute-Force Attacking MACs" \
    src-address-list=portal_bruteforce

 add action=drop chain=forward comment="CAPTIVE PORTAL GUARD: Block Unauthenticated DNS Tunneling" \
    dst-port=53 forward-hotspot=!auth protocol=udp src-address=!192.168.99.0/24

 add action=drop chain=forward comment="CAPTIVE PORTAL GUARD: Block Unauthenticated ICMP Bypass" \
    forward-hotspot=!auth protocol=icmp


 # [FIX-5] MSS clamp over the tunnel. Avoids PMTU stalls for large REST payloads..
 /ip firewall mangle
add chain=output out-interface=wg-magichub protocol=tcp tcp-flags=syn \
    action=change-mss new-mss=1160 passthrough=yes comment="magic-hub mss out"
add chain=forward out-interface=wg-magichub protocol=tcp tcp-flags=syn \
    action=change-mss new-mss=1160 passthrough=yes comment="magic-hub mss fwd"


 :log info "Edge Integration Success. Gateway is now bound to the Cloud Control Plane."