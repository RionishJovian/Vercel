"""Tenant media metadata; no stream URLs or server credentials."""
from alembic import op,context
import sqlalchemy as sa
revision='009'
down_revision='008'
branch_labels=None
depends_on=None
def upgrade():
 op.create_table('media_titles',sa.Column('id',sa.String(36),primary_key=True),sa.Column('tenant_id',sa.String(36),sa.ForeignKey('tenants.id'),nullable=False),sa.Column('name',sa.String(120),nullable=False),sa.Column('description',sa.String(4000),nullable=False),sa.Column('genre',sa.String(80),nullable=False),sa.Column('runtime_minutes',sa.Integer(),nullable=False),sa.Column('release_year',sa.Integer(),nullable=True),sa.Column('published',sa.Boolean(),nullable=False),sa.Column('archived',sa.Boolean(),nullable=False),sa.Column('revision',sa.Integer(),nullable=False),sa.Column('updated',sa.BigInteger(),nullable=False),sa.CheckConstraint('runtime_minutes > 0'),sa.CheckConstraint('revision > 0'))
 op.create_index('ix_media_titles_tenant_id','media_titles',['tenant_id'])
def downgrade():
 if context.is_offline_mode():raise RuntimeError('Media downgrade needs preservation checks')
 if op.get_bind().execute(sa.text('SELECT count(*) FROM media_titles')).scalar():raise RuntimeError('Media catalogue records exist; preserve titles and roll forward')
 op.drop_index('ix_media_titles_tenant_id',table_name='media_titles');op.drop_table('media_titles')
