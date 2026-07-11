// API types mirror the Go store models (internal/store/types.go) and handler
// response shapes. Sealed secrets (community strings, channel config) are never
// returned by the API and so never appear here as readable fields.

export interface AuthStatus {
  configured: boolean
  auth_enabled: boolean
  authenticated: boolean
}

export interface Severity {
  id: number
  name: string
  rank: number
  color: string
  emoji: string
  is_builtin: boolean
}

export interface OIDEntry {
  id: number
  oid: string
  name: string
  description: string
  default_severity_id: number | null
  is_builtin: boolean
}

export interface Channel {
  id: number
  name: string
  kind: string // shoutrrr | whatsapp | webhook
  enabled: boolean
}

export interface Community {
  id: number
  enabled: boolean
}

export interface Device {
  id: number
  source_ip: string
  name: string
  enabled: boolean
}

export interface Rule {
  id: number
  ord: number
  name: string
  enabled: boolean
  continue_on_match: boolean
  bypass_flood_control: boolean
  match_device_id: number | null
  match_oid_glob: string
  match_varbind_regex: string | null
  severity_id: number | null
  channel_ids: number[]
}

export interface TrapView {
  id: number
  received_at: string
  source_ip: string
  snmp_version: string
  trap_oid: string
  resolved_name: string
  severity_id: number | null
  matched_rule_id: number | null
  varbinds_json: string
  aggregation_key: string
  suppressed: boolean
  unmapped: boolean
  severity_name: string | null
  severity_color: string | null
  severity_emoji: string | null
  device_name: string | null
  matched_rule_name: string | null
}

export interface Notification {
  id: number
  trap_id: number
  channel_id: number | null
  status: string // sent | failed | held
  attempts: number
  last_error: string
  sent_at: string | null
}

export interface Dashboard {
  traps_total: number
  traps_by_severity: Record<string, number>
  notifications: Record<string, number>
  recent: TrapView[]
}

export type Settings = Record<string, string>

// Provider-specific config field sets for the Channels form. Sent under
// `config` on create/update; on edit, an empty config preserves the saved
// secret.
export interface ShoutrrrConfig {
  url: string
}

export interface WhatsAppConfig {
  base_url: string
  recipient: string
  endpoint_path?: string
  username?: string
  password?: string
  token?: string
}

export interface WebhookHeader {
  key: string
  value: string
}

export interface WebhookConfig {
  url: string
  headers?: Record<string, string>
  body_template?: string
}

export type ChannelKind = 'shoutrrr' | 'whatsapp' | 'webhook'
