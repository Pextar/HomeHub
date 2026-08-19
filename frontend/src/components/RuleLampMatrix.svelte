<script lang="ts">
    /**
     * Authoring a group or room's lamps one at a time.
     *
     * A THEN action normally says one thing about a whole group — "turn the
     * kitchen on at 40%". That is the right shape most of the time, and it is
     * what the editor offers first. But a group is often *nearly* uniform: the
     * ceiling on, the lamp by the sofa on and dim, the one over the sink left
     * alone. Saying that as three separate actions means three rows on screen
     * and three targets to keep in step, so a group action can instead carry a
     * per-member override map and this is the control for it.
     *
     * Two things make it usable rather than a grid of fiddly rows.
     *
     * **Switching to per-lamp seeds every member from the uniform setting the
     * user already chose**, so the matrix opens as "all the same" and they
     * change the outliers — never as an empty form to fill in.
     *
     * **The bulk bar stays after that**, because "all off except one" is
     * quicker as set-all-off then turn-one-on than as N taps. Its brightness
     * and colour show the first smart member's, which after any bulk apply is
     * what every one of them is set to.
     *
     * Three states per lamp, not two: on, off, and *unchanged* — the middle
     * one being the whole reason this exists, since a rule that touches every
     * lamp in a room is exactly the rule a uniform action already writes.
     *
     * The map is compiled back into one socket action per lamp on save; see
     * `compileAction` in lib/rules.ts.
     */
    import Icon from "./Icon.svelte";
    import LightRow from "./LightRow.svelte";
    import { membersOf } from "../lib/rules";
    import { isSmartProtocol } from "../lib/utils";
    import type { RuleActionDraft } from "../lib/types";

    let { action = $bindable() }: { action: RuleActionDraft } = $props();

    const members = $derived(membersOf(action));
    const anySmart = $derived(members.some((m) => isSmartProtocol(m.protocol)));

    type LampState = "on" | "off" | "ignore";
    type LampCfg = { state: LampState; level: number; color: string };

    /** A member's setting, falling back to the uniform one it was seeded from. */
    function cfg(id: string): LampCfg {
        return (
            action.perLamp?.[id] ?? {
                state: "on" as const,
                level: action.level ?? 100,
                color: action.color ?? "",
            }
        );
    }
    function setLamp(id: string, patch: Partial<LampCfg>) {
        if (!action.perLamp) return;
        action.perLamp[id] = { ...cfg(id), ...patch };
    }

    function setAll(state: LampState) {
        for (const m of members) setLamp(m.id, { state });
    }
    /** A bulk brightness or preset only reaches the lamps that can take one,
     *  and turns them on — setting a light on a lamp left unchanged would be
     *  a setting nobody asked to apply. */
    function setAllLight(level: number, color?: string) {
        for (const m of members) {
            if (!isSmartProtocol(m.protocol)) continue;
            setLamp(m.id, color === undefined ? { level, state: "on" } : { level, color, state: "on" });
        }
    }

    /** The first smart member stands for the bulk values: after any bulk
     *  apply every smart member matches it. */
    const firstSmart = $derived(members.find((m) => isSmartProtocol(m.protocol)));
    const bulkLevel = $derived(firstSmart ? cfg(firstSmart.id).level : 100);
    const bulkColor = $derived(firstSmart ? cfg(firstSmart.id).color : "");
</script>

{#if members.length === 0}
    <div class="lamp-empty mono">No devices in this {action.target_type}</div>
{:else}
    <div class="lamp-matrix">
        <div class="bulk-bar">
            <span class="bulk-lbl">Set all<span class="bulk-n mono">{members.length}</span></span>
            <div class="state-group" role="group" aria-label="Set all lamps">
                <button
                    type="button"
                    class="state-btn"
                    onclick={() => setAll("ignore")}
                    aria-label="Leave all lamps unchanged">—</button>
                <button
                    type="button"
                    class="state-btn s-on"
                    onclick={() => setAll("on")}
                    aria-label="Turn all lamps on">On</button>
                <button
                    type="button"
                    class="state-btn s-off"
                    onclick={() => setAll("off")}
                    aria-label="Turn all lamps off">Off</button>
            </div>
            {#if anySmart}
                <div class="bulk-light">
                    <LightRow
                        level={bulkLevel}
                        color={bulkColor}
                        forWhat="all lamps"
                        onLevel={(lvl) => setAllLight(lvl)}
                        onPreset={(lvl, col) => setAllLight(lvl, col)}
                    />
                </div>
            {/if}
        </div>

        <div class="lamp-rows">
            {#each members as m, mi (m.id)}
                {@const c = cfg(m.id)}
                {#if mi > 0}<div class="row-sep" aria-hidden="true"></div>{/if}
                <div class="lamp-row" class:row-on={c.state === "on"}>
                    <div class="lamp-main">
                        <div
                            class="row-bulb"
                            class:bulb-on={c.state === "on"}
                            class:bulb-off={c.state === "off"}
                            aria-hidden="true"
                        >
                            <Icon name="light" size={14} />
                        </div>
                        <div class="row-info">
                            <span class="row-name">{m.name}</span>
                            <span class="row-room">{m.room || "Unassigned"}</span>
                        </div>
                        <div class="state-group" role="group" aria-label="Action for {m.name}">
                            <button
                                type="button"
                                class="state-btn"
                                class:s-active={c.state === "ignore"}
                                onclick={() => setLamp(m.id, { state: "ignore" })}
                                aria-pressed={c.state === "ignore"}
                                aria-label="Leave {m.name} unchanged">—</button>
                            <button
                                type="button"
                                class="state-btn s-on"
                                class:s-active={c.state === "on"}
                                onclick={() => setLamp(m.id, { state: "on" })}
                                aria-pressed={c.state === "on"}
                                aria-label="Turn {m.name} on">On</button>
                            <button
                                type="button"
                                class="state-btn s-off"
                                class:s-active={c.state === "off"}
                                onclick={() => setLamp(m.id, { state: "off" })}
                                aria-pressed={c.state === "off"}
                                aria-label="Turn {m.name} off">Off</button>
                        </div>
                    </div>
                    <!-- A lamp's own light settings, only while it is being
                         turned on and only where it can take them. -->
                    {#if c.state === "on" && isSmartProtocol(m.protocol)}
                        <div class="light-row">
                            <LightRow
                                level={c.level}
                                color={c.color}
                                forWhat={m.name}
                                onLevel={(lvl) => setLamp(m.id, { level: lvl })}
                                onPreset={(lvl, col) => setLamp(m.id, { level: lvl, color: col })}
                            />
                        </div>
                    {/if}
                </div>
            {/each}
        </div>
    </div>
{/if}

<style>
    .lamp-empty { padding: 14px 12px; text-align: center; font-size: 12px; color: var(--text-dim); }
    .lamp-matrix {
        display: flex; flex-direction: column; gap: 8px;
        margin-top: var(--space-2);
        border: 1px solid var(--hairline); border-radius: var(--r-sm);
        background: var(--card-2); padding: 6px;
    }
    .bulk-bar {
        display: flex; align-items: center; flex-wrap: wrap; gap: 10px;
        padding: 6px 8px; border-radius: var(--r-sm); background: var(--card-3);
    }
    .bulk-lbl {
        display: inline-flex; align-items: center; gap: 6px;
        font-family: var(--font-mono); font-size: 10.5px;
        text-transform: uppercase; letter-spacing: 0.06em; color: var(--text-mute);
        flex-shrink: 0;
    }
    .bulk-n { font-size: 11px; color: var(--text-dim); background: var(--card-2); border-radius: var(--r-pill); padding: 0 6px; }
    .bulk-light { display: flex; align-items: center; gap: 12px; flex: 1; flex-wrap: wrap; min-width: 200px; }
    .bulk-light :global(.bright) { flex: 1; min-width: 140px; }

    .lamp-rows { display: flex; flex-direction: column; }
    .row-sep { height: 1px; background: var(--separator); margin: 0 10px 0 52px; }
    .lamp-row {
        display: flex; flex-direction: column;
        border-radius: var(--r-sm); overflow: hidden;
        transition: background var(--t-fast);
    }
    .lamp-row.row-on { background: var(--on-soft); }
    .lamp-main { display: flex; align-items: center; gap: 12px; padding: 8px 10px; min-height: 46px; }
    .row-bulb {
        width: 28px; height: 28px; border-radius: 50%;
        background: var(--card-3); display: grid; place-items: center;
        color: var(--text-dim); flex-shrink: 0;
        transition: background var(--t-fast), color var(--t-fast), box-shadow var(--t-fast);
    }
    .row-bulb.bulb-on {
        background: var(--on); color: var(--primary-fg);
        box-shadow: 0 0 0 1px var(--on), 0 0 14px 2px var(--on-glow);
    }
    .row-bulb.bulb-off { background: var(--bg-elevated); color: var(--text-dim); }
    .row-info { flex: 1; min-width: 0; display: flex; flex-direction: column; gap: 1px; }
    .row-name { font-size: 13px; font-weight: 500; color: var(--text); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
    .row-room { font-size: 11px; color: var(--text-mute); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }

    .state-group {
        display: flex; background: var(--bg-elevated); border: 1px solid var(--border);
        border-radius: var(--r-pill); padding: 2px; gap: 1px; flex-shrink: 0;
    }
    .state-btn {
        padding: 5px 10px; border-radius: var(--r-pill); border: none;
        background: transparent; font-size: 12px; font-weight: 500;
        color: var(--text-mute); cursor: pointer; touch-action: manipulation;
        transition: background var(--t-fast), color var(--t-fast), box-shadow var(--t-fast);
        white-space: nowrap; line-height: 1;
    }
    .state-btn:hover:not(.s-active) { color: var(--text); }
    .state-btn.s-active { background: var(--card-3); color: var(--text); box-shadow: var(--shadow-sm); }
    .state-btn.s-on.s-active { background: var(--on-soft); color: var(--on); box-shadow: none; }

    .light-row { display: flex; flex-direction: column; gap: 8px; padding: 0 10px 10px 50px; }

    @media (prefers-reduced-motion: reduce) {
        .state-btn, .row-bulb, .lamp-row { transition-duration: 0.001ms; }
    }
    @media (pointer: coarse) {
        .state-btn { min-height: 34px; }
    }
</style>
