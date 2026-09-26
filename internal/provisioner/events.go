package provisioner

// NATS event subjects emitted and consumed by the apply/approve path
// (DESIGN-provisioner.md sections 2-4). The recovery intent itself arrives on
// recovery's own `peer.recover` / `peer.recover.cmd` subjects.
const (
	// SubjectActionRequested carries the proposed plan to the Lead for approval.
	SubjectActionRequested = "provision.action_requested"
	// SubjectApprovalRequest is the request/reply subject a Telegram bridge
	// answers with the Lead's approve/reject decision.
	SubjectApprovalRequest = "provision.approval"
	// SubjectApprovalResult reports the gate's final verdict.
	SubjectApprovalResult = "provision.approval.result"
	// SubjectApplyResult reports apply + verification outcome.
	SubjectApplyResult = "provision.apply.result"
	// SubjectRollbackResult reports an auto-revert after a verification failure.
	SubjectRollbackResult = "provision.rollback.result"
	// SubjectTopologyAction is the explicit device-action intent an owner
	// triggers from the topology map (e.g. force-reboot). Consumed by the
	// topology action consumer; distinct from recovery's peer.recover.cmd.
	SubjectTopologyAction = "topology.action"
)

// Publisher emits provisioner events onto the message bus. It is deliberately
// tiny so *messaging.Client satisfies it directly and tests can inject a
// recording fake without NATS.
type Publisher interface {
	Publish(businessID, subject string, data []byte) error
}
