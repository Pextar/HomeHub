// Body scroll lock with ref-counting so nested sheets (modal-over-drawer, etc.)
// don't release the lock prematurely.
//
// iOS Safari ignores `overflow: hidden` on <body>, so we pin the body with
// `position: fixed` and restore the scroll offset on unlock — the only
// approach that reliably stops background scroll on iOS.
//
// Pinning it also takes the page's scrollbar away, and on a desktop that is a
// classic scrollbar's worth of layout: the viewport widens by ~15px, every
// centred thing slides half of that, and the whole page behind the sheet
// jumps sideways at the exact moment a sheet is opening over it. Worse for
// anything that measured the page first — the player's opening transition
// snapshots the frame it grows out of at the tap, and would then unfold from
// coordinates that no longer describe anything. So the gutter the scrollbar
// occupied is handed back as padding, and the page behind holds still.

let locks = 0;
let savedScrollY = 0;
let savedBodyStyles: {
  position: string;
  top: string;
  left: string;
  right: string;
  width: string;
  overflow: string;
  paddingRight: string;
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
    paddingRight: body.style.paddingRight,
  };
  // Measured before anything moves. Zero on overlay-scrollbar platforms
  // (every phone, and desktops set that way), where there is nothing to
  // compensate for.
  const bar = window.innerWidth - document.documentElement.clientWidth;
  body.style.position = "fixed";
  body.style.top = `-${savedScrollY}px`;
  body.style.left = "0";
  body.style.right = "0";
  body.style.width = "100%";
  body.style.overflow = "hidden";
  if (bar > 0) {
    // `box-sizing: border-box` is global, so this takes the width back out of
    // the content box rather than adding to it: the page keeps the exact
    // geometry it had a frame ago.
    const pad = parseFloat(getComputedStyle(body).paddingRight) || 0;
    body.style.paddingRight = `${pad + bar}px`;
  }
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
    body.style.paddingRight = savedBodyStyles.paddingRight;
  }
  savedBodyStyles = null;
  // Restore scroll without smooth-scroll animation.
  window.scrollTo(0, savedScrollY);
}
