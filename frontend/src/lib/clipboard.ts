// Clipboard helper. The app is served over plain HTTP on the LAN, where
// navigator.clipboard is unavailable (it requires a secure context), so
// we fall back to the legacy execCommand path — including the extra
// dance iOS Safari needs to actually copy from a hidden element.
export async function copyText(text: string): Promise<boolean> {
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      /* fall through to legacy path */
    }
  }
  try {
    const el = document.createElement("textarea");
    el.value = text;
    el.contentEditable = "true";
    el.readOnly = false;
    el.style.position = "fixed";
    el.style.top = "0";
    el.style.left = "0";
    el.style.opacity = "0";
    document.body.appendChild(el);

    const range = document.createRange();
    range.selectNodeContents(el);
    const sel = window.getSelection();
    sel?.removeAllRanges();
    sel?.addRange(range);
    el.setSelectionRange(0, text.length);

    const ok = document.execCommand("copy");
    document.body.removeChild(el);
    return ok;
  } catch {
    return false;
  }
}
