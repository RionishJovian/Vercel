"""Single-use vouchers and explicit entitlement origins."""
from alembic import op,context
import sqlalchemy as sa
revision='003'
down_revision='002'
branch_labels=None
depends_on=None

def upgrade():
 op.create_table('vouchers',
  sa.Column('id',sa.String(36),primary_key=True),
  sa.Column('tenant_id',sa.String(36),sa.ForeignKey('tenants.id'),nullable=False),
  sa.Column('package_id',sa.String(36),sa.ForeignKey('packages.id'),nullable=False),
  sa.Column('policy',sa.JSON(),nullable=False),
  sa.Column('code_hash',sa.String(64),nullable=False,unique=True),
  sa.Column('code_hint',sa.String(4),nullable=False),
  sa.Column('issued_by',sa.String(36),sa.ForeignKey('users.id'),nullable=False),
  sa.Column('request_key',sa.String(80),nullable=False),
  sa.Column('created',sa.BigInteger(),nullable=False),
  sa.Column('expires',sa.BigInteger(),nullable=False),
  sa.Column('redeemed_by',sa.String(36),sa.ForeignKey('users.id'),nullable=True),
  sa.Column('redeemed_at',sa.BigInteger(),nullable=True),
  sa.Column('revoked',sa.Boolean(),nullable=False),
  sa.Column('reason',sa.String(1000),nullable=True),
  sa.UniqueConstraint('issued_by','request_key'),
  sa.CheckConstraint('expires > created'))
 op.create_index('ix_vouchers_tenant_id','vouchers',['tenant_id'])
 with op.batch_alter_table('entitlements') as batch:
  batch.alter_column('order_id',existing_type=sa.String(36),nullable=True)
  batch.add_column(sa.Column('voucher_id',sa.String(36),nullable=True))
  batch.create_foreign_key('fk_entitlement_voucher','vouchers',['voucher_id'],['id'])
  batch.create_unique_constraint('uq_entitlement_voucher',['voucher_id'])
  batch.create_check_constraint('ck_entitlement_source','(order_id IS NOT NULL AND voucher_id IS NULL) OR (order_id IS NULL AND voucher_id IS NOT NULL)')

def downgrade():
 if context.is_offline_mode():raise RuntimeError('Voucher downgrade requires an online check for existing records')
 if op.get_bind().execute(sa.text('SELECT count(*) FROM vouchers')).scalar():raise RuntimeError('Voucher records exist; preserve them and roll forward instead of downgrading')
 with op.batch_alter_table('entitlements') as batch:
  batch.drop_constraint('ck_entitlement_source',type_='check')
  batch.drop_constraint('uq_entitlement_voucher',type_='unique')
  batch.drop_constraint('fk_entitlement_voucher',type_='foreignkey')
  batch.drop_column('voucher_id')
  batch.alter_column('order_id',existing_type=sa.String(36),nullable=False)
 op.drop_index('ix_vouchers_tenant_id',table_name='vouchers');op.drop_table('vouchers')
