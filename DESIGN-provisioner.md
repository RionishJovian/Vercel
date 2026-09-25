# Provisioner Design — Safety-First Router Recovery

## Status
- **Needs sign-off from:** The Lead (you)
- **Touches:** Production CCR2004 at tunnel 10.77.0.4
- **Risk level:** Medium (changes live router config)

## Goal
Enable the provisioner to actually *fix* a recovered MikroTik router that has passed Netinstall. Currently, `peer.recover` is emitted but nothing consumes it.

## Design

### 1. Dry-Run Mode
```go
// CLI:  go run ./cmd/provisioner --dry-run --peer 10.77.0.4
func (p *Provisioner) DryRunAction(ctx context.Context, peerID, cfgFile string) error {
    // Validate config, check connectivity, simulate changes
    // NEVER writes to prod
}
```

### 2. Manual Approve-Gate
- Provisioner emits `provision.action_requested` event via NATS
- Lead receives notification via Telegram bot (Clawclaw)
- Lead replies: "approve" or "reject"
- Only on "approve" does the provisioner apply changes

### 3. Verify-Before-Apply
```go
func (p *Provisioner) ApplyFixes(ctx context.Context, peer Peer) error {
    // 1. Read current config
    // 2. Apply ONLY validated diff
    // 3. Verify: tunnel up, services up, connectivity from cloud
    // 4. Emit verification event; if fail, rollback
}
```

### 4. Rollback on Failure
- Save pre-change snapshot ( WireGuard config, IP routes )
- On verification fail, auto-revert within 60s

## Workflow Diagram
```
peer.recover event → DryRun → emit proposed changes
                              ↓ Lead approves
                     Apply -> Verify -> Success / Rollback
```

## Questions for Sign-Off
1. Approve dry-run + manual-gate pattern?
2. Is 60s rollback window appropriate?
3. Should the Telegram approval require 2FA (e.g., OTP from Lead's phone)?