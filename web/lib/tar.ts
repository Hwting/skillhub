// Minimal USTAR tar packer. No dependencies. Produces a raw (uncompressed)
// tar archive as Uint8Array from a list of {name, data} entries. Intended for
// client-side skill packaging before gzip + upload.
//
// Limitations: regular files only, names split into prefix(155)/name(100) when
// longer than 100 bytes (USTAR max 255); mtime fixed to 0; mode 0644.

const BLOCK = 512;

function octal(n: number, width: number): string {
  // width includes the trailing NUL/space the caller will overwrite; here we
  // produce the zero-padded octal digits without the terminator.
  return n.toString(8).padStart(width - 1, "0");
}

function writeStr(buf: Uint8Array, offset: number, str: string, len: number) {
  const bytes = new TextEncoder().encode(str);
  buf.set(bytes.subarray(0, Math.min(bytes.length, len)), offset);
}

function header(name: string, size: number): Uint8Array {
  let prefix = "";
  let n = name;
  if (n.length > 100) {
    // Split at a '/' so prefix<=155 and name<=100.
    const split = n.length - 100;
    let slash = n.indexOf("/", split);
    if (slash === -1) slash = n.lastIndexOf("/");
    if (slash > 0 && slash <= 155 && n.length - slash - 1 <= 100) {
      prefix = n.slice(0, slash);
      n = n.slice(slash + 1);
    } else {
      n = n.slice(0, 100); // best effort
    }
  }

  const h = new Uint8Array(BLOCK);
  writeStr(h, 0, n, 100);
  writeStr(h, 100, octal(0o644, 8), 8); // mode
  writeStr(h, 108, octal(0, 8), 8); // uid
  writeStr(h, 116, octal(0, 8), 8); // gid
  writeStr(h, 124, octal(size, 12), 12); // size
  writeStr(h, 136, octal(0, 12), 12); // mtime
  // chksum (148, 8): fill with spaces first, sum all bytes, then write octal.
  for (let i = 148; i < 156; i++) h[i] = 0x20;
  h[156] = 0x30; // typeflag '0' = regular file
  writeStr(h, 257, "ustar", 6); // magic
  h[263] = 0x30; // version "00"
  h[264] = 0x30;
  writeStr(h, 345, prefix, 155);

  let sum = 0;
  for (let i = 0; i < BLOCK; i++) sum += h[i];
  writeStr(h, 148, octal(sum, 7), 7);
  h[155] = 0x20; // space terminator
  return h;
}

function padToBlock(data: Uint8Array): Uint8Array {
  const rem = data.length % BLOCK;
  if (rem === 0) return new Uint8Array(0);
  return new Uint8Array(BLOCK - rem);
}

export interface TarEntry {
  name: string;
  data: Uint8Array;
}

export function packTar(entries: TarEntry[]): Uint8Array {
  const parts: Uint8Array[] = [];
  for (const e of entries) {
    parts.push(header(e.name, e.data.length));
    parts.push(e.data);
    parts.push(padToBlock(e.data));
  }
  // Two zero blocks mark end of archive.
  parts.push(new Uint8Array(BLOCK));
  parts.push(new Uint8Array(BLOCK));

  let total = 0;
  for (const p of parts) total += p.length;
  const out = new Uint8Array(total);
  let off = 0;
  for (const p of parts) {
    out.set(p, off);
    off += p.length;
  }
  return out;
}
