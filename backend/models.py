from typing import Optional
from sqlalchemy.orm import DeclarativeBase, Mapped, mapped_column
from sqlalchemy import String, Integer, BigInteger, ForeignKey, JSON, UniqueConstraint, Boolean, CheckConstraint, LargeBinary
import uuid
class Base(DeclarativeBase):pass
def uid():return str(uuid.uuid4())
class Tenant(Base):
 __tablename__='tenants'
 id:Mapped[str]=mapped_column(String(36),primary_key=True,default=uid)
 name:Mapped[str]=mapped_column(String(160))
class User(Base):
 __tablename__='users'
 id:Mapped[str]=mapped_column(String(36),primary_key=True,default=uid)
 username:Mapped[str]=mapped_column(String(160),unique=True)
 password_hash:Mapped[str]=mapped_column(String(255))
 role:Mapped[str]=mapped_column(String(20))
 tenant_id:Mapped[Optional[str]]=mapped_column(ForeignKey('tenants.id'))
 active:Mapped[bool]=mapped_column(Boolean,default=True)
 must_change_password:Mapped[bool]=mapped_column(Boolean,default=False,server_default='0')
 temporary_password_expires:Mapped[Optional[int]]=mapped_column(BigInteger,nullable=True)
 __table_args__=(CheckConstraint("role in ('platform','sme','guest')"),CheckConstraint("role = 'platform' OR tenant_id IS NOT NULL"))
class SessionToken(Base):
 __tablename__='sessions'
 token_hash:Mapped[str]=mapped_column(String(64),primary_key=True)
 user_id:Mapped[str]=mapped_column(ForeignKey('users.id'))
 expires:Mapped[int]=mapped_column(BigInteger)
class LoginAttempt(Base):
 __tablename__='login_attempts'
 key:Mapped[str]=mapped_column(String(64),primary_key=True)
 failures:Mapped[int]=mapped_column(Integer,default=0)
 window_start:Mapped[int]=mapped_column(BigInteger)
class Package(Base):
 __tablename__='packages'
 id:Mapped[str]=mapped_column(String(36),primary_key=True,default=uid)
 tenant_id:Mapped[str]=mapped_column(ForeignKey('tenants.id'),index=True)
 policy:Mapped[dict]=mapped_column(JSON)
 active:Mapped[bool]=mapped_column(Boolean,default=True)
class Order(Base):
 __tablename__='orders'
 event_revision:Mapped[int]=mapped_column(Integer,default=0,server_default='0')
 receipt_revision:Mapped[int]=mapped_column(Integer,default=0,server_default='0')
 id:Mapped[str]=mapped_column(String(36),primary_key=True,default=uid)
 tenant_id:Mapped[str]=mapped_column(ForeignKey('tenants.id'),index=True)
 site_id:Mapped[Optional[str]]=mapped_column(ForeignKey('sites.id'),nullable=True)
 guest_id:Mapped[str]=mapped_column(ForeignKey('users.id'))
 package_id:Mapped[str]=mapped_column(ForeignKey('packages.id'))
 snapshot:Mapped[dict]=mapped_column(JSON)
 payment_context:Mapped[Optional[dict]]=mapped_column(JSON,nullable=True)
 amount:Mapped[int]=mapped_column(BigInteger)
 currency:Mapped[str]=mapped_column(String(3),default='MMK')
 method:Mapped[str]=mapped_column(String(80))
 reference:Mapped[str]=mapped_column(String(160))
 evidence:Mapped[str]=mapped_column(String(4000))
 status:Mapped[str]=mapped_column(String(24),default='pending')
 fulfillment:Mapped[str]=mapped_column(String(24),default='pending')
 reviewer_id:Mapped[Optional[str]]=mapped_column(ForeignKey('users.id'))
 reason:Mapped[Optional[str]]=mapped_column(String(1000))
 created:Mapped[int]=mapped_column(BigInteger)
 request_key:Mapped[str]=mapped_column(String(80))
 __table_args__=(UniqueConstraint('guest_id','request_key'),CheckConstraint('amount > 0'))
class Entitlement(Base):
 __tablename__='entitlements'
 id:Mapped[str]=mapped_column(String(36),primary_key=True,default=uid)
 order_id:Mapped[Optional[str]]=mapped_column(ForeignKey('orders.id'),unique=True,nullable=True)
 voucher_id:Mapped[Optional[str]]=mapped_column(ForeignKey('vouchers.id'),unique=True,nullable=True)
 tenant_id:Mapped[str]=mapped_column(ForeignKey('tenants.id'),index=True)
 site_id:Mapped[Optional[str]]=mapped_column(ForeignKey('sites.id'),nullable=True)
 guest_id:Mapped[str]=mapped_column(ForeignKey('users.id'))
 policy:Mapped[dict]=mapped_column(JSON)
 activated:Mapped[Optional[int]]=mapped_column(BigInteger)
 used_bytes:Mapped[int]=mapped_column(BigInteger,default=0)
 connected_seconds:Mapped[int]=mapped_column(BigInteger,default=0)
 revoked:Mapped[bool]=mapped_column(Boolean,default=False)
 __table_args__=(CheckConstraint('(order_id IS NOT NULL AND voucher_id IS NULL) OR (order_id IS NULL AND voucher_id IS NOT NULL)',name='ck_entitlement_source'),)
class Audit(Base):
 __tablename__='audit_events'
 id:Mapped[str]=mapped_column(String(36),primary_key=True,default=uid)
 tenant_id:Mapped[Optional[str]]=mapped_column(ForeignKey('tenants.id'),index=True)
 actor_id:Mapped[str]=mapped_column(ForeignKey('users.id'))
 action:Mapped[str]=mapped_column(String(100))
 target:Mapped[str]=mapped_column(String(160))
 created:Mapped[int]=mapped_column(BigInteger)
class Outbox(Base):
 __tablename__='outbox_events'
 id:Mapped[str]=mapped_column(String(36),primary_key=True,default=uid)
 tenant_id:Mapped[str]=mapped_column(ForeignKey('tenants.id'),index=True)
 order_id:Mapped[str]=mapped_column(ForeignKey('orders.id'))
 event:Mapped[str]=mapped_column(String(50))
 delivered:Mapped[bool]=mapped_column(Boolean,default=False)
 revision:Mapped[Optional[int]]=mapped_column(Integer,nullable=True)
 created:Mapped[Optional[int]]=mapped_column(BigInteger,nullable=True)
 payload:Mapped[Optional[dict]]=mapped_column(JSON,nullable=True)
 __table_args__=(UniqueConstraint('order_id','revision',name='uq_outbox_order_revision'),)

class PortalSettings(Base):
 __tablename__='portal_settings'
 tenant_id:Mapped[str]=mapped_column(ForeignKey('tenants.id'),primary_key=True)
 revision:Mapped[int]=mapped_column(Integer,default=1)
 draft:Mapped[dict]=mapped_column(JSON)
 published:Mapped[Optional[dict]]=mapped_column(JSON,nullable=True)
 published_revision:Mapped[Optional[int]]=mapped_column(Integer)
 updated:Mapped[int]=mapped_column(BigInteger)

class Voucher(Base):
 __tablename__='vouchers'
 id:Mapped[str]=mapped_column(String(36),primary_key=True,default=uid)
 tenant_id:Mapped[str]=mapped_column(ForeignKey('tenants.id'),index=True)
 site_id:Mapped[Optional[str]]=mapped_column(ForeignKey('sites.id'),nullable=True)
 package_id:Mapped[str]=mapped_column(ForeignKey('packages.id'))
 policy:Mapped[dict]=mapped_column(JSON)
 code_hash:Mapped[str]=mapped_column(String(64),unique=True)
 code_hint:Mapped[str]=mapped_column(String(4))
 issued_by:Mapped[str]=mapped_column(ForeignKey('users.id'))
 request_key:Mapped[str]=mapped_column(String(80))
 created:Mapped[int]=mapped_column(BigInteger)
 expires:Mapped[int]=mapped_column(BigInteger)
 redeemed_by:Mapped[Optional[str]]=mapped_column(ForeignKey('users.id'))
 redeemed_at:Mapped[Optional[int]]=mapped_column(BigInteger)
 revoked:Mapped[bool]=mapped_column(Boolean,default=False)
 reason:Mapped[Optional[str]]=mapped_column(String(1000))
 __table_args__=(UniqueConstraint('issued_by','request_key'),CheckConstraint('expires > created'))

class VoucherSale(Base):
 __tablename__='voucher_sales'
 id:Mapped[str]=mapped_column(String(36),primary_key=True,default=uid)
 voucher_id:Mapped[str]=mapped_column(ForeignKey('vouchers.id'),unique=True)
 tenant_id:Mapped[str]=mapped_column(ForeignKey('tenants.id'),index=True)
 snapshot:Mapped[dict]=mapped_column(JSON)
 amount:Mapped[int]=mapped_column(BigInteger)
 currency:Mapped[str]=mapped_column(String(3),default='MMK')
 method:Mapped[str]=mapped_column(String(80))
 reference:Mapped[str]=mapped_column(String(160))
 evidence:Mapped[str]=mapped_column(String(4000))
 recorded_by:Mapped[str]=mapped_column(ForeignKey('users.id'))
 created:Mapped[int]=mapped_column(BigInteger)
 refunded_at:Mapped[Optional[int]]=mapped_column(BigInteger)
 refunded_by:Mapped[Optional[str]]=mapped_column(ForeignKey('users.id'))
 refund_reason:Mapped[Optional[str]]=mapped_column(String(1000))
 refund_reference:Mapped[Optional[str]]=mapped_column(String(160))
 __table_args__=(CheckConstraint('amount > 0'),)

class Site(Base):
 __tablename__='sites'
 id:Mapped[str]=mapped_column(String(36),primary_key=True,default=uid)
 tenant_id:Mapped[str]=mapped_column(ForeignKey('tenants.id'),index=True)
 name:Mapped[str]=mapped_column(String(120))
 timezone:Mapped[str]=mapped_column(String(80),default='Asia/Yangon')
 created:Mapped[int]=mapped_column(BigInteger)
 __table_args__=(UniqueConstraint('tenant_id','name'),)
class RouterDevice(Base):
 __tablename__='router_devices'
 id:Mapped[str]=mapped_column(String(36),primary_key=True,default=uid)
 tenant_id:Mapped[str]=mapped_column(ForeignKey('tenants.id'),index=True)
 site_id:Mapped[str]=mapped_column(ForeignKey('sites.id'))
 name:Mapped[str]=mapped_column(String(120))
 model:Mapped[str]=mapped_column(String(120))
 reported_routeros:Mapped[str]=mapped_column(String(40))
 registered_by:Mapped[str]=mapped_column(ForeignKey('users.id'))
 created:Mapped[int]=mapped_column(BigInteger)
 __table_args__=(UniqueConstraint('site_id','name'),)
class SitePortal(Base):
 __tablename__='site_portals'
 site_id:Mapped[str]=mapped_column(ForeignKey('sites.id'),primary_key=True)
 revision:Mapped[int]=mapped_column(Integer)
 draft:Mapped[dict]=mapped_column(JSON)
 published:Mapped[Optional[dict]]=mapped_column(JSON,nullable=True)
 published_revision:Mapped[Optional[int]]=mapped_column(Integer)
 updated:Mapped[int]=mapped_column(BigInteger)

class MagicHub(Base):
 __tablename__='magic_hubs'
 id:Mapped[str]=mapped_column(String(36),primary_key=True,default=uid)
 name:Mapped[str]=mapped_column(String(120),unique=True)
 endpoint:Mapped[str]=mapped_column(String(253))
 port:Mapped[int]=mapped_column(Integer)
 pool:Mapped[str]=mapped_column(String(18),unique=True)
 public_key:Mapped[str]=mapped_column(String(44),unique=True)
 next_host:Mapped[int]=mapped_column(Integer,default=2)
 created:Mapped[int]=mapped_column(BigInteger)
 __table_args__=(UniqueConstraint('endpoint','port'),CheckConstraint('port >= 1 AND port <= 65535'),CheckConstraint('next_host >= 2 AND next_host <= 255'))
class HubAssignment(Base):
 __tablename__='hub_assignments'
 hub_id:Mapped[str]=mapped_column(ForeignKey('magic_hubs.id'),primary_key=True)
 tenant_id:Mapped[str]=mapped_column(ForeignKey('tenants.id'),primary_key=True)
 created:Mapped[int]=mapped_column(BigInteger)
class RouterPlan(Base):
 __tablename__='router_plans'
 id:Mapped[str]=mapped_column(String(36),primary_key=True,default=uid)
 tenant_id:Mapped[str]=mapped_column(ForeignKey('tenants.id'),index=True)
 router_id:Mapped[str]=mapped_column(ForeignKey('router_devices.id'),unique=True)
 hub_id:Mapped[str]=mapped_column(ForeignKey('magic_hubs.id'))
 router_public_key:Mapped[str]=mapped_column(String(44),unique=True)
 address:Mapped[str]=mapped_column(String(15))
 snapshot:Mapped[dict]=mapped_column(JSON)
 created:Mapped[int]=mapped_column(BigInteger)
 reviewed_at:Mapped[Optional[int]]=mapped_column(BigInteger,nullable=True)
 __table_args__=(UniqueConstraint('hub_id','address'),)

class FinancialEvent(Base):
 __tablename__='financial_events'
 id:Mapped[str]=mapped_column(String(80),primary_key=True,default=uid)
 tenant_id:Mapped[str]=mapped_column(ForeignKey('tenants.id'),index=True)
 site_id:Mapped[Optional[str]]=mapped_column(ForeignKey('sites.id'),nullable=True)
 source:Mapped[str]=mapped_column(String(20))
 source_id:Mapped[str]=mapped_column(String(36))
 kind:Mapped[str]=mapped_column(String(16))
 amount:Mapped[int]=mapped_column(BigInteger)
 currency:Mapped[str]=mapped_column(String(3))
 service:Mapped[str]=mapped_column(String(20))
 package_name:Mapped[str]=mapped_column(String(120))
 method:Mapped[str]=mapped_column(String(80))
 reference:Mapped[str]=mapped_column(String(160))
 actor_id:Mapped[Optional[str]]=mapped_column(ForeignKey('users.id'),nullable=True)
 occurred_at:Mapped[int]=mapped_column(BigInteger,index=True)
 timestamp_basis:Mapped[str]=mapped_column(String(32))
 __table_args__=(UniqueConstraint('source','source_id','kind'),CheckConstraint('amount > 0'),CheckConstraint("kind IN ('collection','refund')"),CheckConstraint("source IN ('guest_order','voucher_sale')"))

class ThemeSettings(Base):
 __tablename__='theme_settings'
 id:Mapped[str]=mapped_column(String(32),primary_key=True)
 revision:Mapped[int]=mapped_column(Integer)
 draft:Mapped[dict]=mapped_column(JSON)
 published:Mapped[Optional[dict]]=mapped_column(JSON,nullable=True)
 previous:Mapped[Optional[dict]]=mapped_column(JSON,nullable=True)
 published_revision:Mapped[Optional[int]]=mapped_column(Integer,nullable=True)
 updated:Mapped[int]=mapped_column(BigInteger)

class MediaTitle(Base):
 __tablename__='media_titles'
 id:Mapped[str]=mapped_column(String(36),primary_key=True,default=uid)
 tenant_id:Mapped[str]=mapped_column(ForeignKey('tenants.id'),index=True)
 name:Mapped[str]=mapped_column(String(120))
 description:Mapped[str]=mapped_column(String(4000))
 genre:Mapped[str]=mapped_column(String(80))
 runtime_minutes:Mapped[int]=mapped_column(Integer)
 release_year:Mapped[Optional[int]]=mapped_column(Integer,nullable=True)
 published:Mapped[bool]=mapped_column(Boolean,default=False)
 archived:Mapped[bool]=mapped_column(Boolean,default=False)
 revision:Mapped[int]=mapped_column(Integer,default=1)
 updated:Mapped[int]=mapped_column(BigInteger)
 __table_args__=(CheckConstraint('runtime_minutes > 0'),CheckConstraint('revision > 0'))

class KnowledgeCard(Base):
 __tablename__='knowledge_cards'
 id:Mapped[str]=mapped_column(String(36),primary_key=True,default=uid)
 content:Mapped[dict]=mapped_column(JSON)
 published:Mapped[bool]=mapped_column(Boolean,default=False)
 archived:Mapped[bool]=mapped_column(Boolean,default=False)
 revision:Mapped[int]=mapped_column(Integer,default=1)
 updated:Mapped[int]=mapped_column(BigInteger)
 __table_args__=(CheckConstraint('revision > 0'),)

class ReceiptImage(Base):
 __tablename__='receipt_images'
 id:Mapped[str]=mapped_column(String(36),primary_key=True,default=uid)
 order_id:Mapped[str]=mapped_column(ForeignKey('orders.id'),index=True)
 uploaded_by:Mapped[str]=mapped_column(ForeignKey('users.id'))
 request_key:Mapped[str]=mapped_column(String(80))
 original_hash:Mapped[str]=mapped_column(String(64))
 image:Mapped[bytes]=mapped_column(LargeBinary)
 width:Mapped[int]=mapped_column(Integer)
 height:Mapped[int]=mapped_column(Integer)
 created:Mapped[int]=mapped_column(BigInteger)
 __table_args__=(UniqueConstraint('order_id','request_key'),CheckConstraint('width > 0 AND height > 0'))

class ReviewCredential(Base):
 __tablename__='review_credentials'
 id:Mapped[str]=mapped_column(String(36),primary_key=True,default=uid)
 tenant_id:Mapped[str]=mapped_column(ForeignKey('tenants.id'),index=True)
 user_id:Mapped[str]=mapped_column(ForeignKey('users.id'))
 label:Mapped[str]=mapped_column(String(120))
 provider:Mapped[str]=mapped_column(String(16))
 reviewer_identity:Mapped[str]=mapped_column(String(160))
 receive_events:Mapped[bool]=mapped_column(Boolean,default=False,server_default='0')
 can_decide:Mapped[bool]=mapped_column(Boolean)
 token_hash:Mapped[str]=mapped_column(String(64),unique=True)
 created:Mapped[int]=mapped_column(BigInteger)
 expires:Mapped[int]=mapped_column(BigInteger)
 revoked_at:Mapped[Optional[int]]=mapped_column(BigInteger,nullable=True)
 revoke_reason:Mapped[Optional[str]]=mapped_column(String(1000),nullable=True)
 __table_args__=(CheckConstraint('expires > created'),)
class ExternalReviewAction(Base):
 __tablename__='external_review_actions'
 id:Mapped[str]=mapped_column(String(36),primary_key=True,default=uid)
 tenant_id:Mapped[str]=mapped_column(ForeignKey('tenants.id'),index=True)
 credential_id:Mapped[str]=mapped_column(ForeignKey('review_credentials.id'))
 order_id:Mapped[str]=mapped_column(ForeignKey('orders.id'))
 request_key:Mapped[str]=mapped_column(String(80))
 request_hash:Mapped[str]=mapped_column(String(64))
 reviewer_identity:Mapped[str]=mapped_column(String(160))
 provider:Mapped[str]=mapped_column(String(16))
 service_label:Mapped[str]=mapped_column(String(120))
 decision:Mapped[str]=mapped_column(String(24))
 reason:Mapped[str]=mapped_column(String(1000))
 result:Mapped[dict]=mapped_column(JSON)
 created:Mapped[int]=mapped_column(BigInteger)
 __table_args__=(UniqueConstraint('credential_id','request_key'),)

class EventDelivery(Base):
 __tablename__='event_deliveries'
 id:Mapped[str]=mapped_column(String(36),primary_key=True,default=uid)
 tenant_id:Mapped[str]=mapped_column(ForeignKey('tenants.id'),index=True)
 credential_id:Mapped[str]=mapped_column(ForeignKey('review_credentials.id'),index=True)
 event_id:Mapped[str]=mapped_column(ForeignKey('outbox_events.id'))
 state:Mapped[str]=mapped_column(String(16),default='pending')
 attempts:Mapped[int]=mapped_column(Integer,default=0)
 cycle_attempts:Mapped[int]=mapped_column(Integer,default=0)
 available_at:Mapped[int]=mapped_column(BigInteger)
 lease_expires:Mapped[Optional[int]]=mapped_column(BigInteger,nullable=True)
 lease_hash:Mapped[Optional[str]]=mapped_column(String(64),nullable=True)
 completion:Mapped[Optional[dict]]=mapped_column(JSON,nullable=True)
 acknowledged_at:Mapped[Optional[int]]=mapped_column(BigInteger,nullable=True)
 last_error:Mapped[Optional[str]]=mapped_column(String(1000),nullable=True)
 __table_args__=(UniqueConstraint('credential_id','event_id'),CheckConstraint("state IN ('pending','leased','acknowledged','failed')"),CheckConstraint('attempts >= 0 AND cycle_attempts >= 0'))

class BusinessProvisioning(Base):
 __tablename__='business_provisioning'
 id:Mapped[str]=mapped_column(String(36),primary_key=True,default=uid)
 tenant_id:Mapped[str]=mapped_column(ForeignKey('tenants.id'),unique=True)
 owner_id:Mapped[str]=mapped_column(ForeignKey('users.id'),unique=True)
 created_by:Mapped[str]=mapped_column(ForeignKey('users.id'))
 request_key:Mapped[str]=mapped_column(String(80))
 request_hash:Mapped[str]=mapped_column(String(64))
 created:Mapped[int]=mapped_column(BigInteger)
 __table_args__=(UniqueConstraint('created_by','request_key'),)

class PasswordResetRecord(Base):
 __tablename__='password_reset_records'
 id:Mapped[str]=mapped_column(String(36),primary_key=True,default=uid)
 user_id:Mapped[str]=mapped_column(ForeignKey('users.id'))
 actor_id:Mapped[str]=mapped_column(ForeignKey('users.id'))
 reason:Mapped[str]=mapped_column(String(1000))
 created:Mapped[int]=mapped_column(BigInteger)

class SitePlanning(Base):
 __tablename__='site_planning'
 site_id:Mapped[str]=mapped_column(ForeignKey('sites.id'),primary_key=True)
 revision:Mapped[int]=mapped_column(Integer)
 settings:Mapped[dict]=mapped_column(JSON)
 updated:Mapped[int]=mapped_column(BigInteger)

class SubscriptionPlan(Base):
 __tablename__='subscription_plans'
 id:Mapped[str]=mapped_column(String(36),primary_key=True,default=uid)
 settings:Mapped[dict]=mapped_column(JSON)
 created:Mapped[int]=mapped_column(BigInteger)

class StaffGrant(Base):
 __tablename__='staff_grants'
 user_id:Mapped[str]=mapped_column(ForeignKey('users.id'),primary_key=True)
 tenant_id:Mapped[str]=mapped_column(ForeignKey('tenants.id'),index=True)
 profile:Mapped[str]=mapped_column(String(20))
 revision:Mapped[int]=mapped_column(Integer)
 last_reason:Mapped[str]=mapped_column(String(1000),default='')
 created_by:Mapped[str]=mapped_column(ForeignKey('users.id'))
 request_key:Mapped[str]=mapped_column(String(80))
 request_hash:Mapped[str]=mapped_column(String(64))
 created:Mapped[int]=mapped_column(BigInteger)
 updated:Mapped[int]=mapped_column(BigInteger)
 __table_args__=(UniqueConstraint('created_by','request_key'),CheckConstraint("profile in ('manager','cashier','technician')"))
class TenantSubscription(Base):
 __tablename__='tenant_subscriptions'
 tenant_id:Mapped[str]=mapped_column(ForeignKey('tenants.id'),primary_key=True)
 revision:Mapped[int]=mapped_column(Integer)
 plan_id:Mapped[str]=mapped_column(ForeignKey('subscription_plans.id'))
 plan_snapshot:Mapped[dict]=mapped_column(JSON)
 state:Mapped[str]=mapped_column(String(20))
 period_start:Mapped[int]=mapped_column(BigInteger)
 period_end:Mapped[int]=mapped_column(BigInteger)
 reason:Mapped[str]=mapped_column(String(1000))
 updated:Mapped[int]=mapped_column(BigInteger)
 payment_source_invoice_id:Mapped[Optional[str]]=mapped_column(String(36),nullable=True)
 __table_args__=(CheckConstraint("state in ('trial','active','past_due','suspended','cancelled')"),CheckConstraint('period_end > period_start'))

class SubscriptionInvoice(Base):
 __tablename__='subscription_invoices'
 id:Mapped[str]=mapped_column(String(36),primary_key=True,default=uid)
 tenant_id:Mapped[str]=mapped_column(ForeignKey('tenants.id'),index=True)
 plan_id:Mapped[str]=mapped_column(ForeignKey('subscription_plans.id'))
 snapshot:Mapped[dict]=mapped_column(JSON)
 amount:Mapped[int]=mapped_column(BigInteger)
 currency:Mapped[str]=mapped_column(String(3))
 period_start:Mapped[int]=mapped_column(BigInteger)
 period_end:Mapped[int]=mapped_column(BigInteger)
 subscription_revision:Mapped[int]=mapped_column(Integer)
 revision:Mapped[int]=mapped_column(Integer,default=1)
 status:Mapped[str]=mapped_column(String(20),default='unpaid')
 destination:Mapped[str]=mapped_column(String(500))
 instructions:Mapped[str]=mapped_column(String(2000))
 method:Mapped[Optional[str]]=mapped_column(String(80),nullable=True)
 reference:Mapped[Optional[str]]=mapped_column(String(160),nullable=True)
 evidence:Mapped[Optional[str]]=mapped_column(String(4000),nullable=True)
 submission_key:Mapped[Optional[str]]=mapped_column(String(80),nullable=True)
 submission_hash:Mapped[Optional[str]]=mapped_column(String(64),nullable=True)
 reviewed_by:Mapped[Optional[str]]=mapped_column(ForeignKey('users.id'),nullable=True)
 reason:Mapped[Optional[str]]=mapped_column(String(1000),nullable=True)
 created_by:Mapped[str]=mapped_column(ForeignKey('users.id'))
 request_key:Mapped[str]=mapped_column(String(80))
 request_hash:Mapped[str]=mapped_column(String(64))
 created:Mapped[int]=mapped_column(BigInteger)
 updated:Mapped[int]=mapped_column(BigInteger)
 __table_args__=(UniqueConstraint('created_by','request_key'),CheckConstraint('amount > 0'),CheckConstraint('period_end > period_start'),CheckConstraint("status in ('unpaid','pending','needs_evidence','paid','rejected','void')"))
class BillingEvent(Base):
 __tablename__='billing_events'
 id:Mapped[str]=mapped_column(String(36),primary_key=True,default=uid)
 invoice_id:Mapped[str]=mapped_column(ForeignKey('subscription_invoices.id'),index=True)
 actor_id:Mapped[str]=mapped_column(ForeignKey('users.id'))
 event:Mapped[str]=mapped_column(String(40))
 payload:Mapped[dict]=mapped_column(JSON)
 created:Mapped[int]=mapped_column(BigInteger)

class AccountMFA(Base):
 __tablename__='account_mfa'
 user_id:Mapped[str]=mapped_column(ForeignKey('users.id'),primary_key=True)
 enabled:Mapped[bool]=mapped_column(Boolean,default=False)
 encrypted_secret:Mapped[Optional[str]]=mapped_column(String(1000),nullable=True)
 enrollment_expires:Mapped[Optional[int]]=mapped_column(BigInteger,nullable=True)
 last_step:Mapped[int]=mapped_column(BigInteger,default=-1)
 recovery_hashes:Mapped[list]=mapped_column(JSON,default=list)
 updated:Mapped[int]=mapped_column(BigInteger)
