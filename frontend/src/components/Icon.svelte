<!--
  A single source of truth for the icons used across the app. Storing the
  paths in a record keeps every component free of inline SVG strings.
-->
<script lang="ts">
    interface Props {
        name: keyof typeof PATHS;
        size?: number;
    }
    let { name, size = 18 }: Props = $props();

    const PATHS = {
        dashboard: "M3 13h8V3H3v10zm0 8h8v-6H3v6zm10 0h8V11h-8v10zm0-18v6h8V3h-8z",
        socket:    "M7 2h2v4H7zm8 0h2v4h-2zM5 8h14a2 2 0 012 2v4a8 8 0 01-7 7.94V22h-4v-.06A8 8 0 013 14v-4a2 2 0 012-2z",
        groups:    "M12 12a4 4 0 100-8 4 4 0 000 8zm-8 8a8 8 0 0116 0H4z",
        scenes:    "M12 2l2.4 7.4H22l-6.2 4.5 2.4 7.4L12 16.8l-6.2 4.5 2.4-7.4L2 9.4h7.6z",
        clock:     "M12 2a10 10 0 100 20 10 10 0 000-20zm1 11h5v-2h-4V6h-2v7z",
        edit:      "M3 17.25V21h3.75L17.81 9.94l-3.75-3.75L3 17.25zM20.71 7.04a1 1 0 000-1.41l-2.34-2.34a1 1 0 00-1.41 0l-1.83 1.83 3.75 3.75 1.83-1.83z",
        trash:     "M6 19a2 2 0 002 2h8a2 2 0 002-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z",
        timer:     "M15 1H9v2h6V1zm-4 13h2V8h-2v6zm8.03-6.61l1.42-1.42a10 10 0 00-1.41-1.41l-1.42 1.42A8 8 0 0012 4a8 8 0 100 16 8 8 0 007.03-12.61z",
        check:     "M9 16.2L4.8 12l-1.4 1.4L9 19l12-12-1.4-1.4z",
        bolt:      "M12 2L2 22h20L12 2z",
        search:    "M10 2a8 8 0 105.3 14L21 21.7 22.7 20 17 14.3A8 8 0 0010 2zm0 2a6 6 0 110 12 6 6 0 010-12z",
        close:     "M19 6.4L17.6 5 12 10.6 6.4 5 5 6.4 10.6 12 5 17.6 6.4 19 12 13.4 17.6 19 19 17.6 13.4 12z",
        plus:      "M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6z",
        moon:      "M21 12.79A9 9 0 1111.21 3 7 7 0 0021 12.79z",
        sun:       "M12 7a5 5 0 100 10 5 5 0 000-10zm-1-5h2v3h-2zm0 17h2v3h-2zM2 11h3v2H2zm17 0h3v2h-3zM4.2 4.2l1.4 1.4 2 2-1.4 1.4-3.4-3.4zm14.2 14.2l1.4-1.4 3.4 3.4-1.4 1.4zM6.6 18l-1.4 1.4-3.4-3.4 1.4-1.4zm12.6-12.4l-2 2-1.4-1.4 3.4-3.4z",
        logout:    "M10.09 15.59L11.5 17l5-5-5-5-1.41 1.41L12.67 11H3v2h9.67l-2.58 2.59zM19 3H5c-1.11 0-2 .9-2 2v4h2V5h14v14H5v-4H3v4c0 1.1.89 2 2 2h14c1.1 0 2-.9 2-2V5c0-1.1-.9-2-2-2z",
    } as const;
</script>

<svg viewBox="0 0 24 24" width={size} height={size} aria-hidden="true">
    <path fill="currentColor" d={PATHS[name]} />
</svg>
