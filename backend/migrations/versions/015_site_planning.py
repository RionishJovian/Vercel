"""Saved owner WAN and budget plans, without network application."""
from alembic import op,context
import sqlalchemy as sa
revision='015'
down_revision='014'
branch_labels=None
depends_on=None
def upgrade():
 op.create_table('site_planning',sa.Column('site_id',sa.String(36),sa.ForeignKey('sites.id'),primary_key=True),sa.Column('revision',sa.Integer(),nullable=False),sa.Column('settings',sa.JSON(),nullable=False),sa.Column('updated',sa.BigInteger(),nullable=False))
def downgrade():
 if context.is_offline_mode():raise RuntimeError('Site planning downgrade requires preservation checks')
 if op.get_bind().execute(sa.text('SELECT count(*) FROM site_planning')).scalar():raise RuntimeError('Site plans exist; preserve them and roll forward')
 op.drop_table('site_planning')
