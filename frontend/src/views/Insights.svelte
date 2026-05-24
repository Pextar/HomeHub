<script lang="ts">
    import Topbar from "../components/Topbar.svelte";
    import Icon from "../components/Icon.svelte";
    import SensorCard from "../components/SensorCard.svelte";
    import EmptyState from "../components/EmptyState.svelte";
    import { data, route } from "../lib/stores.svelte";
    import { scale } from "svelte/transition";
    import { cubicOut } from "svelte/easing";
    import { dur, stagger } from "../lib/motion";

    const v = $derived(data.value);

    // Honest energy view: only what the power sensors actually report. No
    // fabricated cost / per-room kWh — those need backend metering we don't
    // have yet. We surface live total draw + each meter's real history.
    const powerSensors = $derived(v.sensors.filter(s => s.kind === "power"));
    const reporting = $derived(powerSensors.filter(s => s.last_value != null));
    const totalDraw = $derived(Math.round(reporting.reduce((sum, s) => sum + (s.last_value ?? 0), 0)));
    const unit = $derived(powerSensors[0]?.unit || "W");
</script>

<Topbar title="Insights" subtitle="Live energy from your power sensors" />

{#if powerSensors.length === 0}
    <EmptyState icon="chart" title="No power sensors yet"
        message="Add a sensor of type ‘power’ to see live draw and history here. Energy is read straight from your meters — nothing is estimated.">
        <button class="btn btn-primary" onclick={() => route.go("sensors")}>Go to sensors</button>
    </EmptyState>
{:else}
    <!-- Live total draw hero -->
    <div class="hero tile on">
        <div class="hero-eyebrow mono">Total draw now</div>
        <div class="hero-figure">
            <Icon name="bolt" size={22} />
            <span class="num-display">{totalDraw}</span>
            <span class="hero-unit">{unit}</span>
        </div>
        <div class="hero-sub">
            across <span class="mono">{reporting.length}</span>
            of <span class="mono">{powerSensors.length}</span>
            meter{powerSensors.length === 1 ? "" : "s"} reporting
        </div>
    </div>

    <section class="card">
        <div class="card-header">
            <h2><span class="section-ico"><Icon name="chart" size={15} /></span>Power meters</h2>
            <button class="btn btn-ghost" onclick={() => route.go("sensors")}>Manage sensors</button>
        </div>
        <div class="meters">
            {#each powerSensors as sensor, i (sensor.id)}
                <div class="meter-item"
                    in:scale={{ start: 0.96, opacity: 0, duration: dur(220), delay: stagger(i), easing: cubicOut }}>
                    <SensorCard {sensor} />
                </div>
            {/each}
        </div>
    </section>
{/if}

<style>
    .hero { padding: 20px; gap: 10px; }
    .hero-eyebrow {
        color: var(--on);
        font-size: 11px;
        letter-spacing: 0.1em;
        text-transform: uppercase;
    }
    .hero-figure {
        display: flex;
        align-items: baseline;
        gap: 10px;
        color: var(--text);
    }
    .hero-figure :global(svg) { color: var(--on); align-self: center; }
    .hero-figure .num-display { font-size: 48px; }
    .hero-unit { color: var(--text-mute); font-size: 16px; font-family: var(--font-mono); }
    .hero-sub { color: var(--text-mute); font-size: 12.5px; }

    .section-ico {
        width: 24px; height: 24px;
        border-radius: var(--r-sm);
        display: grid; place-items: center;
        background: var(--on-soft);
        color: var(--on);
        flex-shrink: 0;
    }
    .card-header h2 { display: inline-flex; align-items: center; gap: 6px; }

    .meters {
        display: grid;
        grid-template-columns: 1fr;
        gap: var(--space-3);
    }
    @media (min-width: 600px) {
        .meters { grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); }
    }
    .meter-item { display: flex; }
    .meter-item > :global(.card) { flex: 1; }
</style>
