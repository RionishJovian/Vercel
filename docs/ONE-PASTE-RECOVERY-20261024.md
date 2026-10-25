# One-paste preparation for the deployed API container

The production WireGuard interface belongs to the VPS host, not the API's Docker network namespace. Explicit `wireguard.management_source_cidr` now supports that layout: it must be one private IPv4 /32. Deployment must verify the actual host route/NAT source and the signed Hub's allowed addresses. The generated RouterOS script checks that exact source is included; it never broadens access to a subnet or enables public management. Empty configuration retains interface discovery for native/split-host deployments.

Automatic preparation now preflights the source and absence of an existing verified connection before requesting a new Hub allocation. Existing saved management is never replaced by retrying onboarding.

Pending router installation keys and management proof/key/script state are encrypted with the existing credential encryption key in `hub_pending_enrollments`. Tenant SQL roles have no access; forced RLS, composite tenant/device ownership and identity/expiry-bound encrypted envelopes protect the rows. PostgreSQL advisory locks serialize generation, including across API instances. No plaintext secrets enter logs, source, manifests or browser persistence. Script responses remain no-store.

The router installation key is reusable for at most 24 hours, so a fresh 30-minute management proof can be prepared without replacing the reserved WireGuard identity. Management proofs last 30 minutes. Successful pinned/proven management verification deletes both pending records. Expired artifacts cannot be returned; ciphertext is removed with completed enrollment/device cleanup or overwritten on an explicitly new attempt. No assumption of immediate background physical erasure is made.

Already-reserved peers whose private installation artifacts were lost before this fix remain a separate repair case. Their public keys cannot reconstruct the secret. No automatic peer deletion, key rotation or reassignment is performed. A repair must be explicitly scoped and verify no established router/active migration would be disrupted; connected customers keep their keys.

Tests: real PostgreSQL concurrent generation, encryption, restart, tenant isolation, ciphertext swapping and expiry; HTTP preparation with actual server wiring survives fresh handler state and management-proof renewal without new peer allocation; expired installation secrets refuse silent key replacement; configured source validation and existing ownership/proof tests. These prove software behavior, not physical RouterOS execution.
