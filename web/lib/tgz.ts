// Client-side tar.gz builder. No dependencies: tar via lib/tar, gzip via the
// browser's native CompressionStream. Used by the publish dialog to let users
// upload raw skill files/directories without pre-packaging.

import { packTar, type TarEntry } from "./tar";

async function gzip(data: Uint8Array): Promise<ArrayBuffer> {
  // CompressionStream is native (Chrome 80+, FF 113+, Safari 16.4+).
  const cs = new CompressionStream("gzip");
  const writer = cs.writable.getWriter();
  // Copy into a fresh ArrayBuffer-backed view so the byte source is an
  // ArrayBuffer (not SharedArrayBuffer) as CompressionStream requires.
  const src = new Uint8Array(data.length);
  src.set(data);
  writer.write(src);
  writer.close();
  const chunks: Uint8Array[] = [];
  const reader = cs.readable.getReader();
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    if (value) chunks.push(value);
  }
  let total = 0;
  for (const c of chunks) total += c.length;
  const out = new Uint8Array(total);
  let off = 0;
  for (const c of chunks) {
    out.set(c, off);
    off += c.length;
  }
  return out.buffer.slice(0, total);
}

// Build a tar.gz from a FileList. Directory picks (webkitdirectory) arrive with
// names containing a leading path like "dir/sub/file"; we strip a common leading
// directory component so the archive root is the folder contents.
export async function createTarGz(files: File[]): Promise<ArrayBuffer> {
  const prefix = commonDir(files.map((f) => f.webkitRelativePath || f.name));
  const entries: TarEntry[] = [];
  for (const f of files) {
    const full = f.webkitRelativePath || f.name;
    const name = full.startsWith(prefix) ? full.slice(prefix.length) : full;
    if (!name) continue;
    const data = new Uint8Array(await f.arrayBuffer());
    entries.push({ name, data });
  }
  if (entries.length === 0) throw new Error("没有可打包的文件");
  return gzip(packTar(entries));
}

function commonDir(names: string[]): string {
  if (names.length === 0) return "";
  let prefix = names[0];
  const slash = prefix.lastIndexOf("/");
  prefix = slash >= 0 ? prefix.slice(0, slash + 1) : "";
  for (const n of names) {
    while (prefix && !n.startsWith(prefix)) {
      const s = prefix.lastIndexOf("/", prefix.length - 2);
      prefix = s >= 0 ? prefix.slice(0, s + 1) : "";
    }
    if (!prefix) break;
  }
  return prefix;
}
