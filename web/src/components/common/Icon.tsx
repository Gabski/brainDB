// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

// Single icon wrapper over lucide-react (ISC). Every icon in the UI goes through <Icon name="…"/>.
import {
  Activity,
  ArrowDown,
  ArrowUp,
  Ban,
  BookOpen,
  Bot,
  Brain,
  Camera,
  ChartColumn,
  Check,
  ChevronsRight,
  CircleCheck,
  CircleX,
  Copy,
  Database,
  Dna,
  Download,
  Eye,
  FileCode,
  FileSpreadsheet,
  FileText,
  Filter,
  FlaskConical,
  GitFork,
  Globe,
  Hash,
  History,
  Image,
  Inbox,
  Info,
  KeyRound,
  Layers,
  Link,
  Lock,
  Minus,
  Moon,
  Network,
  Pin,
  Play,
  Plus,
  RefreshCw,
  Replace,
  Route,
  Ruler,
  Save,
  Scissors,
  Search,
  Settings,
  Shield,
  Sparkles,
  Star,
  Sun,
  Table,
  Tag,
  Target,
  Trash2,
  TriangleAlert,
  Unlink,
  Upload,
  User,
  Waypoints,
  X,
  Zap,
  type LucideIcon,
} from 'lucide-react';

export const ICONS = {
  activity: Activity,
  add: Plus,
  admin: Shield,
  bot: Bot,
  brain: Brain,
  bridge: Network,
  chart: ChartColumn,
  check: Check,
  close: X,
  code: FileCode,
  copy: Copy,
  csv: FileSpreadsheet,
  database: Database,
  denied: Ban,
  discover: Sparkles,
  docs: BookOpen,
  download: Download,
  error: CircleX,
  eye: Eye,
  file: FileText,
  filter: Filter,
  focus: Zap,
  goal: Target,
  history: History,
  hybrid: Dna,
  id: Hash,
  image: Image,
  inbox: Inbox,
  info: Info,
  key: KeyRound,
  label: Tag,
  layers: Layers,
  link: Link,
  lock: Lock,
  minus: Minus,
  moon: Moon,
  ok: CircleCheck,
  path: Route,
  pin: Pin,
  play: Play,
  playAll: ChevronsRight,
  refresh: RefreshCw,
  ruler: Ruler,
  save: Save,
  search: Search,
  settings: Settings,
  snapshot: Camera,
  snippet: Scissors,
  source: Globe,
  star: Star,
  sun: Sun,
  supersede: Replace,
  table: Table,
  test: FlaskConical,
  trash: Trash2,
  tree: GitFork,
  unlink: Unlink,
  upload: Upload,
  user: User,
  warning: TriangleAlert,
  weaken: ArrowDown,
  boost: ArrowUp,
  connector: Waypoints,
} satisfies Record<string, LucideIcon>;

export type IconName = keyof typeof ICONS;

export interface IconProps {
  name: IconName;
  /** 14 (inline text), 16 (buttons, rows), 20 (headers) — aligned to the 28px row grid. */
  size?: 14 | 16 | 20;
  /** Accessible label; without it the icon is decorative (aria-hidden). */
  label?: string;
  className?: string;
  strokeWidth?: number;
}

/** Inherits currentColor; vertically centred with text. */
export function Icon({ name, size = 16, label, className, strokeWidth = 1.75 }: IconProps) {
  const C = ICONS[name];
  return (
    <C
      size={size}
      strokeWidth={strokeWidth}
      className={'icon' + (className ? ' ' + className : '')}
      aria-hidden={label ? undefined : true}
      aria-label={label}
      role={label ? 'img' : undefined}
      focusable={false}
    />
  );
}
