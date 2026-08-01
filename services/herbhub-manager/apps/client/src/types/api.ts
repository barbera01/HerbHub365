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
