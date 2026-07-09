// Browser-side file download helpers. Used for export-manifest (generated JSON)
// and any other client-produced blob. Tarball downloads use a plain <a href> to
// the proxied /api endpoint so the browser handles Content-Disposition directly.

export function downloadBlob(filename: string, blob: Blob) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}

export function downloadText(filename: string, text: string, type = "application/json") {
  downloadBlob(filename, new Blob([text], { type }));
}
