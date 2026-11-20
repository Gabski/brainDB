// Copyright 2026 Gabriel Koziestański
// SPDX-License-Identifier: Apache-2.0

import type { ImportFile } from '../../api/types';

// Minimal typings for the (non-standard but widely supported) File System Entry API.
interface FsEntry {
  isFile: boolean;
  isDirectory: boolean;
  name: string;
  fullPath: string;
}
interface FsFileEntry extends FsEntry {
  file(ok: (f: File) => void, err: (e: unknown) => void): void;
}
interface FsDirEntry extends FsEntry {
  createReader(): { readEntries(ok: (e: FsEntry[]) => void, err: (e: unknown) => void): void };
}

async function walk(entry: FsEntry, out: ImportFile[]): Promise<void> {
  if (entry.isFile) {
    const f = await new Promise<File>((ok, err) => (entry as FsFileEntry).file(ok, err));
    out.push({ file: f, path: entry.fullPath.replace(/^\//, '') });
  } else if (entry.isDirectory) {
    const reader = (entry as FsDirEntry).createReader();
    // readEntries returns batches until empty
    for (;;) {
      const batch = await new Promise<FsEntry[]>((ok, err) => reader.readEntries(ok, err));
      if (!batch.length) break;
      for (const e of batch) await walk(e, out);
    }
  }
}

/** Files from a drop event, descending into dropped folders and keeping relative paths. */
export async function filesFromDrop(dt: DataTransfer): Promise<ImportFile[]> {
  const out: ImportFile[] = [];
  const items = [...dt.items].filter((i) => i.kind === 'file');
  const entries = items.map((i) => (i as DataTransferItem & { webkitGetAsEntry?: () => FsEntry | null }).webkitGetAsEntry?.() ?? null);
  if (entries.some(Boolean)) {
    for (const e of entries) if (e) await walk(e, out);
    return out;
  }
  return [...dt.files].map((f) => ({ file: f, path: f.name }));
}

/** Files from <input type=file webkitdirectory multiple> (or a plain multi-file input). */
export function filesFromInput(list: FileList): ImportFile[] {
  return [...list].map((f) => ({ file: f, path: (f as File & { webkitRelativePath?: string }).webkitRelativePath || f.name }));
}
