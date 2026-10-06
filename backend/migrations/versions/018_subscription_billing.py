"""Manual subscription invoice/payment history."""
from alembic import op,context
import sqlalchemy as sa
revision='018'
down_revision='017'
branch_labels=None
depends_on=None
def upgrade():
 op.add_column('tenant_subscriptions',sa.Column('payment_source_invoice_id',sa.String(36),nullable=True))
 columns=[sa.Column('id',sa.String(36),primary_key=True),sa.Column('tenant_id',sa.String(36),sa.ForeignKey('tenants.id'),nullable=False),sa.Column('plan_id',sa.String(36),sa.ForeignKey('subscription_plans.id'),nullable=False),sa.Column('snapshot',sa.JSON(),nullable=False),sa.Column('amount',sa.BigInteger(),nullable=False),sa.Column('currency',sa.String(3),nullable=False),sa.Column('period_start',sa.BigInteger(),nullable=False),sa.Column('period_end',sa.BigInteger(),nullable=False),sa.Column('subscription_revision',sa.Integer(),nullable=False),sa.Column('revision',sa.Integer(),nullable=False),sa.Column('status',sa.String(20),nullable=False),sa.Column('destination',sa.String(500),nullable=False),sa.Column('instructions',sa.String(2000),nullable=False)]
 for key,size in [('method',80),('reference',160),('evidence',4000),('submission_key',80),('submission_hash',64),('reason',1000)]:columns.append(sa.Column(key,sa.String(size),nullable=True))
 columns.extend([sa.Column('reviewed_by',sa.String(36),sa.ForeignKey('users.id'),nullable=True),sa.Column('created_by',sa.String(36),sa.ForeignKey('users.id'),nullable=False),sa.Column('request_key',sa.String(80),nullable=False),sa.Column('request_hash',sa.String(64),nullable=False),sa.Column('created',sa.BigInteger(),nullable=False),sa.Column('updated',sa.BigInteger(),nullable=False)])
 op.create_table('subscription_invoices',*columns,sa.UniqueConstraint('created_by','request_key'),sa.CheckConstraint('amount > 0'),sa.CheckConstraint('period_end > period_start'),sa.CheckConstraint("status in ('unpaid','pending','needs_evidence','paid','rejected','void')"));op.create_index('ix_subscription_invoices_tenant_id','subscription_invoices',['tenant_id'])
 op.create_table('billing_events',sa.Column('id',sa.String(36),primary_key=True),sa.Column('invoice_id',sa.String(36),sa.ForeignKey('subscription_invoices.id'),nullable=False),sa.Column('actor_id',sa.String(36),sa.ForeignKey('users.id'),nullable=False),sa.Column('event',sa.String(40),nullable=False),sa.Column('payload',sa.JSON(),nullable=False),sa.Column('created',sa.BigInteger(),nullable=False));op.create_index('ix_billing_events_invoice_id','billing_events',['invoice_id'])
def downgrade():
 if context.is_offline_mode():raise RuntimeError('Billing downgrade requires preservation checks')
 bind=op.get_bind()
 if bind.execute(sa.text('SELECT count(*) FROM subscription_invoices')).scalar() or bind.execute(sa.text('SELECT count(*) FROM billing_events')).scalar() or bind.execute(sa.text('SELECT count(*) FROM tenant_subscriptions WHERE payment_source_invoice_id IS NOT NULL')).scalar():raise RuntimeError('Billing records exist; preserve history and roll forward')
 op.drop_index('ix_billing_events_invoice_id',table_name='billing_events');op.drop_table('billing_events');op.drop_index('ix_subscription_invoices_tenant_id',table_name='subscription_invoices');op.drop_table('subscription_invoices')
 with op.batch_alter_table('tenant_subscriptions',table_args=[sa.CheckConstraint("state in ('trial','active','past_due','suspended','cancelled')"),sa.CheckConstraint('period_end > period_start')]) as batch:batch.drop_column('payment_source_invoice_id')
