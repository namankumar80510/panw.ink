// DTOs of the custom reader API (internal/catalog).

export type Access = 'public' | 'early' | 'upcoming';

export interface Ref { slug: string; name: string }
export interface TagRef { slug: string; label: string }

export interface WorkCard {
  id: string;
  slug: string;
  kind: 'novel' | 'short_story';
  title: string;
  subtitle: string | null;
  synopsis: string;
  author: Ref;
  genre: Ref;
  tags: TagRef[];
  publication_status: 'ongoing' | 'completed' | 'hiatus' | 'cancelled';
  age_rating: 'general' | 'teen' | 'mature';
  content_warnings: string[];
  word_count: number;
  read_minutes: number;
  public_entry_count: number;
  first_public_at: string | null;
  last_public_update_at: string | null;
  featured: boolean;
  cover_url: string | null;
}

export interface EntryRef {
  id: string;
  slug: string;
  title: string;
  ordinal: number;
  access: Access;
  early_at: string | null;
  public_at: string | null;
}

export interface VolumeRef { number: number; title: string }

export interface WorkDetail extends Omit<WorkCard, 'author'> {
  full_synopsis: string;
  language: string;
  completed_at: string | null;
  author: Ref & { bio: string };
  rating: { state: 'insufficient_data' | 'ok'; average: number | null; count: number };
  start: EntryRef | null;
  next_release: EntryRef | null;
  collections: Ref[];
}

export interface TOCEntry extends EntryRef {
  volume: VolumeRef | null;
  word_count: number;
  read_seconds: number;
}

export interface EntryBody {
  id: string;
  work_id: string;
  work_kind: 'novel' | 'short_story';
  work_slug: string;
  work_title: string;
  author: Ref;
  title: string;
  slug: string;
  ordinal: number;
  volume: VolumeRef | null;
  public_at: string | null;
  early_at: string | null;
  access: { state: Access };
  body_html: string;
  word_count: number;
  read_seconds: number;
  previous: EntryRef | null;
  next: EntryRef | null;
  published_revision: number;
  revised_at: string | null;
  toc_count: number;
}

/** details of a 403 EARLY_ACCESS_REQUIRED / ENTRY_UPCOMING error */
export interface LockedDetails {
  state: Access;
  public_at: string | null;
  early_at: string | null;
  entry: {
    id: string; slug: string; title: string; ordinal: number; volume: VolumeRef | null;
    work_slug: string; work_title: string; work_kind: 'novel' | 'short_story'; author: Ref;
    previous: EntryRef | null; next: EntryRef | null; toc_count: number;
  };
}

export interface Home {
  featured: WorkCard[];
  short_reads: WorkCard[];
  completed: WorkCard[];
  ongoing_novels: WorkCard[];
  recently_updated: WorkCard[];
}

export interface CollectionCard { slug: string; title: string; description: string; author: Ref; story_count: number }
