<script lang="ts">
    import Modal from "../components/Modal.svelte";
    import Icon from "../components/Icon.svelte";
    import { closeModal } from "../lib/modal.svelte";
    import { data } from "../lib/stores.svelte";
    import { homeLayout } from "../lib/home-layout.svelte";
    import { homeSensors } from "../lib/home-layout";
    import { untrack } from "svelte";
    import { SvelteSet } from "svelte/reactivity";
    import type { Sensor, SensorKind } from "../lib/types";

    /**
     * Which sensors the home screen is about.
     *
     * Two questions, one sheet, because they are the same question asked twice
     * — *which thermometer is "the house"*, and *which readings are worth a
     * card* — and answering one usually settles the other. Splitting them
     * would put the second door somewhere the first isn't.
     *
     * The list's order is fixed (by kind, then as the controller lists them)
     * and the saved selection follows it. Floating ticked rows to the top
     * would move a row out from under the finger that just tapped it, and
     * "the order you happened to tick them in" is not an order anyone can
     * predict a week later.
     */

    const v = $derived(data.value);

    const KIND_ORDER: readonly SensorKind[] = [
        "temperature",
        "humidity",
        "power",
        "light",
        "motion",
        "custom",
    ];
    const KIND_LABEL: Record<SensorKind, string> = {
        temperature: "Temperature",
        humidity: "Humidity",
        power: "Power",
        light: "Light",
        motion: "Motion",
        custom: "Other",
    };
    const KIND_ICON: Record<SensorKind, "temperature" | "humidity" | "power" | "light" | "motion" | "sensor"> = {
        temperature: "temperature",
        humidity: "humidity",
        power: "power",
        light: "light",
        motion: "motion",
        custom: "sensor",
    };

    /** Every sensor, grouped and in the order the list renders them. */
    const groups = $derived(
        KIND_ORDER.map((kind) => ({
            kind,
            label: KIND_LABEL[kind],
            items: v.sensors.filter((s) => s.kind === kind),
        })).filter((g) => g.items.length > 0),
    );
    const ordered = $derived(groups.flatMap((g) => g.items));

    const temps = $derived(v.sensors.filter((s) => s.kind === "temperature"));
    // With one thermometer there is nothing to choose between: the average and
    // the sensor are the same number, so the question isn't asked.
    const askTemperature = $derived(temps.length > 1);

    // Opening on "automatic" starts from what is on screen right now, so the
    // first tick is a change to what you can see rather than to an empty list.
    let picked = new SvelteSet(
        untrack(() => homeSensors(data.value.sensors, homeLayout.layout).map((s) => s.id)),
    );
    let temperature = $state(untrack(() => homeLayout.temperature));

    function toggle(id: string, on: boolean) {
        if (on) picked.add(id);
        else picked.delete(id);
    }
    function setAll(on: boolean) {
        picked.clear();
        if (on) for (const s of ordered) picked.add(s.id);
    }

    function reading(s: Sensor): string {
        if (s.last_value === undefined || s.last_value === null) return "—";
        const n = Math.abs(s.last_value) >= 100 ? 0 : Math.abs(s.last_value) >= 10 ? 1 : 2;
        return `${s.last_value.toFixed(n)}${s.unit ?? ""}`;
    }

    function save() {
        homeLayout.setSensors(ordered.filter((s) => picked.has(s.id)).map((s) => s.id));
        // A temperature sensor the user has now hidden is still a fine source
        // for the hero — the two lists answer different questions.
        homeLayout.setTemperature(temperature);
        closeModal();
    }
</script>

<Modal title="Sensors on home" subtitle="What the home screen shows, and where its temperature comes from.">
    {#snippet body()}
        {#if askTemperature}
            <section class="block">
                <h3 class="block-head">Home temperature</h3>
                <p class="field-help">
                    The reading beside the master switch, and the Temperature tile.
                </p>
                <div class="rows">
                    <label class="pick" class:picked={temperature === null}>
                        <input
                            type="radio"
                            name="home-temp"
                            checked={temperature === null}
                            onchange={() => (temperature = null)}
                        />
                        <span class="ico"><Icon name="temperature" size={16} /></span>
                        <span class="text">
                            <span class="name">Average of the house</span>
                            <span class="sub">{temps.length} temperature sensors</span>
                        </span>
                        <span class="mark" aria-hidden="true"><Icon name="check" size={14} /></span>
                    </label>
                    {#each temps as s (s.id)}
                        <label class="pick" class:picked={temperature === s.id}>
                            <input
                                type="radio"
                                name="home-temp"
                                checked={temperature === s.id}
                                onchange={() => (temperature = s.id)}
                            />
                            <span class="ico"><Icon name="temperature" size={16} /></span>
                            <span class="text">
                                <span class="name">{s.name}</span>
                                <span class="sub">{s.room || "No room"}</span>
                            </span>
                            <span class="val mono">{reading(s)}</span>
                            <span class="mark" aria-hidden="true"><Icon name="check" size={14} /></span>
                        </label>
                    {/each}
                </div>
            </section>
        {/if}

        <section class="block">
            <div class="block-top">
                <h3 class="block-head">Cards on the home screen</h3>
                <div class="bulk">
                    <button type="button" class="chip" onclick={() => setAll(true)}>All</button>
                    <button type="button" class="chip" onclick={() => setAll(false)}>None</button>
                </div>
            </div>
            <p class="field-help">
                {picked.size} of {ordered.length} chosen. They appear in the order listed here.
            </p>
            {#each groups as g (g.kind)}
                <h4 class="kind-head mono">{g.label}</h4>
                <div class="rows">
                    {#each g.items as s (s.id)}
                        <label class="pick" class:picked={picked.has(s.id)}>
                            <input
                                type="checkbox"
                                checked={picked.has(s.id)}
                                onchange={(e) => toggle(s.id, e.currentTarget.checked)}
                            />
                            <span class="ico"><Icon name={KIND_ICON[s.kind]} size={16} /></span>
                            <span class="text">
                                <span class="name">{s.name}</span>
                                <span class="sub">{s.room || "No room"}</span>
                            </span>
                            <span class="val mono">{reading(s)}</span>
                            <span class="mark" aria-hidden="true"><Icon name="check" size={14} /></span>
                        </label>
                    {/each}
                </div>
            {/each}
        </section>
    {/snippet}

    {#snippet actions()}
        <button class="btn btn-ghost" onclick={() => closeModal()}>Cancel</button>
        <button class="btn btn-primary" onclick={save}>Save</button>
    {/snippet}
</Modal>

<style>
    .block { display: flex; flex-direction: column; gap: 6px; }
    .block + .block { margin-top: var(--space-6); }
    .block-top {
        display: flex; align-items: center; justify-content: space-between;
        gap: var(--space-3); flex-wrap: wrap;
    }
    .block-head { font-size: 15px; font-weight: 600; letter-spacing: -0.02em; }
    .bulk { display: flex; gap: var(--space-2); }
    .kind-head {
        font-size: 10.5px;
        letter-spacing: 0.08em;
        text-transform: uppercase;
        color: var(--text-mute);
        margin-top: var(--space-4);
    }
    .rows { display: flex; flex-direction: column; gap: var(--space-2); margin-top: var(--space-2); }

    /* A pick row: 36px icon, name over room, its live reading, and the mark.
       The real input carries the state and the keyboard; the mark is what the
       eye reads. */
    .pick {
        display: flex;
        align-items: center;
        gap: var(--space-3);
        min-height: 52px;
        padding: 8px 12px;
        background: var(--card);
        border: 1px solid var(--hairline);
        border-radius: var(--r-md);
        cursor: pointer;
        position: relative;
        transition: border-color var(--t-fast), background var(--t-fast);
    }
    .pick input {
        position: absolute;
        opacity: 0;
        width: 0; height: 0;
        margin: 0;
    }
    .ico {
        width: 36px; height: 36px;
        flex-shrink: 0;
        border-radius: 10px;
        background: var(--card-2);
        color: var(--text-mute);
        display: grid; place-items: center;
        transition: background var(--t-fast), color var(--t-fast);
    }
    .text { display: flex; flex-direction: column; gap: 1px; min-width: 0; flex: 1; }
    .name {
        font-size: 14px; font-weight: 600;
        overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
    }
    .sub { font-size: 11.5px; color: var(--text-mute); }
    .val { font-size: 12.5px; color: var(--text-mute); flex-shrink: 0; }
    .mark {
        width: 22px; height: 22px;
        flex-shrink: 0;
        border-radius: 999px;
        border: 1px solid var(--border);
        color: transparent;
        display: grid; place-items: center;
        transition: background var(--t-fast), border-color var(--t-fast), color var(--t-fast);
    }

    /* Driven by a class rather than `:has(input:checked)` — the wall panel is
       an old iPad, and a selection that renders as "nothing is picked" on it
       would be worse than a slightly longer selector list here. */
    .pick.picked { border-color: var(--on); background: var(--on-soft); }
    .pick.picked .ico { background: var(--on); color: var(--primary-fg); }
    .pick.picked .mark {
        background: var(--on);
        border-color: var(--on);
        color: var(--primary-fg);
    }
    /* The ring belongs to the row, but only the input can say it has keyboard
       focus — so the row's outline is drawn by an overlay the input reaches
       through its sibling. */
    .pick input:focus-visible ~ .mark::after {
        content: "";
        position: absolute;
        inset: 0;
        border-radius: var(--r-md);
        box-shadow: var(--focus-ring);
        pointer-events: none;
    }
    @media (hover: hover) {
        .pick:hover { border-color: var(--border-strong); }
    }
    .pick:active { transform: scale(0.99); transition-duration: 80ms; }
</style>
