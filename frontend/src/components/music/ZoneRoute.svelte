<script lang="ts">
    /**
     * What a zone's next tap will actually do, in the backend's own words.
     *
     * DESIGN.md §15: a streamed mixed zone is genuinely a different thing from
     * a natively grouped one — it lands its speakers a few hundred
     * milliseconds apart, shows on Sonos as a stream rather than as a track,
     * and takes over the account's active Spotify device — so a zone whose
     * route is `stream` **says so**, using the `reason` the backend already
     * wrote, which names the speaker responsible. Nothing here infers a route
     * from the makes it can see: the read carries `route`, `sync` and `reason`
     * precisely so the UI never has to guess, and guessing is how an all-Sonos
     * zone would end up wearing stream affordances it never takes.
     *
     * The corollary is the silence: a zone on `native` or `group` gets one
     * quiet mono line about sync and no more, and a zone of one gets nothing
     * at all — there is nothing to be honest about when there is nothing to
     * keep in step.
     *
     * `airplay` sits between those two cases and gets its own line rather than
     * being folded into either. It is HomeHub decoding, like `stream`, so it
     * is worth saying — but its receivers are on one clock rather than each
     * filling their own buffer, so wearing `stream`'s "buffered" wording would
     * understate it as badly as saying "in sync" would overstate it.
     */
    import Icon from "../Icon.svelte";
    import { verdictLabel } from "../../lib/utils";
    import type { MediaChain, MediaRoute, MediaSync } from "../../lib/types";

    let {
        route = undefined,
        sync = undefined,
        reason = "",
        /** Set instead of a route when nothing can serve the zone. */
        problem = "",
        /** A zone that a start here would stop, on the single Spotify session. */
        interrupts = "",
        /** What this zone would actually sound like, source to speaker. */
        quality = undefined,
        /** `full` carries the reason sentence; `compact` is the label alone. */
        variant = "full",
    }: {
        route?: MediaRoute;
        sync?: MediaSync;
        reason?: string;
        problem?: string;
        interrupts?: string;
        quality?: MediaChain;
        variant?: "full" | "compact";
    } = $props();

    const streamed = $derived(route === "stream");
    const cast = $derived(route === "airplay");
    const verdict = $derived(verdictLabel(quality?.verdict));
</script>

{#if problem}
    <!-- Not a badge: the backend's message names which speaker blocked which
         route, and that is the actionable part. -->
    <p class="z-note bad">
        <Icon name="info" size={13} />
        <span>{problem}</span>
    </p>
{:else if streamed}
    <p class="z-note cool">
        <Icon name="radio" size={13} />
        <span>
            <span class="z-tag mono">HomeHub stream · {sync ?? "buffered"}</span>
            {#if variant === "full" && reason}<span class="z-why">{reason}</span>{/if}
        </span>
    </p>
{:else if cast}
    <p class="z-note cool">
        <Icon name="radio" size={13} />
        <span>
            <!-- "clocked" rather than "in sync": receivers are held to
                 HomeHub's clock, which is tighter than a shared buffer and
                 still not a vendor's own multi-room bus. -->
            <span class="z-tag mono">AirPlay · {sync ?? "clocked"}</span>
            {#if variant === "full" && reason}<span class="z-why">{reason}</span>{/if}
        </span>
    </p>
{:else if sync === "exact"}
    <p class="z-note quiet">
        <span class="z-tag mono">In sync{route === "group" ? " · grouped" : ""}</span>
    </p>
{/if}

<!-- What it will sound like. One line, and only where there is room for the
     sentence behind it: the compact variant is a card subtitle, and a second
     claim there would crowd out the route it qualifies.

     Never a bare "lossless"/"not lossless" badge. The interesting half is
     *what* limits it, because that is the half a listener can act on — and
     when nothing does, saying so is worth a line of its own.

     The tag reads `verdict`, not `lossless`, so the third answer survives the
     trip to the screen: a zone whose speaker fetches the service itself is
     "up to lossless", and flattening that to either neighbour is a lie in one
     direction or the other. -->
{#if quality && variant === "full" && !problem}
    <p class="z-note quiet">
        <span class="z-tag mono" class:lossless={verdict.on}>{verdict.text}</span>
        <span class="z-why">{quality.summary}</span>
    </p>
{/if}

{#if interrupts && !problem}
    <!-- One Spotify account has one active session, so a second streamed or
         Connect zone takes it from the first. Said before the tap, not
         discovered as the other room going quiet. -->
    <p class="z-note quiet">
        <Icon name="info" size={13} />
        <span>Starting this stops <strong>{interrupts}</strong> — one Spotify session at a time.</span>
    </p>
{/if}

<style>
    .z-note {
        display: flex; align-items: flex-start; gap: 6px;
        margin: 0; font-size: 12px; line-height: 1.4;
        color: var(--text-mute);
    }
    .z-note :global(svg) { flex-shrink: 0; margin-top: 1px; }
    .z-note.cool { color: var(--cool); }
    .z-note.bad { color: var(--bad); }
    .z-note.quiet { color: var(--text-dim); }
    .z-tag {
        font-size: 10px; letter-spacing: 0.08em; text-transform: uppercase;
    }
    /* Lossless takes the sanctioned "ON" amber rather than a status colour of
       its own: it is the same kind of fact as a lit lamp. */
    .z-tag.lossless { color: var(--on); }
    /* The reason follows the tag on the same line where there is room, and
       wraps under it where there isn't. */
    .z-why { color: var(--text-mute); margin-left: 6px; }
    .z-note strong { color: var(--text); font-weight: 600; }
</style>
