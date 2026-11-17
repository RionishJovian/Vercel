"""Composite index for tenant-scoped, time-filtered revenue and ledger reads.

Additive and non-destructive: no table or row changes. Every /v1/revenue and /v1/revenue/ledger query filters on
tenant_id and a time range (occurred_at), then orders by occurred_at. The two single-column indexes from 007 cannot
serve that together. The single-column ix_financial_events_tenant_id stays (nothing here removes an existing index);
it is now redundant and can be dropped by a later migration once this one has shipped.
"""
from alembic import op
revision='020'
down_revision='019'
branch_labels=None
depends_on=None
INDEX='ix_financial_events_tenant_occurred'
def upgrade():
 op.create_index(INDEX,'financial_events',['tenant_id','occurred_at'])
def downgrade():
 # Dropping an index loses no data, so no preservation check is needed. The ledger itself is still protected by 007,
 # which refuses to downgrade while financial events exist.
 op.drop_index(INDEX,table_name='financial_events')
