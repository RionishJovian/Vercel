"""Portal revisions and immutable order payment instructions."""
from alembic import op
import sqlalchemy as sa
revision='002'
down_revision='001'
branch_labels=None
depends_on=None

def upgrade():
 op.create_table('portal_settings',
  sa.Column('tenant_id',sa.String(36),sa.ForeignKey('tenants.id'),primary_key=True),
  sa.Column('revision',sa.Integer(),nullable=False),
  sa.Column('draft',sa.JSON(),nullable=False),
  sa.Column('published',sa.JSON(),nullable=True),
  sa.Column('published_revision',sa.Integer(),nullable=True),
  sa.Column('updated',sa.BigInteger(),nullable=False))
 op.add_column('orders',sa.Column('payment_context',sa.JSON(),nullable=True))

def downgrade():
 op.drop_column('orders','payment_context')
 op.drop_table('portal_settings')
