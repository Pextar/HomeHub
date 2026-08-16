/**
 * The heart on the wall.
 *
 * Saving needs a catalog id, which only a Spotify source has — radio and
 * line-in carry an artist line and nothing to save — so the control renders
 * only where `trackURI` does (§15.1 applied to a track rather than a room).
 *
 * The saved *state* is read on whatever login the panel has, because
 * reading has always been in the grant; only the write needs the newer
 * scope. So an old login shows an honest heart and refuses the tap, rather
 * than being offered a control that will fail.
 */

import { api } from "../api";
import type { PanelRunner } from "./timers.svelte";

export interface PanelSavedStore {
    /** The login may write to the library. False hides the heart rather
     *  than offering a tap that will be refused. */
    readonly canSave: boolean;
    /** Whether what's playing is in the account's library. Meaningless
     *  unless the featured source has a trackURI. */
    readonly saved: boolean;
    toggle(): void;
}

export interface PanelSavedDeps {
    /** The featured source's catalog id, or undefined where there is none.
     *  A getter: what is playing moves under this. */
    trackURI: () => string | undefined;
    run: PanelRunner;
}

export function createPanelSaved(deps: PanelSavedDeps): PanelSavedStore {
    let savedURI = $state("");
    let saved = $state(false);
    let canSave = $state(false);
    let seq = 0;

    void api
        .spotifyStatus()
        .then((st) => {
            canSave = st.connected && !!st.library;
        })
        .catch(() => {
            canSave = false;
        });

    $effect(() => {
        const uri = deps.trackURI() ?? "";
        if (uri === savedURI) return;
        savedURI = uri;
        saved = false;
        const mine = ++seq;
        if (!uri) return;
        void api
            .spotifySaved(uri)
            .then((r) => {
                if (mine === seq) saved = r.saved;
            })
            .catch(() => {});
    });

    return {
        get canSave() {
            return canSave;
        },
        get saved() {
            return saved;
        },
        toggle() {
            const uri = deps.trackURI();
            if (!uri || !canSave) return;
            const next = !saved;
            // Optimistic: the heart is the confirmation, and a wall panel
            // has nothing else to show while a round trip to Spotify runs.
            saved = next;
            void deps
                .run(
                    "save:" + uri,
                    () => api.spotifySetSaved(uri, next),
                    next ? "Couldn't save that song" : "Couldn't remove that song",
                )
                .then(() => {
                    // Then re-read, because the optimistic flip above is a
                    // guess until Spotify agrees — and a refused write (an
                    // older grant, a dropped connection) has already been
                    // toasted by run(), which must not leave a heart
                    // claiming otherwise.
                    if (deps.trackURI() !== uri) return;
                    void api
                        .spotifySaved(uri)
                        .then((r) => {
                            if (deps.trackURI() === uri) saved = r.saved;
                        })
                        .catch(() => {});
                });
        },
    };
}
