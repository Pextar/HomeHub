/**
 * The eight sheets the Music view opens about *equipment* rather than music,
 * and — the part worth having in one place — what has to be re-read after
 * each one closes.
 *
 * Registering, editing or removing a device changes more than the bridge that
 * owns it. It changes what rooms hold, because the backend cascades a removal
 * out of them; it changes what the room editor's picker can offer; and for
 * the quality sheets it changes what every zone read *reports*, since a route
 * a zone can take depends on what is registered and what it can decode.
 *
 * Those follow-on reads were written out at each call site, and they had
 * already gone their own ways — the same close doing four reads in one place
 * and two in another, with nothing saying which was the considered answer.
 * `afterDevices` states it once: whatever brands changed, plus the zones and
 * the endpoint list, always.
 *
 * The one sheet that legitimately differs is the add flow. It carries a brand
 * picker, so a registration made from it could have been any of the four, and
 * it re-reads all of them rather than guessing from which chip was tapped.
 *
 * The bridges arrive as the two shapes this module actually uses rather than
 * as their full types: nothing here does anything with a speaker list, and a
 * narrow contract is one a test can satisfy with an object literal.
 */

import { openModal } from "../modal.svelte";
import SpeakerModal from "../../modals/SpeakerModal.svelte";
import SonosEventsModal from "../../modals/SonosEventsModal.svelte";
import MusicQualityModal from "../../modals/MusicQualityModal.svelte";
import SpotifyConnectModal from "../../modals/SpotifyConnectModal.svelte";
import QobuzConnectModal from "../../modals/QobuzConnectModal.svelte";
import type {
  AirPlaySpeakerView,
  KEFSpeakerView,
  SonosSpeakerView,
  UPnPRenderer,
} from "../types";

/** Anything that can be asked to read its devices again. */
export interface Reloadable {
  refresh(): Promise<unknown> | void;
}
/** The zone bridge also owns the list the room editor's picker offers. */
export interface ZoneReloadable extends Reloadable {
  loadEndpoints(): Promise<unknown> | void;
}

export interface DeviceSheetDeps {
  sonos: Reloadable;
  kef: Reloadable;
  airplay: Reloadable;
  upnp: Reloadable;
  zones: ZoneReloadable;
  /**
   * A KEF speaker was edited. The Speakers screen closes its settings pane
   * for that id — the pane is about a registration that may no longer say
   * what it did, or may not exist.
   */
  onKefEdited?: (id: string) => void;
}

export interface DeviceSheets {
  /** The add/edit sheet. With no speaker it is the add flow, which carries
   *  the brand picker. */
  openSpeaker(sp?: SonosSpeakerView): Promise<void>;
  openKEF(sp: KEFSpeakerView): Promise<void>;
  openAirPlay(sp: AirPlaySpeakerView): Promise<void>;
  openUPnP(rn: UPnPRenderer): Promise<void>;
  /** What the audio actually is on each path, and the decode setting. */
  openQuality(): Promise<void>;
  /** Where the Spotify account is playing, and moving it. */
  openConnect(): Promise<void>;
  openQobuz(): Promise<void>;
  /** Sonos push subscriptions — retrying inside can turn them on, which
   *  changes which poll interval the view should be using. */
  openEvents(): Promise<void>;
}

export function createDeviceSheets(deps: DeviceSheetDeps): DeviceSheets {
  /**
   * The reads a device change is owed: the brands named, and always the
   * zones and the picker's endpoint list.
   */
  function afterDevices(...brands: Reloadable[]) {
    for (const b of brands) void b.refresh();
    void deps.zones.refresh();
    void deps.zones.loadEndpoints();
  }

  /** Every device sheet answers `true` when something actually changed. */
  async function sheet(props: Record<string, unknown>): Promise<boolean> {
    return !!(await openModal<boolean>(SpeakerModal, props));
  }

  return {
    async openSpeaker(sp) {
      const changed = await sheet(sp ? { existing: sp, brand: "sonos" as const } : {});
      // The add sheet carries a brand picker, so a registration made from it
      // could have been any of the four.
      if (changed) afterDevices(deps.sonos, deps.kef, deps.airplay, deps.upnp);
    },

    async openKEF(sp) {
      if (await sheet({ existing: sp, brand: "kef" as const })) {
        deps.onKefEdited?.(sp.id);
        afterDevices(deps.kef);
      }
    },

    async openAirPlay(sp) {
      // UPnP too: the same box often advertises both, so editing it under one
      // name can change what the other one reports.
      if (await sheet({ existing: sp, brand: "airplay" as const })) {
        afterDevices(deps.airplay, deps.upnp);
      }
    },

    async openUPnP(rn) {
      if (await sheet({ existing: rn, brand: "upnp" as const })) afterDevices(deps.upnp);
    },

    async openQuality() {
      await openModal(MusicQualityModal, {});
      // A changed decode quality changes what every zone read reports.
      void deps.zones.refresh();
    },

    async openConnect() {
      await openModal(SpotifyConnectModal, {});
      // A transfer made in there can take the account's session away from a
      // room HomeHub was feeding, and the backend will have released it.
      void deps.zones.refresh();
    },

    async openQobuz() {
      await openModal(QobuzConnectModal, {});
      // Qobuz is the one provider that can answer "lossless", so signing in
      // changes what every zone read says about quality.
      void deps.zones.refresh();
    },

    async openEvents() {
      await openModal(SonosEventsModal, {});
      void deps.sonos.refresh();
    },
  };
}
