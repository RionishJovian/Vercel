"""Cash event history, backfilled from audit or explicitly estimated legacy dates."""
from alembic import op,context
import sqlalchemy as sa
revision='007'
down_revision='006'
branch_labels=None
depends_on=None

def upgrade():
 events=op.create_table('financial_events',sa.Column('id',sa.String(80),primary_key=True),sa.Column('tenant_id',sa.String(36),sa.ForeignKey('tenants.id'),nullable=False),sa.Column('site_id',sa.String(36),sa.ForeignKey('sites.id'),nullable=True),sa.Column('source',sa.String(20),nullable=False),sa.Column('source_id',sa.String(36),nullable=False),sa.Column('kind',sa.String(16),nullable=False),sa.Column('amount',sa.BigInteger(),nullable=False),sa.Column('currency',sa.String(3),nullable=False),sa.Column('service',sa.String(20),nullable=False),sa.Column('package_name',sa.String(120),nullable=False),sa.Column('method',sa.String(80),nullable=False),sa.Column('reference',sa.String(160),nullable=False),sa.Column('actor_id',sa.String(36),sa.ForeignKey('users.id'),nullable=True),sa.Column('occurred_at',sa.BigInteger(),nullable=False),sa.Column('timestamp_basis',sa.String(32),nullable=False),sa.UniqueConstraint('source','source_id','kind'),sa.CheckConstraint('amount > 0'),sa.CheckConstraint("kind IN ('collection','refund')"),sa.CheckConstraint("source IN ('guest_order','voucher_sale')"))
 op.create_index('ix_financial_events_tenant_id','financial_events',['tenant_id']);op.create_index('ix_financial_events_occurred_at','financial_events',['occurred_at'])
 cols=['id','tenant_id','site_id','source','source_id','kind','amount','currency','service','package_name','method','reference','actor_id','occurred_at','timestamp_basis']
 orders=sa.table('orders',*[sa.column(n,t) for n,t in [('id',sa.String),('tenant_id',sa.String),('site_id',sa.String),('snapshot',sa.JSON),('amount',sa.BigInteger),('currency',sa.String),('method',sa.String),('reference',sa.String),('reviewer_id',sa.String),('created',sa.BigInteger),('status',sa.String)]])
 audit=sa.table('audit_events',sa.column('id',sa.String),sa.column('actor_id',sa.String),sa.column('target',sa.String),sa.column('tenant_id',sa.String),sa.column('action',sa.String),sa.column('created',sa.BigInteger))
 for kind,action,states in [('collection','payment_approved',['approved','refunded']),('refund','refund_recorded',['refunded'])]:
  clock=sa.select(sa.func.min(audit.c.created)).where(audit.c.tenant_id==orders.c.tenant_id,audit.c.target==orders.c.id,audit.c.action==action).scalar_subquery()
  actor=sa.select(audit.c.actor_id).where(audit.c.tenant_id==orders.c.tenant_id,audit.c.target==orders.c.id,audit.c.action==action).order_by(audit.c.created,audit.c.id).limit(1).scalar_subquery()
  q=sa.select(sa.literal('guest_order:'+kind+':')+orders.c.id,orders.c.tenant_id,orders.c.site_id,sa.literal('guest_order'),orders.c.id,sa.literal(kind),orders.c.amount,orders.c.currency,sa.func.coalesce(orders.c.snapshot['service'].as_string(),'unknown'),sa.func.coalesce(orders.c.snapshot['name'].as_string(),'Legacy package'),orders.c.method,orders.c.reference,sa.func.coalesce(actor,orders.c.reviewer_id) if kind=='collection' else actor,sa.func.coalesce(clock,orders.c.created),sa.case((clock.is_not(None),'migrated_audit'),else_='legacy_submission_estimate')).where(orders.c.status.in_(states))
  op.execute(events.insert().from_select(cols,q))
 sales=sa.table('voucher_sales',*[sa.column(n,t) for n,t in [('id',sa.String),('tenant_id',sa.String),('voucher_id',sa.String),('snapshot',sa.JSON),('amount',sa.BigInteger),('currency',sa.String),('method',sa.String),('reference',sa.String),('recorded_by',sa.String),('created',sa.BigInteger),('refunded_at',sa.BigInteger),('refunded_by',sa.String),('refund_reference',sa.String)]])
 vouchers=sa.table('vouchers',sa.column('id',sa.String),sa.column('tenant_id',sa.String),sa.column('site_id',sa.String))
 for kind in ['collection','refund']:
  refund=kind=='refund'
  q=sa.select(sa.literal('voucher_sale:'+kind+':')+sales.c.id,sales.c.tenant_id,vouchers.c.site_id,sa.literal('voucher_sale'),sales.c.id,sa.literal(kind),sales.c.amount,sales.c.currency,sa.func.coalesce(sales.c.snapshot['service'].as_string(),'unknown'),sa.func.coalesce(sales.c.snapshot['name'].as_string(),'Legacy package'),sales.c.method,sa.func.coalesce(sales.c.refund_reference,sales.c.reference) if refund else sales.c.reference,sales.c.refunded_by if refund else sales.c.recorded_by,sales.c.refunded_at if refund else sales.c.created,sa.literal('migrated_record')).select_from(sales.join(vouchers,sa.and_(vouchers.c.id==sales.c.voucher_id,vouchers.c.tenant_id==sales.c.tenant_id)))
  if refund:q=q.where(sales.c.refunded_at.is_not(None))
  op.execute(events.insert().from_select(cols,q))

def downgrade():
 if context.is_offline_mode():raise RuntimeError('Financial ledger downgrade needs preservation checks')
 if op.get_bind().execute(sa.text('SELECT count(*) FROM financial_events')).scalar():raise RuntimeError('Financial events exist; preserve ledger and roll forward')
 op.drop_index('ix_financial_events_occurred_at',table_name='financial_events');op.drop_index('ix_financial_events_tenant_id',table_name='financial_events');op.drop_table('financial_events')
