// Barrel for the app's TypeScript interfaces, split by domain.
//
// Everything is re-exported here, so `import type { Socket } from
// "../lib/types"` keeps working from anywhere. Import from this barrel
// rather than reaching into a domain file directly — that way moving a
// type between domains stays a one-file change.
//
// When adding a type, put it in the domain file it belongs to. If none
// fits, add a new file here rather than growing system.ts into the
// dumping ground this split just undid.

export * from "./devices";
export * from "./automation";
export * from "./users";
export * from "./system";
export * from "./sensors";
export * from "./sonos";
export * from "./kef";
export * from "./spotify";
export * from "./assistant";
export * from "./media";
