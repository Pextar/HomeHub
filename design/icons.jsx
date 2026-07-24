/* Minimal inline icon set — kept simple (lines/shapes only). */

const Icon = ({ d, size = 18, stroke = 1.6, fill = "none", style }) => (
  <svg className="ico" width={size} height={size} viewBox="0 0 24 24" fill={fill} stroke="currentColor" strokeWidth={stroke} strokeLinecap="round" strokeLinejoin="round" style={style}>
    {typeof d === "string" ? <path d={d}/> : d}
  </svg>
);

// Each icon = a tiny path. Kept abstract — no decorative svgs.
const I = {
  home:     "M3 11l9-7 9 7v9a1 1 0 0 1-1 1h-5v-7h-6v7H4a1 1 0 0 1-1-1z",
  bulb:     "M9 18h6M10 21h4M12 3a6 6 0 0 0-4 10.5c.7.8 1 1.5 1 2.5h6c0-1 .3-1.7 1-2.5A6 6 0 0 0 12 3z",
  rooms:    "M3 4h7v7H3zM14 4h7v7h-7zM3 13h7v7H3zM14 13h7v7h-7z",
  music:    "M9 18V5l11-2v13M9 18a3 3 0 1 1-6 0 3 3 0 0 1 6 0zM20 16a3 3 0 1 1-6 0 3 3 0 0 1 6 0z",
  scenes:   "M12 3l2.4 5.8L20 10l-4.6 3.4L17 19l-5-3-5 3 1.6-5.6L4 10l5.6-1.2z",
  schedule: "M12 7v5l3 2M12 22a10 10 0 1 1 0-20 10 10 0 0 1 0 20z",
  sensor:   "M12 9v6M12 2a3 3 0 0 0-3 3v7a5 5 0 1 0 6 0V5a3 3 0 0 0-3-3z",
  settings: "M12 8.5a3.5 3.5 0 1 0 0 7 3.5 3.5 0 0 0 0-7z M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.9.3l-.1.1A2 2 0 1 1 4.1 17l.1-.1a1.7 1.7 0 0 0 .3-1.9 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1A1.7 1.7 0 0 0 4.6 9a1.7 1.7 0 0 0-.3-1.9l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.9.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.9-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.9V9c.3.6 1 1 1.6 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1z",
  plus:     "M12 5v14M5 12h14",
  back:     "M15 6l-6 6 6 6",
  close:    "M6 6l12 12M6 18L18 6",
  more:     "M5 12h.01M12 12h.01M19 12h.01",
  search:   "M11 19a8 8 0 1 1 0-16 8 8 0 0 1 0 16zM21 21l-4.3-4.3",
  sun:      "M12 4V2M12 22v-2M4 12H2M22 12h-2M5.6 5.6L4.2 4.2M19.8 19.8l-1.4-1.4M5.6 18.4l-1.4 1.4M19.8 4.2l-1.4 1.4M12 7a5 5 0 1 0 0 10 5 5 0 0 0 0-10z",
  moon:     "M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z",
  power:    "M12 3v10M6.4 6.4a8 8 0 1 0 11.2 0",
  chevR:    "M9 6l6 6-6 6",
  chevD:    "M6 9l6 6 6-6",
  rf:       "M5 7c4-4 10-4 14 0M3 4c5.5-5.5 13.5-5.5 19 0M8 10c2-2 6-2 8 0M12 13v.01",
  wifi:     "M5 12.5a10 10 0 0 1 14 0M2 9a14 14 0 0 1 20 0M8.5 16a5 5 0 0 1 7 0M12 19v.01",
  matter:   "M12 3l3 3-3 3-3-3zM21 12l-3 3-3-3 3-3zM12 21l-3-3 3-3 3 3zM3 12l3-3 3 3-3 3z",
  drop:     "M12 3s-6 7-6 11a6 6 0 0 0 12 0c0-4-6-11-6-11z",
  motion:   "M5 12c0-4 3-7 7-7M19 12c0 4-3 7-7 7M9 12a3 3 0 1 0 6 0 3 3 0 0 0-6 0z",
  bell:     "M6 8a6 6 0 0 1 12 0c0 7 3 7 3 9H3c0-2 3-2 3-9zM10 22a2 2 0 0 0 4 0",
  user:     "M12 12a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM4 21a8 8 0 0 1 16 0",
  energy:   "M13 2L4 14h7l-1 8 9-12h-7z",
  star:     "M12 2l3 7 7 1-5 5 1 7-6-3-6 3 1-7-5-5 7-1z",
  group:    "M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8zM17 11a3 3 0 1 0 0-6 3 3 0 0 0 0 6zM2 21a7 7 0 0 1 14 0M16 13a5 5 0 0 1 6 5",
  thermo:   "M14 4a2 2 0 1 0-4 0v10a4 4 0 1 0 4 0V4z",
  sliders:  "M4 6h7M14 6h6M4 12h3M10 12h10M4 18h12M19 18h1M11 4v4M7 10v4M16 16v4",
};

Object.assign(window, { Icon, I });
