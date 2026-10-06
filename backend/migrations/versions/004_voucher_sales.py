"""Confirmed counter collections for vouchers, separate from issuance."""
from alembic import op,context
import sqlalchemy as sa
revision='004'
down_revision='003'
branch_labels=None
depends_on=None

def upgrade():
 op.create_table('voucher_sales',
  sa.Column('id',sa.String(36),primary_key=True),
  sa.Column('voucher_id',sa.String(36),sa.ForeignKey('vouchers.id'),nullable=False,unique=True),
  sa.Column('tenant_id',sa.String(36),sa.ForeignKey('tenants.id'),nullable=False),
  sa.Column('snapshot',sa.JSON(),nullable=False),
  sa.Column('amount',sa.BigInteger(),nullable=False),
  sa.Column('currency',sa.String(3),nullable=False),
  sa.Column('method',sa.String(80),nullable=False),
  sa.Column('reference',sa.String(160),nullable=False),
  sa.Column('evidence',sa.String(4000),nullable=False),
  sa.Column('recorded_by',sa.String(36),sa.ForeignKey('users.id'),nullable=False),
  sa.Column('created',sa.BigInteger(),nullable=False),
  sa.Column('refunded_at',sa.BigInteger(),nullable=True),
  sa.Column('refunded_by',sa.String(36),sa.ForeignKey('users.id'),nullable=True),
  sa.Column('refund_reason',sa.String(1000),nullable=True),
  sa.Column('refund_reference',sa.String(160),nullable=True),
  sa.CheckConstraint('amount > 0'))
 op.create_index('ix_voucher_sales_tenant_id','voucher_sales',['tenant_id'])

def downgrade():
 if context.is_offline_mode():raise RuntimeError('Sales downgrade requires online preservation check')
 if op.get_bind().execute(sa.text('SELECT count(*) FROM voucher_sales')).scalar():raise RuntimeError('Voucher sale records exist; preserve accounting history and roll forward')
 op.drop_index('ix_voucher_sales_tenant_id',table_name='voucher_sales');op.drop_table('voucher_sales')
