-- mm:baseline-version 20261102000000
-- Mega Magic database baseline, taken from the production schema on 2026-11-02 (pg_dump --schema-only,
-- no owners, no privileges). It is the starting point of the migration chain: a new database is built by applying
-- this file and then every file in db/migrations in order (see docs/MIGRATIONS.md); an existing database is marked as
-- being at this baseline with `mm-migrate baseline`, never re-run.
--
-- Left out on purpose: the runtime role's policies (mm_runtime_service) and grants. They are created by
-- deploy/runtime-db-role.sql, which the migration tool re-runs after every migration.
-- The role `authenticated` must exist before the policies below can be created.
DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'authenticated') THEN CREATE ROLE authenticated NOLOGIN; END IF; END $$;

--
-- PostgreSQL database dump
--


-- Dumped from database version 16.15
-- Dumped by pg_dump version 16.15

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: pgcrypto; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;


--
-- Name: EXTENSION pgcrypto; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON EXTENSION pgcrypto IS 'cryptographic functions';


--
-- Name: uuid-ossp; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;


--
-- Name: EXTENSION "uuid-ossp"; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON EXTENSION "uuid-ossp" IS 'generate universally unique identifiers (UUIDs)';


--
-- Name: assign_new_business_primary_login(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.assign_new_business_primary_login() RETURNS trigger
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'pg_catalog', 'public'
    AS $$
BEGIN
 IF NEW.user_role='OWNER' AND NOT EXISTS(SELECT 1 FROM public.users WHERE business_id=NEW.business_id AND is_active AND user_role='DEVELOPER') AND EXISTS(SELECT 1 FROM public.businesses b WHERE b.business_id=NEW.business_id AND b.owner_email=NEW.email) THEN
  INSERT INTO public.business_app_primary_users(business_id,user_id) VALUES(NEW.business_id,NEW.user_id) ON CONFLICT(business_id) DO NOTHING;
 END IF;
 RETURN NEW;
END $$;


--
-- Name: create_magic_coin_wallet_for_user(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.create_magic_coin_wallet_for_user() RETURNS trigger
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public'
    AS $$
BEGIN
    INSERT INTO magic_coin_wallets (user_id, business_id)
    VALUES (NEW.user_id, NEW.business_id)
    ON CONFLICT (user_id) DO NOTHING;
    RETURN NEW;
END;
$$;


--
-- Name: enforce_business_app_resource_cap(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.enforce_business_app_resource_cap() RETURNS trigger
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'pg_catalog', 'public'
    AS $$
DECLARE used integer;
BEGIN
 IF NOT EXISTS(SELECT 1 FROM public.app_account_pass_rollouts WHERE rollout_key='individual-v1' AND enforcement_enabled AND starts_at<=now()) THEN RETURN NEW; END IF;
 IF EXISTS(SELECT 1 FROM public.users WHERE business_id=NEW.business_id AND is_active AND user_role='DEVELOPER') THEN RETURN NEW; END IF;
 PERFORM pg_advisory_xact_lock(hashtextextended('business-app-resources:'||NEW.business_id::text,0));
 IF TG_TABLE_NAME='sites' THEN
  IF TG_OP='UPDATE' AND NEW.business_id=OLD.business_id THEN RETURN NEW; END IF;
  SELECT count(*) INTO used FROM public.sites WHERE business_id=NEW.business_id AND site_id<>NEW.site_id;
  IF used>=1 THEN RAISE EXCEPTION 'One site per business is included' USING ERRCODE='23514',CONSTRAINT='app_business_site_limit'; END IF;
 ELSE
  IF NEW.deployment_layer NOT IN ('MAIN_GATEWAY','DOWNSTREAM_SWITCH') THEN RETURN NEW; END IF;
  IF TG_OP='UPDATE' AND NEW.business_id=OLD.business_id AND OLD.deployment_layer IN ('MAIN_GATEWAY','DOWNSTREAM_SWITCH') THEN RETURN NEW; END IF;
  SELECT count(*) INTO used FROM public.devices WHERE business_id=NEW.business_id AND deployment_layer IN ('MAIN_GATEWAY','DOWNSTREAM_SWITCH') AND device_id<>NEW.device_id;
  IF used>=1 THEN RAISE EXCEPTION 'One router per business is included' USING ERRCODE='23514',CONSTRAINT='app_business_router_limit'; END IF;
 END IF;
 RETURN NEW;
END $$;


--
-- Name: guard_guest_access_recovery(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.guard_guest_access_recovery() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF (NEW.voucher_id,NEW.business_id,NEW.device_id,NEW.secret_hash)
    IS DISTINCT FROM (OLD.voucher_id,OLD.business_id,OLD.device_id,OLD.secret_hash) THEN
  RAISE EXCEPTION 'recovery ownership and credential are immutable';
 END IF;
 IF NEW.used_bytes<OLD.used_bytes THEN
  RAISE EXCEPTION 'recovery cannot replenish consumed data';
 END IF;
 IF NEW.current_mac IS DISTINCT FROM OLD.current_mac AND NOT
    (OLD.state='frozen' AND NEW.state='active' AND NEW.current_mac=OLD.pending_mac) THEN
  RAISE EXCEPTION 'connection binding needs a verified frozen transfer';
 END IF;
 IF NEW.completed_transfer_id IS DISTINCT FROM OLD.completed_transfer_id AND
    (OLD.state='frozen' AND NEW.state='active' AND NEW.completed_transfer_id=OLD.transfer_id
     AND NEW.current_mac=OLD.pending_mac) IS NOT TRUE THEN
  RAISE EXCEPTION 'completion receipt needs a verified frozen transfer';
 END IF;
 IF OLD.state='blocked' AND NEW.state<>'blocked' THEN
  RAISE EXCEPTION 'blocked recovery needs a separate audited repair workflow';
 END IF;
 RETURN NEW;
END $$;


--
-- Name: guard_guest_voucher_activation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.guard_guest_voucher_activation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF OLD.activated_at IS NOT NULL AND NEW.activated_at IS DISTINCT FROM OLD.activated_at THEN
  RAISE EXCEPTION 'guest voucher activation time is immutable';
 END IF;
 IF OLD.activated_at IS NULL AND NEW.activated_at IS NOT NULL AND
    (OLD.state<>'provisioning' OR NEW.state<>'active' OR NEW.first_used_at IS NULL
     OR NEW.activated_at<NEW.first_used_at OR NEW.activated_at>=NEW.expires_at) THEN
  RAISE EXCEPTION 'guest voucher activation requires a live provisioning claim';
 END IF;
 RETURN NEW;
END $$;


--
-- Name: guard_guest_voucher_issue_batch(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.guard_guest_voucher_issue_batch() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF (NEW.business_id,NEW.id,NEW.device_id,NEW.request_hash,NEW.plan_label,NEW.price_mmk,NEW.minutes,NEW.data_mb,NEW.total_count,NEW.created_at)
 IS DISTINCT FROM (OLD.business_id,OLD.id,OLD.device_id,OLD.request_hash,OLD.plan_label,OLD.price_mmk,OLD.minutes,OLD.data_mb,OLD.total_count,OLD.created_at) THEN
  RAISE EXCEPTION 'guest voucher issue batch is immutable';
 END IF;
 RETURN NEW;
END $$;


--
-- Name: guard_guest_voucher_issue_item(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.guard_guest_voucher_issue_item() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF (NEW.business_id,NEW.batch_id,NEW.voucher_id,NEW.position)
 IS DISTINCT FROM (OLD.business_id,OLD.batch_id,OLD.voucher_id,OLD.position) THEN
  RAISE EXCEPTION 'guest voucher issue membership is immutable';
 END IF;
 IF OLD.install_state='verified' AND (NEW.install_state,NEW.verified_at) IS DISTINCT FROM (OLD.install_state,OLD.verified_at) THEN
  RAISE EXCEPTION 'verified guest voucher installation cannot be reset';
 END IF;
 RETURN NEW;
END $$;


--
-- Name: guard_guest_voucher_snapshot(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.guard_guest_voucher_snapshot() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF (NEW.voucher_id,NEW.business_id,NEW.device_id,NEW.minutes,NEW.data_mb)
    IS DISTINCT FROM (OLD.voucher_id,OLD.business_id,OLD.device_id,OLD.minutes,OLD.data_mb) THEN
  RAISE EXCEPTION 'guest voucher allowance snapshot is immutable';
 END IF;
 IF OLD.first_used_at IS NOT NULL AND
    (NEW.client_mac,NEW.first_used_at,NEW.expires_at)
    IS DISTINCT FROM (OLD.client_mac,OLD.first_used_at,OLD.expires_at) THEN
  RAISE EXCEPTION 'guest voucher binding and deadline are immutable';
 END IF;
 IF OLD.state IN ('expired','revoked') AND NEW.state<>OLD.state THEN
  RAISE EXCEPTION 'terminal guest voucher cannot be reactivated';
 END IF;
 RETURN NEW;
END $$;


--
-- Name: guard_hub_management_address_move(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.guard_hub_management_address_move() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF OLD.transport='tunnel' AND OLD.management_ip IS NOT NULL AND OLD.ssh_secret_ciphertext IS NOT NULL AND NEW.transport IS DISTINCT FROM OLD.transport THEN
  RAISE EXCEPTION 'disconnect the verified Hub connection before changing transport';
 END IF;
 IF OLD.transport='tunnel' AND OLD.management_ip IS NOT NULL AND OLD.ssh_secret_ciphertext IS NOT NULL AND NEW.management_ip IS DISTINCT FROM OLD.management_ip THEN
  IF NOT EXISTS (SELECT 1 FROM management_address_migrations m WHERE m.id::text=current_setting('app.management_migration_id',true) AND m.device_id=OLD.device_id AND m.business_id=OLD.business_id AND m.phase='router_retained' AND m.old_address=OLD.management_ip AND m.new_address=NEW.management_ip AND m.fingerprint=OLD.ssh_host_key_fingerprint) THEN
   RAISE EXCEPTION 'use verified management address migration';
  END IF;
 END IF;
 RETURN NEW;
END $$;


--
-- Name: guard_pending_management_device_delete(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.guard_pending_management_device_delete() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM management_address_migrations WHERE device_id=OLD.device_id AND business_id=OLD.business_id AND phase NOT IN ('completed','rolled_back')) THEN
  RAISE EXCEPTION USING ERRCODE='55000',MESSAGE='management_migration_pending';
 END IF;
 RETURN OLD;
END $$;


--
-- Name: magic_coin_transaction_immutable(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.magic_coin_transaction_immutable() RETURNS trigger
    LANGUAGE plpgsql
    SET search_path TO 'public'
    AS $$
BEGIN
    RAISE EXCEPTION 'MAGIC_COIN_TRANSACTION_IMMUTABLE';
END;
$$;


--
-- Name: revoke_changed_user_sessions(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.revoke_changed_user_sessions() RETURNS trigger
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'pg_catalog'
    AS $$
BEGIN
 IF OLD.password_hash IS DISTINCT FROM NEW.password_hash
 OR OLD.user_role IS DISTINCT FROM NEW.user_role
 OR OLD.business_id IS DISTINCT FROM NEW.business_id
 OR (OLD.is_active AND NOT NEW.is_active) THEN
 UPDATE public.auth_sessions SET revoked=true WHERE user_id=NEW.user_id;
 END IF;
 RETURN NEW;
END;
$$;


--
-- Name: revoke_suspended_business_sessions(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.revoke_suspended_business_sessions() RETURNS trigger
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'pg_catalog'
    AS $$
BEGIN
 IF OLD.is_active AND NOT NEW.is_active THEN
 UPDATE public.auth_sessions SET revoked=true WHERE business_id=NEW.business_id;
 END IF;
 RETURN NEW;
END;
$$;


--
-- Name: update_updated_at_column(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.update_updated_at_column() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$;


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: account_entitlements; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.account_entitlements (
    user_id uuid NOT NULL,
    business_id uuid NOT NULL,
    tier text NOT NULL,
    tier_expires_at timestamp with time zone,
    plus boolean DEFAULT false NOT NULL,
    plus_since timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT account_entitlements_tier_check CHECK ((tier = ANY (ARRAY['spark'::text, 'trial'::text, 'monthly'::text, 'annual'::text])))
);


--
-- Name: admin_notifications; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.admin_notifications (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    recipient_user_id uuid NOT NULL,
    kind text NOT NULL,
    title text NOT NULL,
    body text,
    data jsonb DEFAULT '{}'::jsonb NOT NULL,
    read_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: ai_usage_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ai_usage_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    business_id uuid NOT NULL,
    user_id uuid,
    feature text NOT NULL,
    model text,
    prompt_tokens integer DEFAULT 0 NOT NULL,
    completion_tokens integer DEFAULT 0 NOT NULL,
    total_tokens integer DEFAULT 0 NOT NULL,
    cached boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: alert_policies; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.alert_policies (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    business_id uuid NOT NULL,
    kind text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    severity text DEFAULT 'warning'::text NOT NULL,
    threshold numeric(10,2),
    cooldown_minutes integer DEFAULT 30 NOT NULL,
    notification_channels text[] DEFAULT '{}'::text[],
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT alert_policies_kind_check CHECK ((kind = ANY (ARRAY['site_offline'::text, 'wan_degraded'::text, 'connector_stale'::text, 'poe_ap_offline'::text, 'provisioning_failed'::text, 'cpu_high'::text, 'memory_high'::text, 'handshake_stale'::text]))),
    CONSTRAINT alert_policies_severity_check CHECK ((severity = ANY (ARRAY['info'::text, 'warning'::text, 'critical'::text])))
);


--
-- Name: app_account_pass_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.app_account_pass_events (
    id bigint NOT NULL,
    user_id uuid NOT NULL,
    business_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    revision bigint NOT NULL,
    details jsonb NOT NULL,
    recorded_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.app_account_pass_events FORCE ROW LEVEL SECURITY;


--
-- Name: app_account_pass_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.app_account_pass_events ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.app_account_pass_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: app_account_pass_renewals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.app_account_pass_renewals (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    user_id uuid NOT NULL,
    business_id uuid NOT NULL,
    plan text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    decided_at timestamp with time zone,
    decided_by uuid,
    pass_revision bigint,
    amount_mmk integer,
    bank_account_id uuid,
    bank_reference text,
    bank_snapshot jsonb,
    idempotency_key uuid,
    CONSTRAINT account_pass_payment_evidence CHECK ((((amount_mmk IS NULL) AND (bank_account_id IS NULL) AND (bank_reference IS NULL) AND (bank_snapshot IS NULL) AND (idempotency_key IS NULL)) OR ((amount_mmk IS NOT NULL) AND ((amount_mmk >= 1) AND (amount_mmk <= 1000000000)) AND (bank_reference IS NOT NULL) AND ((length(btrim(bank_reference)) >= 1) AND (length(btrim(bank_reference)) <= 120)) AND (bank_snapshot IS NOT NULL) AND (jsonb_typeof(bank_snapshot) = 'object'::text) AND (idempotency_key IS NOT NULL) AND (bank_account_id IS NOT NULL)))),
    CONSTRAINT app_account_pass_renewals_plan_check CHECK ((plan = ANY (ARRAY['monthly'::text, 'annual'::text]))),
    CONSTRAINT app_account_pass_renewals_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'approved'::text, 'rejected'::text])))
);

ALTER TABLE ONLY public.app_account_pass_renewals FORCE ROW LEVEL SECURITY;


--
-- Name: app_account_pass_rollouts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.app_account_pass_rollouts (
    rollout_key text NOT NULL,
    starts_at timestamp with time zone NOT NULL,
    transition_days integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    enforcement_enabled boolean DEFAULT false NOT NULL,
    CONSTRAINT app_account_pass_rollouts_rollout_key_check CHECK ((rollout_key = 'individual-v1'::text)),
    CONSTRAINT app_account_pass_rollouts_transition_days_check CHECK ((transition_days = 7))
);


--
-- Name: app_account_pass_transitions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.app_account_pass_transitions (
    user_id uuid NOT NULL,
    business_id uuid NOT NULL,
    rollout_key text NOT NULL,
    expires_at timestamp with time zone NOT NULL
);

ALTER TABLE ONLY public.app_account_pass_transitions FORCE ROW LEVEL SECURITY;


--
-- Name: app_account_passes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.app_account_passes (
    user_id uuid NOT NULL,
    business_id uuid NOT NULL,
    plan text NOT NULL,
    activated_at timestamp with time zone NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    revision bigint DEFAULT 1 NOT NULL,
    recorded_by uuid NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT app_account_passes_check CHECK ((expires_at > activated_at)),
    CONSTRAINT app_account_passes_plan_check CHECK ((plan = ANY (ARRAY['monthly'::text, 'annual'::text]))),
    CONSTRAINT app_account_passes_revision_check CHECK ((revision > 0))
);

ALTER TABLE ONLY public.app_account_passes FORCE ROW LEVEL SECURITY;


--
-- Name: app_account_trials; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.app_account_trials (
    user_id uuid NOT NULL,
    business_id uuid NOT NULL,
    started_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    CONSTRAINT app_account_trial_duration CHECK ((expires_at = (started_at + '7 days'::interval)))
);

ALTER TABLE ONLY public.app_account_trials FORCE ROW LEVEL SECURITY;


--
-- Name: audit_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.audit_events (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    business_id uuid NOT NULL,
    user_id uuid,
    user_role text NOT NULL,
    action text NOT NULL,
    entity_type text NOT NULL,
    entity_id text NOT NULL,
    before_state jsonb,
    after_state jsonb,
    reason text NOT NULL,
    ip_address inet,
    user_agent text,
    "timestamp" timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: auth_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.auth_sessions (
    session_id text NOT NULL,
    user_id uuid NOT NULL,
    business_id uuid NOT NULL,
    password_digest text NOT NULL,
    user_role text NOT NULL,
    refresh_digest text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked boolean DEFAULT false NOT NULL,
    last_seen_at timestamp with time zone,
    last_active_at timestamp with time zone,
    last_visible_at timestamp with time zone
);


--
-- Name: business_app_primary_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.business_app_primary_events (
    id bigint NOT NULL,
    business_id uuid NOT NULL,
    user_id uuid NOT NULL,
    previous_user_id uuid,
    changed_by uuid NOT NULL,
    reason text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT business_app_primary_events_reason_check CHECK (((length(reason) >= 5) AND (length(reason) <= 500)))
);

ALTER TABLE ONLY public.business_app_primary_events FORCE ROW LEVEL SECURITY;


--
-- Name: business_app_primary_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.business_app_primary_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: business_app_primary_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.business_app_primary_events_id_seq OWNED BY public.business_app_primary_events.id;


--
-- Name: business_app_primary_users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.business_app_primary_users (
    business_id uuid NOT NULL,
    user_id uuid NOT NULL,
    updated_by uuid,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.business_app_primary_users FORCE ROW LEVEL SECURITY;


--
-- Name: business_invitations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.business_invitations (
    invitation_id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    business_id uuid NOT NULL,
    email text NOT NULL,
    full_name text NOT NULL,
    role text NOT NULL,
    token_hash text NOT NULL,
    invited_by uuid NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    accepted_at timestamp with time zone,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT business_invitations_check CHECK (((accepted_at IS NULL) OR (revoked_at IS NULL))),
    CONSTRAINT business_invitations_email_check CHECK ((length(btrim(email)) > 0)),
    CONSTRAINT business_invitations_full_name_check CHECK ((length(btrim(full_name)) > 0)),
    CONSTRAINT business_invitations_role_check CHECK ((role = ANY (ARRAY['ADMIN'::text, 'VIEWER'::text])))
);


--
-- Name: business_memberships; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.business_memberships (
    membership_id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    business_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role text NOT NULL,
    status text DEFAULT 'active'::text NOT NULL,
    invited_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT business_memberships_role_check CHECK ((role = ANY (ARRAY['OWNER'::text, 'ADMIN'::text, 'VIEWER'::text]))),
    CONSTRAINT business_memberships_status_check CHECK ((status = ANY (ARRAY['active'::text, 'suspended'::text, 'revoked'::text])))
);


--
-- Name: business_metered_quotas; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.business_metered_quotas (
    business_id uuid NOT NULL,
    purchased_extra_slots integer DEFAULT 0 NOT NULL,
    webfig_pass_expires_at timestamp with time zone,
    ai_token_balance bigint DEFAULT 0 NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: business_telegram_bots; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.business_telegram_bots (
    business_id uuid NOT NULL,
    bot_token text,
    target_chat_id bigint,
    approver_user_id bigint DEFAULT 0 NOT NULL,
    enabled boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT business_telegram_bots_approver_user_id_check CHECK ((approver_user_id >= 0))
);


--
-- Name: businesses; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.businesses (
    business_id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    company_name text NOT NULL,
    owner_email text NOT NULL,
    access_tier text DEFAULT 'SMALL_WISP'::text NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    device_quota_limit integer DEFAULT 10 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT businesses_access_tier_check CHECK ((access_tier = ANY (ARRAY['SMALL_WISP'::text, 'ENTERPRISE_ISP'::text, 'GLOBAL_ADMIN'::text])))
);


--
-- Name: device_allowances; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.device_allowances (
    business_id uuid NOT NULL,
    routers integer DEFAULT 1 NOT NULL,
    controllers integer DEFAULT 0 NOT NULL,
    sites integer DEFAULT 0 NOT NULL,
    included_router_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT device_allowances_controllers_check CHECK ((controllers >= 0)),
    CONSTRAINT device_allowances_routers_check CHECK ((routers >= 0)),
    CONSTRAINT device_allowances_sites_check CHECK ((sites >= 0))
);


--
-- Name: device_change_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.device_change_events (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    business_id uuid NOT NULL,
    device_id uuid NOT NULL,
    user_role text DEFAULT ''::text NOT NULL,
    action text NOT NULL,
    reason text DEFAULT ''::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: device_config_backups; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.device_config_backups (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    business_id uuid NOT NULL,
    device_id uuid NOT NULL,
    label text DEFAULT ''::text NOT NULL,
    kind text DEFAULT 'snapshot'::text NOT NULL,
    content text DEFAULT ''::text NOT NULL,
    size_bytes integer DEFAULT 0 NOT NULL,
    created_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT device_config_backups_kind_check CHECK ((kind = ANY (ARRAY['export'::text, 'snapshot'::text])))
);


--
-- Name: device_discovery_labels; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.device_discovery_labels (
    business_id uuid NOT NULL,
    device_id uuid NOT NULL,
    mac_address macaddr NOT NULL,
    name text NOT NULL,
    kind text NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT device_discovery_labels_kind_check CHECK ((kind = ANY (ARRAY['switch'::text, 'ap'::text, 'computer'::text, 'printer'::text, 'controller'::text, 'other'::text])))
);

ALTER TABLE ONLY public.device_discovery_labels FORCE ROW LEVEL SECURITY;


--
-- Name: device_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.device_requests (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    business_id uuid NOT NULL,
    user_id uuid,
    kind text NOT NULL,
    quantity integer DEFAULT 1 NOT NULL,
    reason text,
    status text DEFAULT 'pending'::text NOT NULL,
    decided_by uuid,
    decided_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT device_requests_quantity_check CHECK (((quantity >= 1) AND (quantity <= 100)))
);


--
-- Name: devices; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.devices (
    device_id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    business_id uuid NOT NULL,
    parent_gateway_id uuid,
    hardware_name text NOT NULL,
    device_model text,
    deployment_layer text DEFAULT 'MAIN_GATEWAY'::text NOT NULL,
    vendor text DEFAULT 'mikrotik'::text NOT NULL,
    category text DEFAULT 'router'::text NOT NULL,
    transport text DEFAULT 'direct'::text NOT NULL,
    mac_address macaddr,
    management_ip inet,
    is_online boolean DEFAULT false NOT NULL,
    last_seen_at timestamp with time zone,
    routeros_version text,
    routeros_channel text,
    version_checked_at timestamp with time zone,
    security_posture text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    site_id uuid,
    router_connection_id uuid,
    ssh_port integer DEFAULT 22 NOT NULL,
    ssh_username text,
    ssh_secret_ciphertext text,
    ssh_secret_kind text,
    ssh_host_key_fingerprint text,
    CONSTRAINT devices_category_check CHECK ((category = ANY (ARRAY['router'::text, 'gateway'::text, 'switch'::text, 'ap'::text, 'controller'::text]))),
    CONSTRAINT devices_deployment_layer_check CHECK ((deployment_layer = ANY (ARRAY['MAIN_GATEWAY'::text, 'DOWNSTREAM_SWITCH'::text, 'ACCESS_POINT'::text, 'CONTROLLER'::text]))),
    CONSTRAINT devices_ssh_creds_complete CHECK ((((ssh_username IS NULL) AND (ssh_secret_ciphertext IS NULL) AND (ssh_secret_kind IS NULL) AND (ssh_host_key_fingerprint IS NULL)) OR ((ssh_username IS NOT NULL) AND (ssh_secret_ciphertext IS NOT NULL) AND (ssh_secret_kind = ANY (ARRAY['password'::text, 'key'::text])) AND (ssh_host_key_fingerprint IS NOT NULL) AND (ssh_host_key_fingerprint ~ '^SHA256:[A-Za-z0-9+/]{42}[AEIMQUYcgkosw048]$'::text)))),
    CONSTRAINT devices_ssh_port_valid CHECK (((ssh_port >= 1) AND (ssh_port <= 65535))),
    CONSTRAINT devices_transport_check CHECK ((transport = ANY (ARRAY['direct'::text, 'connector'::text, 'tunnel'::text, 'controller'::text]))),
    CONSTRAINT devices_vendor_check CHECK ((vendor = ANY (ARRAY['mikrotik'::text, 'ruijie'::text, 'cisco'::text, 'tplink'::text, 'ubiquiti'::text, 'generic'::text])))
);


--
-- Name: fleet_scan_runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.fleet_scan_runs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    business_id uuid NOT NULL,
    kind text NOT NULL,
    payload jsonb DEFAULT '{}'::jsonb NOT NULL,
    max_severity text NOT NULL,
    router_count integer DEFAULT 0 NOT NULL,
    generated_at timestamp with time zone DEFAULT now() NOT NULL,
    triggered_by uuid,
    CONSTRAINT fleet_scan_runs_kind_check CHECK ((kind = ANY (ARRAY['ai'::text, 'security'::text]))),
    CONSTRAINT fleet_scan_runs_max_severity_check CHECK ((max_severity = ANY (ARRAY['critical'::text, 'warning'::text, 'info'::text, 'ok'::text])))
);


--
-- Name: guest_access_recovery; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.guest_access_recovery (
    voucher_id uuid NOT NULL,
    business_id uuid NOT NULL,
    device_id uuid NOT NULL,
    secret_hash bytea NOT NULL,
    current_mac macaddr NOT NULL,
    pending_mac macaddr,
    state text DEFAULT 'active'::text NOT NULL,
    transfer_id uuid,
    used_bytes bigint DEFAULT 0 NOT NULL,
    transfer_count integer DEFAULT 0 NOT NULL,
    window_started_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    completed_transfer_id uuid,
    CONSTRAINT guest_access_recovery_check CHECK ((((state = 'active'::text) AND (pending_mac IS NULL) AND (transfer_id IS NULL)) OR ((state <> 'active'::text) AND (pending_mac IS NOT NULL) AND (transfer_id IS NOT NULL) AND (pending_mac <> current_mac)))),
    CONSTRAINT guest_access_recovery_secret_hash_check CHECK ((octet_length(secret_hash) = 32)),
    CONSTRAINT guest_access_recovery_state_check CHECK ((state = ANY (ARRAY['active'::text, 'transferring'::text, 'frozen'::text, 'blocked'::text]))),
    CONSTRAINT guest_access_recovery_transfer_count_check CHECK (((transfer_count >= 0) AND (transfer_count <= 3))),
    CONSTRAINT guest_access_recovery_used_bytes_check CHECK ((used_bytes >= 0))
);

ALTER TABLE ONLY public.guest_access_recovery FORCE ROW LEVEL SECURITY;


--
-- Name: guest_purchase_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.guest_purchase_requests (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    business_id uuid NOT NULL,
    device_id uuid,
    client_mac text,
    client_ip inet,
    plan_key text NOT NULL,
    amount integer DEFAULT 0 NOT NULL,
    data_mb integer DEFAULT 0 NOT NULL,
    paid_phone text,
    tx_last6 text,
    bank_reference text,
    receipt_path text,
    grace_started_at timestamp with time zone,
    grace_expires_at timestamp with time zone,
    grace_data_mb integer DEFAULT 50 NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    decided_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    purchased_minutes integer,
    CONSTRAINT guest_purchase_requests_grace_data_mb_check CHECK (((grace_data_mb >= 1) AND (grace_data_mb <= 10240))),
    CONSTRAINT guest_purchase_requests_purchased_minutes_check CHECK (((purchased_minutes IS NULL) OR ((purchased_minutes >= 0) AND (purchased_minutes <= 525600)))),
    CONSTRAINT guest_purchase_requests_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'approved'::text, 'rejected'::text])))
);


--
-- Name: guest_voucher_batch_reseller_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.guest_voucher_batch_reseller_events (
    id bigint NOT NULL,
    business_id uuid NOT NULL,
    batch_id uuid NOT NULL,
    actor_id uuid NOT NULL,
    previous_reseller_id uuid,
    reseller_id uuid,
    details jsonb NOT NULL,
    recorded_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL
);

ALTER TABLE ONLY public.guest_voucher_batch_reseller_events FORCE ROW LEVEL SECURITY;


--
-- Name: guest_voucher_batch_reseller_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.guest_voucher_batch_reseller_events ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.guest_voucher_batch_reseller_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: guest_voucher_batch_resellers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.guest_voucher_batch_resellers (
    business_id uuid NOT NULL,
    batch_id uuid NOT NULL,
    reseller_id uuid NOT NULL,
    shop_name text NOT NULL,
    code_prefix text NOT NULL,
    assigned_by uuid NOT NULL,
    assigned_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL
);

ALTER TABLE ONLY public.guest_voucher_batch_resellers FORCE ROW LEVEL SECURITY;


--
-- Name: guest_voucher_grants; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.guest_voucher_grants (
    voucher_id uuid NOT NULL,
    business_id uuid NOT NULL,
    device_id uuid NOT NULL,
    minutes integer NOT NULL,
    data_mb integer NOT NULL,
    state text DEFAULT 'unused'::text NOT NULL,
    client_mac macaddr,
    first_used_at timestamp with time zone,
    expires_at timestamp with time zone,
    lease_token text,
    lease_until timestamp with time zone,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    activated_at timestamp with time zone,
    CONSTRAINT guest_voucher_grants_check CHECK ((((state = 'unused'::text) AND (client_mac IS NULL) AND (first_used_at IS NULL) AND (expires_at IS NULL)) OR ((state <> 'unused'::text) AND (client_mac IS NOT NULL) AND (first_used_at IS NOT NULL) AND (expires_at = (first_used_at + ((minutes)::double precision * '00:01:00'::interval)))))),
    CONSTRAINT guest_voucher_grants_check1 CHECK ((((state = 'provisioning'::text) AND (lease_token IS NOT NULL) AND (lease_until IS NOT NULL)) OR ((state <> 'provisioning'::text) AND (lease_token IS NULL) AND (lease_until IS NULL)))),
    CONSTRAINT guest_voucher_grants_data_mb_check CHECK (((data_mb >= 1) AND (data_mb <= 1048576))),
    CONSTRAINT guest_voucher_grants_minutes_check CHECK (((minutes >= 1) AND (minutes <= 525600))),
    CONSTRAINT guest_voucher_grants_state_check CHECK ((state = ANY (ARRAY['unused'::text, 'provisioning'::text, 'active'::text, 'expired'::text, 'revoked'::text])))
);

ALTER TABLE ONLY public.guest_voucher_grants FORCE ROW LEVEL SECURITY;


--
-- Name: guest_voucher_issue_batches; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.guest_voucher_issue_batches (
    business_id uuid NOT NULL,
    id uuid NOT NULL,
    device_id uuid NOT NULL,
    request_hash text NOT NULL,
    plan_label text NOT NULL,
    price_mmk integer NOT NULL,
    minutes integer NOT NULL,
    data_mb integer NOT NULL,
    total_count integer NOT NULL,
    lease_token text,
    lease_until timestamp with time zone,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT guest_voucher_issue_batches_check CHECK (((lease_token IS NULL) = (lease_until IS NULL))),
    CONSTRAINT guest_voucher_issue_batches_data_mb_check CHECK (((data_mb >= 1) AND (data_mb <= 1048576))),
    CONSTRAINT guest_voucher_issue_batches_minutes_check CHECK (((minutes >= 1) AND (minutes <= 525600))),
    CONSTRAINT guest_voucher_issue_batches_plan_label_check CHECK (((length(plan_label) >= 1) AND (length(plan_label) <= 100))),
    CONSTRAINT guest_voucher_issue_batches_price_mmk_check CHECK (((price_mmk >= 0) AND (price_mmk <= 1000000000))),
    CONSTRAINT guest_voucher_issue_batches_request_hash_check CHECK ((length(request_hash) = 64)),
    CONSTRAINT guest_voucher_issue_batches_total_count_check CHECK (((total_count >= 1) AND (total_count <= 200)))
);

ALTER TABLE ONLY public.guest_voucher_issue_batches FORCE ROW LEVEL SECURITY;


--
-- Name: guest_voucher_issue_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.guest_voucher_issue_items (
    business_id uuid NOT NULL,
    batch_id uuid NOT NULL,
    voucher_id uuid NOT NULL,
    "position" integer NOT NULL,
    install_state text DEFAULT 'pending'::text NOT NULL,
    verified_at timestamp with time zone,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT guest_voucher_issue_items_check CHECK (((install_state = 'verified'::text) = (verified_at IS NOT NULL))),
    CONSTRAINT guest_voucher_issue_items_install_state_check CHECK ((install_state = ANY (ARRAY['pending'::text, 'unknown'::text, 'verified'::text]))),
    CONSTRAINT guest_voucher_issue_items_position_check CHECK ((("position" >= 0) AND ("position" <= 199)))
);

ALTER TABLE ONLY public.guest_voucher_issue_items FORCE ROW LEVEL SECURITY;


--
-- Name: guest_voucher_usage; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.guest_voucher_usage (
    business_id uuid NOT NULL,
    voucher_id uuid NOT NULL,
    device_id uuid NOT NULL,
    bytes_used bigint NOT NULL,
    uptime_seconds bigint NOT NULL,
    observed_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT guest_voucher_usage_bytes_used_check CHECK ((bytes_used >= 0)),
    CONSTRAINT guest_voucher_usage_uptime_seconds_check CHECK ((uptime_seconds >= 0))
);

ALTER TABLE ONLY public.guest_voucher_usage FORCE ROW LEVEL SECURITY;


--
-- Name: hub_pending_enrollments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.hub_pending_enrollments (
    business_id uuid NOT NULL,
    device_id uuid NOT NULL,
    purpose text NOT NULL,
    ciphertext text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    CONSTRAINT hub_pending_enrollments_purpose_check CHECK ((purpose = ANY (ARRAY['router_key'::text, 'management'::text])))
);

ALTER TABLE ONLY public.hub_pending_enrollments FORCE ROW LEVEL SECURITY;


--
-- Name: i18n_overrides; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.i18n_overrides (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    locale text NOT NULL,
    ns text NOT NULL,
    key text NOT NULL,
    value text NOT NULL,
    updated_by uuid,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT i18n_overrides_locale_check CHECK ((locale = ANY (ARRAY['en'::text, 'zh'::text, 'my'::text]))),
    CONSTRAINT i18n_overrides_ns_check CHECK ((ns = ANY (ARRAY['ui'::text, 'copy'::text])))
);


--
-- Name: incidents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.incidents (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    business_id uuid NOT NULL,
    device_id uuid,
    policy_id uuid,
    kind text NOT NULL,
    severity text DEFAULT 'warning'::text NOT NULL,
    subject_id text NOT NULL,
    subject_label text NOT NULL,
    detail text,
    suggested_action jsonb,
    opened_at timestamp with time zone DEFAULT now() NOT NULL,
    acknowledged_at timestamp with time zone,
    acknowledged_by uuid,
    resolved_at timestamp with time zone,
    resolved_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT incidents_severity_check CHECK ((severity = ANY (ARRAY['info'::text, 'warning'::text, 'critical'::text])))
);


--
-- Name: magic_coin_transactions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.magic_coin_transactions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    business_id uuid NOT NULL,
    user_id uuid NOT NULL,
    delta numeric(12,2) NOT NULL,
    balance_after numeric(12,2) NOT NULL,
    kind text NOT NULL,
    service_purchase_id uuid,
    note text NOT NULL,
    actor_user_id uuid,
    webfig_key_activation_id uuid,
    reseller_add_key_activation_id uuid,
    router_unlock_key_activation_id uuid,
    magic_dude_unlock_activation_id uuid,
    switch_unlock_key_activation_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT magic_coin_transactions_delta_check CHECK ((delta <> (0)::numeric)),
    CONSTRAINT magic_coin_transactions_kind_check CHECK ((kind = ANY (ARRAY['agent_earn'::text, 'agent_reversal'::text, 'service_payment'::text, 'service_refund'::text, 'feature_key_purchase'::text, 'purchase_credit'::text, 'manual_credit'::text, 'manual_debit'::text])))
);


--
-- Name: magic_coin_wallets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.magic_coin_wallets (
    user_id uuid NOT NULL,
    business_id uuid NOT NULL,
    balance numeric(12,2) DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: magic_dude_knowledge_notes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.magic_dude_knowledge_notes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    title text NOT NULL,
    body_md text NOT NULL,
    sort integer DEFAULT 0 NOT NULL,
    active boolean DEFAULT true NOT NULL,
    updated_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT magic_dude_knowledge_notes_body_md_check CHECK (((char_length(btrim(body_md)) >= 1) AND (char_length(btrim(body_md)) <= 20000))),
    CONSTRAINT magic_dude_knowledge_notes_title_check CHECK (((char_length(btrim(title)) >= 1) AND (char_length(btrim(title)) <= 200)))
);


--
-- Name: magic_dude_solved_cases; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.magic_dude_solved_cases (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    business_id uuid NOT NULL,
    user_id uuid,
    question text NOT NULL,
    answer text NOT NULL,
    language text,
    helpful_count integer DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    last_used_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT magic_dude_solved_cases_answer_check CHECK (((char_length(btrim(answer)) >= 1) AND (char_length(btrim(answer)) <= 8000))),
    CONSTRAINT magic_dude_solved_cases_helpful_count_check CHECK ((helpful_count >= 1)),
    CONSTRAINT magic_dude_solved_cases_language_check CHECK (((language IS NULL) OR (language = ANY (ARRAY['en'::text, 'my'::text, 'zh'::text])))),
    CONSTRAINT magic_dude_solved_cases_question_check CHECK (((char_length(btrim(question)) >= 1) AND (char_length(btrim(question)) <= 4000)))
);


--
-- Name: magic_media_publications; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.magic_media_publications (
    business_id uuid NOT NULL,
    public_id text NOT NULL,
    design jsonb NOT NULL,
    revision bigint DEFAULT 1 NOT NULL,
    published_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT magic_media_publications_design_check CHECK (((jsonb_typeof(design) = 'object'::text) AND (octet_length((design)::text) <= 3145728))),
    CONSTRAINT magic_media_publications_public_id_check CHECK ((public_id ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT magic_media_publications_revision_check CHECK ((revision > 0))
);

ALTER TABLE ONLY public.magic_media_publications FORCE ROW LEVEL SECURITY;


--
-- Name: management_address_migrations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.management_address_migrations (
    id uuid NOT NULL,
    business_id uuid NOT NULL,
    device_id uuid NOT NULL,
    requested_by uuid NOT NULL,
    old_address inet NOT NULL,
    new_address inet NOT NULL,
    interface_name text NOT NULL,
    old_record text NOT NULL,
    fingerprint text NOT NULL,
    router_key text NOT NULL,
    guard_job text DEFAULT ''::text NOT NULL,
    backup_name text DEFAULT ''::text NOT NULL,
    phase text NOT NULL,
    revision integer DEFAULT 0 NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT management_address_migrations_check CHECK (((family(old_address) = 4) AND (family(new_address) = 4) AND (masklen(old_address) = 32) AND (masklen(new_address) = 32) AND (old_address <> new_address))),
    CONSTRAINT management_address_migrations_phase_check CHECK ((phase = ANY (ARRAY['planned'::text, 'starting'::text, 'hub_staged'::text, 'router_staged'::text, 'new_verified'::text, 'hub_committed'::text, 'router_retained'::text, 'connection_saved'::text, 'router_cleaned'::text, 'completed'::text, 'rolled_back'::text])))
);

ALTER TABLE ONLY public.management_address_migrations FORCE ROW LEVEL SECURITY;


--
-- Name: nats_subjects; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.nats_subjects (
    business_id uuid NOT NULL,
    subject_prefix text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: notification_prefs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.notification_prefs (
    business_id uuid NOT NULL,
    in_app boolean DEFAULT true NOT NULL,
    email boolean DEFAULT false NOT NULL,
    quiet_hours_start smallint,
    quiet_hours_end smallint,
    min_severity text DEFAULT 'warning'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT notification_prefs_min_severity_check CHECK ((min_severity = ANY (ARRAY['info'::text, 'warning'::text, 'critical'::text]))),
    CONSTRAINT notification_prefs_quiet_hours_end_check CHECK (((quiet_hours_end >= 0) AND (quiet_hours_end <= 23))),
    CONSTRAINT notification_prefs_quiet_hours_start_check CHECK (((quiet_hours_start >= 0) AND (quiet_hours_start <= 23)))
);


--
-- Name: operator_feature_grants; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.operator_feature_grants (
    user_id uuid NOT NULL,
    business_id uuid NOT NULL,
    feature text NOT NULL,
    granted_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT operator_feature_grants_feature_check CHECK ((feature = ANY (ARRAY['vouchers'::text, 'portal_deploy'::text, 'cash_sales'::text, 'bank_edit'::text, 'router_config'::text, 'reboot'::text, 'alerts'::text, 'syslog_ai'::text, 'poe'::text, 'telegram'::text])))
);


--
-- Name: operator_feature_role_defaults; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.operator_feature_role_defaults (
    role text NOT NULL,
    feature text NOT NULL,
    legacy_role text NOT NULL,
    updated_by uuid,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT operator_feature_role_defaults_feature_check CHECK ((feature = ANY (ARRAY['vouchers'::text, 'portal_deploy'::text, 'cash_sales'::text, 'bank_edit'::text, 'router_config'::text, 'reboot'::text, 'alerts'::text, 'syslog_ai'::text, 'poe'::text, 'telegram'::text]))),
    CONSTRAINT operator_feature_role_defaults_role_check CHECK ((role = ANY (ARRAY['OWNER'::text, 'ADMIN'::text, 'VIEWER'::text, 'DEVELOPER'::text])))
);


--
-- Name: owner_daily_closes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.owner_daily_closes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    business_id uuid NOT NULL,
    site_id uuid,
    router_connection_id uuid,
    business_day date NOT NULL,
    counted_cash_mmk integer DEFAULT 0 NOT NULL,
    note text,
    closed_by uuid,
    closed_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: peer_heartbeats; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.peer_heartbeats (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    business_id uuid NOT NULL,
    peer_id text NOT NULL,
    device_id uuid,
    handshake_at timestamp with time zone,
    state text DEFAULT 'unknown'::text NOT NULL,
    rx_bytes bigint DEFAULT 0 NOT NULL,
    tx_bytes bigint DEFAULT 0 NOT NULL,
    observed_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT peer_heartbeats_state_check CHECK ((state = ANY (ARRAY['connected'::text, 'disconnected'::text, 'error'::text, 'unknown'::text])))
);


--
-- Name: platform_admins; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.platform_admins (
    user_id uuid NOT NULL,
    note text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: platform_announcement; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.platform_announcement (
    id boolean DEFAULT true NOT NULL,
    active boolean DEFAULT false NOT NULL,
    message text,
    speed_seconds integer,
    updated_by uuid,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT platform_announcement_id_check CHECK (id),
    CONSTRAINT platform_announcement_speed_seconds_check CHECK (((speed_seconds >= 8) AND (speed_seconds <= 120)))
);


--
-- Name: platform_service_bank_accounts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.platform_service_bank_accounts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    slot integer NOT NULL,
    holder_name text NOT NULL,
    bank_name text NOT NULL,
    account_number text NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    sort integer DEFAULT 0 NOT NULL,
    configured_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT platform_service_bank_accounts_slot_check CHECK ((slot = ANY (ARRAY[1, 2])))
);


--
-- Name: portal_configs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.portal_configs (
    business_id uuid NOT NULL,
    business_name text DEFAULT 'MikroTik Magic Wi-Fi'::text NOT NULL,
    welcome_text text DEFAULT 'Enter your voucher code to connect.'::text NOT NULL,
    terms_text text DEFAULT 'By connecting you agree to our terms.'::text NOT NULL,
    primary_hex text DEFAULT '#ffb547'::text NOT NULL,
    glass_tint_hex text DEFAULT '#7ad0ff'::text NOT NULL,
    consent_required boolean DEFAULT true NOT NULL,
    consent_text text,
    bank_name text,
    bank_account text,
    bank_holder text,
    mmqr_url text,
    guest_grace_minutes integer DEFAULT 10 NOT NULL,
    guest_grace_data_mb integer DEFAULT 50 NOT NULL,
    guest_plans jsonb DEFAULT '[]'::jsonb NOT NULL,
    voucher_login_enabled boolean DEFAULT true NOT NULL,
    mobile_payment_enabled boolean DEFAULT true NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    media_host text,
    media_port text DEFAULT '8096'::text NOT NULL,
    payment_instructions text,
    template_id text DEFAULT ''::text NOT NULL,
    CONSTRAINT portal_configs_access_method_check CHECK ((voucher_login_enabled OR mobile_payment_enabled)),
    CONSTRAINT portal_configs_guest_grace_data_mb_check CHECK (((guest_grace_data_mb >= 1) AND (guest_grace_data_mb <= 10240))),
    CONSTRAINT portal_configs_media_host_check CHECK (((media_host IS NULL) OR ((media_host ~ '^([0-9]{1,3}\.){3}[0-9]{1,3}$'::text) AND (((split_part(media_host, '.'::text, 1))::integer >= 1) AND ((split_part(media_host, '.'::text, 1))::integer <= 255)) AND (((split_part(media_host, '.'::text, 2))::integer >= 0) AND ((split_part(media_host, '.'::text, 2))::integer <= 255)) AND (((split_part(media_host, '.'::text, 3))::integer >= 0) AND ((split_part(media_host, '.'::text, 3))::integer <= 255)) AND (((split_part(media_host, '.'::text, 4))::integer >= 1) AND ((split_part(media_host, '.'::text, 4))::integer <= 254)) AND ((split_part(media_host, '.'::text, 1))::integer < 224) AND (((split_part(media_host, '.'::text, 1))::integer = 10) OR (((split_part(media_host, '.'::text, 1))::integer = 192) AND ((split_part(media_host, '.'::text, 2))::integer = 168)) OR (((split_part(media_host, '.'::text, 1))::integer = 172) AND (((split_part(media_host, '.'::text, 2))::integer >= 16) AND ((split_part(media_host, '.'::text, 2))::integer <= 31))))))),
    CONSTRAINT portal_configs_media_port_check CHECK (((media_port ~ '^[0-9]{1,5}$'::text) AND ((media_port)::integer >= 1) AND ((media_port)::integer <= 65535)))
);


--
-- Name: portal_mode_role_defaults; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.portal_mode_role_defaults (
    role text NOT NULL,
    mode text NOT NULL,
    legacy_role text NOT NULL,
    updated_by uuid,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT portal_mode_role_defaults_mode_check CHECK ((mode = ANY (ARRAY['hybrid_light'::text, 'commerce'::text]))),
    CONSTRAINT portal_mode_role_defaults_role_check CHECK ((role = ANY (ARRAY['OWNER'::text, 'ADMIN'::text, 'VIEWER'::text, 'DEVELOPER'::text])))
);


--
-- Name: portal_sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.portal_sessions (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    business_id uuid NOT NULL,
    device_id uuid,
    voucher_id uuid,
    client_mac macaddr NOT NULL,
    client_ip inet,
    authenticated_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone,
    is_active boolean DEFAULT true NOT NULL,
    consent_given boolean DEFAULT false NOT NULL,
    consent_at timestamp with time zone
);


--
-- Name: pricing_promo; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pricing_promo (
    id boolean DEFAULT true NOT NULL,
    active boolean DEFAULT false NOT NULL,
    starts_on date,
    ends_on date,
    label text,
    monthly_promo_mmk integer,
    monthly_standard_mmk integer,
    annual_promo_mmk integer,
    annual_standard_mmk integer,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT pricing_promo_id_check CHECK (id)
);


--
-- Name: purchase_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.purchase_requests (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    business_id uuid NOT NULL,
    device_id uuid,
    plan_key text NOT NULL,
    amount integer DEFAULT 0 NOT NULL,
    bank_reference text NOT NULL,
    duration_minutes integer NOT NULL,
    receipt_path text,
    status text DEFAULT 'pending'::text NOT NULL,
    decided_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT purchase_requests_bank_reference_check CHECK ((length(btrim(bank_reference)) > 0)),
    CONSTRAINT purchase_requests_duration_minutes_check CHECK ((duration_minutes > 0)),
    CONSTRAINT purchase_requests_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'approved'::text, 'rejected'::text])))
);


--
-- Name: router_connections; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.router_connections (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    business_id uuid NOT NULL,
    name text NOT NULL,
    host text,
    port integer,
    use_tls boolean DEFAULT true NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    site_id uuid,
    connection_mode text,
    cloud_status text,
    cloud_last_seen_at timestamp with time zone,
    environment text,
    legacy_password_ciphertext text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: ruijie_cloud_connections; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ruijie_cloud_connections (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    business_id uuid NOT NULL,
    region text,
    base_url text,
    legacy_app_id_ciphertext text,
    legacy_app_secret_ciphertext text,
    site_ref text,
    device_ref text,
    enabled boolean DEFAULT true NOT NULL,
    last_test_at timestamp with time zone,
    last_test_status text,
    last_sync_at timestamp with time zone,
    created_by uuid,
    token_path text,
    device_path text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: service_purchase_audit; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.service_purchase_audit (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    business_id uuid NOT NULL,
    purchase_id uuid NOT NULL,
    actor_user_id uuid,
    action text NOT NULL,
    note text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: service_purchases; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.service_purchases (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    business_id uuid NOT NULL,
    user_id uuid NOT NULL,
    service_key text NOT NULL,
    service_label text NOT NULL,
    price_mmk integer NOT NULL,
    coin_amount numeric(12,2),
    magic_coin_purchase_amount numeric(12,2),
    payment_method text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    bank_account_id uuid,
    bank_reference text,
    receipt_path text,
    idempotency_key text NOT NULL,
    decided_by uuid,
    decided_at timestamp with time zone,
    reject_reason text,
    activated_expires_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT service_purchases_check CHECK ((((payment_method = 'bank_transfer'::text) AND (bank_account_id IS NOT NULL) AND (bank_reference IS NOT NULL)) OR ((payment_method = 'magic_coins'::text) AND (bank_account_id IS NULL)))),
    CONSTRAINT service_purchases_check1 CHECK ((((service_key = 'magic_coins'::text) AND (magic_coin_purchase_amount > (0)::numeric)) OR ((service_key <> 'magic_coins'::text) AND (magic_coin_purchase_amount IS NULL)))),
    CONSTRAINT service_purchases_payment_method_check CHECK ((payment_method = ANY (ARRAY['bank_transfer'::text, 'magic_coins'::text]))),
    CONSTRAINT service_purchases_price_mmk_check CHECK ((price_mmk > 0)),
    CONSTRAINT service_purchases_service_key_check CHECK ((service_key = ANY (ARRAY['monthly'::text, 'annual'::text, 'magic_coins'::text]))),
    CONSTRAINT service_purchases_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'approved'::text, 'rejected'::text, 'cancelled'::text, 'refunded'::text])))
);


--
-- Name: site_portals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.site_portals (
    site_id uuid NOT NULL,
    business_id uuid NOT NULL,
    draft_config jsonb DEFAULT '{}'::jsonb NOT NULL,
    published_config jsonb DEFAULT '{}'::jsonb NOT NULL,
    draft_revision bigint DEFAULT 1 NOT NULL,
    published_revision bigint DEFAULT 0 NOT NULL,
    published_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT site_portals_draft_revision_check CHECK ((draft_revision > 0)),
    CONSTRAINT site_portals_published_revision_check CHECK ((published_revision >= 0))
);


--
-- Name: site_topology_config; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.site_topology_config (
    site_id uuid NOT NULL,
    router_id uuid NOT NULL,
    port_labels jsonb DEFAULT '[]'::jsonb NOT NULL,
    downstream_devices jsonb DEFAULT '[]'::jsonb NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: sites; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sites (
    site_id uuid DEFAULT gen_random_uuid() NOT NULL,
    business_id uuid NOT NULL,
    name text NOT NULL,
    location text,
    timezone text,
    latitude numeric(9,6),
    longitude numeric(9,6),
    notes text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: store_addon_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.store_addon_requests (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    business_id uuid NOT NULL,
    user_id uuid NOT NULL,
    addon_id uuid NOT NULL,
    price_mmk integer NOT NULL,
    period text NOT NULL,
    bank_reference text NOT NULL,
    idempotency_key text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    decision_note text DEFAULT ''::text NOT NULL,
    decided_by uuid,
    decided_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT store_addon_requests_bank_reference_check CHECK (((length(bank_reference) >= 4) AND (length(bank_reference) <= 80))),
    CONSTRAINT store_addon_requests_decision_note_check CHECK ((length(decision_note) <= 500)),
    CONSTRAINT store_addon_requests_idempotency_key_check CHECK (((length(idempotency_key) >= 8) AND (length(idempotency_key) <= 80))),
    CONSTRAINT store_addon_requests_period_check CHECK ((period = ANY (ARRAY['one_time'::text, 'monthly'::text, 'annual'::text]))),
    CONSTRAINT store_addon_requests_price_mmk_check CHECK ((price_mmk >= 0)),
    CONSTRAINT store_addon_requests_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'approved'::text, 'rejected'::text])))
);

ALTER TABLE ONLY public.store_addon_requests FORCE ROW LEVEL SECURITY;


--
-- Name: store_addons; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.store_addons (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    addon_key text NOT NULL,
    kind text NOT NULL,
    name_en text NOT NULL,
    name_my text NOT NULL,
    description_en text DEFAULT ''::text NOT NULL,
    description_my text DEFAULT ''::text NOT NULL,
    price_mmk integer NOT NULL,
    period text NOT NULL,
    active boolean DEFAULT true NOT NULL,
    sort_order integer DEFAULT 100 NOT NULL,
    created_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    updated_at timestamp with time zone DEFAULT clock_timestamp() NOT NULL,
    CONSTRAINT store_addons_addon_key_check CHECK ((addon_key ~ '^[a-z0-9_]{3,40}$'::text)),
    CONSTRAINT store_addons_description_en_check CHECK ((length(description_en) <= 600)),
    CONSTRAINT store_addons_description_my_check CHECK ((length(description_my) <= 600)),
    CONSTRAINT store_addons_kind_check CHECK ((kind = ANY (ARRAY['router_slot'::text, 'server'::text, 'feature'::text]))),
    CONSTRAINT store_addons_name_en_check CHECK (((length(name_en) >= 1) AND (length(name_en) <= 120))),
    CONSTRAINT store_addons_name_my_check CHECK (((length(name_my) >= 1) AND (length(name_my) <= 120))),
    CONSTRAINT store_addons_period_check CHECK ((period = ANY (ARRAY['one_time'::text, 'monthly'::text, 'annual'::text]))),
    CONSTRAINT store_addons_price_mmk_check CHECK ((price_mmk >= 0))
);


--
-- Name: syslog_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.syslog_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    business_id uuid NOT NULL,
    router_connection_id uuid,
    site_id uuid,
    source_ip text,
    facility text,
    severity text NOT NULL,
    program text,
    message text NOT NULL,
    ai_summary text,
    ai_severity text,
    received_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT syslog_events_ai_severity_check CHECK (((ai_severity IS NULL) OR (ai_severity = ANY (ARRAY['info'::text, 'warning'::text, 'critical'::text])))),
    CONSTRAINT syslog_events_severity_check CHECK ((severity = ANY (ARRAY['info'::text, 'warning'::text, 'critical'::text])))
);


--
-- Name: telemetry_samples; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.telemetry_samples (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    business_id uuid NOT NULL,
    device_id uuid,
    subject_kind text NOT NULL,
    subject_id text NOT NULL,
    cpu_pct numeric(5,2),
    memory_pct numeric(5,2),
    latency_ms integer,
    uptime_seconds bigint,
    wan_state text,
    connector_state text,
    tunnel_state text,
    observed_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT telemetry_samples_connector_state_check CHECK ((connector_state = ANY (ARRAY['online'::text, 'stale'::text, 'offline'::text, 'none'::text]))),
    CONSTRAINT telemetry_samples_subject_kind_check CHECK ((subject_kind = ANY (ARRAY['site'::text, 'router'::text, 'device'::text, 'connector'::text]))),
    CONSTRAINT telemetry_samples_tunnel_state_check CHECK ((tunnel_state = ANY (ARRAY['up'::text, 'down'::text, 'none'::text]))),
    CONSTRAINT telemetry_samples_wan_state_check CHECK ((wan_state = ANY (ARRAY['up'::text, 'degraded'::text, 'down'::text, 'unknown'::text])))
);


--
-- Name: terminal_history; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.terminal_history (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    business_id uuid NOT NULL,
    user_id uuid,
    router_connection_id uuid,
    site_id uuid,
    method text NOT NULL,
    path text NOT NULL,
    body text,
    status integer,
    ms integer,
    response_snippet text,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: terminal_templates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.terminal_templates (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    business_id uuid NOT NULL,
    created_by uuid,
    name text NOT NULL,
    description text,
    category text NOT NULL,
    method text NOT NULL,
    path text NOT NULL,
    body text,
    is_builtin boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT terminal_templates_method_check CHECK ((method = ANY (ARRAY['GET'::text, 'POST'::text, 'PATCH'::text, 'PUT'::text, 'DELETE'::text]))),
    CONSTRAINT terminal_templates_path_check CHECK ((path ~~ '/%'::text))
);


--
-- Name: unlock_key_activations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.unlock_key_activations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    business_id uuid NOT NULL,
    user_id uuid NOT NULL,
    key_kind text NOT NULL,
    key_id integer,
    key_name text,
    price_coins numeric(12,2) NOT NULL,
    attributes text NOT NULL,
    account_bound boolean DEFAULT true NOT NULL,
    non_transferable boolean DEFAULT true NOT NULL,
    unique_router_key boolean DEFAULT false NOT NULL,
    grace_days integer,
    consumed_at timestamp with time zone,
    consumed_ref_id uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    CONSTRAINT unlock_key_activations_check CHECK ((expires_at > created_at)),
    CONSTRAINT unlock_key_activations_check1 CHECK (((consumed_at IS NULL) = (consumed_ref_id IS NULL))),
    CONSTRAINT unlock_key_activations_key_kind_check CHECK ((key_kind = ANY (ARRAY['webfig'::text, 'magic_dude'::text, 'router'::text, 'switch'::text, 'reseller'::text])))
);


--
-- Name: user_profile_settings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_profile_settings (
    user_id uuid NOT NULL,
    business_id uuid NOT NULL,
    language text DEFAULT 'en'::text NOT NULL,
    avatar_data bytea,
    avatar_mime text,
    cover_data bytea,
    cover_mime text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT user_profile_settings_avatar_data_check CHECK (((avatar_data IS NULL) OR (octet_length(avatar_data) <= 2097152))),
    CONSTRAINT user_profile_settings_avatar_mime_check CHECK (((avatar_mime IS NULL) OR (avatar_mime = ANY (ARRAY['image/jpeg'::text, 'image/png'::text, 'image/webp'::text])))),
    CONSTRAINT user_profile_settings_cover_data_check CHECK (((cover_data IS NULL) OR (octet_length(cover_data) <= 5242880))),
    CONSTRAINT user_profile_settings_cover_mime_check CHECK (((cover_mime IS NULL) OR (cover_mime = ANY (ARRAY['image/jpeg'::text, 'image/png'::text, 'image/webp'::text])))),
    CONSTRAINT user_profile_settings_language_check CHECK ((language = ANY (ARRAY['en'::text, 'my'::text, 'zh'::text])))
);


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users (
    user_id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    business_id uuid NOT NULL,
    full_name text NOT NULL,
    email text NOT NULL,
    password_hash text NOT NULL,
    user_role text DEFAULT 'VIEWER'::text NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    last_login_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT users_user_role_check CHECK ((user_role = ANY (ARRAY['OWNER'::text, 'ADMIN'::text, 'VIEWER'::text, 'DEVELOPER'::text])))
);


--
-- Name: voucher_print_layouts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.voucher_print_layouts (
    business_id uuid NOT NULL,
    business_name text NOT NULL,
    wifi_name text NOT NULL,
    support_contact text NOT NULL,
    terms text NOT NULL,
    paper_width_mm integer NOT NULL,
    show_qr boolean DEFAULT true NOT NULL,
    show_price boolean DEFAULT true NOT NULL,
    show_expiry boolean DEFAULT true NOT NULL,
    a4_columns integer NOT NULL,
    a4_rows_per_page integer NOT NULL,
    thermal_columns integer NOT NULL,
    thermal_rows_per_strip integer NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT voucher_print_layouts_a4_columns_check CHECK ((a4_columns = ANY (ARRAY[2, 3, 4, 5]))),
    CONSTRAINT voucher_print_layouts_a4_rows_per_page_check CHECK (((a4_rows_per_page >= 4) AND (a4_rows_per_page <= 50))),
    CONSTRAINT voucher_print_layouts_business_name_check CHECK (((char_length(business_name) >= 1) AND (char_length(business_name) <= 80))),
    CONSTRAINT voucher_print_layouts_paper_width_mm_check CHECK ((paper_width_mm = ANY (ARRAY[58, 80]))),
    CONSTRAINT voucher_print_layouts_support_contact_check CHECK (((char_length(support_contact) >= 1) AND (char_length(support_contact) <= 120))),
    CONSTRAINT voucher_print_layouts_terms_check CHECK (((char_length(terms) >= 1) AND (char_length(terms) <= 600))),
    CONSTRAINT voucher_print_layouts_thermal_columns_check CHECK ((thermal_columns = ANY (ARRAY[1, 2]))),
    CONSTRAINT voucher_print_layouts_thermal_rows_per_strip_check CHECK (((thermal_rows_per_strip >= 4) AND (thermal_rows_per_strip <= 40))),
    CONSTRAINT voucher_print_layouts_wifi_name_check CHECK (((char_length(wifi_name) >= 1) AND (char_length(wifi_name) <= 80)))
);


--
-- Name: voucher_resellers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.voucher_resellers (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    business_id uuid NOT NULL,
    shop_name text NOT NULL,
    contact_name text NOT NULL,
    location text,
    phone text,
    active boolean DEFAULT true NOT NULL,
    code_prefix text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT voucher_resellers_code_prefix_check CHECK ((((char_length(code_prefix) >= 2) AND (char_length(code_prefix) <= 4)) AND (code_prefix = upper(code_prefix)) AND (code_prefix ~ '^[ABCDEFGHJKLMNPQRSTUVWXYZ23456789]+$'::text))),
    CONSTRAINT voucher_resellers_contact_name_check CHECK (((char_length(contact_name) >= 1) AND (char_length(contact_name) <= 120))),
    CONSTRAINT voucher_resellers_shop_name_check CHECK (((char_length(shop_name) >= 1) AND (char_length(shop_name) <= 120)))
);


--
-- Name: vouchers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.vouchers (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    business_id uuid NOT NULL,
    device_id uuid,
    plan_key text NOT NULL,
    plan_label text NOT NULL,
    code text NOT NULL,
    price_mmk integer DEFAULT 0 NOT NULL,
    duration_minutes integer,
    device_mac macaddr,
    status text DEFAULT 'unused'::text NOT NULL,
    first_seen_at timestamp with time zone,
    expires_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT vouchers_status_check CHECK ((status = ANY (ARRAY['unused'::text, 'active'::text, 'used'::text, 'expired'::text, 'revoked'::text, 'banned'::text])))
);


--
-- Name: webhook_deliveries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.webhook_deliveries (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    webhook_id uuid,
    business_id uuid NOT NULL,
    event_type text NOT NULL,
    payload jsonb NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    last_status_code integer,
    last_error text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    delivered_at timestamp with time zone,
    CONSTRAINT webhook_deliveries_status_check CHECK ((status = ANY (ARRAY['pending'::text, 'delivered'::text, 'failed'::text])))
);


--
-- Name: webhook_endpoints; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.webhook_endpoints (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    business_id uuid NOT NULL,
    url text NOT NULL,
    secret text,
    event_types text[] DEFAULT '{incident.opened,incident.resolved}'::text[] NOT NULL,
    enabled boolean DEFAULT true NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT webhook_endpoints_url_check CHECK ((url ~ '^https?://'::text))
);


--
-- Name: business_app_primary_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_app_primary_events ALTER COLUMN id SET DEFAULT nextval('public.business_app_primary_events_id_seq'::regclass);


--
-- Name: account_entitlements account_entitlements_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_entitlements
    ADD CONSTRAINT account_entitlements_pkey PRIMARY KEY (user_id);


--
-- Name: admin_notifications admin_notifications_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_notifications
    ADD CONSTRAINT admin_notifications_pkey PRIMARY KEY (id);


--
-- Name: ai_usage_events ai_usage_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_usage_events
    ADD CONSTRAINT ai_usage_events_pkey PRIMARY KEY (id);


--
-- Name: alert_policies alert_policies_business_id_kind_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alert_policies
    ADD CONSTRAINT alert_policies_business_id_kind_key UNIQUE (business_id, kind);


--
-- Name: alert_policies alert_policies_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alert_policies
    ADD CONSTRAINT alert_policies_pkey PRIMARY KEY (id);


--
-- Name: app_account_pass_events app_account_pass_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.app_account_pass_events
    ADD CONSTRAINT app_account_pass_events_pkey PRIMARY KEY (id);


--
-- Name: app_account_pass_renewals app_account_pass_renewals_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.app_account_pass_renewals
    ADD CONSTRAINT app_account_pass_renewals_pkey PRIMARY KEY (id);


--
-- Name: app_account_pass_rollouts app_account_pass_rollouts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.app_account_pass_rollouts
    ADD CONSTRAINT app_account_pass_rollouts_pkey PRIMARY KEY (rollout_key);


--
-- Name: app_account_pass_transitions app_account_pass_transitions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.app_account_pass_transitions
    ADD CONSTRAINT app_account_pass_transitions_pkey PRIMARY KEY (user_id);


--
-- Name: app_account_passes app_account_passes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.app_account_passes
    ADD CONSTRAINT app_account_passes_pkey PRIMARY KEY (user_id);


--
-- Name: app_account_trials app_account_trials_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.app_account_trials
    ADD CONSTRAINT app_account_trials_pkey PRIMARY KEY (user_id);


--
-- Name: audit_events audit_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_events
    ADD CONSTRAINT audit_events_pkey PRIMARY KEY (id);


--
-- Name: auth_sessions auth_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auth_sessions
    ADD CONSTRAINT auth_sessions_pkey PRIMARY KEY (session_id);


--
-- Name: business_app_primary_events business_app_primary_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_app_primary_events
    ADD CONSTRAINT business_app_primary_events_pkey PRIMARY KEY (id);


--
-- Name: business_app_primary_users business_app_primary_users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_app_primary_users
    ADD CONSTRAINT business_app_primary_users_pkey PRIMARY KEY (business_id);


--
-- Name: business_invitations business_invitations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_invitations
    ADD CONSTRAINT business_invitations_pkey PRIMARY KEY (invitation_id);


--
-- Name: business_invitations business_invitations_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_invitations
    ADD CONSTRAINT business_invitations_token_hash_key UNIQUE (token_hash);


--
-- Name: business_memberships business_memberships_business_id_user_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_memberships
    ADD CONSTRAINT business_memberships_business_id_user_id_key UNIQUE (business_id, user_id);


--
-- Name: business_memberships business_memberships_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_memberships
    ADD CONSTRAINT business_memberships_pkey PRIMARY KEY (membership_id);


--
-- Name: business_metered_quotas business_metered_quotas_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_metered_quotas
    ADD CONSTRAINT business_metered_quotas_pkey PRIMARY KEY (business_id);


--
-- Name: business_telegram_bots business_telegram_bots_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_telegram_bots
    ADD CONSTRAINT business_telegram_bots_pkey PRIMARY KEY (business_id);


--
-- Name: businesses businesses_owner_email_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.businesses
    ADD CONSTRAINT businesses_owner_email_key UNIQUE (owner_email);


--
-- Name: businesses businesses_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.businesses
    ADD CONSTRAINT businesses_pkey PRIMARY KEY (business_id);


--
-- Name: device_allowances device_allowances_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_allowances
    ADD CONSTRAINT device_allowances_pkey PRIMARY KEY (business_id);


--
-- Name: device_change_events device_change_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_change_events
    ADD CONSTRAINT device_change_events_pkey PRIMARY KEY (id);


--
-- Name: device_config_backups device_config_backups_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_config_backups
    ADD CONSTRAINT device_config_backups_pkey PRIMARY KEY (id);


--
-- Name: device_discovery_labels device_discovery_labels_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_discovery_labels
    ADD CONSTRAINT device_discovery_labels_pkey PRIMARY KEY (business_id, device_id, mac_address);


--
-- Name: device_requests device_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_requests
    ADD CONSTRAINT device_requests_pkey PRIMARY KEY (id);


--
-- Name: devices devices_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.devices
    ADD CONSTRAINT devices_pkey PRIMARY KEY (device_id);


--
-- Name: fleet_scan_runs fleet_scan_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fleet_scan_runs
    ADD CONSTRAINT fleet_scan_runs_pkey PRIMARY KEY (id);


--
-- Name: guest_access_recovery guest_access_recovery_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_access_recovery
    ADD CONSTRAINT guest_access_recovery_pkey PRIMARY KEY (voucher_id);


--
-- Name: guest_purchase_requests guest_purchase_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_purchase_requests
    ADD CONSTRAINT guest_purchase_requests_pkey PRIMARY KEY (id);


--
-- Name: guest_voucher_batch_reseller_events guest_voucher_batch_reseller_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_voucher_batch_reseller_events
    ADD CONSTRAINT guest_voucher_batch_reseller_events_pkey PRIMARY KEY (id);


--
-- Name: guest_voucher_batch_resellers guest_voucher_batch_resellers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_voucher_batch_resellers
    ADD CONSTRAINT guest_voucher_batch_resellers_pkey PRIMARY KEY (business_id, batch_id);


--
-- Name: guest_voucher_grants guest_voucher_grants_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_voucher_grants
    ADD CONSTRAINT guest_voucher_grants_pkey PRIMARY KEY (voucher_id);


--
-- Name: guest_voucher_issue_batches guest_voucher_issue_batches_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_voucher_issue_batches
    ADD CONSTRAINT guest_voucher_issue_batches_pkey PRIMARY KEY (business_id, id);


--
-- Name: guest_voucher_issue_items guest_voucher_issue_items_business_id_batch_id_position_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_voucher_issue_items
    ADD CONSTRAINT guest_voucher_issue_items_business_id_batch_id_position_key UNIQUE (business_id, batch_id, "position");


--
-- Name: guest_voucher_issue_items guest_voucher_issue_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_voucher_issue_items
    ADD CONSTRAINT guest_voucher_issue_items_pkey PRIMARY KEY (business_id, voucher_id);


--
-- Name: guest_voucher_usage guest_voucher_usage_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_voucher_usage
    ADD CONSTRAINT guest_voucher_usage_pkey PRIMARY KEY (business_id, voucher_id);


--
-- Name: hub_pending_enrollments hub_pending_enrollments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hub_pending_enrollments
    ADD CONSTRAINT hub_pending_enrollments_pkey PRIMARY KEY (business_id, device_id, purpose);


--
-- Name: i18n_overrides i18n_overrides_locale_ns_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.i18n_overrides
    ADD CONSTRAINT i18n_overrides_locale_ns_key_key UNIQUE (locale, ns, key);


--
-- Name: i18n_overrides i18n_overrides_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.i18n_overrides
    ADD CONSTRAINT i18n_overrides_pkey PRIMARY KEY (id);


--
-- Name: incidents incidents_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.incidents
    ADD CONSTRAINT incidents_pkey PRIMARY KEY (id);


--
-- Name: magic_coin_transactions magic_coin_transactions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.magic_coin_transactions
    ADD CONSTRAINT magic_coin_transactions_pkey PRIMARY KEY (id);


--
-- Name: magic_coin_wallets magic_coin_wallets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.magic_coin_wallets
    ADD CONSTRAINT magic_coin_wallets_pkey PRIMARY KEY (user_id);


--
-- Name: magic_dude_knowledge_notes magic_dude_knowledge_notes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.magic_dude_knowledge_notes
    ADD CONSTRAINT magic_dude_knowledge_notes_pkey PRIMARY KEY (id);


--
-- Name: magic_dude_solved_cases magic_dude_solved_cases_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.magic_dude_solved_cases
    ADD CONSTRAINT magic_dude_solved_cases_pkey PRIMARY KEY (id);


--
-- Name: magic_media_publications magic_media_publications_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.magic_media_publications
    ADD CONSTRAINT magic_media_publications_pkey PRIMARY KEY (business_id);


--
-- Name: magic_media_publications magic_media_publications_public_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.magic_media_publications
    ADD CONSTRAINT magic_media_publications_public_id_key UNIQUE (public_id);


--
-- Name: management_address_migrations management_address_migrations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.management_address_migrations
    ADD CONSTRAINT management_address_migrations_pkey PRIMARY KEY (id);


--
-- Name: nats_subjects nats_subjects_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nats_subjects
    ADD CONSTRAINT nats_subjects_pkey PRIMARY KEY (business_id);


--
-- Name: nats_subjects nats_subjects_subject_prefix_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nats_subjects
    ADD CONSTRAINT nats_subjects_subject_prefix_key UNIQUE (subject_prefix);


--
-- Name: notification_prefs notification_prefs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notification_prefs
    ADD CONSTRAINT notification_prefs_pkey PRIMARY KEY (business_id);


--
-- Name: operator_feature_grants operator_feature_grants_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operator_feature_grants
    ADD CONSTRAINT operator_feature_grants_pkey PRIMARY KEY (user_id, feature);


--
-- Name: operator_feature_role_defaults operator_feature_role_defaults_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operator_feature_role_defaults
    ADD CONSTRAINT operator_feature_role_defaults_pkey PRIMARY KEY (legacy_role, feature);


--
-- Name: owner_daily_closes owner_daily_closes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.owner_daily_closes
    ADD CONSTRAINT owner_daily_closes_pkey PRIMARY KEY (id);


--
-- Name: peer_heartbeats peer_heartbeats_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.peer_heartbeats
    ADD CONSTRAINT peer_heartbeats_pkey PRIMARY KEY (id);


--
-- Name: platform_admins platform_admins_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.platform_admins
    ADD CONSTRAINT platform_admins_pkey PRIMARY KEY (user_id);


--
-- Name: platform_announcement platform_announcement_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.platform_announcement
    ADD CONSTRAINT platform_announcement_pkey PRIMARY KEY (id);


--
-- Name: platform_service_bank_accounts platform_service_bank_accounts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.platform_service_bank_accounts
    ADD CONSTRAINT platform_service_bank_accounts_pkey PRIMARY KEY (id);


--
-- Name: platform_service_bank_accounts platform_service_bank_accounts_slot_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.platform_service_bank_accounts
    ADD CONSTRAINT platform_service_bank_accounts_slot_key UNIQUE (slot);


--
-- Name: portal_configs portal_configs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.portal_configs
    ADD CONSTRAINT portal_configs_pkey PRIMARY KEY (business_id);


--
-- Name: portal_mode_role_defaults portal_mode_role_defaults_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.portal_mode_role_defaults
    ADD CONSTRAINT portal_mode_role_defaults_pkey PRIMARY KEY (legacy_role, mode);


--
-- Name: portal_sessions portal_sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.portal_sessions
    ADD CONSTRAINT portal_sessions_pkey PRIMARY KEY (id);


--
-- Name: pricing_promo pricing_promo_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pricing_promo
    ADD CONSTRAINT pricing_promo_pkey PRIMARY KEY (id);


--
-- Name: purchase_requests purchase_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_requests
    ADD CONSTRAINT purchase_requests_pkey PRIMARY KEY (id);


--
-- Name: router_connections router_connections_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.router_connections
    ADD CONSTRAINT router_connections_pkey PRIMARY KEY (id);


--
-- Name: ruijie_cloud_connections ruijie_cloud_connections_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ruijie_cloud_connections
    ADD CONSTRAINT ruijie_cloud_connections_pkey PRIMARY KEY (id);


--
-- Name: service_purchase_audit service_purchase_audit_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_purchase_audit
    ADD CONSTRAINT service_purchase_audit_pkey PRIMARY KEY (id);


--
-- Name: service_purchases service_purchases_business_id_user_id_idempotency_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_purchases
    ADD CONSTRAINT service_purchases_business_id_user_id_idempotency_key_key UNIQUE (business_id, user_id, idempotency_key);


--
-- Name: service_purchases service_purchases_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_purchases
    ADD CONSTRAINT service_purchases_pkey PRIMARY KEY (id);


--
-- Name: site_portals site_portals_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.site_portals
    ADD CONSTRAINT site_portals_pkey PRIMARY KEY (site_id);


--
-- Name: site_topology_config site_topology_config_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.site_topology_config
    ADD CONSTRAINT site_topology_config_pkey PRIMARY KEY (site_id, router_id);


--
-- Name: sites sites_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sites
    ADD CONSTRAINT sites_pkey PRIMARY KEY (site_id);


--
-- Name: store_addon_requests store_addon_requests_business_id_idempotency_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.store_addon_requests
    ADD CONSTRAINT store_addon_requests_business_id_idempotency_key_key UNIQUE (business_id, idempotency_key);


--
-- Name: store_addon_requests store_addon_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.store_addon_requests
    ADD CONSTRAINT store_addon_requests_pkey PRIMARY KEY (id);


--
-- Name: store_addons store_addons_addon_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.store_addons
    ADD CONSTRAINT store_addons_addon_key_key UNIQUE (addon_key);


--
-- Name: store_addons store_addons_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.store_addons
    ADD CONSTRAINT store_addons_pkey PRIMARY KEY (id);


--
-- Name: syslog_events syslog_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.syslog_events
    ADD CONSTRAINT syslog_events_pkey PRIMARY KEY (id);


--
-- Name: telemetry_samples telemetry_samples_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_samples
    ADD CONSTRAINT telemetry_samples_pkey PRIMARY KEY (id);


--
-- Name: terminal_history terminal_history_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.terminal_history
    ADD CONSTRAINT terminal_history_pkey PRIMARY KEY (id);


--
-- Name: terminal_templates terminal_templates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.terminal_templates
    ADD CONSTRAINT terminal_templates_pkey PRIMARY KEY (id);


--
-- Name: unlock_key_activations unlock_key_activations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.unlock_key_activations
    ADD CONSTRAINT unlock_key_activations_pkey PRIMARY KEY (id);


--
-- Name: user_profile_settings user_profile_settings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_profile_settings
    ADD CONSTRAINT user_profile_settings_pkey PRIMARY KEY (user_id);


--
-- Name: users users_email_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_email_key UNIQUE (email);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (user_id);


--
-- Name: voucher_print_layouts voucher_print_layouts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.voucher_print_layouts
    ADD CONSTRAINT voucher_print_layouts_pkey PRIMARY KEY (business_id);


--
-- Name: voucher_resellers voucher_resellers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.voucher_resellers
    ADD CONSTRAINT voucher_resellers_pkey PRIMARY KEY (id);


--
-- Name: vouchers vouchers_business_id_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vouchers
    ADD CONSTRAINT vouchers_business_id_code_key UNIQUE (business_id, code);


--
-- Name: vouchers vouchers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vouchers
    ADD CONSTRAINT vouchers_pkey PRIMARY KEY (id);


--
-- Name: webhook_deliveries webhook_deliveries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhook_deliveries
    ADD CONSTRAINT webhook_deliveries_pkey PRIMARY KEY (id);


--
-- Name: webhook_endpoints webhook_endpoints_business_id_url_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhook_endpoints
    ADD CONSTRAINT webhook_endpoints_business_id_url_key UNIQUE (business_id, url);


--
-- Name: webhook_endpoints webhook_endpoints_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhook_endpoints
    ADD CONSTRAINT webhook_endpoints_pkey PRIMARY KEY (id);


--
-- Name: account_pass_payment_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX account_pass_payment_key ON public.app_account_pass_renewals USING btree (user_id, idempotency_key) WHERE (idempotency_key IS NOT NULL);


--
-- Name: admin_notifications_recipient_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX admin_notifications_recipient_idx ON public.admin_notifications USING btree (recipient_user_id, created_at DESC);


--
-- Name: ai_usage_events_business_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ai_usage_events_business_idx ON public.ai_usage_events USING btree (business_id, created_at DESC);


--
-- Name: app_account_one_pending_renewal; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX app_account_one_pending_renewal ON public.app_account_pass_renewals USING btree (user_id) WHERE (status = 'pending'::text);


--
-- Name: auth_sessions_user_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX auth_sessions_user_id_idx ON public.auth_sessions USING btree (user_id);


--
-- Name: devices_id_business_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX devices_id_business_key ON public.devices USING btree (device_id, business_id);


--
-- Name: devices_migration_tenant_identity; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX devices_migration_tenant_identity ON public.devices USING btree (device_id, business_id);


--
-- Name: devices_ssh_target_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX devices_ssh_target_idx ON public.devices USING btree (device_id) WHERE ((management_ip IS NOT NULL) AND (ssh_username IS NOT NULL));


--
-- Name: fleet_scan_runs_business_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX fleet_scan_runs_business_idx ON public.fleet_scan_runs USING btree (business_id, generated_at DESC);


--
-- Name: guest_voucher_activation_business_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX guest_voucher_activation_business_time ON public.guest_voucher_grants USING btree (business_id, activated_at) WHERE (activated_at IS NOT NULL);


--
-- Name: guest_voucher_grants_expiry; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX guest_voucher_grants_expiry ON public.guest_voucher_grants USING btree (expires_at) WHERE (state = ANY (ARRAY['provisioning'::text, 'active'::text]));


--
-- Name: guest_voucher_grants_tenant_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX guest_voucher_grants_tenant_key ON public.guest_voucher_grants USING btree (voucher_id, business_id);


--
-- Name: guest_voucher_issue_items_batch; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX guest_voucher_issue_items_batch ON public.guest_voucher_issue_items USING btree (business_id, batch_id);


--
-- Name: guest_voucher_usage_device; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX guest_voucher_usage_device ON public.guest_voucher_usage USING btree (business_id, device_id);


--
-- Name: idx_audit_business_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_business_time ON public.audit_events USING btree (business_id, "timestamp" DESC);


--
-- Name: idx_audit_entity; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_entity ON public.audit_events USING btree (entity_type, entity_id, "timestamp" DESC);


--
-- Name: idx_audit_user_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_audit_user_time ON public.audit_events USING btree (user_id, "timestamp" DESC);


--
-- Name: idx_business_invitations_business; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_business_invitations_business ON public.business_invitations USING btree (business_id, created_at DESC);


--
-- Name: idx_business_invitations_email; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_business_invitations_email ON public.business_invitations USING btree (lower(email), expires_at);


--
-- Name: idx_business_memberships_business; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_business_memberships_business ON public.business_memberships USING btree (business_id, status, role);


--
-- Name: idx_business_memberships_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_business_memberships_user ON public.business_memberships USING btree (user_id, status);


--
-- Name: idx_businesses_access_tier; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_businesses_access_tier ON public.businesses USING btree (access_tier);


--
-- Name: idx_businesses_owner_email; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_businesses_owner_email ON public.businesses USING btree (owner_email);


--
-- Name: idx_device_backups_business_device; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_device_backups_business_device ON public.device_config_backups USING btree (business_id, device_id, created_at DESC);


--
-- Name: idx_device_change_events_business_device; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_device_change_events_business_device ON public.device_change_events USING btree (business_id, device_id, created_at DESC);


--
-- Name: idx_device_discovery_labels_device; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_device_discovery_labels_device ON public.device_discovery_labels USING btree (device_id);


--
-- Name: idx_device_discovery_labels_mac; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_device_discovery_labels_mac ON public.device_discovery_labels USING btree (mac_address);


--
-- Name: idx_devices_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_devices_business_id ON public.devices USING btree (business_id);


--
-- Name: idx_devices_is_online; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_devices_is_online ON public.devices USING btree (is_online);


--
-- Name: idx_devices_parent_gateway; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_devices_parent_gateway ON public.devices USING btree (parent_gateway_id);


--
-- Name: idx_guest_requests_business; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_guest_requests_business ON public.guest_purchase_requests USING btree (business_id);


--
-- Name: idx_guest_requests_grace_client; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_guest_requests_grace_client ON public.guest_purchase_requests USING btree (business_id, device_id, upper(client_mac), created_at DESC);


--
-- Name: idx_guest_requests_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_guest_requests_status ON public.guest_purchase_requests USING btree (status);


--
-- Name: idx_heartbeats_business_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_heartbeats_business_time ON public.peer_heartbeats USING btree (business_id, observed_at DESC);


--
-- Name: idx_heartbeats_peer; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_heartbeats_peer ON public.peer_heartbeats USING btree (peer_id, observed_at DESC);


--
-- Name: idx_incidents_business_open; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_incidents_business_open ON public.incidents USING btree (business_id, resolved_at) WHERE (resolved_at IS NULL);


--
-- Name: idx_incidents_device; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_incidents_device ON public.incidents USING btree (device_id, opened_at DESC);


--
-- Name: idx_portal_sessions_business; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_portal_sessions_business ON public.portal_sessions USING btree (business_id, authenticated_at DESC);


--
-- Name: idx_portal_sessions_client_mac; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_portal_sessions_client_mac ON public.portal_sessions USING btree (client_mac);


--
-- Name: idx_purchase_requests_bank_ref_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_purchase_requests_bank_ref_unique ON public.purchase_requests USING btree (business_id, lower(bank_reference)) WHERE (status <> 'rejected'::text);


--
-- Name: idx_purchase_requests_business; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_purchase_requests_business ON public.purchase_requests USING btree (business_id, status, created_at DESC);


--
-- Name: idx_telemetry_business_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_telemetry_business_time ON public.telemetry_samples USING btree (business_id, observed_at DESC);


--
-- Name: idx_telemetry_device_time; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_telemetry_device_time ON public.telemetry_samples USING btree (device_id, observed_at DESC);


--
-- Name: idx_telemetry_subject; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_telemetry_subject ON public.telemetry_samples USING btree (business_id, subject_kind, subject_id, observed_at DESC);


--
-- Name: idx_user_profile_settings_business; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_profile_settings_business ON public.user_profile_settings USING btree (business_id);


--
-- Name: idx_users_business_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_business_id ON public.users USING btree (business_id);


--
-- Name: idx_users_email; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_email ON public.users USING btree (email);


--
-- Name: idx_vouchers_business_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_vouchers_business_status ON public.vouchers USING btree (business_id, status, expires_at);


--
-- Name: idx_vouchers_device_mac; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_vouchers_device_mac ON public.vouchers USING btree (device_mac);


--
-- Name: idx_webhook_deliveries_business; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_webhook_deliveries_business ON public.webhook_deliveries USING btree (business_id);


--
-- Name: idx_webhook_deliveries_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_webhook_deliveries_status ON public.webhook_deliveries USING btree (status);


--
-- Name: idx_webhook_deliveries_webhook; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_webhook_deliveries_webhook ON public.webhook_deliveries USING btree (webhook_id);


--
-- Name: idx_webhook_endpoints_business; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_webhook_endpoints_business ON public.webhook_endpoints USING btree (business_id);


--
-- Name: magic_coin_tx_service_payment_once; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX magic_coin_tx_service_payment_once ON public.magic_coin_transactions USING btree (service_purchase_id) WHERE (kind = 'service_payment'::text);


--
-- Name: magic_coin_tx_service_refund_once; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX magic_coin_tx_service_refund_once ON public.magic_coin_transactions USING btree (service_purchase_id) WHERE (kind = 'service_refund'::text);


--
-- Name: magic_coin_tx_user_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX magic_coin_tx_user_created_idx ON public.magic_coin_transactions USING btree (user_id, created_at DESC);


--
-- Name: management_migration_one_active; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX management_migration_one_active ON public.management_address_migrations USING btree (device_id) WHERE (phase <> ALL (ARRAY['completed'::text, 'rolled_back'::text]));


--
-- Name: owner_daily_closes_business_day_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX owner_daily_closes_business_day_unique ON public.owner_daily_closes USING btree (business_id, business_day);


--
-- Name: owner_daily_closes_business_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX owner_daily_closes_business_idx ON public.owner_daily_closes USING btree (business_id, business_day DESC);


--
-- Name: reseller_actor_tenant_identity; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX reseller_actor_tenant_identity ON public.users USING btree (user_id, business_id);


--
-- Name: router_connections_business_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX router_connections_business_idx ON public.router_connections USING btree (business_id);


--
-- Name: service_purchase_audit_tenant; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX service_purchase_audit_tenant ON public.service_purchase_audit USING btree (business_id, purchase_id, created_at DESC);


--
-- Name: service_purchases_bank_reference_once; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX service_purchases_bank_reference_once ON public.service_purchases USING btree (business_id, lower(bank_reference)) WHERE ((bank_reference IS NOT NULL) AND (status <> 'rejected'::text));


--
-- Name: service_purchases_one_pending; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX service_purchases_one_pending ON public.service_purchases USING btree (business_id, user_id, service_key) WHERE (status = 'pending'::text);


--
-- Name: service_purchases_tenant_history; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX service_purchases_tenant_history ON public.service_purchases USING btree (business_id, created_at DESC);


--
-- Name: site_portals_business_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX site_portals_business_idx ON public.site_portals USING btree (business_id);


--
-- Name: sites_business_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX sites_business_idx ON public.sites USING btree (business_id);


--
-- Name: sites_site_business_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX sites_site_business_unique ON public.sites USING btree (site_id, business_id);


--
-- Name: store_addon_one_pending; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX store_addon_one_pending ON public.store_addon_requests USING btree (business_id, addon_id) WHERE (status = 'pending'::text);


--
-- Name: store_addon_requests_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX store_addon_requests_status ON public.store_addon_requests USING btree (status, created_at DESC);


--
-- Name: syslog_events_business_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX syslog_events_business_idx ON public.syslog_events USING btree (business_id, received_at DESC);


--
-- Name: terminal_history_business_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX terminal_history_business_idx ON public.terminal_history USING btree (business_id, created_at DESC);


--
-- Name: terminal_templates_business_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX terminal_templates_business_idx ON public.terminal_templates USING btree (business_id, category);


--
-- Name: unlock_key_activations_user_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX unlock_key_activations_user_idx ON public.unlock_key_activations USING btree (user_id, created_at DESC);


--
-- Name: uq_business_pending_invitation; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX uq_business_pending_invitation ON public.business_invitations USING btree (business_id, lower(email)) WHERE ((accepted_at IS NULL) AND (revoked_at IS NULL));


--
-- Name: users_user_business_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX users_user_business_key ON public.users USING btree (user_id, business_id);


--
-- Name: voucher_reseller_tenant_identity; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX voucher_reseller_tenant_identity ON public.voucher_resellers USING btree (id, business_id);


--
-- Name: voucher_resellers_business_prefix_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX voucher_resellers_business_prefix_unique ON public.voucher_resellers USING btree (business_id, upper(code_prefix));


--
-- Name: vouchers_id_business_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX vouchers_id_business_key ON public.vouchers USING btree (id, business_id);


--
-- Name: devices app_router_capacity; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER app_router_capacity BEFORE INSERT OR UPDATE OF business_id, deployment_layer ON public.devices FOR EACH ROW EXECUTE FUNCTION public.enforce_business_app_resource_cap();


--
-- Name: sites app_site_capacity; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER app_site_capacity BEFORE INSERT OR UPDATE OF business_id ON public.sites FOR EACH ROW EXECUTE FUNCTION public.enforce_business_app_resource_cap();


--
-- Name: devices guard_hub_management_address_move; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER guard_hub_management_address_move BEFORE UPDATE OF management_ip, transport ON public.devices FOR EACH ROW EXECUTE FUNCTION public.guard_hub_management_address_move();


--
-- Name: devices guard_pending_management_device_delete; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER guard_pending_management_device_delete BEFORE DELETE ON public.devices FOR EACH ROW EXECUTE FUNCTION public.guard_pending_management_device_delete();


--
-- Name: guest_access_recovery guest_access_recovery_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER guest_access_recovery_guard BEFORE UPDATE ON public.guest_access_recovery FOR EACH ROW EXECUTE FUNCTION public.guard_guest_access_recovery();


--
-- Name: guest_voucher_grants guest_voucher_activation_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER guest_voucher_activation_guard BEFORE UPDATE ON public.guest_voucher_grants FOR EACH ROW EXECUTE FUNCTION public.guard_guest_voucher_activation();


--
-- Name: guest_voucher_issue_batches guest_voucher_issue_batch_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER guest_voucher_issue_batch_guard BEFORE UPDATE ON public.guest_voucher_issue_batches FOR EACH ROW EXECUTE FUNCTION public.guard_guest_voucher_issue_batch();


--
-- Name: guest_voucher_issue_items guest_voucher_issue_item_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER guest_voucher_issue_item_guard BEFORE UPDATE ON public.guest_voucher_issue_items FOR EACH ROW EXECUTE FUNCTION public.guard_guest_voucher_issue_item();


--
-- Name: guest_voucher_grants guest_voucher_snapshot_guard; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER guest_voucher_snapshot_guard BEFORE UPDATE ON public.guest_voucher_grants FOR EACH ROW EXECUTE FUNCTION public.guard_guest_voucher_snapshot();


--
-- Name: users new_business_primary_login; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER new_business_primary_login AFTER INSERT ON public.users FOR EACH ROW EXECUTE FUNCTION public.assign_new_business_primary_login();


--
-- Name: users revoke_changed_user_sessions; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER revoke_changed_user_sessions AFTER UPDATE ON public.users FOR EACH ROW EXECUTE FUNCTION public.revoke_changed_user_sessions();


--
-- Name: businesses revoke_suspended_business_sessions; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER revoke_suspended_business_sessions AFTER UPDATE ON public.businesses FOR EACH ROW EXECUTE FUNCTION public.revoke_suspended_business_sessions();


--
-- Name: magic_coin_transactions trg_magic_coin_transaction_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_magic_coin_transaction_immutable BEFORE DELETE OR UPDATE ON public.magic_coin_transactions FOR EACH ROW EXECUTE FUNCTION public.magic_coin_transaction_immutable();


--
-- Name: users trg_user_magic_coin_wallet; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_user_magic_coin_wallet AFTER INSERT ON public.users FOR EACH ROW EXECUTE FUNCTION public.create_magic_coin_wallet_for_user();


--
-- Name: alert_policies update_alert_policies_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER update_alert_policies_updated_at BEFORE UPDATE ON public.alert_policies FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- Name: businesses update_businesses_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER update_businesses_updated_at BEFORE UPDATE ON public.businesses FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- Name: devices update_devices_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER update_devices_updated_at BEFORE UPDATE ON public.devices FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- Name: incidents update_incidents_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER update_incidents_updated_at BEFORE UPDATE ON public.incidents FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- Name: portal_configs update_portal_configs_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER update_portal_configs_updated_at BEFORE UPDATE ON public.portal_configs FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- Name: users update_users_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON public.users FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- Name: vouchers update_vouchers_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER update_vouchers_updated_at BEFORE UPDATE ON public.vouchers FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- Name: account_entitlements account_entitlements_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_entitlements
    ADD CONSTRAINT account_entitlements_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: account_entitlements account_entitlements_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_entitlements
    ADD CONSTRAINT account_entitlements_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON DELETE CASCADE;


--
-- Name: admin_notifications admin_notifications_recipient_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.admin_notifications
    ADD CONSTRAINT admin_notifications_recipient_user_id_fkey FOREIGN KEY (recipient_user_id) REFERENCES public.users(user_id) ON DELETE CASCADE;


--
-- Name: ai_usage_events ai_usage_events_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_usage_events
    ADD CONSTRAINT ai_usage_events_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: ai_usage_events ai_usage_events_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ai_usage_events
    ADD CONSTRAINT ai_usage_events_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON DELETE SET NULL;


--
-- Name: alert_policies alert_policies_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.alert_policies
    ADD CONSTRAINT alert_policies_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: app_account_pass_events app_account_pass_events_actor_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.app_account_pass_events
    ADD CONSTRAINT app_account_pass_events_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES public.users(user_id);


--
-- Name: app_account_pass_events app_account_pass_events_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.app_account_pass_events
    ADD CONSTRAINT app_account_pass_events_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id);


--
-- Name: app_account_pass_events app_account_pass_events_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.app_account_pass_events
    ADD CONSTRAINT app_account_pass_events_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(user_id);


--
-- Name: app_account_pass_renewals app_account_pass_renewals_bank_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.app_account_pass_renewals
    ADD CONSTRAINT app_account_pass_renewals_bank_account_id_fkey FOREIGN KEY (bank_account_id) REFERENCES public.platform_service_bank_accounts(id);


--
-- Name: app_account_pass_renewals app_account_pass_renewals_decided_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.app_account_pass_renewals
    ADD CONSTRAINT app_account_pass_renewals_decided_by_fkey FOREIGN KEY (decided_by) REFERENCES public.users(user_id);


--
-- Name: app_account_pass_renewals app_account_pass_renewals_user_id_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.app_account_pass_renewals
    ADD CONSTRAINT app_account_pass_renewals_user_id_business_id_fkey FOREIGN KEY (user_id, business_id) REFERENCES public.users(user_id, business_id);


--
-- Name: app_account_pass_transitions app_account_pass_transitions_rollout_key_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.app_account_pass_transitions
    ADD CONSTRAINT app_account_pass_transitions_rollout_key_fkey FOREIGN KEY (rollout_key) REFERENCES public.app_account_pass_rollouts(rollout_key);


--
-- Name: app_account_pass_transitions app_account_pass_transitions_user_id_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.app_account_pass_transitions
    ADD CONSTRAINT app_account_pass_transitions_user_id_business_id_fkey FOREIGN KEY (user_id, business_id) REFERENCES public.users(user_id, business_id);


--
-- Name: app_account_passes app_account_passes_recorded_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.app_account_passes
    ADD CONSTRAINT app_account_passes_recorded_by_fkey FOREIGN KEY (recorded_by) REFERENCES public.users(user_id);


--
-- Name: app_account_passes app_account_passes_user_id_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.app_account_passes
    ADD CONSTRAINT app_account_passes_user_id_business_id_fkey FOREIGN KEY (user_id, business_id) REFERENCES public.users(user_id, business_id);


--
-- Name: app_account_trials app_account_trials_user_id_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.app_account_trials
    ADD CONSTRAINT app_account_trials_user_id_business_id_fkey FOREIGN KEY (user_id, business_id) REFERENCES public.users(user_id, business_id);


--
-- Name: audit_events audit_events_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_events
    ADD CONSTRAINT audit_events_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: audit_events audit_events_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.audit_events
    ADD CONSTRAINT audit_events_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON DELETE SET NULL;


--
-- Name: auth_sessions auth_sessions_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auth_sessions
    ADD CONSTRAINT auth_sessions_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: auth_sessions auth_sessions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.auth_sessions
    ADD CONSTRAINT auth_sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON DELETE CASCADE;


--
-- Name: business_app_primary_events business_app_primary_events_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_app_primary_events
    ADD CONSTRAINT business_app_primary_events_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id);


--
-- Name: business_app_primary_events business_app_primary_events_changed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_app_primary_events
    ADD CONSTRAINT business_app_primary_events_changed_by_fkey FOREIGN KEY (changed_by) REFERENCES public.users(user_id);


--
-- Name: business_app_primary_events business_app_primary_events_user_id_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_app_primary_events
    ADD CONSTRAINT business_app_primary_events_user_id_business_id_fkey FOREIGN KEY (user_id, business_id) REFERENCES public.users(user_id, business_id);


--
-- Name: business_app_primary_users business_app_primary_users_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_app_primary_users
    ADD CONSTRAINT business_app_primary_users_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id);


--
-- Name: business_app_primary_users business_app_primary_users_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_app_primary_users
    ADD CONSTRAINT business_app_primary_users_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(user_id);


--
-- Name: business_app_primary_users business_app_primary_users_user_id_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_app_primary_users
    ADD CONSTRAINT business_app_primary_users_user_id_business_id_fkey FOREIGN KEY (user_id, business_id) REFERENCES public.users(user_id, business_id);


--
-- Name: business_invitations business_invitations_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_invitations
    ADD CONSTRAINT business_invitations_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: business_invitations business_invitations_invited_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_invitations
    ADD CONSTRAINT business_invitations_invited_by_fkey FOREIGN KEY (invited_by) REFERENCES public.users(user_id) ON DELETE RESTRICT;


--
-- Name: business_memberships business_memberships_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_memberships
    ADD CONSTRAINT business_memberships_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: business_memberships business_memberships_invited_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_memberships
    ADD CONSTRAINT business_memberships_invited_by_fkey FOREIGN KEY (invited_by) REFERENCES public.users(user_id) ON DELETE SET NULL;


--
-- Name: business_memberships business_memberships_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_memberships
    ADD CONSTRAINT business_memberships_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON DELETE CASCADE;


--
-- Name: business_metered_quotas business_metered_quotas_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_metered_quotas
    ADD CONSTRAINT business_metered_quotas_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: business_telegram_bots business_telegram_bots_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.business_telegram_bots
    ADD CONSTRAINT business_telegram_bots_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: device_allowances device_allowances_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_allowances
    ADD CONSTRAINT device_allowances_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: device_allowances device_allowances_included_router_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_allowances
    ADD CONSTRAINT device_allowances_included_router_id_fkey FOREIGN KEY (included_router_id) REFERENCES public.router_connections(id) ON DELETE SET NULL;


--
-- Name: device_change_events device_change_events_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_change_events
    ADD CONSTRAINT device_change_events_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: device_change_events device_change_events_device_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_change_events
    ADD CONSTRAINT device_change_events_device_id_fkey FOREIGN KEY (device_id) REFERENCES public.devices(device_id) ON DELETE CASCADE;


--
-- Name: device_config_backups device_config_backups_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_config_backups
    ADD CONSTRAINT device_config_backups_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: device_config_backups device_config_backups_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_config_backups
    ADD CONSTRAINT device_config_backups_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(user_id) ON DELETE SET NULL;


--
-- Name: device_config_backups device_config_backups_device_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_config_backups
    ADD CONSTRAINT device_config_backups_device_id_fkey FOREIGN KEY (device_id) REFERENCES public.devices(device_id) ON DELETE CASCADE;


--
-- Name: device_discovery_labels device_discovery_labels_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_discovery_labels
    ADD CONSTRAINT device_discovery_labels_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: device_discovery_labels device_discovery_labels_device_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_discovery_labels
    ADD CONSTRAINT device_discovery_labels_device_id_fkey FOREIGN KEY (device_id) REFERENCES public.devices(device_id) ON DELETE CASCADE;


--
-- Name: device_requests device_requests_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_requests
    ADD CONSTRAINT device_requests_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: device_requests device_requests_decided_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_requests
    ADD CONSTRAINT device_requests_decided_by_fkey FOREIGN KEY (decided_by) REFERENCES public.users(user_id) ON DELETE SET NULL;


--
-- Name: device_requests device_requests_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_requests
    ADD CONSTRAINT device_requests_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON DELETE SET NULL;


--
-- Name: devices devices_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.devices
    ADD CONSTRAINT devices_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: devices devices_parent_gateway_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.devices
    ADD CONSTRAINT devices_parent_gateway_id_fkey FOREIGN KEY (parent_gateway_id) REFERENCES public.devices(device_id) ON DELETE SET NULL;


--
-- Name: devices devices_router_connection_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.devices
    ADD CONSTRAINT devices_router_connection_id_fkey FOREIGN KEY (router_connection_id) REFERENCES public.router_connections(id) ON DELETE SET NULL;


--
-- Name: devices devices_site_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.devices
    ADD CONSTRAINT devices_site_id_fkey FOREIGN KEY (site_id) REFERENCES public.sites(site_id) ON DELETE SET NULL;


--
-- Name: device_discovery_labels discovery_label_device_tenant; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_discovery_labels
    ADD CONSTRAINT discovery_label_device_tenant FOREIGN KEY (device_id, business_id) REFERENCES public.devices(device_id, business_id) ON DELETE CASCADE;


--
-- Name: fleet_scan_runs fleet_scan_runs_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fleet_scan_runs
    ADD CONSTRAINT fleet_scan_runs_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: fleet_scan_runs fleet_scan_runs_triggered_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.fleet_scan_runs
    ADD CONSTRAINT fleet_scan_runs_triggered_by_fkey FOREIGN KEY (triggered_by) REFERENCES public.users(user_id) ON DELETE SET NULL;


--
-- Name: guest_access_recovery guest_access_recovery_device_id_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_access_recovery
    ADD CONSTRAINT guest_access_recovery_device_id_business_id_fkey FOREIGN KEY (device_id, business_id) REFERENCES public.devices(device_id, business_id);


--
-- Name: guest_access_recovery guest_access_recovery_voucher_id_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_access_recovery
    ADD CONSTRAINT guest_access_recovery_voucher_id_business_id_fkey FOREIGN KEY (voucher_id, business_id) REFERENCES public.guest_voucher_grants(voucher_id, business_id);


--
-- Name: guest_purchase_requests guest_purchase_requests_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_purchase_requests
    ADD CONSTRAINT guest_purchase_requests_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: guest_voucher_batch_reseller_events guest_voucher_batch_reseller_events_actor_id_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_voucher_batch_reseller_events
    ADD CONSTRAINT guest_voucher_batch_reseller_events_actor_id_business_id_fkey FOREIGN KEY (actor_id, business_id) REFERENCES public.users(user_id, business_id);


--
-- Name: guest_voucher_batch_reseller_events guest_voucher_batch_reseller_events_business_id_batch_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_voucher_batch_reseller_events
    ADD CONSTRAINT guest_voucher_batch_reseller_events_business_id_batch_id_fkey FOREIGN KEY (business_id, batch_id) REFERENCES public.guest_voucher_issue_batches(business_id, id) ON DELETE CASCADE;


--
-- Name: guest_voucher_batch_resellers guest_voucher_batch_resellers_assigned_by_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_voucher_batch_resellers
    ADD CONSTRAINT guest_voucher_batch_resellers_assigned_by_business_id_fkey FOREIGN KEY (assigned_by, business_id) REFERENCES public.users(user_id, business_id);


--
-- Name: guest_voucher_batch_resellers guest_voucher_batch_resellers_business_id_batch_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_voucher_batch_resellers
    ADD CONSTRAINT guest_voucher_batch_resellers_business_id_batch_id_fkey FOREIGN KEY (business_id, batch_id) REFERENCES public.guest_voucher_issue_batches(business_id, id) ON DELETE CASCADE;


--
-- Name: guest_voucher_batch_resellers guest_voucher_batch_resellers_reseller_id_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_voucher_batch_resellers
    ADD CONSTRAINT guest_voucher_batch_resellers_reseller_id_business_id_fkey FOREIGN KEY (reseller_id, business_id) REFERENCES public.voucher_resellers(id, business_id) ON DELETE RESTRICT;


--
-- Name: guest_voucher_grants guest_voucher_grants_device_id_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_voucher_grants
    ADD CONSTRAINT guest_voucher_grants_device_id_business_id_fkey FOREIGN KEY (device_id, business_id) REFERENCES public.devices(device_id, business_id);


--
-- Name: guest_voucher_grants guest_voucher_grants_voucher_id_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_voucher_grants
    ADD CONSTRAINT guest_voucher_grants_voucher_id_business_id_fkey FOREIGN KEY (voucher_id, business_id) REFERENCES public.vouchers(id, business_id);


--
-- Name: guest_voucher_issue_batches guest_voucher_issue_batches_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_voucher_issue_batches
    ADD CONSTRAINT guest_voucher_issue_batches_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id);


--
-- Name: guest_voucher_issue_batches guest_voucher_issue_batches_device_id_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_voucher_issue_batches
    ADD CONSTRAINT guest_voucher_issue_batches_device_id_business_id_fkey FOREIGN KEY (device_id, business_id) REFERENCES public.devices(device_id, business_id);


--
-- Name: guest_voucher_issue_items guest_voucher_issue_items_business_id_batch_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_voucher_issue_items
    ADD CONSTRAINT guest_voucher_issue_items_business_id_batch_id_fkey FOREIGN KEY (business_id, batch_id) REFERENCES public.guest_voucher_issue_batches(business_id, id);


--
-- Name: guest_voucher_issue_items guest_voucher_issue_items_voucher_id_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_voucher_issue_items
    ADD CONSTRAINT guest_voucher_issue_items_voucher_id_business_id_fkey FOREIGN KEY (voucher_id, business_id) REFERENCES public.vouchers(id, business_id);


--
-- Name: guest_voucher_usage guest_voucher_usage_voucher_id_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.guest_voucher_usage
    ADD CONSTRAINT guest_voucher_usage_voucher_id_business_id_fkey FOREIGN KEY (voucher_id, business_id) REFERENCES public.vouchers(id, business_id) ON DELETE CASCADE;


--
-- Name: hub_pending_enrollments hub_pending_enrollments_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hub_pending_enrollments
    ADD CONSTRAINT hub_pending_enrollments_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id);


--
-- Name: hub_pending_enrollments hub_pending_enrollments_device_id_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.hub_pending_enrollments
    ADD CONSTRAINT hub_pending_enrollments_device_id_business_id_fkey FOREIGN KEY (device_id, business_id) REFERENCES public.devices(device_id, business_id) ON DELETE CASCADE;


--
-- Name: i18n_overrides i18n_overrides_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.i18n_overrides
    ADD CONSTRAINT i18n_overrides_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(user_id) ON DELETE SET NULL;


--
-- Name: incidents incidents_acknowledged_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.incidents
    ADD CONSTRAINT incidents_acknowledged_by_fkey FOREIGN KEY (acknowledged_by) REFERENCES public.users(user_id) ON DELETE SET NULL;


--
-- Name: incidents incidents_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.incidents
    ADD CONSTRAINT incidents_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: incidents incidents_device_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.incidents
    ADD CONSTRAINT incidents_device_id_fkey FOREIGN KEY (device_id) REFERENCES public.devices(device_id) ON DELETE SET NULL;


--
-- Name: incidents incidents_policy_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.incidents
    ADD CONSTRAINT incidents_policy_id_fkey FOREIGN KEY (policy_id) REFERENCES public.alert_policies(id) ON DELETE SET NULL;


--
-- Name: incidents incidents_resolved_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.incidents
    ADD CONSTRAINT incidents_resolved_by_fkey FOREIGN KEY (resolved_by) REFERENCES public.users(user_id) ON DELETE SET NULL;


--
-- Name: magic_coin_transactions magic_coin_transactions_actor_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.magic_coin_transactions
    ADD CONSTRAINT magic_coin_transactions_actor_user_id_fkey FOREIGN KEY (actor_user_id) REFERENCES public.users(user_id) ON DELETE SET NULL;


--
-- Name: magic_coin_transactions magic_coin_transactions_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.magic_coin_transactions
    ADD CONSTRAINT magic_coin_transactions_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: magic_coin_transactions magic_coin_transactions_service_purchase_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.magic_coin_transactions
    ADD CONSTRAINT magic_coin_transactions_service_purchase_id_fkey FOREIGN KEY (service_purchase_id) REFERENCES public.purchase_requests(id) ON DELETE SET NULL;


--
-- Name: magic_coin_transactions magic_coin_transactions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.magic_coin_transactions
    ADD CONSTRAINT magic_coin_transactions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON DELETE CASCADE;


--
-- Name: magic_coin_wallets magic_coin_wallets_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.magic_coin_wallets
    ADD CONSTRAINT magic_coin_wallets_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: magic_coin_wallets magic_coin_wallets_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.magic_coin_wallets
    ADD CONSTRAINT magic_coin_wallets_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON DELETE CASCADE;


--
-- Name: magic_dude_knowledge_notes magic_dude_knowledge_notes_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.magic_dude_knowledge_notes
    ADD CONSTRAINT magic_dude_knowledge_notes_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(user_id) ON DELETE SET NULL;


--
-- Name: magic_dude_solved_cases magic_dude_solved_cases_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.magic_dude_solved_cases
    ADD CONSTRAINT magic_dude_solved_cases_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: magic_dude_solved_cases magic_dude_solved_cases_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.magic_dude_solved_cases
    ADD CONSTRAINT magic_dude_solved_cases_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON DELETE SET NULL;


--
-- Name: magic_media_publications magic_media_publications_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.magic_media_publications
    ADD CONSTRAINT magic_media_publications_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: management_address_migrations management_address_migrations_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.management_address_migrations
    ADD CONSTRAINT management_address_migrations_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id);


--
-- Name: management_address_migrations management_address_migrations_device_id_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.management_address_migrations
    ADD CONSTRAINT management_address_migrations_device_id_business_id_fkey FOREIGN KEY (device_id, business_id) REFERENCES public.devices(device_id, business_id) ON DELETE CASCADE;


--
-- Name: management_address_migrations management_address_migrations_requested_by_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.management_address_migrations
    ADD CONSTRAINT management_address_migrations_requested_by_business_id_fkey FOREIGN KEY (requested_by, business_id) REFERENCES public.users(user_id, business_id);


--
-- Name: nats_subjects nats_subjects_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nats_subjects
    ADD CONSTRAINT nats_subjects_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: notification_prefs notification_prefs_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notification_prefs
    ADD CONSTRAINT notification_prefs_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: operator_feature_grants operator_feature_grants_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operator_feature_grants
    ADD CONSTRAINT operator_feature_grants_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: operator_feature_grants operator_feature_grants_granted_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operator_feature_grants
    ADD CONSTRAINT operator_feature_grants_granted_by_fkey FOREIGN KEY (granted_by) REFERENCES public.users(user_id) ON DELETE SET NULL;


--
-- Name: operator_feature_grants operator_feature_grants_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operator_feature_grants
    ADD CONSTRAINT operator_feature_grants_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON DELETE CASCADE;


--
-- Name: operator_feature_role_defaults operator_feature_role_defaults_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.operator_feature_role_defaults
    ADD CONSTRAINT operator_feature_role_defaults_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(user_id) ON DELETE SET NULL;


--
-- Name: owner_daily_closes owner_daily_closes_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.owner_daily_closes
    ADD CONSTRAINT owner_daily_closes_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: owner_daily_closes owner_daily_closes_closed_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.owner_daily_closes
    ADD CONSTRAINT owner_daily_closes_closed_by_fkey FOREIGN KEY (closed_by) REFERENCES public.users(user_id) ON DELETE SET NULL;


--
-- Name: owner_daily_closes owner_daily_closes_router_connection_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.owner_daily_closes
    ADD CONSTRAINT owner_daily_closes_router_connection_id_fkey FOREIGN KEY (router_connection_id) REFERENCES public.router_connections(id) ON DELETE SET NULL;


--
-- Name: owner_daily_closes owner_daily_closes_site_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.owner_daily_closes
    ADD CONSTRAINT owner_daily_closes_site_id_fkey FOREIGN KEY (site_id) REFERENCES public.sites(site_id) ON DELETE SET NULL;


--
-- Name: peer_heartbeats peer_heartbeats_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.peer_heartbeats
    ADD CONSTRAINT peer_heartbeats_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: peer_heartbeats peer_heartbeats_device_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.peer_heartbeats
    ADD CONSTRAINT peer_heartbeats_device_id_fkey FOREIGN KEY (device_id) REFERENCES public.devices(device_id) ON DELETE SET NULL;


--
-- Name: platform_admins platform_admins_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.platform_admins
    ADD CONSTRAINT platform_admins_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON DELETE CASCADE;


--
-- Name: platform_announcement platform_announcement_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.platform_announcement
    ADD CONSTRAINT platform_announcement_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(user_id) ON DELETE SET NULL;


--
-- Name: platform_service_bank_accounts platform_service_bank_accounts_configured_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.platform_service_bank_accounts
    ADD CONSTRAINT platform_service_bank_accounts_configured_by_fkey FOREIGN KEY (configured_by) REFERENCES public.users(user_id) ON DELETE SET NULL;


--
-- Name: portal_configs portal_configs_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.portal_configs
    ADD CONSTRAINT portal_configs_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: portal_mode_role_defaults portal_mode_role_defaults_updated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.portal_mode_role_defaults
    ADD CONSTRAINT portal_mode_role_defaults_updated_by_fkey FOREIGN KEY (updated_by) REFERENCES public.users(user_id) ON DELETE SET NULL;


--
-- Name: portal_sessions portal_sessions_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.portal_sessions
    ADD CONSTRAINT portal_sessions_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: portal_sessions portal_sessions_device_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.portal_sessions
    ADD CONSTRAINT portal_sessions_device_id_fkey FOREIGN KEY (device_id) REFERENCES public.devices(device_id) ON DELETE SET NULL;


--
-- Name: portal_sessions portal_sessions_voucher_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.portal_sessions
    ADD CONSTRAINT portal_sessions_voucher_id_fkey FOREIGN KEY (voucher_id) REFERENCES public.vouchers(id) ON DELETE SET NULL;


--
-- Name: purchase_requests purchase_requests_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_requests
    ADD CONSTRAINT purchase_requests_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: purchase_requests purchase_requests_device_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.purchase_requests
    ADD CONSTRAINT purchase_requests_device_id_fkey FOREIGN KEY (device_id) REFERENCES public.devices(device_id) ON DELETE SET NULL;


--
-- Name: router_connections router_connections_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.router_connections
    ADD CONSTRAINT router_connections_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: router_connections router_connections_site_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.router_connections
    ADD CONSTRAINT router_connections_site_id_fkey FOREIGN KEY (site_id) REFERENCES public.sites(site_id) ON DELETE SET NULL;


--
-- Name: ruijie_cloud_connections ruijie_cloud_connections_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ruijie_cloud_connections
    ADD CONSTRAINT ruijie_cloud_connections_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: ruijie_cloud_connections ruijie_cloud_connections_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ruijie_cloud_connections
    ADD CONSTRAINT ruijie_cloud_connections_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(user_id) ON DELETE SET NULL;


--
-- Name: service_purchase_audit service_purchase_audit_actor_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_purchase_audit
    ADD CONSTRAINT service_purchase_audit_actor_user_id_fkey FOREIGN KEY (actor_user_id) REFERENCES public.users(user_id) ON DELETE RESTRICT;


--
-- Name: service_purchase_audit service_purchase_audit_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_purchase_audit
    ADD CONSTRAINT service_purchase_audit_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: service_purchase_audit service_purchase_audit_purchase_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_purchase_audit
    ADD CONSTRAINT service_purchase_audit_purchase_id_fkey FOREIGN KEY (purchase_id) REFERENCES public.service_purchases(id) ON DELETE RESTRICT;


--
-- Name: service_purchases service_purchases_bank_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_purchases
    ADD CONSTRAINT service_purchases_bank_account_id_fkey FOREIGN KEY (bank_account_id) REFERENCES public.platform_service_bank_accounts(id) ON DELETE RESTRICT;


--
-- Name: service_purchases service_purchases_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_purchases
    ADD CONSTRAINT service_purchases_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: service_purchases service_purchases_decided_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_purchases
    ADD CONSTRAINT service_purchases_decided_by_fkey FOREIGN KEY (decided_by) REFERENCES public.users(user_id) ON DELETE RESTRICT;


--
-- Name: service_purchases service_purchases_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_purchases
    ADD CONSTRAINT service_purchases_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON DELETE RESTRICT;


--
-- Name: site_portals site_portals_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.site_portals
    ADD CONSTRAINT site_portals_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: site_portals site_portals_business_site_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.site_portals
    ADD CONSTRAINT site_portals_business_site_fk FOREIGN KEY (site_id, business_id) REFERENCES public.sites(site_id, business_id) ON DELETE CASCADE;


--
-- Name: site_portals site_portals_site_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.site_portals
    ADD CONSTRAINT site_portals_site_id_fkey FOREIGN KEY (site_id) REFERENCES public.sites(site_id) ON DELETE CASCADE;


--
-- Name: site_topology_config site_topology_config_router_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.site_topology_config
    ADD CONSTRAINT site_topology_config_router_id_fkey FOREIGN KEY (router_id) REFERENCES public.router_connections(id) ON DELETE CASCADE;


--
-- Name: site_topology_config site_topology_config_site_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.site_topology_config
    ADD CONSTRAINT site_topology_config_site_id_fkey FOREIGN KEY (site_id) REFERENCES public.sites(site_id) ON DELETE CASCADE;


--
-- Name: sites sites_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sites
    ADD CONSTRAINT sites_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: store_addon_requests store_addon_requests_addon_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.store_addon_requests
    ADD CONSTRAINT store_addon_requests_addon_id_fkey FOREIGN KEY (addon_id) REFERENCES public.store_addons(id) ON DELETE RESTRICT;


--
-- Name: store_addon_requests store_addon_requests_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.store_addon_requests
    ADD CONSTRAINT store_addon_requests_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: store_addon_requests store_addon_requests_user_id_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.store_addon_requests
    ADD CONSTRAINT store_addon_requests_user_id_business_id_fkey FOREIGN KEY (user_id, business_id) REFERENCES public.users(user_id, business_id);


--
-- Name: syslog_events syslog_events_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.syslog_events
    ADD CONSTRAINT syslog_events_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: syslog_events syslog_events_router_connection_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.syslog_events
    ADD CONSTRAINT syslog_events_router_connection_id_fkey FOREIGN KEY (router_connection_id) REFERENCES public.router_connections(id) ON DELETE SET NULL;


--
-- Name: syslog_events syslog_events_site_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.syslog_events
    ADD CONSTRAINT syslog_events_site_id_fkey FOREIGN KEY (site_id) REFERENCES public.sites(site_id) ON DELETE SET NULL;


--
-- Name: telemetry_samples telemetry_samples_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_samples
    ADD CONSTRAINT telemetry_samples_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: telemetry_samples telemetry_samples_device_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.telemetry_samples
    ADD CONSTRAINT telemetry_samples_device_id_fkey FOREIGN KEY (device_id) REFERENCES public.devices(device_id) ON DELETE CASCADE;


--
-- Name: terminal_history terminal_history_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.terminal_history
    ADD CONSTRAINT terminal_history_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: terminal_history terminal_history_router_connection_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.terminal_history
    ADD CONSTRAINT terminal_history_router_connection_id_fkey FOREIGN KEY (router_connection_id) REFERENCES public.router_connections(id) ON DELETE SET NULL;


--
-- Name: terminal_history terminal_history_site_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.terminal_history
    ADD CONSTRAINT terminal_history_site_id_fkey FOREIGN KEY (site_id) REFERENCES public.sites(site_id) ON DELETE SET NULL;


--
-- Name: terminal_history terminal_history_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.terminal_history
    ADD CONSTRAINT terminal_history_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON DELETE SET NULL;


--
-- Name: terminal_templates terminal_templates_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.terminal_templates
    ADD CONSTRAINT terminal_templates_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: terminal_templates terminal_templates_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.terminal_templates
    ADD CONSTRAINT terminal_templates_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(user_id) ON DELETE SET NULL;


--
-- Name: unlock_key_activations unlock_key_activations_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.unlock_key_activations
    ADD CONSTRAINT unlock_key_activations_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: unlock_key_activations unlock_key_activations_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.unlock_key_activations
    ADD CONSTRAINT unlock_key_activations_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON DELETE CASCADE;


--
-- Name: user_profile_settings user_profile_settings_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_profile_settings
    ADD CONSTRAINT user_profile_settings_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: user_profile_settings user_profile_settings_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_profile_settings
    ADD CONSTRAINT user_profile_settings_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(user_id) ON DELETE CASCADE;


--
-- Name: users users_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: voucher_print_layouts voucher_print_layouts_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.voucher_print_layouts
    ADD CONSTRAINT voucher_print_layouts_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: voucher_resellers voucher_resellers_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.voucher_resellers
    ADD CONSTRAINT voucher_resellers_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: vouchers vouchers_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vouchers
    ADD CONSTRAINT vouchers_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: vouchers vouchers_device_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vouchers
    ADD CONSTRAINT vouchers_device_id_fkey FOREIGN KEY (device_id) REFERENCES public.devices(device_id) ON DELETE SET NULL;


--
-- Name: webhook_deliveries webhook_deliveries_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhook_deliveries
    ADD CONSTRAINT webhook_deliveries_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: webhook_deliveries webhook_deliveries_webhook_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhook_deliveries
    ADD CONSTRAINT webhook_deliveries_webhook_id_fkey FOREIGN KEY (webhook_id) REFERENCES public.webhook_endpoints(id) ON DELETE SET NULL;


--
-- Name: webhook_endpoints webhook_endpoints_business_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhook_endpoints
    ADD CONSTRAINT webhook_endpoints_business_id_fkey FOREIGN KEY (business_id) REFERENCES public.businesses(business_id) ON DELETE CASCADE;


--
-- Name: account_entitlements; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.account_entitlements ENABLE ROW LEVEL SECURITY;

--
-- Name: account_entitlements account_entitlements_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY account_entitlements_tenant_isolation ON public.account_entitlements TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: ai_usage_events; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.ai_usage_events ENABLE ROW LEVEL SECURITY;

--
-- Name: ai_usage_events ai_usage_events_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY ai_usage_events_tenant_isolation ON public.ai_usage_events TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: alert_policies; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.alert_policies ENABLE ROW LEVEL SECURITY;

--
-- Name: alert_policies alert_policies_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY alert_policies_tenant_isolation ON public.alert_policies TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: app_account_pass_events; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.app_account_pass_events ENABLE ROW LEVEL SECURITY;

--
-- Name: app_account_pass_renewals; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.app_account_pass_renewals ENABLE ROW LEVEL SECURITY;

--
-- Name: app_account_pass_renewals app_account_pass_renewals_tenant; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY app_account_pass_renewals_tenant ON public.app_account_pass_renewals TO authenticated USING ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid)) WITH CHECK ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid));


--
-- Name: app_account_pass_transitions; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.app_account_pass_transitions ENABLE ROW LEVEL SECURITY;

--
-- Name: app_account_pass_transitions app_account_pass_transitions_tenant_read; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY app_account_pass_transitions_tenant_read ON public.app_account_pass_transitions FOR SELECT TO authenticated USING ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid));


--
-- Name: app_account_passes; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.app_account_passes ENABLE ROW LEVEL SECURITY;

--
-- Name: app_account_passes app_account_passes_tenant_read; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY app_account_passes_tenant_read ON public.app_account_passes FOR SELECT TO authenticated USING ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid));


--
-- Name: app_account_trials; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.app_account_trials ENABLE ROW LEVEL SECURITY;

--
-- Name: app_account_trials app_account_trials_tenant; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY app_account_trials_tenant ON public.app_account_trials USING ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid)) WITH CHECK ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid));


--
-- Name: audit_events; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.audit_events ENABLE ROW LEVEL SECURITY;

--
-- Name: audit_events audit_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY audit_tenant_isolation ON public.audit_events FOR SELECT TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: auth_sessions; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.auth_sessions ENABLE ROW LEVEL SECURITY;

--
-- Name: business_app_primary_events; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.business_app_primary_events ENABLE ROW LEVEL SECURITY;

--
-- Name: business_app_primary_users business_app_primary_read; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY business_app_primary_read ON public.business_app_primary_users FOR SELECT TO authenticated USING ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid));


--
-- Name: business_app_primary_users; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.business_app_primary_users ENABLE ROW LEVEL SECURITY;

--
-- Name: business_metered_quotas; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.business_metered_quotas ENABLE ROW LEVEL SECURITY;

--
-- Name: business_metered_quotas business_metered_quotas_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY business_metered_quotas_tenant_isolation ON public.business_metered_quotas TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: business_telegram_bots; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.business_telegram_bots ENABLE ROW LEVEL SECURITY;

--
-- Name: business_telegram_bots business_telegram_bots_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY business_telegram_bots_tenant_isolation ON public.business_telegram_bots TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: businesses; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.businesses ENABLE ROW LEVEL SECURITY;

--
-- Name: businesses businesses_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY businesses_tenant_isolation ON public.businesses TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: device_allowances; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.device_allowances ENABLE ROW LEVEL SECURITY;

--
-- Name: device_allowances device_allowances_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY device_allowances_tenant_isolation ON public.device_allowances TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: device_change_events; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.device_change_events ENABLE ROW LEVEL SECURITY;

--
-- Name: device_change_events device_change_events_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY device_change_events_tenant_isolation ON public.device_change_events TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: device_config_backups; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.device_config_backups ENABLE ROW LEVEL SECURITY;

--
-- Name: device_config_backups device_config_backups_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY device_config_backups_tenant_isolation ON public.device_config_backups TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: device_discovery_labels; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.device_discovery_labels ENABLE ROW LEVEL SECURITY;

--
-- Name: device_discovery_labels device_discovery_labels_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY device_discovery_labels_tenant_isolation ON public.device_discovery_labels TO authenticated USING ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid)) WITH CHECK ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid));


--
-- Name: device_requests; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.device_requests ENABLE ROW LEVEL SECURITY;

--
-- Name: device_requests device_requests_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY device_requests_tenant_isolation ON public.device_requests TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: devices; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.devices ENABLE ROW LEVEL SECURITY;

--
-- Name: devices devices_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY devices_tenant_isolation ON public.devices TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: fleet_scan_runs; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.fleet_scan_runs ENABLE ROW LEVEL SECURITY;

--
-- Name: fleet_scan_runs fleet_scan_runs_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY fleet_scan_runs_tenant_isolation ON public.fleet_scan_runs TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: guest_access_recovery; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.guest_access_recovery ENABLE ROW LEVEL SECURITY;

--
-- Name: guest_access_recovery guest_access_recovery_tenant; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY guest_access_recovery_tenant ON public.guest_access_recovery TO authenticated USING ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid)) WITH CHECK ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid));


--
-- Name: guest_voucher_batch_reseller_events guest_batch_reseller_events_tenant; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY guest_batch_reseller_events_tenant ON public.guest_voucher_batch_reseller_events USING ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid)) WITH CHECK ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid));


--
-- Name: guest_voucher_batch_resellers guest_batch_reseller_tenant; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY guest_batch_reseller_tenant ON public.guest_voucher_batch_resellers USING ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid)) WITH CHECK ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid));


--
-- Name: guest_purchase_requests; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.guest_purchase_requests ENABLE ROW LEVEL SECURITY;

--
-- Name: guest_purchase_requests guest_requests_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY guest_requests_tenant_isolation ON public.guest_purchase_requests TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: guest_voucher_batch_reseller_events; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.guest_voucher_batch_reseller_events ENABLE ROW LEVEL SECURITY;

--
-- Name: guest_voucher_batch_resellers; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.guest_voucher_batch_resellers ENABLE ROW LEVEL SECURITY;

--
-- Name: guest_voucher_grants; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.guest_voucher_grants ENABLE ROW LEVEL SECURITY;

--
-- Name: guest_voucher_grants guest_voucher_grants_tenant; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY guest_voucher_grants_tenant ON public.guest_voucher_grants TO authenticated USING ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid)) WITH CHECK ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid));


--
-- Name: guest_voucher_issue_batches; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.guest_voucher_issue_batches ENABLE ROW LEVEL SECURITY;

--
-- Name: guest_voucher_issue_batches guest_voucher_issue_batches_tenant; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY guest_voucher_issue_batches_tenant ON public.guest_voucher_issue_batches TO authenticated USING ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid)) WITH CHECK ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid));


--
-- Name: guest_voucher_issue_items; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.guest_voucher_issue_items ENABLE ROW LEVEL SECURITY;

--
-- Name: guest_voucher_issue_items guest_voucher_issue_items_tenant; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY guest_voucher_issue_items_tenant ON public.guest_voucher_issue_items TO authenticated USING ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid)) WITH CHECK ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid));


--
-- Name: guest_voucher_usage; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.guest_voucher_usage ENABLE ROW LEVEL SECURITY;

--
-- Name: guest_voucher_usage guest_voucher_usage_tenant; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY guest_voucher_usage_tenant ON public.guest_voucher_usage USING ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid)) WITH CHECK ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid));


--
-- Name: peer_heartbeats heartbeats_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY heartbeats_tenant_isolation ON public.peer_heartbeats FOR SELECT TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: hub_pending_enrollments; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.hub_pending_enrollments ENABLE ROW LEVEL SECURITY;

--
-- Name: incidents; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.incidents ENABLE ROW LEVEL SECURITY;

--
-- Name: incidents incidents_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY incidents_tenant_isolation ON public.incidents TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: magic_coin_transactions; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.magic_coin_transactions ENABLE ROW LEVEL SECURITY;

--
-- Name: magic_coin_transactions magic_coin_transactions_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY magic_coin_transactions_tenant_isolation ON public.magic_coin_transactions TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: magic_coin_wallets; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.magic_coin_wallets ENABLE ROW LEVEL SECURITY;

--
-- Name: magic_coin_wallets magic_coin_wallets_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY magic_coin_wallets_tenant_isolation ON public.magic_coin_wallets TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: magic_dude_solved_cases; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.magic_dude_solved_cases ENABLE ROW LEVEL SECURITY;

--
-- Name: magic_dude_solved_cases magic_dude_solved_cases_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY magic_dude_solved_cases_tenant_isolation ON public.magic_dude_solved_cases TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: magic_media_publications; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.magic_media_publications ENABLE ROW LEVEL SECURITY;

--
-- Name: management_address_migrations; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.management_address_migrations ENABLE ROW LEVEL SECURITY;

--
-- Name: management_address_migrations management_migration_tenant; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY management_migration_tenant ON public.management_address_migrations USING ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid)) WITH CHECK ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid));


--
-- Name: magic_media_publications media_publications_tenant; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY media_publications_tenant ON public.magic_media_publications USING ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid)) WITH CHECK ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid));


--
-- Name: notification_prefs; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.notification_prefs ENABLE ROW LEVEL SECURITY;

--
-- Name: notification_prefs notification_prefs_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY notification_prefs_tenant_isolation ON public.notification_prefs TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: operator_feature_grants; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.operator_feature_grants ENABLE ROW LEVEL SECURITY;

--
-- Name: operator_feature_grants operator_feature_grants_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY operator_feature_grants_tenant_isolation ON public.operator_feature_grants TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: owner_daily_closes; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.owner_daily_closes ENABLE ROW LEVEL SECURITY;

--
-- Name: owner_daily_closes owner_daily_closes_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY owner_daily_closes_tenant_isolation ON public.owner_daily_closes TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: peer_heartbeats; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.peer_heartbeats ENABLE ROW LEVEL SECURITY;

--
-- Name: portal_configs; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.portal_configs ENABLE ROW LEVEL SECURITY;

--
-- Name: portal_sessions; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.portal_sessions ENABLE ROW LEVEL SECURITY;

--
-- Name: portal_sessions portal_sessions_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY portal_sessions_tenant_isolation ON public.portal_sessions TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: portal_configs portal_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY portal_tenant_isolation ON public.portal_configs TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: purchase_requests; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.purchase_requests ENABLE ROW LEVEL SECURITY;

--
-- Name: purchase_requests purchase_requests_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY purchase_requests_tenant_isolation ON public.purchase_requests TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: router_connections; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.router_connections ENABLE ROW LEVEL SECURITY;

--
-- Name: router_connections router_connections_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY router_connections_tenant_isolation ON public.router_connections TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: ruijie_cloud_connections; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.ruijie_cloud_connections ENABLE ROW LEVEL SECURITY;

--
-- Name: ruijie_cloud_connections ruijie_cloud_connections_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY ruijie_cloud_connections_tenant_isolation ON public.ruijie_cloud_connections TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: service_purchase_audit; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.service_purchase_audit ENABLE ROW LEVEL SECURITY;

--
-- Name: service_purchase_audit service_purchase_audit_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY service_purchase_audit_tenant_isolation ON public.service_purchase_audit USING ((business_id = (current_setting('app.current_business_id'::text, true))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text, true))::uuid));


--
-- Name: service_purchases; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.service_purchases ENABLE ROW LEVEL SECURITY;

--
-- Name: service_purchases service_purchases_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY service_purchases_tenant_isolation ON public.service_purchases USING ((business_id = (current_setting('app.current_business_id'::text, true))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text, true))::uuid));


--
-- Name: site_portals; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.site_portals ENABLE ROW LEVEL SECURITY;

--
-- Name: site_portals site_portals_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY site_portals_tenant_isolation ON public.site_portals TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: site_topology_config; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.site_topology_config ENABLE ROW LEVEL SECURITY;

--
-- Name: site_topology_config site_topology_config_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY site_topology_config_tenant_isolation ON public.site_topology_config TO authenticated USING ((site_id IN ( SELECT s.site_id
   FROM public.sites s
  WHERE (s.business_id = (current_setting('app.current_business_id'::text))::uuid)))) WITH CHECK ((site_id IN ( SELECT s.site_id
   FROM public.sites s
  WHERE (s.business_id = (current_setting('app.current_business_id'::text))::uuid))));


--
-- Name: sites; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.sites ENABLE ROW LEVEL SECURITY;

--
-- Name: sites sites_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY sites_tenant_isolation ON public.sites TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: store_addon_requests; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.store_addon_requests ENABLE ROW LEVEL SECURITY;

--
-- Name: store_addon_requests store_addon_requests_tenant; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY store_addon_requests_tenant ON public.store_addon_requests USING ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid)) WITH CHECK ((business_id = (NULLIF(current_setting('app.current_business_id'::text, true), ''::text))::uuid));


--
-- Name: syslog_events; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.syslog_events ENABLE ROW LEVEL SECURITY;

--
-- Name: syslog_events syslog_events_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY syslog_events_tenant_isolation ON public.syslog_events TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: telemetry_samples; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.telemetry_samples ENABLE ROW LEVEL SECURITY;

--
-- Name: telemetry_samples telemetry_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY telemetry_tenant_isolation ON public.telemetry_samples TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: terminal_history; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.terminal_history ENABLE ROW LEVEL SECURITY;

--
-- Name: terminal_history terminal_history_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY terminal_history_tenant_isolation ON public.terminal_history TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: terminal_templates; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.terminal_templates ENABLE ROW LEVEL SECURITY;

--
-- Name: terminal_templates terminal_templates_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY terminal_templates_tenant_isolation ON public.terminal_templates TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: unlock_key_activations; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.unlock_key_activations ENABLE ROW LEVEL SECURITY;

--
-- Name: unlock_key_activations unlock_key_activations_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY unlock_key_activations_tenant_isolation ON public.unlock_key_activations TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: user_profile_settings; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.user_profile_settings ENABLE ROW LEVEL SECURITY;

--
-- Name: user_profile_settings user_profile_settings_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY user_profile_settings_tenant_isolation ON public.user_profile_settings TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: users; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.users ENABLE ROW LEVEL SECURITY;

--
-- Name: users users_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY users_tenant_isolation ON public.users TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: voucher_print_layouts; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.voucher_print_layouts ENABLE ROW LEVEL SECURITY;

--
-- Name: voucher_print_layouts voucher_print_layouts_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY voucher_print_layouts_tenant_isolation ON public.voucher_print_layouts TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: voucher_resellers; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.voucher_resellers ENABLE ROW LEVEL SECURITY;

--
-- Name: voucher_resellers voucher_resellers_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY voucher_resellers_tenant_isolation ON public.voucher_resellers TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: vouchers; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.vouchers ENABLE ROW LEVEL SECURITY;

--
-- Name: vouchers vouchers_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY vouchers_tenant_isolation ON public.vouchers TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: webhook_deliveries; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.webhook_deliveries ENABLE ROW LEVEL SECURITY;

--
-- Name: webhook_deliveries webhook_deliveries_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY webhook_deliveries_tenant_isolation ON public.webhook_deliveries TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- Name: webhook_endpoints; Type: ROW SECURITY; Schema: public; Owner: -
--

ALTER TABLE public.webhook_endpoints ENABLE ROW LEVEL SECURITY;

--
-- Name: webhook_endpoints webhook_endpoints_tenant_isolation; Type: POLICY; Schema: public; Owner: -
--

CREATE POLICY webhook_endpoints_tenant_isolation ON public.webhook_endpoints TO authenticated USING ((business_id = (current_setting('app.current_business_id'::text))::uuid)) WITH CHECK ((business_id = (current_setting('app.current_business_id'::text))::uuid));


--
-- PostgreSQL database dump complete
--


