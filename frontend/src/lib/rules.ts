/**
 * What an automation rule *means*, apart from the form that edits it.
 *
 * This lived in `RuleEditor.svelte`'s module script, which made it a library
 * two modals had to import from a `.svelte` file to reach — SceneModal for
 * the colour presets, AutomationModal for the compiler. None of it draws
 * anything: it is the vocabulary of a rule (what a target can be, what a
 * blank action starts as) and the one piece of real logic in the feature,
 * `compileAction`.
 *
 * That compiler is the reason this is worth its own file. A rule is authored
 * in a shape the form can edit — a group action with per-lamp overrides is
 * one row on screen — and sent in a shape the backend runs, where that same
 * row is one action per lamp. Nothing else in the app knows the two shapes
 * differ, so the translation between them has exactly one home.
 */

import { data } from "./stores.svelte";
import { isSmartProtocol } from "./utils";
import type { RuleActionDraft, TargetType, AutomationAction, Socket } from "./types";

/** Member sockets of a group/room action target. Rooms are matched by the
 *  socket's room name (sockets reference rooms by name, targets by id). */
export function membersOf(a: RuleActionDraft): Socket[] {
    const v = data.value;
    if (a.target_type === "group") {
        const g = v.groups.find(x => x.id === a.target_id);
        return (g?.socket_ids ?? [])
            .map(id => v.sockets.find(s => s.id === id))
            .filter((s): s is Socket => !!s);
    }
    if (a.target_type === "room") {
        const rn = v.rooms.find(r => r.id === a.target_id)?.name;
        return v.sockets.filter(s => s.room === rn);
    }
    return [];
}

/** Expand one THEN draft action into the API actions it implies. A per-lamp
 *  group/room action becomes one socket action per configured member; every
 *  other action maps 1:1, mirroring the legacy inline builders. */
export function compileAction(a: RuleActionDraft): AutomationAction[] {
    if ((a.target_type === "group" || a.target_type === "room") && a.action === "on" && a.perLamp) {
        const out: AutomationAction[] = [];
        for (const m of membersOf(a)) {
            const cfg = a.perLamp[m.id] ?? { state: "on", level: a.level ?? 100, color: a.color ?? "" };
            if (cfg.state === "ignore") continue;
            const act: AutomationAction = { target_type: "socket", target_id: m.id, action: cfg.state };
            if (cfg.state === "on" && isSmartProtocol(m.protocol)) {
                act.level = cfg.level ?? 100;
                if (cfg.color) act.color = cfg.color;
            }
            out.push(act);
        }
        return out;
    }
    const base: AutomationAction = {
        target_type: a.target_type,
        target_id: a.target_id,
        action: (a.target_type === "scene" ? "activate" : a.action) as AutomationAction["action"],
    };
    if (a.action === "on" || a.action === "set") {
        if (a.target_type === "socket") {
            base.level = a.level ?? 100;
            if (a.color) base.color = a.color;
        } else if (a.target_type === "group" || a.target_type === "room") {
            // "set" changes brightness/colour only, so always carry the
            // level; an "on" only attaches lighting when moved off default.
            if (a.action === "set" || a.color || a.level !== 100) {
                base.level = a.level ?? 100;
                if (a.color) base.color = a.color;
            }
        }
    }
    return [base];
}

/** Colour presets for smart socket targets. Also used by the
 *  SceneModal snapshot section — keep the two in sync by importing. */
export const COLOURS: { hex: string; name: string }[] = [
    { hex: "", name: "Auto" },
    { hex: "f5bd6e", name: "Warm" },
    { hex: "ffe9c4", name: "Soft" },
    { hex: "ffffff", name: "Bright" },
    { hex: "c4a4e0", name: "Lilac" },
    { hex: "7aa4d9", name: "Cool" },
];

/** Selectable targets for a THEN action's target type. */
export function targetsFor(type: string): { id: string; label: string }[] {
    const v = data.value;
    if (type === "socket") return v.sockets.map(s => ({ id: s.id, label: s.name }));
    if (type === "group")  return v.groups.map(g => ({ id: g.id, label: g.name }));
    if (type === "room")   return [...v.rooms].sort((a, b) => a.name.localeCompare(b.name)).map(r => ({ id: r.id, label: r.name }));
    return v.scenes.map(s => ({ id: s.id, label: s.name }));
}

/** First target type that has at least one entity to point at. */
export function firstTargetType(): TargetType {
    const v = data.value;
    return v.sockets.length ? "socket" : v.groups.length ? "group" : v.rooms.length ? "room" : "scene";
}

/** A fresh THEN row, seeded with the first valid target so a newly
 *  added row never needs to be "fixed up" later. */
export function blankRuleAction(): RuleActionDraft {
    const target_type = firstTargetType();
    return {
        target_type,
        target_id: targetsFor(target_type)[0]?.id ?? "",
        action: target_type === "scene" ? "activate" : "on",
        level: 100,
        color: "",
    };
}

/**
 * Matter lamp presets — the same palette MatterLightModal offers, so a rule
 * that says "Relax" and a lamp set to "Relax" by hand mean the same light.
 *
 * The white presets carry a CT-approximated hex. It is there to be *shown*,
 * and to nudge the RGB mode toward the right temperature on lamps that don't
 * expose colour temperature through this bridge path.
 */
export type MatterPreset = { label: string; level: number; color: string; cssColor: string };

export const MATTER_PRESETS: MatterPreset[] = [
    { label: "Reading",     level: 100, color: "dcdbd6", cssColor: "#dcdbd6" },
    { label: "Concentrate", level: 100, color: "d2e5f4", cssColor: "#d2e5f4" },
    { label: "Daylight",    level: 100, color: "d5e2eb", cssColor: "#d5e2eb" },
    { label: "Warm",        level: 80,  color: "edcaa2", cssColor: "#edcaa2" },
    { label: "Relax",       level: 40,  color: "f1c696", cssColor: "#f1c696" },
    { label: "Night",       level: 12,  color: "f9bf7f", cssColor: "#f9bf7f" },
    { label: "Sunset",      level: 70,  color: "ff6a3d", cssColor: "#ff6a3d" },
    { label: "Forest",      level: 60,  color: "3dbf6a", cssColor: "#3dbf6a" },
    { label: "Ocean",       level: 70,  color: "3dafff", cssColor: "#3dafff" },
    { label: "Lavender",    level: 60,  color: "b47cff", cssColor: "#b47cff" },
    { label: "Rose",        level: 60,  color: "ff6fa3", cssColor: "#ff6fa3" },
];
