export type Json =
  | string
  | number
  | boolean
  | null
  | { [key: string]: Json | undefined }
  | Json[]

export type Database = {
  // Allows to automatically instantiate createClient with right options
  // instead of createClient<Database, { PostgrestVersion: 'XX' }>(URL, KEY)
  __InternalSupabase: {
    PostgrestVersion: "14.5"
  }
  public: {
    Tables: {
      account_entitlements: {
        Row: {
          created_at: string
          owner_id: string | null
          plus: boolean
          plus_since: string | null
          tier: string
          tier_expires_at: string | null
          updated_at: string
          user_id: string
        }
        Insert: {
          created_at?: string
          owner_id?: string | null
          plus?: boolean
          plus_since?: string | null
          tier?: string
          tier_expires_at?: string | null
          updated_at?: string
          user_id: string
        }
        Update: {
          created_at?: string
          owner_id?: string | null
          plus?: boolean
          plus_since?: string | null
          tier?: string
          tier_expires_at?: string | null
          updated_at?: string
          user_id?: string
        }
        Relationships: []
      }
      account_referrals: {
        Row: {
          agent_id: string
          created_at: string
          user_id: string
        }
        Insert: {
          agent_id: string
          created_at?: string
          user_id: string
        }
        Update: {
          agent_id?: string
          created_at?: string
          user_id?: string
        }
        Relationships: []
      }
      admin_notifications: {
        Row: {
          body: string | null
          created_at: string
          data: Json
          id: string
          kind: string
          read_at: string | null
          recipient_id: string
          title: string
        }
        Insert: {
          body?: string | null
          created_at?: string
          data?: Json
          id?: string
          kind: string
          read_at?: string | null
          recipient_id: string
          title: string
        }
        Update: {
          body?: string | null
          created_at?: string
          data?: Json
          id?: string
          kind?: string
          read_at?: string | null
          recipient_id?: string
          title?: string
        }
        Relationships: []
      }
      agent_points: {
        Row: {
          agent_id: string
          billing_period: string | null
          created_at: string
          id: string
          kind: string
          note: string | null
          owner_id: string | null
          points: number
          referred_user_id: string | null
          reverses_point_id: string | null
          service_key: string | null
          service_purchase_id: string | null
          source_status: string | null
        }
        Insert: {
          agent_id: string
          billing_period?: string | null
          created_at?: string
          id?: string
          kind: string
          note?: string | null
          owner_id?: string | null
          points: number
          referred_user_id?: string | null
          reverses_point_id?: string | null
          service_key?: string | null
          service_purchase_id?: string | null
          source_status?: string | null
        }
        Update: {
          agent_id?: string
          billing_period?: string | null
          created_at?: string
          id?: string
          kind?: string
          note?: string | null
          owner_id?: string | null
          points?: number
          referred_user_id?: string | null
          reverses_point_id?: string | null
          service_key?: string | null
          service_purchase_id?: string | null
          source_status?: string | null
        }
        Relationships: [
          {
            foreignKeyName: "agent_points_reverses_point_id_fkey"
            columns: ["reverses_point_id"]
            isOneToOne: false
            referencedRelation: "agent_points"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "agent_points_service_purchase_id_fkey"
            columns: ["service_purchase_id"]
            isOneToOne: false
            referencedRelation: "service_purchases"
            referencedColumns: ["id"]
          },
        ]
      }
      ai_scan_limits: {
        Row: {
          created_at: string
          monthly_limit: number
          owner_id: string | null
          updated_at: string
          user_id: string
        }
        Insert: {
          created_at?: string
          monthly_limit?: number
          owner_id?: string | null
          updated_at?: string
          user_id: string
        }
        Update: {
          created_at?: string
          monthly_limit?: number
          owner_id?: string | null
          updated_at?: string
          user_id?: string
        }
        Relationships: []
      }
      ai_usage_events: {
        Row: {
          cached: boolean
          completion_tokens: number
          created_at: string
          feature: string
          id: string
          model: string | null
          owner_id: string
          prompt_tokens: number
          total_tokens: number
          user_id: string
        }
        Insert: {
          cached?: boolean
          completion_tokens?: number
          created_at?: string
          feature: string
          id?: string
          model?: string | null
          owner_id: string
          prompt_tokens?: number
          total_tokens?: number
          user_id: string
        }
        Update: {
          cached?: boolean
          completion_tokens?: number
          created_at?: string
          feature?: string
          id?: string
          model?: string | null
          owner_id?: string
          prompt_tokens?: number
          total_tokens?: number
          user_id?: string
        }
        Relationships: []
      }
      alert_rules: {
        Row: {
          cooldown_minutes: number
          created_at: string
          enabled: boolean
          id: string
          kind: string
          owner_id: string
          severity: string
          threshold: number
          updated_at: string
        }
        Insert: {
          cooldown_minutes?: number
          created_at?: string
          enabled?: boolean
          id?: string
          kind: string
          owner_id: string
          severity?: string
          threshold?: number
          updated_at?: string
        }
        Update: {
          cooldown_minutes?: number
          created_at?: string
          enabled?: boolean
          id?: string
          kind?: string
          owner_id?: string
          severity?: string
          threshold?: number
          updated_at?: string
        }
        Relationships: []
      }
      ap_actions_audit: {
        Row: {
          action: string
          brand: string
          controller_id: string | null
          created_at: string
          detail: Json
          error_message: string | null
          id: string
          owner_id: string
          success: boolean
          target: string | null
          user_id: string
        }
        Insert: {
          action: string
          brand: string
          controller_id?: string | null
          created_at?: string
          detail?: Json
          error_message?: string | null
          id?: string
          owner_id: string
          success: boolean
          target?: string | null
          user_id: string
        }
        Update: {
          action?: string
          brand?: string
          controller_id?: string | null
          created_at?: string
          detail?: Json
          error_message?: string | null
          id?: string
          owner_id?: string
          success?: boolean
          target?: string | null
          user_id?: string
        }
        Relationships: [
          {
            foreignKeyName: "ap_actions_audit_controller_id_fkey"
            columns: ["controller_id"]
            isOneToOne: false
            referencedRelation: "unifi_controllers"
            referencedColumns: ["id"]
          },
        ]
      }
      ap_devices: {
        Row: {
          client_count: number
          controller_id: string
          cpu_pct: number | null
          created_at: string
          id: string
          last_seen_at: string | null
          last_state: string | null
          location: string | null
          mac: string
          mem_pct: number | null
          model: string | null
          name: string | null
          notes: string | null
          owner_id: string
          updated_at: string
        }
        Insert: {
          client_count?: number
          controller_id: string
          cpu_pct?: number | null
          created_at?: string
          id?: string
          last_seen_at?: string | null
          last_state?: string | null
          location?: string | null
          mac: string
          mem_pct?: number | null
          model?: string | null
          name?: string | null
          notes?: string | null
          owner_id: string
          updated_at?: string
        }
        Update: {
          client_count?: number
          controller_id?: string
          cpu_pct?: number | null
          created_at?: string
          id?: string
          last_seen_at?: string | null
          last_state?: string | null
          location?: string | null
          mac?: string
          mem_pct?: number | null
          model?: string | null
          name?: string | null
          notes?: string | null
          owner_id?: string
          updated_at?: string
        }
        Relationships: [
          {
            foreignKeyName: "ap_devices_controller_id_fkey"
            columns: ["controller_id"]
            isOneToOne: false
            referencedRelation: "unifi_controllers"
            referencedColumns: ["id"]
          },
        ]
      }
      connector_bootstrap_audit: {
        Row: {
          action: string
          connector_id: string
          created_at: string
          detail: Json
          discovered_router_id: string | null
          id: string
          owner_id: string
          status: string
          updated_at: string
        }
        Insert: {
          action: string
          connector_id: string
          created_at?: string
          detail?: Json
          discovered_router_id?: string | null
          id?: string
          owner_id: string
          status?: string
          updated_at?: string
        }
        Update: {
          action?: string
          connector_id?: string
          created_at?: string
          detail?: Json
          discovered_router_id?: string | null
          id?: string
          owner_id?: string
          status?: string
          updated_at?: string
        }
        Relationships: [
          {
            foreignKeyName: "connector_bootstrap_audit_connector_id_fkey"
            columns: ["connector_id"]
            isOneToOne: false
            referencedRelation: "connectors"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "connector_bootstrap_audit_discovered_router_id_fkey"
            columns: ["discovered_router_id"]
            isOneToOne: false
            referencedRelation: "connector_discovered_routers"
            referencedColumns: ["id"]
          },
        ]
      }
      connector_discovered_routers: {
        Row: {
          api_password_encrypted: string | null
          api_username: string | null
          backup_name: string | null
          connector_id: string
          created_at: string
          fingerprint: string
          id: string
          identity: string | null
          ip: string | null
          last_error: string | null
          last_seen_at: string
          mac: string | null
          model: string | null
          os_version: string | null
          owner_id: string
          platform: string | null
          rollback_meta: Json
          rollback_script: string | null
          router_connection_id: string | null
          serial: string | null
          state: string
          tls_fingerprint: string | null
          updated_at: string
        }
        Insert: {
          api_password_encrypted?: string | null
          api_username?: string | null
          backup_name?: string | null
          connector_id: string
          created_at?: string
          fingerprint: string
          id?: string
          identity?: string | null
          ip?: string | null
          last_error?: string | null
          last_seen_at?: string
          mac?: string | null
          model?: string | null
          os_version?: string | null
          owner_id: string
          platform?: string | null
          rollback_meta?: Json
          rollback_script?: string | null
          router_connection_id?: string | null
          serial?: string | null
          state?: string
          tls_fingerprint?: string | null
          updated_at?: string
        }
        Update: {
          api_password_encrypted?: string | null
          api_username?: string | null
          backup_name?: string | null
          connector_id?: string
          created_at?: string
          fingerprint?: string
          id?: string
          identity?: string | null
          ip?: string | null
          last_error?: string | null
          last_seen_at?: string
          mac?: string | null
          model?: string | null
          os_version?: string | null
          owner_id?: string
          platform?: string | null
          rollback_meta?: Json
          rollback_script?: string | null
          router_connection_id?: string | null
          serial?: string | null
          state?: string
          tls_fingerprint?: string | null
          updated_at?: string
        }
        Relationships: [
          {
            foreignKeyName: "connector_discovered_routers_connector_id_fkey"
            columns: ["connector_id"]
            isOneToOne: false
            referencedRelation: "connectors"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "connector_discovered_routers_router_connection_id_fkey"
            columns: ["router_connection_id"]
            isOneToOne: false
            referencedRelation: "router_connections"
            referencedColumns: ["id"]
          },
        ]
      }
      connector_jobs: {
        Row: {
          claimed_at: string | null
          completed_at: string | null
          connector_id: string
          created_at: string
          error: string | null
          expires_at: string
          id: string
          kind: string
          owner_id: string
          request: Json
          response: Json | null
          status: string
          updated_at: string
        }
        Insert: {
          claimed_at?: string | null
          completed_at?: string | null
          connector_id: string
          created_at?: string
          error?: string | null
          expires_at?: string
          id?: string
          kind?: string
          owner_id: string
          request?: Json
          response?: Json | null
          status?: string
          updated_at?: string
        }
        Update: {
          claimed_at?: string | null
          completed_at?: string | null
          connector_id?: string
          created_at?: string
          error?: string | null
          expires_at?: string
          id?: string
          kind?: string
          owner_id?: string
          request?: Json
          response?: Json | null
          status?: string
          updated_at?: string
        }
        Relationships: [
          {
            foreignKeyName: "connector_jobs_connector_id_fkey"
            columns: ["connector_id"]
            isOneToOne: false
            referencedRelation: "connectors"
            referencedColumns: ["id"]
          },
        ]
      }
      connector_rate_limits: {
        Row: {
          bucket_key: string
          created_at: string
          hits: number
          updated_at: string
          window_start: string
        }
        Insert: {
          bucket_key: string
          created_at?: string
          hits?: number
          updated_at?: string
          window_start?: string
        }
        Update: {
          bucket_key?: string
          created_at?: string
          hits?: number
          updated_at?: string
          window_start?: string
        }
        Relationships: []
      }
      connectors: {
        Row: {
          created_at: string
          enabled: boolean
          hostname: string | null
          id: string
          last_seen_at: string | null
          local_ip: string | null
          local_subnet: string | null
          name: string
          owner_id: string
          paired_at: string | null
          pairing_code_expires_at: string | null
          pairing_code_hash: string | null
          public_id: string
          status: string
          token_hash: string | null
          updated_at: string
          version: string | null
        }
        Insert: {
          created_at?: string
          enabled?: boolean
          hostname?: string | null
          id?: string
          last_seen_at?: string | null
          local_ip?: string | null
          local_subnet?: string | null
          name: string
          owner_id: string
          paired_at?: string | null
          pairing_code_expires_at?: string | null
          pairing_code_hash?: string | null
          public_id: string
          status?: string
          token_hash?: string | null
          updated_at?: string
          version?: string | null
        }
        Update: {
          created_at?: string
          enabled?: boolean
          hostname?: string | null
          id?: string
          last_seen_at?: string | null
          local_ip?: string | null
          local_subnet?: string | null
          name?: string
          owner_id?: string
          paired_at?: string | null
          pairing_code_expires_at?: string | null
          pairing_code_hash?: string | null
          public_id?: string
          status?: string
          token_hash?: string | null
          updated_at?: string
          version?: string | null
        }
        Relationships: []
      }
      deployment_history: {
        Row: {
          actor_user_id: string | null
          backup_ref: string | null
          created_at: string
          device_id: string | null
          diff_summary: Json
          failure_reason: string | null
          id: string
          idempotency_key: string
          intent: string
          mode: string
          owner_id: string
          plan_hash: string
          rolled_back_at: string | null
          router_id: string | null
          site_id: string | null
          status: string
          verification: Json
        }
        Insert: {
          actor_user_id?: string | null
          backup_ref?: string | null
          created_at?: string
          device_id?: string | null
          diff_summary?: Json
          failure_reason?: string | null
          id?: string
          idempotency_key: string
          intent: string
          mode?: string
          owner_id: string
          plan_hash: string
          rolled_back_at?: string | null
          router_id?: string | null
          site_id?: string | null
          status?: string
          verification?: Json
        }
        Update: {
          actor_user_id?: string | null
          backup_ref?: string | null
          created_at?: string
          device_id?: string | null
          diff_summary?: Json
          failure_reason?: string | null
          id?: string
          idempotency_key?: string
          intent?: string
          mode?: string
          owner_id?: string
          plan_hash?: string
          rolled_back_at?: string | null
          router_id?: string | null
          site_id?: string | null
          status?: string
          verification?: Json
        }
        Relationships: [
          {
            foreignKeyName: "deployment_history_device_id_fkey"
            columns: ["device_id"]
            isOneToOne: false
            referencedRelation: "managed_devices"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "deployment_history_router_id_fkey"
            columns: ["router_id"]
            isOneToOne: false
            referencedRelation: "router_connections"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "deployment_history_site_id_fkey"
            columns: ["site_id"]
            isOneToOne: false
            referencedRelation: "sites"
            referencedColumns: ["id"]
          },
        ]
      }
      device_allowances: {
        Row: {
          controllers: number
          created_at: string
          included_router_id: string | null
          owner_id: string
          routers: number
          sites: number
          updated_at: string
        }
        Insert: {
          controllers?: number
          created_at?: string
          included_router_id?: string | null
          owner_id: string
          routers?: number
          sites?: number
          updated_at?: string
        }
        Update: {
          controllers?: number
          created_at?: string
          included_router_id?: string | null
          owner_id?: string
          routers?: number
          sites?: number
          updated_at?: string
        }
        Relationships: [
          {
            foreignKeyName: "device_allowances_included_router_id_fkey"
            columns: ["included_router_id"]
            isOneToOne: false
            referencedRelation: "router_connections"
            referencedColumns: ["id"]
          },
        ]
      }
      device_health_samples: {
        Row: {
          connector_state: string | null
          cpu_usage_pct: number | null
          detail: Json
          device_id: string | null
          free_memory_bytes: number | null
          id: string
          latency_ms: number | null
          memory_usage_pct: number | null
          observed_at: string
          owner_id: string
          reachable: boolean | null
          router_id: string | null
          site_id: string | null
          subject_id: string
          subject_kind: string
          total_memory_bytes: number | null
          tunnel_state: string | null
          uptime_seconds: number | null
          wan_state: string | null
        }
        Insert: {
          connector_state?: string | null
          cpu_usage_pct?: number | null
          detail?: Json
          device_id?: string | null
          free_memory_bytes?: number | null
          id?: string
          latency_ms?: number | null
          memory_usage_pct?: number | null
          observed_at?: string
          owner_id: string
          reachable?: boolean | null
          router_id?: string | null
          site_id?: string | null
          subject_id: string
          subject_kind: string
          total_memory_bytes: number | null
          tunnel_state?: string | null
          uptime_seconds?: number | null
          wan_state?: string | null
        }
        Update: {
          connector_state?: string | null
          cpu_usage_pct?: number | null
          detail?: Json
          device_id?: string | null
          free_memory_bytes?: number | null
          id?: string
          latency_ms?: number | null
          memory_usage_pct?: number | null
          observed_at?: string
          owner_id?: string
          reachable?: boolean | null
          router_id?: string | null
          site_id?: string | null
          subject_id?: string
          subject_kind?: string
          total_memory_bytes?: number | null
          tunnel_state?: string | null
          uptime_seconds?: number | null
          wan_state?: string | null
        }
        Relationships: [
          {
            foreignKeyName: "device_health_samples_device_id_fkey"
            columns: ["device_id"]
            isOneToOne: false
            referencedRelation: "managed_devices"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "device_health_samples_router_id_fkey"
            columns: ["router_id"]
            isOneToOne: false
            referencedRelation: "router_connections"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "device_health_samples_site_id_fkey"
            columns: ["site_id"]
            isOneToOne: false
            referencedRelation: "sites"
            referencedColumns: ["id"]
          },
        ]
      }
      device_requests: {
        Row: {
          created_at: string
          decided_at: string | null
          decided_by: string | null
          id: string
          kind: string
          owner_id: string
          reason: string | null
          status: string
          updated_at: string
          user_id: string
        }
        Insert: {
          created_at?: string
          decided_at?: string | null
          decided_by?: string | null
          id?: string
          kind: string
          owner_id: string
          reason?: string | null
          status?: string
          updated_at?: string
          user_id: string
        }
        Update: {
          created_at?: string
          decided_at?: string | null
          decided_by?: string | null
          id?: string
          kind?: string
          owner_id?: string
          reason?: string | null
          status?: string
          updated_at?: string
          user_id?: string
        }
        Relationships: []
      }
      fleet_scan_runs: {
        Row: {
          generated_at: string
          id: string
          kind: string
          max_severity: string
          owner_id: string
          payload: Json
          router_count: number
          triggered_by: string | null
        }
        Insert: {
          generated_at?: string
          id?: string
          kind: string
          max_severity?: string
          owner_id: string
          payload: Json
          router_count?: number
          triggered_by?: string | null
        }
        Update: {
          generated_at?: string
          id?: string
          kind?: string
          max_severity?: string
          owner_id?: string
          payload?: Json
          router_count?: number
          triggered_by?: string | null
        }
        Relationships: []
      }
      hotspot_sessions: {
        Row: {
          bytes_in: number
          bytes_out: number
          code: string | null
          created_at: string
          device_ip: string | null
          device_mac: string | null
          duration_seconds: number
          ended_at: string | null
          external_session_id: string | null
          id: string
          owner_id: string
          plan_key: string | null
          plan_label: string | null
          reconcile_note: string | null
          reconcile_status: string
          router_id: string | null
          site_id: string | null
          source_seen_at: string | null
          started_at: string
          termination_reason: string | null
          updated_at: string
          username: string | null
          voucher_code_id: string | null
        }
        Insert: {
          bytes_in?: number
          bytes_out?: number
          code?: string | null
          created_at?: string
          device_ip?: string | null
          device_mac?: string | null
          duration_seconds?: number
          ended_at?: string | null
          external_session_id?: string | null
          id?: string
          owner_id: string
          plan_key?: string | null
          plan_label?: string | null
          reconcile_note?: string | null
          reconcile_status?: string
          router_id?: string | null
          site_id?: string | null
          source_seen_at?: string | null
          started_at?: string
          termination_reason?: string | null
          updated_at?: string
          username?: string | null
          voucher_code_id?: string | null
        }
        Update: {
          bytes_in?: number
          bytes_out?: number
          code?: string | null
          created_at?: string
          device_ip?: string | null
          device_mac?: string | null
          duration_seconds?: number
          ended_at?: string | null
          external_session_id?: string | null
          id?: string
          owner_id?: string
          plan_key?: string | null
          plan_label?: string | null
          reconcile_note?: string | null
          reconcile_status?: string
          router_id?: string | null
          site_id?: string | null
          source_seen_at?: string | null
          started_at?: string
          termination_reason?: string | null
          updated_at?: string
          username?: string | null
          voucher_code_id?: string | null
        }
        Relationships: [
          {
            foreignKeyName: "hotspot_sessions_router_id_fkey"
            columns: ["router_id"]
            isOneToOne: false
            referencedRelation: "router_connections"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "hotspot_sessions_site_id_fkey"
            columns: ["site_id"]
            isOneToOne: false
            referencedRelation: "sites"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "hotspot_sessions_voucher_code_id_fkey"
            columns: ["voucher_code_id"]
            isOneToOne: false
            referencedRelation: "voucher_codes"
            referencedColumns: ["id"]
          },
        ]
      }
      incidents: {
        Row: {
          acknowledged_at: string | null
          acknowledged_by: string | null
          detail: string | null
          device_id: string | null
          id: string
          kind: string
          last_notified_at: string | null
          last_seen_at: string
          opened_at: string
          owner_id: string
          resolved_at: string | null
          severity: string
          site_id: string | null
          subject_id: string
          subject_label: string
        }
        Insert: {
          acknowledged_at?: string | null
          acknowledged_by?: string | null
          detail?: string | null
          device_id?: string | null
          id?: string
          kind: string
          last_notified_at?: string | null
          last_seen_at?: string
          opened_at?: string
          owner_id: string
          resolved_at?: string | null
          severity?: string
          site_id?: string | null
          subject_id: string
          subject_label: string
        }
        Update: {
          acknowledged_at?: string | null
          acknowledged_by?: string | null
          detail?: string | null
          device_id?: string | null
          id?: string
          kind?: string
          last_notified_at?: string | null
          last_seen_at?: string
          opened_at?: string
          owner_id?: string
          resolved_at?: string | null
          severity?: string
          site_id?: string | null
          subject_id?: string
          subject_label?: string
        }
        Relationships: [
          {
            foreignKeyName: "incidents_device_id_fkey"
            columns: ["device_id"]
            isOneToOne: false
            referencedRelation: "managed_devices"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "incidents_site_id_fkey"
            columns: ["site_id"]
            isOneToOne: false
            referencedRelation: "sites"
            referencedColumns: ["id"]
          },
        ]
      }
      magic_coin_transactions: {
        Row: {
          actor_user_id: string | null
          balance_after: number
          created_at: string
          delta: number
          id: string
          kind: string
          magic_dude_unlock_activation_id: string | null
          note: string
          owner_id: string
          reseller_add_key_activation_id: string | null
          router_unlock_key_activation_id: string | null
          service_purchase_id: string | null
          source_agent_point_id: string | null
          user_id: string
          webfig_key_activation_id: string | null
        }
        Insert: {
          actor_user_id?: string | null
          balance_after: number
          created_at?: string
          delta: number
          id?: string
          kind: string
          magic_dude_unlock_activation_id?: string | null
          note: string
          owner_id: string
          reseller_add_key_activation_id?: string | null
          router_unlock_key_activation_id?: string | null
          service_purchase_id?: string | null
          source_agent_point_id?: string | null
          user_id: string
          webfig_key_activation_id?: string | null
        }
        Update: {
          actor_user_id?: string | null
          balance_after?: number
          created_at?: string
          delta?: number
          id?: string
          kind?: string
          magic_dude_unlock_activation_id?: string | null
          note?: string
          owner_id?: string
          reseller_add_key_activation_id?: string | null
          router_unlock_key_activation_id?: string | null
          service_purchase_id?: string | null
          source_agent_point_id?: string | null
          user_id?: string
          webfig_key_activation_id?: string | null
        }
        Relationships: [
          {
            foreignKeyName: "magic_coin_transactions_magic_dude_unlock_activation_id_fkey"
            columns: ["magic_dude_unlock_activation_id"]
            isOneToOne: false
            referencedRelation: "magic_dude_unlock_activations"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "magic_coin_transactions_reseller_add_key_activation_id_fkey"
            columns: ["reseller_add_key_activation_id"]
            isOneToOne: false
            referencedRelation: "reseller_add_key_activations"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "magic_coin_transactions_router_unlock_key_activation_id_fkey"
            columns: ["router_unlock_key_activation_id"]
            isOneToOne: false
            referencedRelation: "router_unlock_key_activations"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "magic_coin_transactions_service_purchase_id_fkey"
            columns: ["service_purchase_id"]
            isOneToOne: false
            referencedRelation: "service_purchases"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "magic_coin_transactions_source_agent_point_id_fkey"
            columns: ["source_agent_point_id"]
            isOneToOne: false
            referencedRelation: "agent_points"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "magic_coin_transactions_webfig_key_activation_id_fkey"
            columns: ["webfig_key_activation_id"]
            isOneToOne: false
            referencedRelation: "webfig_unlock_key_activations"
            referencedColumns: ["id"]
          },
        ]
      }
      magic_coin_wallets: {
        Row: {
          balance: number
          created_at: string
          updated_at: string
          user_id: string
        }
        Insert: {
          balance?: number
          created_at?: string
          updated_at?: string
          user_id: string
        }
        Update: {
          balance?: number
          created_at?: string
          updated_at?: string
          user_id?: string
        }
        Relationships: []
      }
      magic_dude_unlock_activations: {
        Row: {
          account_bound: boolean
          attributes: string
          created_at: string
          expires_at: string
          id: string
          non_transferable: boolean
          owner_id: string
          price_coins: number
          user_id: string
        }
        Insert: {
          account_bound?: boolean
          attributes?: string
          created_at?: string
          expires_at: string
          id?: string
          non_transferable?: boolean
          owner_id: string
          price_coins?: number
          user_id: string
        }
        Update: {
          account_bound?: boolean
          attributes?: string
          created_at?: string
          expires_at?: string
          id?: string
          non_transferable?: boolean
          owner_id?: string
          price_coins?: number
          user_id?: string
        }
        Relationships: []
      }
      managed_devices: {
        Row: {
          capabilities: Json
          category: string
          connector_id: string | null
          controller_id: string | null
          created_at: string
          host: string | null
          id: string
          location: string | null
          mac: string | null
          model: string | null
          name: string
          notes: string | null
          owner_id: string
          router_id: string | null
          site_id: string | null
          transport: string
          updated_at: string
          vendor: string
        }
        Insert: {
          capabilities?: Json
          category: string
          connector_id?: string | null
          controller_id?: string | null
          created_at?: string
          host?: string | null
          id?: string
          location?: string | null
          mac?: string | null
          model?: string | null
          name: string
          notes?: string | null
          owner_id: string
          router_id?: string | null
          site_id?: string | null
          transport?: string
          updated_at?: string
          vendor: string
        }
        Update: {
          capabilities?: Json
          category?: string
          connector_id?: string | null
          controller_id?: string | null
          created_at?: string
          host?: string | null
          id?: string
          location?: string | null
          mac?: string | null
          model?: string | null
          name?: string
          notes?: string | null
          owner_id?: string
          router_id?: string | null
          site_id?: string | null
          transport?: string
          updated_at?: string
          vendor?: string
        }
        Relationships: [
          {
            foreignKeyName: "managed_devices_connector_id_fkey"
            columns: ["connector_id"]
            isOneToOne: false
            referencedRelation: "connectors"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "managed_devices_controller_id_fkey"
            columns: ["controller_id"]
            isOneToOne: false
            referencedRelation: "unifi_controllers"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "managed_devices_router_id_fkey"
            columns: ["router_id"]
            isOneToOne: false
            referencedRelation: "router_connections"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "managed_devices_site_id_fkey"
            columns: ["site_id"]
            isOneToOne: false
            referencedRelation: "sites"
            referencedColumns: ["id"]
          },
        ]
      }
      notification_prefs: {
        Row: {
          created_at: string
          email: boolean
          in_app: boolean
          min_severity: string
          owner_id: string
          quiet_hours_end: number | null
          quiet_hours_start: number | null
          updated_at: string
        }
        Insert: {
          created_at?: string
          email?: boolean
          in_app?: boolean
          min_severity?: string
          owner_id: string
          quiet_hours_end?: number | null
          quiet_hours_start?: number | null
          updated_at?: string
        }
        Update: {
          created_at?: string
          email?: boolean
          in_app?: boolean
          min_severity?: string
          owner_id?: string
          quiet_hours_end?: number | null
          quiet_hours_start?: number | null
          updated_at?: string
        }
        Relationships: []
      }
      notifier_audit: {
        Row: {
          action: string
          actor_user_id: string | null
          channel: string
          created_at: string
          detail: string | null
          id: string
          outcome: string
          owner_id: string
        }
        Insert: {
          action: string
          actor_user_id?: string | null
          channel?: string
          created_at?: string
          detail?: string | null
          id?: string
          outcome?: string
          owner_id: string
        }
        Update: {
          action?: string
          actor_user_id?: string | null
          channel?: string
          created_at?: string
          detail?: string | null
          id?: string
          outcome?: string
          owner_id?: string
        }
        Relationships: []
      }
      operator_feature_grants: {
        Row: {
          created_at: string
          feature: string
          granted_by: string
          user_id: string
        }
        Insert: {
          created_at?: string
          feature: string
          granted_by: string
          user_id: string
        }
        Update: {
          created_at?: string
          feature?: string
          granted_by?: string
          user_id?: string
        }
        Relationships: []
      }
      operator_feature_role_defaults: {
        Row: {
          feature: string
          role: Database["public"]["Enums"]["app_role"]
          updated_at: string
          updated_by: string
        }
        Insert: {
          feature: string
          role: Database["public"]["Enums"]["app_role"]
          updated_at?: string
          updated_by: string
        }
        Update: {
          feature?: string
          role?: Database["public"]["Enums"]["app_role"]
          updated_at?: string
          updated_by?: string
        }
        Relationships: []
      }
      owner_accounts: {
        Row: {
          auth_email: string
          created_at: string
          id: string
          updated_at: string
          user_id: string
          username: string
        }
        Insert: {
          auth_email: string
          created_at?: string
          id?: string
          updated_at?: string
          user_id: string
          username: string
        }
        Update: {
          auth_email?: string
          created_at?: string
          id?: string
          updated_at?: string
          user_id?: string
          username?: string
        }
        Relationships: []
      }
      owner_daily_closes: {
        Row: {
          business_day: string
          closed_at: string
          closed_by: string
          counted_cash_mmk: number
          id: string
          note: string | null
          owner_id: string
          router_id: string | null
          site_id: string | null
        }
        Insert: {
          business_day: string
          closed_at?: string
          closed_by: string
          counted_cash_mmk?: number
          id?: string
          note?: string | null
          owner_id: string
          router_id?: string | null
          site_id?: string | null
        }
        Update: {
          business_day?: string
          closed_at?: string
          closed_by?: string
          counted_cash_mmk?: number
          id?: string
          note?: string | null
          owner_id?: string
          router_id?: string | null
          site_id?: string | null
        }
        Relationships: [
          {
            foreignKeyName: "owner_daily_closes_router_id_fkey"
            columns: ["router_id"]
            isOneToOne: false
            referencedRelation: "router_connections"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "owner_daily_closes_site_id_fkey"
            columns: ["site_id"]
            isOneToOne: false
            referencedRelation: "sites"
            referencedColumns: ["id"]
          },
        ]
      }
      payment_bank_accounts: {
        Row: {
          account_number: string
          bank_name: string
          created_at: string
          enabled: boolean
          holder_name: string
          id: string
          owner_id: string
          slot: number
          sort: number
          updated_at: string
        }
        Insert: {
          account_number?: string
          bank_name?: string
          created_at?: string
          enabled?: boolean
          holder_name?: string
          id?: string
          owner_id: string
          slot: number
          sort?: number
          updated_at?: string
        }
        Update: {
          account_number?: string
          bank_name?: string
          created_at?: string
          enabled?: boolean
          holder_name?: string
          id?: string
          owner_id?: string
          slot?: number
          sort?: number
          updated_at?: string
        }
        Relationships: []
      }
      payment_events: {
        Row: {
          created_at: string
          event_id: string
          event_type: string
          id: string
          order_id: string | null
          outcome: string | null
          owner_id: string | null
          payload: Json
          processed_at: string | null
          provider: string
        }
        Insert: {
          created_at?: string
          event_id: string
          event_type: string
          id?: string
          order_id?: string | null
          outcome?: string | null
          owner_id?: string | null
          payload?: Json
          processed_at?: string | null
          provider: string
        }
        Update: {
          created_at?: string
          event_id?: string
          event_type?: string
          id?: string
          order_id?: string | null
          outcome?: string | null
          owner_id?: string | null
          payload?: Json
          processed_at?: string | null
          provider?: string
        }
        Relationships: [
          {
            foreignKeyName: "payment_events_order_id_fkey"
            columns: ["order_id"]
            isOneToOne: false
            referencedRelation: "payment_orders"
            referencedColumns: ["id"]
          },
        ]
      }
      payment_order_audit: {
        Row: {
          actor: string
          actor_user_id: string | null
          created_at: string
          from_status: string | null
          id: string
          note: string | null
          order_id: string
          owner_id: string
          to_status: string
        }
        Insert: {
          actor: string
          actor_user_id?: string | null
          created_at?: string
          from_status?: string | null
          id?: string
          note?: string | null
          order_id: string
          owner_id: string
          to_status: string
        }
        Update: {
          actor?: string
          actor_user_id?: string | null
          created_at?: string
          from_status?: string | null
          id?: string
          note?: string | null
          order_id?: string
          owner_id?: string
          to_status?: string
        }
        Relationships: [
          {
            foreignKeyName: "payment_order_audit_order_id_fkey"
            columns: ["order_id"]
            isOneToOne: false
            referencedRelation: "payment_orders"
            referencedColumns: ["id"]
          },
        ]
      }
      payment_orders: {
        Row: {
          amount_minor: number
          checkout_url: string | null
          contact_hint: string | null
          created_at: string
          created_by: string | null
          currency: string
          device_mac: string | null
          failure_reason: string | null
          fulfilled_at: string | null
          id: string
          idempotency_key: string
          issued_code: string | null
          method: string
          note: string | null
          owner_id: string
          plan_id: string | null
          plan_key: string | null
          plan_label: string
          provider: string
          provider_ref: string | null
          refunded_at: string | null
          router_id: string | null
          settled_at: string | null
          site_id: string | null
          status: string
          updated_at: string
          voucher_code_id: string | null
        }
        Insert: {
          amount_minor: number
          checkout_url?: string | null
          contact_hint?: string | null
          created_at?: string
          created_by?: string | null
          currency?: string
          device_mac?: string | null
          failure_reason?: string | null
          fulfilled_at?: string | null
          id?: string
          idempotency_key: string
          issued_code?: string | null
          method?: string
          note?: string | null
          owner_id: string
          plan_id?: string | null
          plan_key?: string | null
          plan_label: string
          provider?: string
          provider_ref?: string | null
          refunded_at?: string | null
          router_id?: string | null
          settled_at?: string | null
          site_id?: string | null
          status?: string
          updated_at?: string
          voucher_code_id?: string | null
        }
        Update: {
          amount_minor?: number
          checkout_url?: string | null
          contact_hint?: string | null
          created_at?: string
          created_by?: string | null
          currency?: string
          device_mac?: string | null
          failure_reason?: string | null
          fulfilled_at?: string | null
          id?: string
          idempotency_key?: string
          issued_code?: string | null
          method?: string
          note?: string | null
          owner_id?: string
          plan_id?: string | null
          plan_key?: string | null
          plan_label?: string
          provider?: string
          provider_ref?: string | null
          refunded_at?: string | null
          router_id?: string | null
          settled_at?: string | null
          site_id?: string | null
          status?: string
          updated_at?: string
          voucher_code_id?: string | null
        }
        Relationships: [
          {
            foreignKeyName: "payment_orders_plan_id_fkey"
            columns: ["plan_id"]
            isOneToOne: false
            referencedRelation: "portal_plans"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "payment_orders_router_id_fkey"
            columns: ["router_id"]
            isOneToOne: false
            referencedRelation: "router_connections"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "payment_orders_site_id_fkey"
            columns: ["site_id"]
            isOneToOne: false
            referencedRelation: "sites"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "payment_orders_voucher_code_id_fkey"
            columns: ["voucher_code_id"]
            isOneToOne: false
            referencedRelation: "voucher_codes"
            referencedColumns: ["id"]
          },
        ]
      }
      payment_receipts: {
        Row: {
          created_at: string
          id: string
          mime_type: string
          object_key: string
          order_id: string
          owner_id: string
          reference: string | null
          reject_reason: string | null
          reviewed_at: string | null
          reviewed_by: string | null
          size_bytes: number
          status: string
          submitted_at: string
          updated_at: string
        }
        Insert: {
          created_at?: string
          id?: string
          mime_type: string
          object_key: string
          order_id: string
          owner_id: string
          reference?: string | null
          reject_reason?: string | null
          reviewed_at?: string | null
          reviewed_by?: string | null
          size_bytes: number
          status?: string
          submitted_at?: string
          updated_at?: string
        }
        Update: {
          created_at?: string
          id?: string
          mime_type?: string
          object_key?: string
          order_id?: string
          owner_id?: string
          reference?: string | null
          reject_reason?: string | null
          reviewed_at?: string | null
          reviewed_by?: string | null
          size_bytes?: number
          status?: string
          submitted_at?: string
          updated_at?: string
        }
        Relationships: [
          {
            foreignKeyName: "payment_receipts_order_id_fkey"
            columns: ["order_id"]
            isOneToOne: false
            referencedRelation: "payment_orders"
            referencedColumns: ["id"]
          },
        ]
      }
      payment_review_tokens: {
        Row: {
          actor_ref: string | null
          channel: string
          created_at: string
          expires_at: string
          id: string
          order_id: string
          owner_id: string
          receipt_id: string | null
          token_hash: string
          used_action: string | null
          used_at: string | null
        }
        Insert: {
          actor_ref?: string | null
          channel?: string
          created_at?: string
          expires_at: string
          id?: string
          order_id: string
          owner_id: string
          receipt_id?: string | null
          token_hash: string
          used_action?: string | null
          used_at?: string | null
        }
        Update: {
          actor_ref?: string | null
          channel?: string
          created_at?: string
          expires_at?: string
          id?: string
          order_id?: string
          owner_id?: string
          receipt_id?: string | null
          token_hash?: string
          used_action?: string | null
          used_at?: string | null
        }
        Relationships: [
          {
            foreignKeyName: "payment_review_tokens_order_id_fkey"
            columns: ["order_id"]
            isOneToOne: false
            referencedRelation: "payment_orders"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "payment_review_tokens_receipt_id_fkey"
            columns: ["receipt_id"]
            isOneToOne: false
            referencedRelation: "payment_receipts"
            referencedColumns: ["id"]
          },
        ]
      }
      platform_admins: {
        Row: {
          created_at: string
          note: string | null
          user_id: string
        }
        Insert: {
          created_at?: string
          note?: string | null
          user_id: string
        }
        Update: {
          created_at?: string
          note?: string | null
          user_id?: string
        }
        Relationships: []
      }
      platform_service_bank_accounts: {
        Row: {
          account_number: string
          bank_name: string
          configured_by: string
          created_at: string
          enabled: boolean
          holder_name: string
          id: string
          slot: number
          sort: number
          updated_at: string
        }
        Insert: {
          account_number?: string
          bank_name?: string
          configured_by: string
          created_at?: string
          enabled?: boolean
          holder_name?: string
          id?: string
          slot: number
          sort?: number
          updated_at?: string
        }
        Update: {
          account_number?: string
          bank_name?: string
          configured_by?: string
          created_at?: string
          enabled?: boolean
          holder_name?: string
          id?: string
          slot?: number
          sort?: number
          updated_at?: string
        }
        Relationships: []
      }
      portal_deploy_audit: {
        Row: {
          created_at: string
          duration_ms: number | null
          error: string | null
          files: Json
          id: string
          ok: boolean
          owner_id: string
          router_id: string
          router_name: string
          snapshot: Json
          user_id: string
        }
        Insert: {
          created_at?: string
          duration_ms?: number | null
          error?: string | null
          files?: Json
          id?: string
          ok?: boolean
          owner_id: string
          router_id: string
          router_name: string
          snapshot?: Json
          user_id: string
        }
        Update: {
          created_at?: string
          duration_ms?: number | null
          error?: string | null
          files?: Json
          id?: string
          ok?: boolean
          owner_id?: string
          router_id?: string
          router_name?: string
          snapshot?: Json
          user_id?: string
        }
        Relationships: [
          {
            foreignKeyName: "portal_deploy_audit_router_id_fkey"
            columns: ["router_id"]
            isOneToOne: false
            referencedRelation: "router_connections"
            referencedColumns: ["id"]
          },
        ]
      }
      portal_mode_grants: {
        Row: {
          created_at: string
          granted_by: string
          mode: string
          user_id: string
        }
        Insert: {
          created_at?: string
          granted_by: string
          mode: string
          user_id: string
        }
        Update: {
          created_at?: string
          granted_by?: string
          mode?: string
          user_id?: string
        }
        Relationships: []
      }
      portal_mode_role_defaults: {
        Row: {
          mode: string
          role: Database["public"]["Enums"]["app_role"]
          updated_at: string
          updated_by: string
        }
        Insert: {
          mode: string
          role: Database["public"]["Enums"]["app_role"]
          updated_at?: string
          updated_by: string
        }
        Update: {
          mode?: string
          role?: Database["public"]["Enums"]["app_role"]
          updated_at?: string
          updated_by?: string
        }
        Relationships: []
      }
      portal_plans: {
        Row: {
          created_at: string
          data_quota_mb: number | null
          device_limit: number
          duration_label: string
          duration_minutes: number | null
          id: string
          is_vip: boolean
          label: string
          manual_code: string | null
          owner_id: string
          plan_key: string | null
          price_label: string
          price_mmk: number
          rate_limit: string | null
          sort: number
          status: string
          updated_at: string
          validity_days: number | null
        }
        Insert: {
          created_at?: string
          data_quota_mb?: number | null
          device_limit?: number
          duration_label?: string
          duration_minutes?: number | null
          id?: string
          is_vip?: boolean
          label: string
          manual_code?: string | null
          owner_id: string
          plan_key?: string | null
          price_label?: string
          price_mmk?: number
          rate_limit?: string | null
          sort?: number
          status?: string
          updated_at?: string
          validity_days?: number | null
        }
        Update: {
          created_at?: string
          data_quota_mb?: number | null
          device_limit?: number
          duration_label?: string
          duration_minutes?: number | null
          id?: string
          is_vip?: boolean
          label?: string
          manual_code?: string | null
          owner_id?: string
          plan_key?: string | null
          price_label?: string
          price_mmk?: number
          rate_limit?: string | null
          sort?: number
          status?: string
          updated_at?: string
          validity_days?: number | null
        }
        Relationships: []
      }
      portal_settings: {
        Row: {
          ask_desk_hint: string
          business_name: string
          commerce_channels: Json
          glass_tint_hex: string
          guest_mode: string
          hero_path: string | null
          logo_path: string | null
          need_code_label: string
          notify_ticket_activation: boolean
          owner_id: string
          payment_methods: Json
          primary_hex: string
          seller_label: string
          seller_phone: string | null
          terms: string
          trial_cooldown_hours: number
          trial_minutes: number
          updated_at: string
          welcome_text: string
        }
        Insert: {
          ask_desk_hint?: string
          business_name?: string
          commerce_channels?: Json
          glass_tint_hex?: string
          guest_mode?: string
          hero_path?: string | null
          logo_path?: string | null
          need_code_label?: string
          notify_ticket_activation?: boolean
          owner_id: string
          payment_methods?: Json
          primary_hex?: string
          seller_label?: string
          seller_phone?: string | null
          terms?: string
          trial_cooldown_hours?: number
          trial_minutes?: number
          updated_at?: string
          welcome_text?: string
        }
        Update: {
          ask_desk_hint?: string
          business_name?: string
          commerce_channels?: Json
          glass_tint_hex?: string
          guest_mode?: string
          hero_path?: string | null
          logo_path?: string | null
          need_code_label?: string
          notify_ticket_activation?: boolean
          owner_id?: string
          payment_methods?: Json
          primary_hex?: string
          seller_label?: string
          seller_phone?: string | null
          terms?: string
          trial_cooldown_hours?: number
          trial_minutes?: number
          updated_at?: string
          welcome_text?: string
        }
        Relationships: []
      }
      pricing_promo: {
        Row: {
          active: boolean
          annual_promo_mmk: number
          annual_standard_mmk: number
          ends_on: string
          id: boolean
          label: string
          monthly_promo_mmk: number
          monthly_standard_mmk: number
          starts_on: string
          updated_at: string
        }
        Insert: {
          active?: boolean
          annual_promo_mmk?: number
          annual_standard_mmk?: number
          ends_on?: string
          id?: boolean
          label?: string
          monthly_promo_mmk?: number
          monthly_standard_mmk?: number
          starts_on?: string
          updated_at?: string
        }
        Update: {
          active?: boolean
          annual_promo_mmk?: number
          annual_standard_mmk?: number
          ends_on?: string
          id?: boolean
          label?: string
          monthly_promo_mmk?: number
          monthly_standard_mmk?: number
          starts_on?: string
          updated_at?: string
        }
        Relationships: []
      }
      platform_announcement: {
        Row: {
          id: boolean
          active: boolean
          message: string
          speed_seconds: number
          updated_at: string
          updated_by: string | null
        }
        Insert: {
          id?: boolean
          active?: boolean
          message?: string
          speed_seconds?: number
          updated_at?: string
          updated_by?: string | null
        }
        Update: {
          id?: boolean
          active?: boolean
          message?: string
          speed_seconds?: number
          updated_at?: string
          updated_by?: string | null
        }
        Relationships: []
      }
      profiles: {
        Row: {
          avatar_url: string | null
          created_at: string
          display_name: string | null
          display_name_changed_at: string | null
          id: string
          language: string
          updated_at: string
          username: string | null
        }
        Insert: {
          avatar_url?: string | null
          created_at?: string
          display_name?: string | null
          display_name_changed_at?: string | null
          id: string
          language?: string
          updated_at?: string
          username?: string | null
        }
        Update: {
          avatar_url?: string | null
          created_at?: string
          display_name?: string | null
          display_name_changed_at?: string | null
          id?: string
          language?: string
          updated_at?: string
          username?: string | null
        }
        Relationships: []
      }
      reseller_add_key_activations: {
        Row: {
          account_bound: boolean
          attributes: string
          consumed_at: string | null
          consumed_reseller_id: string | null
          created_at: string
          expires_at: string
          id: string
          key_id: number
          key_name: string
          non_transferable: boolean
          owner_id: string
          price_coins: number
          user_id: string
        }
        Insert: {
          account_bound?: boolean
          attributes?: string
          consumed_at?: string | null
          consumed_reseller_id?: string | null
          created_at?: string
          expires_at: string
          id?: string
          key_id?: number
          key_name?: string
          non_transferable?: boolean
          owner_id: string
          price_coins?: number
          user_id: string
        }
        Update: {
          account_bound?: boolean
          attributes?: string
          consumed_at?: string | null
          consumed_reseller_id?: string | null
          created_at?: string
          expires_at?: string
          id?: string
          key_id?: number
          key_name?: string
          non_transferable?: boolean
          owner_id?: string
          price_coins?: number
          user_id?: string
        }
        Relationships: [
          {
            foreignKeyName: "reseller_add_key_activations_consumed_reseller_id_fkey"
            columns: ["consumed_reseller_id"]
            isOneToOne: false
            referencedRelation: "voucher_resellers"
            referencedColumns: ["id"]
          },
        ]
      }
      router_connections: {
        Row: {
          allow_insecure_tls: boolean
          cloud_last_error: string | null
          cloud_last_handshake_at: string | null
          cloud_last_seen_at: string | null
          cloud_peer_id: string | null
          cloud_status: string
          cloud_wg_address: string | null
          cloud_wg_private_key_ciphertext: string | null
          cloud_wg_public_key: string | null
          connection_mode: string
          connection_preference: string
          connector_id: string | null
          created_at: string
          environment: string
          host: string
          id: string
          insecure_tls_approved_at: string | null
          insecure_tls_approved_by: string | null
          insecure_tls_reason: string | null
          is_default: boolean
          is_virtual: boolean
          last_active_path: string | null
          last_rotated_at: string | null
          name: string
          owner_id: string
          password_ciphertext: string
          pending_tunnel_private_key_ciphertext: string | null
          pending_tunnel_public_key: string | null
          port: number
          rotation_started_at: string | null
          site_id: string | null
          tunnel_address: string | null
          tunnel_hub_id: string | null
          tunnel_last_check_at: string | null
          tunnel_last_ok: boolean | null
          tunnel_listen_port: number | null
          tunnel_private_key_ciphertext: string | null
          tunnel_public_key: string | null
          updated_at: string
          use_tls: boolean
          username: string
        }
        Insert: {
          allow_insecure_tls?: boolean
          cloud_last_error?: string | null
          cloud_last_handshake_at?: string | null
          cloud_last_seen_at?: string | null
          cloud_peer_id?: string | null
          cloud_status?: string
          cloud_wg_address?: string | null
          cloud_wg_private_key_ciphertext?: string | null
          cloud_wg_public_key?: string | null
          connection_mode?: string
          connection_preference?: string
          connector_id?: string | null
          created_at?: string
          environment?: string
          host: string
          id?: string
          insecure_tls_approved_at?: string | null
          insecure_tls_approved_by?: string | null
          insecure_tls_reason?: string | null
          is_default?: boolean
          is_virtual?: boolean
          last_active_path?: string | null
          last_rotated_at?: string | null
          name: string
          owner_id: string
          password_ciphertext: string
          pending_tunnel_private_key_ciphertext?: string | null
          pending_tunnel_public_key?: string | null
          port?: number
          rotation_started_at?: string | null
          site_id?: string | null
          tunnel_address?: string | null
          tunnel_hub_id?: string | null
          tunnel_last_check_at?: string | null
          tunnel_last_ok?: boolean | null
          tunnel_listen_port?: number | null
          tunnel_private_key_ciphertext?: string | null
          tunnel_public_key?: string | null
          updated_at?: string
          use_tls?: boolean
          username: string
        }
        Update: {
          allow_insecure_tls?: boolean
          cloud_last_error?: string | null
          cloud_last_handshake_at?: string | null
          cloud_last_seen_at?: string | null
          cloud_peer_id?: string | null
          cloud_status?: string
          cloud_wg_address?: string | null
          cloud_wg_private_key_ciphertext?: string | null
          cloud_wg_public_key?: string | null
          connection_mode?: string
          connection_preference?: string
          connector_id?: string | null
          created_at?: string
          environment?: string
          host?: string
          id?: string
          insecure_tls_approved_at?: string | null
          insecure_tls_approved_by?: string | null
          insecure_tls_reason?: string | null
          is_default?: boolean
          is_virtual?: boolean
          last_active_path?: string | null
          last_rotated_at?: string | null
          name?: string
          owner_id?: string
          password_ciphertext?: string
          pending_tunnel_private_key_ciphertext?: string | null
          pending_tunnel_public_key?: string | null
          port?: number
          rotation_started_at?: string | null
          site_id?: string | null
          tunnel_address?: string | null
          tunnel_hub_id?: string | null
          tunnel_last_check_at?: string | null
          tunnel_last_ok?: boolean | null
          tunnel_listen_port?: number | null
          tunnel_private_key_ciphertext?: string | null
          tunnel_public_key?: string | null
          updated_at?: string
          use_tls?: boolean
          username?: string
        }
        Relationships: [
          {
            foreignKeyName: "router_connections_connector_id_fkey"
            columns: ["connector_id"]
            isOneToOne: false
            referencedRelation: "connectors"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "router_connections_site_id_fkey"
            columns: ["site_id"]
            isOneToOne: false
            referencedRelation: "sites"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "router_connections_tunnel_hub_id_fkey"
            columns: ["tunnel_hub_id"]
            isOneToOne: false
            referencedRelation: "tunnel_hubs"
            referencedColumns: ["id"]
          },
        ]
      }
      router_ops_audit: {
        Row: {
          action: string
          created_at: string
          detail: string | null
          duration_ms: number | null
          environment: string
          error_message: string | null
          id: string
          outcome: string
          owner_id: string | null
          router_id: string | null
          router_name: string | null
          user_id: string
        }
        Insert: {
          action: string
          created_at?: string
          detail?: string | null
          duration_ms?: number | null
          environment?: string
          error_message?: string | null
          id?: string
          outcome?: string
          owner_id?: string | null
          router_id?: string | null
          router_name?: string | null
          user_id: string
        }
        Update: {
          action?: string
          created_at?: string
          detail?: string | null
          duration_ms?: number | null
          environment?: string
          error_message?: string | null
          id?: string
          outcome?: string
          owner_id?: string | null
          router_id?: string | null
          router_name?: string | null
          user_id?: string
        }
        Relationships: []
      }
      router_save_audit: {
        Row: {
          action: string
          attempted_host: string | null
          attempted_name: string | null
          created_at: string
          error_code: string | null
          error_message: string | null
          id: string
          owner_id: string | null
          router_id: string | null
          success: boolean
          user_id: string
        }
        Insert: {
          action: string
          attempted_host?: string | null
          attempted_name?: string | null
          created_at?: string
          error_code?: string | null
          error_message?: string | null
          id?: string
          owner_id?: string | null
          router_id?: string | null
          success: boolean
          user_id: string
        }
        Update: {
          action?: string
          attempted_host?: string | null
          attempted_name?: string | null
          created_at?: string
          error_code?: string | null
          error_message?: string | null
          id?: string
          owner_id?: string | null
          router_id?: string | null
          success?: boolean
          user_id?: string
        }
        Relationships: []
      }
      router_support_grants: {
        Row: {
          actions: string[]
          created_at: string
          created_by: string
          expires_at: string
          grantee_user_id: string
          id: string
          owner_id: string
          reason: string | null
          revoked_at: string | null
          router_id: string
          updated_at: string
        }
        Insert: {
          actions?: string[]
          created_at?: string
          created_by: string
          expires_at: string
          grantee_user_id: string
          id?: string
          owner_id: string
          reason?: string | null
          revoked_at?: string | null
          router_id: string
          updated_at?: string
        }
        Update: {
          actions?: string[]
          created_at?: string
          created_by?: string
          expires_at?: string
          grantee_user_id?: string
          id?: string
          owner_id?: string
          reason?: string | null
          revoked_at?: string | null
          router_id?: string
          updated_at?: string
        }
        Relationships: [
          {
            foreignKeyName: "router_support_grants_router_id_fkey"
            columns: ["router_id"]
            isOneToOne: false
            referencedRelation: "router_connections"
            referencedColumns: ["id"]
          },
        ]
      }
      router_unlock_key_activations: {
        Row: {
          account_bound: boolean
          attributes: string
          consumed_at: string | null
          consumed_router_id: string | null
          created_at: string
          expires_at: string
          id: string
          key_id: number
          key_name: string
          non_transferable: boolean
          owner_id: string
          price_coins: number
          unique_router_key: boolean
          user_id: string
        }
        Insert: {
          account_bound?: boolean
          attributes?: string
          consumed_at?: string | null
          consumed_router_id?: string | null
          created_at?: string
          expires_at: string
          id?: string
          key_id?: number
          key_name?: string
          non_transferable?: boolean
          owner_id: string
          price_coins?: number
          unique_router_key?: boolean
          user_id: string
        }
        Update: {
          account_bound?: boolean
          attributes?: string
          consumed_at?: string | null
          consumed_router_id?: string | null
          created_at?: string
          expires_at?: string
          id?: string
          key_id?: number
          key_name?: string
          non_transferable?: boolean
          owner_id?: string
          price_coins?: number
          unique_router_key?: boolean
          user_id?: string
        }
        Relationships: [
          {
            foreignKeyName: "router_unlock_key_activations_consumed_router_id_fkey"
            columns: ["consumed_router_id"]
            isOneToOne: false
            referencedRelation: "router_connections"
            referencedColumns: ["id"]
          },
        ]
      }
      ruijie_cloud_audit: {
        Row: {
          action: string
          created_at: string
          id: string
          metadata: Json
          owner_id: string
          result: string
          user_id: string
        }
        Insert: {
          action: string
          created_at?: string
          id?: string
          metadata?: Json
          owner_id: string
          result: string
          user_id: string
        }
        Update: {
          action?: string
          created_at?: string
          id?: string
          metadata?: Json
          owner_id?: string
          result?: string
          user_id?: string
        }
        Relationships: []
      }
      ruijie_cloud_connections: {
        Row: {
          app_id_ciphertext: string | null
          app_secret_ciphertext: string | null
          base_url: string
          created_at: string
          created_by: string
          device_path: string | null
          device_ref: string | null
          enabled: boolean
          id: string
          last_sync_at: string | null
          last_test_at: string | null
          last_test_status: string | null
          owner_id: string
          region: string
          site_ref: string | null
          token_path: string | null
          updated_at: string
        }
        Insert: {
          app_id_ciphertext?: string | null
          app_secret_ciphertext?: string | null
          base_url: string
          created_at?: string
          created_by: string
          device_path?: string | null
          device_ref?: string | null
          enabled?: boolean
          id?: string
          last_sync_at?: string | null
          last_test_at?: string | null
          last_test_status?: string | null
          owner_id: string
          region?: string
          site_ref?: string | null
          token_path?: string | null
          updated_at?: string
        }
        Update: {
          app_id_ciphertext?: string | null
          app_secret_ciphertext?: string | null
          base_url?: string
          created_at?: string
          created_by?: string
          device_path?: string | null
          device_ref?: string | null
          enabled?: boolean
          id?: string
          last_sync_at?: string | null
          last_test_at?: string | null
          last_test_status?: string | null
          owner_id?: string
          region?: string
          site_ref?: string | null
          token_path?: string | null
          updated_at?: string
        }
        Relationships: []
      }
      sandbox_routers: {
        Row: {
          attached_aps: Json
          created_at: string
          id: string
          model: string
          owner_id: string
          router_connection_id: string
          updated_at: string
        }
        Insert: {
          attached_aps?: Json
          created_at?: string
          id?: string
          model: string
          owner_id: string
          router_connection_id: string
          updated_at?: string
        }
        Update: {
          attached_aps?: Json
          created_at?: string
          id?: string
          model?: string
          owner_id?: string
          router_connection_id?: string
          updated_at?: string
        }
        Relationships: [
          {
            foreignKeyName: "sandbox_routers_router_connection_id_fkey"
            columns: ["router_connection_id"]
            isOneToOne: true
            referencedRelation: "router_connections"
            referencedColumns: ["id"]
          },
        ]
      }
      sandbox_state: {
        Row: {
          owner_id: string
          router_connection_id: string
          updated_at: string
          world: Json
        }
        Insert: {
          owner_id: string
          router_connection_id: string
          updated_at?: string
          world?: Json
        }
        Update: {
          owner_id?: string
          router_connection_id?: string
          updated_at?: string
          world?: Json
        }
        Relationships: [
          {
            foreignKeyName: "sandbox_state_router_connection_id_fkey"
            columns: ["router_connection_id"]
            isOneToOne: true
            referencedRelation: "router_connections"
            referencedColumns: ["id"]
          },
        ]
      }
      service_purchase_audit: {
        Row: {
          action: string
          actor_user_id: string | null
          created_at: string
          id: string
          note: string | null
          purchase_id: string
        }
        Insert: {
          action: string
          actor_user_id?: string | null
          created_at?: string
          id?: string
          note?: string | null
          purchase_id: string
        }
        Update: {
          action?: string
          actor_user_id?: string | null
          created_at?: string
          id?: string
          note?: string | null
          purchase_id?: string
        }
        Relationships: [
          {
            foreignKeyName: "service_purchase_audit_purchase_id_fkey"
            columns: ["purchase_id"]
            isOneToOne: false
            referencedRelation: "service_purchases"
            referencedColumns: ["id"]
          },
        ]
      }
      service_purchases: {
        Row: {
          activated_expires_at: string | null
          coin_amount: number | null
          created_at: string
          decided_at: string | null
          decided_by: string | null
          developer_id: string | null
          id: string
          idempotency_key: string
          magic_coin_purchase_amount: number | null
          owner_id: string
          payment_method: string
          price_mmk: number
          receipt_mime: string | null
          receipt_object_key: string | null
          receipt_size_bytes: number | null
          reference: string | null
          refund_reason: string | null
          refunded_at: string | null
          refunded_by: string | null
          reject_reason: string | null
          service_key: string
          service_label: string
          status: string
          updated_at: string
          user_id: string
        }
        Insert: {
          activated_expires_at?: string | null
          coin_amount?: number | null
          created_at?: string
          decided_at?: string | null
          decided_by?: string | null
          developer_id?: string | null
          id?: string
          idempotency_key: string
          magic_coin_purchase_amount?: number | null
          owner_id: string
          payment_method?: string
          price_mmk: number
          receipt_mime?: string | null
          receipt_object_key?: string | null
          receipt_size_bytes?: number | null
          reference?: string | null
          refund_reason?: string | null
          refunded_at?: string | null
          refunded_by?: string | null
          reject_reason?: string | null
          service_key: string
          service_label: string
          status?: string
          updated_at?: string
          user_id: string
        }
        Update: {
          activated_expires_at?: string | null
          coin_amount?: number | null
          created_at?: string
          decided_at?: string | null
          decided_by?: string | null
          developer_id?: string | null
          id?: string
          idempotency_key?: string
          magic_coin_purchase_amount?: number | null
          owner_id?: string
          payment_method?: string
          price_mmk?: number
          receipt_mime?: string | null
          receipt_object_key?: string | null
          receipt_size_bytes?: number | null
          reference?: string | null
          refund_reason?: string | null
          refunded_at?: string | null
          refunded_by?: string | null
          reject_reason?: string | null
          service_key?: string
          service_label?: string
          status?: string
          updated_at?: string
          user_id?: string
        }
        Relationships: []
      }
      site_topology_config: {
        Row: {
          downstream_devices: Json
          port_labels: Json
          router_id: string | null
          site_id: string
          updated_at: string
        }
        Insert: {
          downstream_devices?: Json
          port_labels?: Json
          router_id?: string | null
          site_id: string
          updated_at?: string
        }
        Update: {
          downstream_devices?: Json
          port_labels?: Json
          router_id?: string | null
          site_id?: string
          updated_at?: string
        }
        Relationships: [
          {
            foreignKeyName: "site_topology_config_router_id_fkey"
            columns: ["router_id"]
            isOneToOne: false
            referencedRelation: "router_connections"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "site_topology_config_site_id_fkey"
            columns: ["site_id"]
            isOneToOne: true
            referencedRelation: "sites"
            referencedColumns: ["id"]
          },
        ]
      }
      sites: {
        Row: {
          created_at: string
          id: string
          latitude: number | null
          location: string | null
          longitude: number | null
          name: string
          notes: string | null
          owner_id: string
          timezone: string | null
          updated_at: string
        }
        Insert: {
          created_at?: string
          id?: string
          latitude?: number | null
          location?: string | null
          longitude?: number | null
          name: string
          notes?: string | null
          owner_id: string
          timezone?: string | null
          updated_at?: string
        }
        Update: {
          created_at?: string
          id?: string
          latitude?: number | null
          location?: string | null
          longitude?: number | null
          name?: string
          notes?: string | null
          owner_id?: string
          timezone?: string | null
          updated_at?: string
        }
        Relationships: []
      }
      syslog_events: {
        Row: {
          ai_severity: string | null
          ai_summary: string | null
          facility: string | null
          id: string
          message: string
          owner_id: string
          program: string | null
          received_at: string
          router_id: string | null
          severity: string
          site_id: string | null
          source_ip: string | null
          token_id: string | null
        }
        Insert: {
          ai_severity?: string | null
          ai_summary?: string | null
          facility?: string | null
          id?: string
          message: string
          owner_id: string
          program?: string | null
          received_at?: string
          router_id?: string | null
          severity?: string
          site_id?: string | null
          source_ip?: string | null
          token_id?: string | null
        }
        Update: {
          ai_severity?: string | null
          ai_summary?: string | null
          facility?: string | null
          id?: string
          message?: string
          owner_id?: string
          program?: string | null
          received_at?: string
          router_id?: string | null
          severity?: string
          site_id?: string | null
          source_ip?: string | null
          token_id?: string | null
        }
        Relationships: [
          {
            foreignKeyName: "syslog_events_router_id_fkey"
            columns: ["router_id"]
            isOneToOne: false
            referencedRelation: "router_connections"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "syslog_events_site_id_fkey"
            columns: ["site_id"]
            isOneToOne: false
            referencedRelation: "sites"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "syslog_events_token_id_fkey"
            columns: ["token_id"]
            isOneToOne: false
            referencedRelation: "syslog_tokens"
            referencedColumns: ["id"]
          },
        ]
      }
      syslog_tokens: {
        Row: {
          created_at: string
          id: string
          label: string | null
          last_used_at: string | null
          owner_id: string
          router_id: string | null
          site_id: string | null
          token_hash: string
          token_prefix: string | null
        }
        Insert: {
          created_at?: string
          id?: string
          label?: string | null
          last_used_at?: string | null
          owner_id: string
          router_id?: string | null
          site_id?: string | null
          token_hash: string
          token_prefix?: string | null
        }
        Update: {
          created_at?: string
          id?: string
          label?: string | null
          last_used_at?: string | null
          owner_id?: string
          router_id?: string | null
          site_id?: string | null
          token_hash?: string
          token_prefix?: string | null
        }
        Relationships: [
          {
            foreignKeyName: "syslog_tokens_router_id_fkey"
            columns: ["router_id"]
            isOneToOne: false
            referencedRelation: "router_connections"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "syslog_tokens_site_id_fkey"
            columns: ["site_id"]
            isOneToOne: false
            referencedRelation: "sites"
            referencedColumns: ["id"]
          },
        ]
      }
      terminal_history: {
        Row: {
          body: string | null
          created_at: string
          id: string
          method: string
          ms: number
          owner_id: string
          path: string
          response_snippet: string | null
          router_id: string
          site_id: string | null
          status: number
          user_id: string
        }
        Insert: {
          body?: string | null
          created_at?: string
          id?: string
          method: string
          ms?: number
          owner_id: string
          path: string
          response_snippet?: string | null
          router_id: string
          site_id?: string | null
          status?: number
          user_id: string
        }
        Update: {
          body?: string | null
          created_at?: string
          id?: string
          method?: string
          ms?: number
          owner_id?: string
          path?: string
          response_snippet?: string | null
          router_id?: string
          site_id?: string | null
          status?: number
          user_id?: string
        }
        Relationships: [
          {
            foreignKeyName: "terminal_history_router_id_fkey"
            columns: ["router_id"]
            isOneToOne: false
            referencedRelation: "router_connections"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "terminal_history_site_id_fkey"
            columns: ["site_id"]
            isOneToOne: false
            referencedRelation: "sites"
            referencedColumns: ["id"]
          },
        ]
      }
      terminal_templates: {
        Row: {
          body: string | null
          category: string
          created_at: string
          created_by: string | null
          description: string | null
          id: string
          is_builtin: boolean
          method: string
          name: string
          owner_id: string
          path: string
          updated_at: string
        }
        Insert: {
          body?: string | null
          category?: string
          created_at?: string
          created_by?: string | null
          description?: string | null
          id?: string
          is_builtin?: boolean
          method?: string
          name: string
          owner_id: string
          path: string
          updated_at?: string
        }
        Update: {
          body?: string | null
          category?: string
          created_at?: string
          created_by?: string | null
          description?: string | null
          id?: string
          is_builtin?: boolean
          method?: string
          name?: string
          owner_id?: string
          path?: string
          updated_at?: string
        }
        Relationships: []
      }
      tunnel_hubs: {
        Row: {
          created_at: string
          https_base_url: string
          hub_public_key: string
          id: string
          name: string
          notes: string | null
          owner_id: string
          subnet_cidr: string
          updated_at: string
          wg_endpoint: string
        }
        Insert: {
          created_at?: string
          https_base_url: string
          hub_public_key: string
          id?: string
          name: string
          notes?: string | null
          owner_id: string
          subnet_cidr?: string
          updated_at?: string
          wg_endpoint: string
        }
        Update: {
          created_at?: string
          https_base_url?: string
          hub_public_key?: string
          id?: string
          name?: string
          notes?: string | null
          owner_id?: string
          subnet_cidr?: string
          updated_at?: string
          wg_endpoint?: string
        }
        Relationships: []
      }
      unifi_controllers: {
        Row: {
          allow_insecure_tls: boolean
          api_base_path: string | null
          brand: string
          capabilities: Json
          connector_id: string | null
          created_at: string
          host: string
          id: string
          is_unifi_os: boolean
          name: string
          owner_id: string
          password_ciphertext: string
          port: number
          router_id: string | null
          site_id: string | null
          unifi_site: string
          updated_at: string
          username: string
        }
        Insert: {
          allow_insecure_tls?: boolean
          api_base_path?: string | null
          brand?: string
          capabilities?: Json
          connector_id?: string | null
          created_at?: string
          host: string
          id?: string
          is_unifi_os?: boolean
          name: string
          owner_id: string
          password_ciphertext: string
          port?: number
          router_id?: string | null
          site_id?: string | null
          unifi_site?: string
          updated_at?: string
          username: string
        }
        Update: {
          allow_insecure_tls?: boolean
          api_base_path?: string | null
          brand?: string
          capabilities?: Json
          connector_id?: string | null
          created_at?: string
          host?: string
          id?: string
          is_unifi_os?: boolean
          name?: string
          owner_id?: string
          password_ciphertext?: string
          port?: number
          router_id?: string | null
          site_id?: string | null
          unifi_site?: string
          updated_at?: string
          username?: string
        }
        Relationships: [
          {
            foreignKeyName: "unifi_controllers_connector_id_fkey"
            columns: ["connector_id"]
            isOneToOne: false
            referencedRelation: "connectors"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "unifi_controllers_router_id_fkey"
            columns: ["router_id"]
            isOneToOne: false
            referencedRelation: "router_connections"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "unifi_controllers_site_id_fkey"
            columns: ["site_id"]
            isOneToOne: false
            referencedRelation: "sites"
            referencedColumns: ["id"]
          },
        ]
      }
      user_roles: {
        Row: {
          created_at: string
          expires_at: string | null
          id: string
          owner_id: string | null
          role: Database["public"]["Enums"]["app_role"]
          user_id: string
        }
        Insert: {
          created_at?: string
          expires_at?: string | null
          id?: string
          owner_id?: string | null
          role: Database["public"]["Enums"]["app_role"]
          user_id: string
        }
        Update: {
          created_at?: string
          expires_at?: string | null
          id?: string
          owner_id?: string | null
          role?: Database["public"]["Enums"]["app_role"]
          user_id?: string
        }
        Relationships: []
      }
      voucher_codes: {
        Row: {
          code: string
          created_at: string
          deploy_version: string | null
          device_mac: string | null
          duration_minutes: number | null
          expires_at: string | null
          first_seen_at: string | null
          hotspot_profile: string | null
          id: string
          order_id: string | null
          owner_id: string
          plan_id: string | null
          plan_key: string
          plan_label: string
          price_mmk: number
          profile_bound_at: string | null
          router_id: string | null
          site_id: string | null
          status: string
          updated_at: string
        }
        Insert: {
          code: string
          created_at?: string
          deploy_version?: string | null
          device_mac?: string | null
          duration_minutes?: number | null
          expires_at?: string | null
          first_seen_at?: string | null
          hotspot_profile?: string | null
          id?: string
          order_id?: string | null
          owner_id: string
          plan_id?: string | null
          plan_key: string
          plan_label: string
          price_mmk?: number
          profile_bound_at?: string | null
          router_id?: string | null
          site_id?: string | null
          status?: string
          updated_at?: string
        }
        Update: {
          code?: string
          created_at?: string
          deploy_version?: string | null
          device_mac?: string | null
          duration_minutes?: number | null
          expires_at?: string | null
          first_seen_at?: string | null
          hotspot_profile?: string | null
          id?: string
          order_id?: string | null
          owner_id?: string
          plan_id?: string | null
          plan_key?: string
          plan_label?: string
          price_mmk?: number
          profile_bound_at?: string | null
          router_id?: string | null
          site_id?: string | null
          status?: string
          updated_at?: string
        }
        Relationships: [
          {
            foreignKeyName: "voucher_codes_order_id_fkey"
            columns: ["order_id"]
            isOneToOne: false
            referencedRelation: "payment_orders"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "voucher_codes_plan_id_fkey"
            columns: ["plan_id"]
            isOneToOne: false
            referencedRelation: "portal_plans"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "voucher_codes_router_id_fkey"
            columns: ["router_id"]
            isOneToOne: false
            referencedRelation: "router_connections"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "voucher_codes_site_id_fkey"
            columns: ["site_id"]
            isOneToOne: false
            referencedRelation: "sites"
            referencedColumns: ["id"]
          },
        ]
      }
      voucher_ledger_reconciliations: {
        Row: {
          action: string
          actor_user_id: string
          after_status: string
          before_status: string
          code: string
          created_at: string
          id: string
          owner_id: string
          reason: string
          router_id: string | null
          voucher_id: string
        }
        Insert: {
          action: string
          actor_user_id: string
          after_status: string
          before_status: string
          code: string
          created_at?: string
          id?: string
          owner_id: string
          reason: string
          router_id?: string | null
          voucher_id: string
        }
        Update: {
          action?: string
          actor_user_id?: string
          after_status?: string
          before_status?: string
          code?: string
          created_at?: string
          id?: string
          owner_id?: string
          reason?: string
          router_id?: string | null
          voucher_id?: string
        }
        Relationships: [
          {
            foreignKeyName: "voucher_ledger_reconciliations_router_id_fkey"
            columns: ["router_id"]
            isOneToOne: false
            referencedRelation: "router_connections"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "voucher_ledger_reconciliations_voucher_id_fkey"
            columns: ["voucher_id"]
            isOneToOne: false
            referencedRelation: "voucher_codes"
            referencedColumns: ["id"]
          },
        ]
      }
      voucher_legacy_imports: {
        Row: {
          actor_user_id: string
          code: string
          created_at: string
          id: string
          import_reason: string
          owner_id: string
          plan_id: string
          router_id: string
          router_profile: string
          voucher_id: string
        }
        Insert: {
          actor_user_id: string
          code: string
          created_at?: string
          id?: string
          import_reason: string
          owner_id: string
          plan_id: string
          router_id: string
          router_profile: string
          voucher_id: string
        }
        Update: {
          actor_user_id?: string
          code?: string
          created_at?: string
          id?: string
          import_reason?: string
          owner_id?: string
          plan_id?: string
          router_id?: string
          router_profile?: string
          voucher_id?: string
        }
        Relationships: [
          {
            foreignKeyName: "voucher_legacy_imports_plan_id_fkey"
            columns: ["plan_id"]
            isOneToOne: false
            referencedRelation: "portal_plans"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "voucher_legacy_imports_router_id_fkey"
            columns: ["router_id"]
            isOneToOne: false
            referencedRelation: "router_connections"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "voucher_legacy_imports_voucher_id_fkey"
            columns: ["voucher_id"]
            isOneToOne: false
            referencedRelation: "voucher_codes"
            referencedColumns: ["id"]
          },
        ]
      }
      voucher_prices: {
        Row: {
          created_at: string
          currency: string
          id: string
          owner_id: string
          price_cents: number
          profile: string
          router_id: string | null
          site_id: string | null
          updated_at: string
        }
        Insert: {
          created_at?: string
          currency?: string
          id?: string
          owner_id: string
          price_cents: number
          profile: string
          router_id?: string | null
          site_id?: string | null
          updated_at?: string
        }
        Update: {
          created_at?: string
          currency?: string
          id?: string
          owner_id?: string
          price_cents?: number
          profile?: string
          router_id?: string | null
          site_id?: string | null
          updated_at?: string
        }
        Relationships: [
          {
            foreignKeyName: "voucher_prices_router_id_fkey"
            columns: ["router_id"]
            isOneToOne: false
            referencedRelation: "router_connections"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "voucher_prices_site_id_fkey"
            columns: ["site_id"]
            isOneToOne: false
            referencedRelation: "sites"
            referencedColumns: ["id"]
          },
        ]
      }
      voucher_print_layouts: {
        Row: {
          a4_columns: number
          a4_rows_per_page: number
          business_name: string
          created_at: string
          owner_id: string
          paper_width_mm: number
          show_expiry: boolean
          show_price: boolean
          show_qr: boolean
          support_contact: string
          terms: string
          thermal_columns: number
          thermal_rows_per_strip: number
          updated_at: string
          wifi_name: string
        }
        Insert: {
          a4_columns?: number
          a4_rows_per_page?: number
          business_name?: string
          created_at?: string
          owner_id: string
          paper_width_mm?: number
          show_expiry?: boolean
          show_price?: boolean
          show_qr?: boolean
          support_contact?: string
          terms?: string
          thermal_columns?: number
          thermal_rows_per_strip?: number
          updated_at?: string
          wifi_name?: string
        }
        Update: {
          a4_columns?: number
          a4_rows_per_page?: number
          business_name?: string
          created_at?: string
          owner_id?: string
          paper_width_mm?: number
          show_expiry?: boolean
          show_price?: boolean
          show_qr?: boolean
          support_contact?: string
          terms?: string
          thermal_columns?: number
          thermal_rows_per_strip?: number
          updated_at?: string
          wifi_name?: string
        }
        Relationships: []
      }
      voucher_reseller_assignments: {
        Row: {
          cash_due_mmk: number
          created_at: string
          id: string
          issued_at: string
          note: string | null
          owner_id: string
          reseller_id: string
          settled_at: string | null
          status: string
          updated_at: string
          voucher_id: string
        }
        Insert: {
          cash_due_mmk?: number
          created_at?: string
          id?: string
          issued_at?: string
          note?: string | null
          owner_id: string
          reseller_id: string
          settled_at?: string | null
          status?: string
          updated_at?: string
          voucher_id: string
        }
        Update: {
          cash_due_mmk?: number
          created_at?: string
          id?: string
          issued_at?: string
          note?: string | null
          owner_id?: string
          reseller_id?: string
          settled_at?: string | null
          status?: string
          updated_at?: string
          voucher_id?: string
        }
        Relationships: [
          {
            foreignKeyName: "voucher_reseller_assignments_reseller_id_fkey"
            columns: ["reseller_id"]
            isOneToOne: false
            referencedRelation: "voucher_resellers"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "voucher_reseller_assignments_voucher_id_fkey"
            columns: ["voucher_id"]
            isOneToOne: false
            referencedRelation: "voucher_codes"
            referencedColumns: ["id"]
          },
        ]
      }
      voucher_resellers: {
        Row: {
          active: boolean
          code_prefix: string
          contact_name: string
          created_at: string
          id: string
          location: string | null
          owner_id: string
          phone: string | null
          shop_name: string
          updated_at: string
        }
        Insert: {
          active?: boolean
          code_prefix: string
          contact_name: string
          created_at?: string
          id?: string
          location?: string | null
          owner_id: string
          phone?: string | null
          shop_name: string
          updated_at?: string
        }
        Update: {
          active?: boolean
          code_prefix?: string
          contact_name?: string
          created_at?: string
          id?: string
          location?: string | null
          owner_id?: string
          phone?: string | null
          shop_name?: string
          updated_at?: string
        }
        Relationships: []
      }
      voucher_sales: {
        Row: {
          code: string
          created_at: string
          currency: string
          id: string
          note: string | null
          owner_id: string
          price_cents: number
          profile: string
          router_id: string | null
          site_id: string | null
          sold_at: string
          sold_by: string | null
        }
        Insert: {
          code: string
          created_at?: string
          currency?: string
          id?: string
          note?: string | null
          owner_id: string
          price_cents: number
          profile: string
          router_id?: string | null
          site_id?: string | null
          sold_at?: string
          sold_by?: string | null
        }
        Update: {
          code?: string
          created_at?: string
          currency?: string
          id?: string
          note?: string | null
          owner_id?: string
          price_cents?: number
          profile?: string
          router_id?: string | null
          site_id?: string | null
          sold_at?: string
          sold_by?: string | null
        }
        Relationships: [
          {
            foreignKeyName: "voucher_sales_router_id_fkey"
            columns: ["router_id"]
            isOneToOne: false
            referencedRelation: "router_connections"
            referencedColumns: ["id"]
          },
          {
            foreignKeyName: "voucher_sales_site_id_fkey"
            columns: ["site_id"]
            isOneToOne: false
            referencedRelation: "sites"
            referencedColumns: ["id"]
          },
        ]
      }
      webfig_unlock_key_activations: {
        Row: {
          account_bound: boolean
          attributes: string
          created_at: string
          expires_at: string
          id: string
          key_id: number
          key_name: string
          non_transferable: boolean
          owner_id: string
          price_coins: number
          user_id: string
        }
        Insert: {
          account_bound?: boolean
          attributes?: string
          created_at?: string
          expires_at: string
          id?: string
          key_id?: number
          key_name?: string
          non_transferable?: boolean
          owner_id: string
          price_coins?: number
          user_id: string
        }
        Update: {
          account_bound?: boolean
          attributes?: string
          created_at?: string
          expires_at?: string
          id?: string
          key_id?: number
          key_name?: string
          non_transferable?: boolean
          owner_id?: string
          price_coins?: number
          user_id?: string
        }
        Relationships: []
      }
    }
    Views: {
      [_ in never]: never
    }
    Functions: {
      can_manage_router_tenant: {
        Args: { _tenant_owner_id: string; _user_id: string }
        Returns: boolean
      }
      can_operate_portal_guest_mode: {
        Args: { _mode: string; _user_id: string }
        Returns: boolean
      }
      connector_rate_hit: {
        Args: { _key: string; _limit: number; _window_seconds: number }
        Returns: boolean
      }
      create_reseller_with_key: {
        Args: {
          _code_prefix?: string
          _contact_name: string
          _location?: string
          _phone?: string
          _shop_name: string
        }
        Returns: Json
      }
      effective_owner: { Args: { _user_id: string }; Returns: string }
      expire_stale_clients: { Args: never; Returns: number }
      get_magic_coin_wallet: { Args: never; Returns: Json }
      get_magic_dude_unlock: { Args: never; Returns: Json }
      get_reseller_add_keys: { Args: never; Returns: Json }
      get_router_unlock_keys: { Args: never; Returns: Json }
      get_webfig_unlock_key: { Args: never; Returns: Json }
      has_active_magic_dude_unlock: { Args: never; Returns: boolean }
      has_active_webfig_unlock_key: { Args: never; Returns: boolean }
      has_feature: {
        Args: { _feature: string; _user_id: string }
        Returns: boolean
      }
      has_role: {
        Args: {
          _role: Database["public"]["Enums"]["app_role"]
          _user_id: string
        }
        Returns: boolean
      }
      has_tenant_role: {
        Args: {
          _owner_id: string
          _role: Database["public"]["Enums"]["app_role"]
          _user_id: string
        }
        Returns: boolean
      }
      import_legacy_router_vouchers: {
        Args: {
          _actor_user_id: string
          _imports: Json
          _owner_id: string
          _reason: string
          _router_id: string
        }
        Returns: Json
      }
      is_expired: { Args: { _user_id: string }; Returns: boolean }
      is_platform_admin: { Args: { _user_id: string }; Returns: boolean }
      is_ruijie_privileged: { Args: { _user_id: string }; Returns: boolean }
      list_features: { Args: { _user_id: string }; Returns: string[] }
      magic_dude_is_active_user: {
        Args: { _user_id: string }
        Returns: boolean
      }
      owner_operations_can_manage_reseller_inventory: {
        Args: { _owner: string }
        Returns: boolean
      }
      owner_operations_can_read: { Args: { _owner: string }; Returns: boolean }
      owner_operations_can_write: { Args: { _owner: string }; Returns: boolean }
      owner_operations_is_trial_account: {
        Args: { _user_id: string }
        Returns: boolean
      }
      pay_service_with_magic_coins: {
        Args: { _service: string }
        Returns: Json
      }
      purchase_magic_dude_unlock: { Args: never; Returns: Json }
      purchase_reseller_add_key: { Args: never; Returns: Json }
      purchase_router_unlock_key: { Args: never; Returns: Json }
      purchase_webfig_unlock_key: { Args: never; Returns: Json }
      reactivate_router_with_key: {
        Args: { _router_id: string }
        Returns: Json
      }
      reconcile_voucher_ledger: {
        Args: { _action: string; _reason: string; _voucher_ids: string[] }
        Returns: Json
      }
      router_unlock_key_accessible: {
        Args: { _router_id: string }
        Returns: boolean
      }
      router_unlock_key_accessible_for_owner: {
        Args: { _owner_id: string; _router_id: string }
        Returns: boolean
      }
      seed_owner_defaults: { Args: { _owner: string }; Returns: undefined }
      seed_terminal_common_configuration_templates: {
        Args: { _owner: string }
        Returns: undefined
      }
      seed_terminal_templates: { Args: { _owner: string }; Returns: undefined }
      // ── Hand-pinned 2026-09-19: these RPCs exist in the live DB, but this
      // generated file predates their migrations. Regenerate rather than edit
      // once the Supabase CLI is available here. ─────────────────────────────
      delete_i18n_override: {
        Args: { _key: string; _locale: string; _ns: string }
        Returns: undefined
      }
      get_i18n_overrides: { Args: never; Returns: Json }
      get_switch_unlock_key: { Args: never; Returns: Json }
      purchase_switch_unlock_key: { Args: never; Returns: Json }
      set_i18n_override: {
        Args: { _key: string; _locale: string; _ns: string; _value: string }
        Returns: undefined
      }
      switch_unlock_key_active: { Args: never; Returns: boolean }
    }
    Enums: {
      app_role:
        | "primary"
        | "client"
        | "expired"
        | "admin"
        | "site_manager"
        | "read_only"
        | "agent"
        | "pending"
        | "dev"
    }
    CompositeTypes: {
      [_ in never]: never
    }
  }
}

type DatabaseWithoutInternals = Omit<Database, "__InternalSupabase">

type DefaultSchema = DatabaseWithoutInternals[Extract<keyof Database, "public">]

export type Tables<
  DefaultSchemaTableNameOrOptions extends
    | keyof (DefaultSchema["Tables"] & DefaultSchema["Views"])
    | { schema: keyof DatabaseWithoutInternals },
  TableName extends (DefaultSchemaTableNameOrOptions extends {
    schema: keyof DatabaseWithoutInternals
  }
    ? keyof (DatabaseWithoutInternals[DefaultSchemaTableNameOrOptions["schema"]]["Tables"] &
        DatabaseWithoutInternals[DefaultSchemaTableNameOrOptions["schema"]]["Views"])
    : never) = never,
> = DefaultSchemaTableNameOrOptions extends {
  schema: keyof DatabaseWithoutInternals
}
  ? (DatabaseWithoutInternals[DefaultSchemaTableNameOrOptions["schema"]]["Tables"] &
      DatabaseWithoutInternals[DefaultSchemaTableNameOrOptions["schema"]]["Views"])[TableName] extends {
      Row: infer R
    }
    ? R
    : never
  : DefaultSchemaTableNameOrOptions extends keyof (DefaultSchema["Tables"] &
        DefaultSchema["Views"])
    ? (DefaultSchema["Tables"] &
        DefaultSchema["Views"])[DefaultSchemaTableNameOrOptions] extends {
        Row: infer R
      }
      ? R
      : never
    : never

export type TablesInsert<
  DefaultSchemaTableNameOrOptions extends
    | keyof DefaultSchema["Tables"]
    | { schema: keyof DatabaseWithoutInternals },
  TableName extends (DefaultSchemaTableNameOrOptions extends {
    schema: keyof DatabaseWithoutInternals
  }
    ? keyof DatabaseWithoutInternals[DefaultSchemaTableNameOrOptions["schema"]]["Tables"]
    : never) = never,
> = DefaultSchemaTableNameOrOptions extends {
  schema: keyof DatabaseWithoutInternals
}
  ? DatabaseWithoutInternals[DefaultSchemaTableNameOrOptions["schema"]]["Tables"][TableName] extends {
      Insert: infer I
    }
    ? I
    : never
  : DefaultSchemaTableNameOrOptions extends keyof DefaultSchema["Tables"]
    ? DefaultSchema["Tables"][DefaultSchemaTableNameOrOptions] extends {
        Insert: infer I
      }
      ? I
      : never
    : never

export type TablesUpdate<
  DefaultSchemaTableNameOrOptions extends
    | keyof DefaultSchema["Tables"]
    | { schema: keyof DatabaseWithoutInternals },
  TableName extends (DefaultSchemaTableNameOrOptions extends {
    schema: keyof DatabaseWithoutInternals
  }
    ? keyof DatabaseWithoutInternals[DefaultSchemaTableNameOrOptions["schema"]]["Tables"]
    : never) = never,
> = DefaultSchemaTableNameOrOptions extends {
  schema: keyof DatabaseWithoutInternals
}
  ? DatabaseWithoutInternals[DefaultSchemaTableNameOrOptions["schema"]]["Tables"][TableName] extends {
      Update: infer U
    }
    ? U
    : never
  : DefaultSchemaTableNameOrOptions extends keyof DefaultSchema["Tables"]
    ? DefaultSchema["Tables"][DefaultSchemaTableNameOrOptions] extends {
        Update: infer U
      }
      ? U
      : never
    : never

export type Enums<
  DefaultSchemaEnumNameOrOptions extends
    | keyof DefaultSchema["Enums"]
    | { schema: keyof DatabaseWithoutInternals },
  EnumName extends (DefaultSchemaEnumNameOrOptions extends {
    schema: keyof DatabaseWithoutInternals
  }
    ? keyof DatabaseWithoutInternals[DefaultSchemaEnumNameOrOptions["schema"]]["Enums"]
    : never) = never,
> = DefaultSchemaEnumNameOrOptions extends {
  schema: keyof DatabaseWithoutInternals
}
  ? DatabaseWithoutInternals[DefaultSchemaEnumNameOrOptions["schema"]]["Enums"][EnumName]
  : DefaultSchemaEnumNameOrOptions extends keyof DefaultSchema["Enums"]
    ? DefaultSchema["Enums"][DefaultSchemaEnumNameOrOptions]
    : never

export type CompositeTypes<
  PublicCompositeTypeNameOrOptions extends
    | keyof DefaultSchema["CompositeTypes"]
    | { schema: keyof DatabaseWithoutInternals },
  CompositeTypeName extends (PublicCompositeTypeNameOrOptions extends {
    schema: keyof DatabaseWithoutInternals
  }
    ? keyof DatabaseWithoutInternals[PublicCompositeTypeNameOrOptions["schema"]]["CompositeTypes"]
    : never) = never,
> = PublicCompositeTypeNameOrOptions extends {
  schema: keyof DatabaseWithoutInternals
}
  ? DatabaseWithoutInternals[PublicCompositeTypeNameOrOptions["schema"]]["CompositeTypes"][CompositeTypeName]
  : PublicCompositeTypeNameOrOptions extends keyof DefaultSchema["CompositeTypes"]
    ? DefaultSchema["CompositeTypes"][PublicCompositeTypeNameOrOptions]
    : never

export const Constants = {
  public: {
    Enums: {
      app_role: [
        "primary",
        "client",
        "expired",
        "admin",
        "site_manager",
        "read_only",
        "agent",
        "pending",
        "dev",
      ],
    },
  },
} as const
