export type PostListItem = {
  slug: string
  title: string
  date: string
  excerpt: string
  filename: string
  has_video: boolean
  published: boolean
  video_file?: string
  youtube_url?: string
}

export type JobStatus = {
  id: string
  slug: string
  avatar_id: string
  phase: string
  progress: number
  error?: string
  video_file?: string
  created_at: string
  updated_at: string
  concat_enabled: boolean
  concat_intro?: string
  concat_outro?: string
  chroma_key_enabled: boolean
  chroma_key_bg?: string
}

export type QueueItem = {
  id: string
  slug: string
  title: string
  phase: string
  avatar_id?: string
  concat_enabled: boolean
  concat_intro?: string
  concat_outro?: string
  chroma_key_enabled: boolean
  chroma_key_bg?: string
  created_at: string
}

export type VideoFile = {
  name: string
  size: number
  size_mb: string
  modified: string
}

export type TimelapseBuildJob = {
  id: string
  status: string
  output_file?: string
  error?: string
  params?: {
    from?: string
    to?: string
  }
  created_at?: string
  updated_at?: string
}

export type TimelapseNarrateJob = {
  id: string
  slug: string
  phase: string
  progress: number
  error?: string
  video_file?: string
  created_at?: string
  updated_at?: string
}

export type Resources = {
  intros: string[]
  outros: string[]
  backgrounds: string[]
}

export type NarratorConfig = {
  narrator_url: string
  narrator_online: boolean
  musetalk_url?: string
  default_avatar?: string
  avatars?: string[]
  posts_dir?: string
  output_dir?: string
  concat_enabled?: boolean
  chroma_key_enabled?: boolean
  poll_interval?: string
  max_wait?: string
}

export type MessagingQueueCounters = {
  ready: number
  unacked: number
  consumers: number
}

export type MessagingDriftDetail = {
  resource: string
  field: string
  expected: string
  actual: string
}

export type MessagingCatalogueStatus = {
  id: string
  name: string
  state: 'ready' | 'missing' | 'drifted' | 'unavailable' | 'disabled'
  queues?: Record<string, MessagingQueueCounters>
  drift?: MessagingDriftDetail[]
  error?: string
}

export type MessagingTemplateInfo = {
  id: string
  name: string
  catalogue_id: string
  description: string
  requires_confirmation: boolean
  allowed_routing_keys?: string[]
  default_routing_key?: string
  default_payload: Record<string, unknown>
}

export type MessagingPrometheusSummary = {
  available: boolean
  queues?: Record<string, MessagingQueueCounters>
  error?: string
}

export type MessagingOverview = {
  enabled: boolean
  broker_status: string
  grafana_url?: string
  catalogues: MessagingCatalogueStatus[]
  templates: MessagingTemplateInfo[]
  prometheus?: MessagingPrometheusSummary
  error?: string
}

export type MessagingPublishRequest = {
  payload: Record<string, unknown>
  routing_key?: string
  confirmed: boolean
}

export type MessagingPublishResult = {
  message_id: string
  exchange: string
  routing_key: string
  routed: boolean
}

export type AutomaticPlantKey = 'basil' | 'chilli' | 'oregano'

export type AutomaticWateringPlantConfig = {
  enabled: boolean
  threshold_percent: number
  metric_label_value: string
}

export type AutomaticWateringConfig = {
  enabled: boolean
  evaluation_interval_seconds: number
  cooldown_seconds: number
  max_metric_age_seconds: number
  prometheus_timeout_seconds: number
  message_expiry_seconds: number
  moisture_metric: string
  plant_label: string
  plants: Record<AutomaticPlantKey, AutomaticWateringPlantConfig>
}

export type AutomaticWateringPlantRuntime = {
  last_evaluated_at?: string
  last_sample_at?: string
  last_value?: number
  last_decision?: string
  last_error?: string
  cooldown_until?: string
  last_message_id?: string
}

export type AutomaticWateringStatus = {
  running: boolean
  faulted: boolean
  fault?: string
  next_evaluation_at?: string
  instance_id?: string
  plants: Record<AutomaticPlantKey, AutomaticWateringPlantRuntime>
}

export type AutomaticWateringRepresentation = {
  config_revision: number
  config: AutomaticWateringConfig
  status: AutomaticWateringStatus
}
