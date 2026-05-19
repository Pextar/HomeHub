// Body scroll lock with ref-counting so nested sheets (modal-over-drawer, etc.)
// don't release the lock prematurely.
//
// iOS Safari ignores `overflow: hidden` on <body>, so we pin the body with
// `position: fixed` and restore the scroll offset on unlock — the only
// approach that reliably stops background scroll on iOS.

let locks = 0;
let savedScrollY = 0;
let savedBodyStyles: {
  position: string;
  top: string;
  left: string;
  right: string;
  width: string;
  overflow: string;
} | null = null;

export function lockBodyScroll() {
  locks++;
  if (locks > 1) return;

  savedScrollY = window.scrollY;
  const body = document.body;
  savedBodyStyles = {
    position: body.style.position,
    top: body.style.top,
    left: body.style.left,
    right: body.style.right,
    width: body.style.width,
    overflow: body.style.overflow,
  };
  body.style.position = "fixed";
  body.style.top = `-${savedScrollY}px`;
  body.style.left = "0";
  body.style.right = "0";
  body.style.width = "100%";
  body.style.overflow = "hidden";
}

export function unlockBodyScroll() {
  if (locks === 0) return;
  locks--;
  if (locks > 0) return;

  const body = document.body;
  if (savedBodyStyles) {
    body.style.position = savedBodyStyles.position;
    body.style.top = savedBodyStyles.top;
    body.style.left = savedBodyStyles.left;
    body.style.right = savedBodyStyles.right;
    body.style.width = savedBodyStyles.width;
    body.style.overflow = savedBodyStyles.overflow;
  }
  savedBodyStyles = null;
  // Restore scroll without smooth-scroll animation.
  window.scrollTo(0, savedScrollY);
}
