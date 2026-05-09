/* RF Socket Controller — frontend
 *
 * Single-file SPA, no build step. Hash-routed (/#/dashboard, /#/sockets,
 * /#/schedules) so it works fine when served by the Go backend's static
 * file handler.
 */

"use strict";

const API = "/api";
const REFRESH_MS = 30_000;
const DAYS_SHORT = ["S", "M", "T", "W", "T", "F", "S"];
const DAY_NAMES  = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
const PROTOCOLS  = [
    { value: "nexa", label: "Nexa / Proove" },
    { value: "kaku", label: "KlikAanKlikUit (KAKU)" },
    { value: "intertechno", label: "Intertechno" },
    { value: "raw", label: "Raw / custom" },
];

// ---------- API client ----------
const api = {
    async req(path, { method = "GET", body, signal } = {}) {
        const opts = { method, signal, headers: {} };
        if (body !== undefined) {
            opts.headers["Content-Type"] = "application/json";
            opts.body = JSON.stringify(body);
        }
        const res = await fetch(API + path, opts);
        if (res.status === 204) return null;
        const text = await res.text();
        const data = text ? JSON.parse(text) : null;
        if (!res.ok) {
            const msg = (data && data.error) || res.statusText || "Request failed";
            const err = new Error(msg);
            err.status = res.status;
            throw err;
        }
        return data;
    },
    health()                 { return this.req("/health"); },
    listSockets()            { return this.req("/sockets"); },
    createSocket(body)       { return this.req("/sockets", { method: "POST", body }); },
    updateSocket(id, body)   { return this.req(`/sockets/${encodeURIComponent(id)}`, { method: "PUT", body }); },
    deleteSocket(id)         { return this.req(`/sockets/${encodeURIComponent(id)}`, { method: "DELETE" }); },
    socketOn(id)             { return this.req(`/sockets/${encodeURIComponent(id)}/on`, { method: "POST" }); },
    socketOff(id)            { return this.req(`/sockets/${encodeURIComponent(id)}/off`, { method: "POST" }); },
    socketToggle(id)         { return this.req(`/sockets/${encodeURIComponent(id)}/toggle`, { method: "POST" }); },
    allOn()                  { return this.req("/sockets/all/on", { method: "POST" }); },
    allOff()                 { return this.req("/sockets/all/off", { method: "POST" }); },
    roomOn(room)             { return this.req(`/rooms/${encodeURIComponent(room)}/on`, { method: "POST" }); },
    roomOff(room)            { return this.req(`/rooms/${encodeURIComponent(room)}/off`, { method: "POST" }); },
    listRooms()              { return this.req("/rooms"); },
    listSchedules()          { return this.req("/schedules"); },
    createSchedule(body)     { return this.req("/schedules", { method: "POST", body }); },
    updateSchedule(id, body) { return this.req(`/schedules/${encodeURIComponent(id)}`, { method: "PUT", body }); },
    deleteSchedule(id)       { return this.req(`/schedules/${encodeURIComponent(id)}`, { method: "DELETE" }); },
};

// ---------- App state ----------
const state = {
    sockets: [],
    schedules: [],
    rooms: [],
    search: "",
    roomFilter: "",
    loadedOnce: false,
};

// ---------- Utilities ----------
const $  = (sel, root = document) => root.querySelector(sel);
const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));

function el(tag, attrs = {}, ...children) {
    const node = document.createElement(tag);
    for (const [k, v] of Object.entries(attrs)) {
        if (v == null || v === false) continue;
        if (k === "class") node.className = v;
        else if (k === "dataset") Object.assign(node.dataset, v);
        else if (k.startsWith("on") && typeof v === "function") node.addEventListener(k.slice(2), v);
        else if (k === "html") node.innerHTML = v;
        else node.setAttribute(k, v === true ? "" : v);
    }
    for (const c of children.flat()) {
        if (c == null || c === false) continue;
        node.appendChild(c instanceof Node ? c : document.createTextNode(String(c)));
    }
    return node;
}

function tpl(id) {
    const t = document.getElementById(id);
    return t.content.firstElementChild.cloneNode(true);
}

function debounce(fn, ms) {
    let t;
    return (...args) => {
        clearTimeout(t);
        t = setTimeout(() => fn(...args), ms);
    };
}

function formatDays(days) {
    if (!days || days.length === 0) return "Every day";
    if (days.length === 7) return "Every day";
    const sorted = [...days].sort((a, b) => a - b);
    const isWeekdays = sorted.length === 5 && sorted.every((d, i) => d === i + 1);
    if (isWeekdays) return "Weekdays";
    const isWeekends = sorted.length === 2 && sorted[0] === 0 && sorted[1] === 6;
    if (isWeekends) return "Weekends";
    return sorted.map(d => DAY_NAMES[d]).join(", ");
}

// ---------- Toasts ----------
const toasts = {
    show({ title, message = "", tone = "info", timeout = 3500 } = {}) {
        const root = $("#toasts");
        const node = el("div", { class: "toast", dataset: { tone }, role: "status" },
            el("div", { style: "flex:1; min-width:0" },
                el("div", { class: "toast-title" }, title),
                message ? el("div", { class: "toast-msg" }, message) : null,
            ),
            el("button", {
                class: "toast-close",
                "aria-label": "Dismiss",
                onclick: () => node.remove(),
            }, "×"),
        );
        root.appendChild(node);
        if (timeout > 0) setTimeout(() => node.remove(), timeout);
    },
    success(title, message) { this.show({ title, message, tone: "success" }); },
    error(title, message)   { this.show({ title, message, tone: "error", timeout: 5000 }); },
    info(title, message)    { this.show({ title, message, tone: "info" }); },
    warn(title, message)    { this.show({ title, message, tone: "warn" }); },
};

// ---------- Modal ----------
const modal = {
    _previousFocus: null,
    open({ title, subtitle, body, actions = [] }) {
        this.close({ silent: true });
        this._previousFocus = document.activeElement;
        const root = $("#modal-root");
        root.innerHTML = "";
        root.hidden = false;

        const dialog = el("div", { class: "modal", role: "dialog", "aria-modal": "true", "aria-labelledby": "modal-title" });
        const head = el("div", { class: "modal-head" },
            el("div", null,
                el("h2", { id: "modal-title" }, title),
                subtitle ? el("p", { class: "modal-subtitle" }, subtitle) : null,
            ),
            el("button", { class: "icon-btn", "aria-label": "Close", onclick: () => this.close() }, "×"),
        );
        const bodyEl = el("div", { class: "modal-body" });
        if (body instanceof Node) bodyEl.appendChild(body);
        else if (typeof body === "string") bodyEl.innerHTML = body;

        const actionsEl = el("div", { class: "modal-actions" });
        for (const a of actions) {
            const btn = el("button", {
                class: `btn ${a.class || ""}`,
                onclick: a.onClick,
                type: a.type || "button",
            }, a.label);
            actionsEl.appendChild(btn);
        }

        dialog.append(head, bodyEl, actionsEl);
        root.appendChild(dialog);

        // Focus management
        root.addEventListener("click", (e) => { if (e.target === root) this.close(); });
        document.addEventListener("keydown", this._onKeydown);
        const focusables = $$(
            "button, [href], input, select, textarea, [tabindex]:not([tabindex='-1'])",
            dialog
        ).filter(n => !n.hasAttribute("disabled"));
        if (focusables[0]) focusables[0].focus();
    },
    close({ silent = false } = {}) {
        const root = $("#modal-root");
        if (root.hidden) return;
        root.innerHTML = "";
        root.hidden = true;
        document.removeEventListener("keydown", this._onKeydown);
        if (!silent && this._previousFocus && this._previousFocus.focus) {
            this._previousFocus.focus();
        }
        this._previousFocus = null;
    },
    confirm({ title, message, confirmLabel = "Confirm", danger = false }) {
        return new Promise(resolve => {
            this.open({
                title,
                body: el("p", { class: "modal-subtitle" }, message),
                actions: [
                    { label: "Cancel", class: "btn-ghost", onClick: () => { this.close(); resolve(false); } },
                    { label: confirmLabel, class: danger ? "btn-danger" : "btn-primary", onClick: () => { this.close(); resolve(true); } },
                ],
            });
        });
    },
    _onKeydown(e) {
        if (e.key === "Escape") modal.close();
    },
};

// ---------- Theme ----------
const theme = {
    init() {
        const saved = localStorage.getItem("theme");
        if (saved === "light" || saved === "dark") {
            document.documentElement.dataset.theme = saved;
        } else if (window.matchMedia("(prefers-color-scheme: light)").matches) {
            document.documentElement.dataset.theme = "light";
        }
        $("#theme-toggle").addEventListener("click", () => this.toggle());
    },
    toggle() {
        const next = document.documentElement.dataset.theme === "light" ? "dark" : "light";
        document.documentElement.dataset.theme = next;
        localStorage.setItem("theme", next);
    },
};

// ---------- Health polling ----------
async function refreshHealth() {
    const dot = $(".health-dot");
    const label = $(".health-label");
    try {
        await api.health();
        dot.dataset.state = "ok";
        label.textContent = "Connected";
    } catch {
        dot.dataset.state = "error";
        label.textContent = "Backend offline";
    }
}

// ---------- Data loading ----------
async function loadAll() {
    try {
        const [sockets, schedules, rooms] = await Promise.all([
            api.listSockets(),
            api.listSchedules(),
            api.listRooms(),
        ]);
        state.sockets = sockets || [];
        state.schedules = schedules || [];
        state.rooms = rooms || [];
        state.loadedOnce = true;
        renderCurrentRoute();
    } catch (e) {
        toasts.error("Failed to load data", e.message);
    }
}

// ---------- Rendering: top bar ----------
function setTopbar({ title, subtitle, actions = [] }) {
    $("#view-title").textContent = title;
    $("#view-subtitle").textContent = subtitle;
    const root = $("#topbar-actions");
    root.innerHTML = "";
    for (const a of actions) {
        const btn = el("button", {
            class: `btn ${a.class || "btn-secondary"}`,
            onclick: a.onClick,
        }, a.label);
        root.appendChild(btn);
    }
}

// ---------- Views ----------
const views = {
    dashboard() {
        setTopbar({
            title: "Dashboard",
            subtitle: "Overview of your RF sockets",
            actions: [{ label: "Add socket", class: "btn-primary", onClick: () => openSocketModal() }],
        });

        const root = $("#view-root");
        root.innerHTML = "";
        root.appendChild(tpl("tpl-dashboard"));

        // Stats
        const total = state.sockets.length;
        const on = state.sockets.filter(s => s.state).length;
        const enabledSchedules = state.schedules.filter(s => s.enabled).length;
        const roomCount = state.rooms.length;
        $("[data-stat=total]", root).textContent = total;
        $("[data-stat=on]", root).textContent = on;
        $("[data-stat=schedules]", root).textContent = enabledSchedules;
        $("[data-stat=rooms]", root).textContent = roomCount;

        // Quick actions
        $("[data-action=all-on]", root).addEventListener("click", () => withConfirm({
            title: "Turn all sockets ON?",
            message: `This will switch on ${total} socket${total === 1 ? "" : "s"}.`,
            confirmLabel: "Turn all on",
        }, async () => {
            const r = await api.allOn();
            toasts.success("All on", `${r.updated} updated, ${r.failures.length} failed.`);
            await loadAll();
        }));
        $("[data-action=all-off]", root).addEventListener("click", () => withConfirm({
            title: "Turn all sockets OFF?",
            message: `This will switch off ${total} socket${total === 1 ? "" : "s"}.`,
            confirmLabel: "Turn all off",
            danger: true,
        }, async () => {
            const r = await api.allOff();
            toasts.success("All off", `${r.updated} updated, ${r.failures.length} failed.`);
            await loadAll();
        }));
        $("[data-action=refresh]", root).addEventListener("click", loadAll);

        // Rooms
        const roomsRoot = $("[data-rooms]", root);
        if (state.rooms.length === 0) {
            roomsRoot.appendChild(el("p", { class: "field-help" }, "No rooms yet. Create sockets and assign rooms to them."));
        } else {
            for (const r of state.rooms) {
                roomsRoot.appendChild(renderRoomCard(r));
            }
        }
    },

    sockets() {
        setTopbar({
            title: "Sockets",
            subtitle: `${state.sockets.length} configured`,
            actions: [{ label: "Add socket", class: "btn-primary", onClick: () => openSocketModal() }],
        });

        const root = $("#view-root");
        root.innerHTML = "";
        const view = tpl("tpl-sockets");
        root.appendChild(view);

        // Toolbar
        const searchInput = $("[data-search]", view);
        searchInput.value = state.search;
        searchInput.addEventListener("input", debounce(() => {
            state.search = searchInput.value.trim().toLowerCase();
            renderSocketsList();
        }, 100));

        const roomFilter = $("[data-room-filter]", view);
        const allRooms = [...new Set(state.sockets.map(s => s.room || "Unassigned"))].sort();
        for (const r of allRooms) {
            roomFilter.appendChild(el("option", { value: r }, r));
        }
        roomFilter.value = state.roomFilter;
        roomFilter.addEventListener("change", () => {
            state.roomFilter = roomFilter.value;
            renderSocketsList();
        });

        renderSocketsList();
    },

    schedules() {
        setTopbar({
            title: "Schedules",
            subtitle: `${state.schedules.length} configured`,
            actions: [{
                label: "Add schedule",
                class: "btn-primary",
                onClick: () => openScheduleModal(),
            }],
        });

        const root = $("#view-root");
        root.innerHTML = "";
        const view = tpl("tpl-schedules");
        root.appendChild(view);

        const list = $("[data-list]", view);
        if (state.schedules.length === 0) {
            const empty = tpl("tpl-empty-schedules");
            $("[data-action=add-schedule]", empty).addEventListener("click", () => openScheduleModal());
            list.appendChild(empty);
            return;
        }

        const wrap = el("div", { class: "schedule-list" });
        for (const s of state.schedules) {
            wrap.appendChild(renderScheduleRow(s));
        }
        list.appendChild(wrap);
    },
};

// ---------- Sockets list rendering ----------
function renderSocketsList() {
    const view = $("#view-root");
    let host = $("[data-grouped]", view);
    host.innerHTML = "";

    let filtered = state.sockets;
    if (state.search) {
        const q = state.search;
        filtered = filtered.filter(s =>
            (s.name || "").toLowerCase().includes(q) ||
            (s.room || "").toLowerCase().includes(q) ||
            (s.code || "").toLowerCase().includes(q)
        );
    }
    if (state.roomFilter) {
        filtered = filtered.filter(s => (s.room || "Unassigned") === state.roomFilter);
    }

    if (state.sockets.length === 0) {
        const empty = tpl("tpl-empty-sockets");
        $("[data-action=add-socket]", empty).addEventListener("click", () => openSocketModal());
        host.appendChild(empty);
        return;
    }
    if (filtered.length === 0) {
        host.appendChild(tpl("tpl-empty-search"));
        return;
    }

    // Group by room
    const groups = new Map();
    for (const s of filtered) {
        const room = s.room || "Unassigned";
        if (!groups.has(room)) groups.set(room, []);
        groups.get(room).push(s);
    }
    const sortedRooms = [...groups.keys()].sort((a, b) => a.localeCompare(b));

    for (const room of sortedRooms) {
        const items = groups.get(room);
        const onCount = items.filter(s => s.state).length;

        const section = el("section", { class: "room-section" });
        const header = el("div", { class: "room-header" },
            el("h3", null, `${room} · ${items.length} sockets · ${onCount} on`),
            el("div", { class: "room-actions" },
                el("button", {
                    class: "btn btn-ghost",
                    onclick: async () => {
                        try { await api.roomOn(room); toasts.success("Room on", room); await loadAll(); }
                        catch (e) { toasts.error("Failed", e.message); }
                    },
                }, "All on"),
                el("button", {
                    class: "btn btn-ghost",
                    onclick: async () => {
                        try { await api.roomOff(room); toasts.success("Room off", room); await loadAll(); }
                        catch (e) { toasts.error("Failed", e.message); }
                    },
                }, "All off"),
            ),
        );
        const grid = el("div", { class: "sockets-grid" });
        for (const s of items) grid.appendChild(renderSocketCard(s));
        section.append(header, grid);
        host.appendChild(section);
    }
}

function renderSocketCard(s) {
    const card = el("article", {
        class: `socket-card ${s.state ? "is-on" : ""}`,
        dataset: { id: s.id },
    },
        el("div", { class: "socket-card-head" },
            el("div", { style: "min-width:0" },
                el("div", { class: "socket-name", title: s.name }, s.name),
                el("div", { class: "socket-meta" }, s.room || "Unassigned"),
            ),
            el("div", { class: "socket-menu" },
                el("button", {
                    class: "icon-btn",
                    "aria-label": "Edit",
                    onclick: () => openSocketModal(s),
                }, iconSVG("edit")),
                el("button", {
                    class: "icon-btn danger",
                    "aria-label": "Delete",
                    onclick: () => withConfirm({
                        title: "Delete socket?",
                        message: `“${s.name}” and any schedules pointing to it will be removed.`,
                        confirmLabel: "Delete",
                        danger: true,
                    }, async () => {
                        await api.deleteSocket(s.id);
                        toasts.success("Socket deleted", s.name);
                        await loadAll();
                    }),
                }, iconSVG("trash")),
            ),
        ),
        el("div", { class: "status-row" },
            el("span", { class: "status-dot" }),
            el("span", { class: "status-text" }, s.state ? "ON" : "OFF"),
            el("span", { class: "code-chip", title: "RF code" }, `${s.protocol || "raw"} · ${s.code}`),
        ),
        el("div", { class: "socket-controls" },
            el("button", {
                class: "btn btn-success",
                disabled: s.state,
                onclick: async () => action(() => api.socketOn(s.id), `Turned on ${s.name}`),
            }, "On"),
            el("button", {
                class: "btn btn-danger",
                disabled: !s.state,
                onclick: async () => action(() => api.socketOff(s.id), `Turned off ${s.name}`),
            }, "Off"),
            el("button", {
                class: "btn",
                onclick: async () => action(() => api.socketToggle(s.id), `Toggled ${s.name}`),
            }, "Toggle"),
        ),
    );
    return card;
}

function renderRoomCard(r) {
    return el("div", { class: "room-card" },
        el("div", { class: "room-card-name" }, r.name),
        el("div", { class: "room-card-meta" }, `${r.sockets} socket${r.sockets === 1 ? "" : "s"} · ${r.on} on`),
        el("div", { class: "room-card-actions" },
            el("button", {
                class: "btn btn-success",
                onclick: async () => action(() => api.roomOn(r.name), `${r.name} on`),
            }, "On"),
            el("button", {
                class: "btn btn-danger",
                onclick: async () => action(() => api.roomOff(r.name), `${r.name} off`),
            }, "Off"),
        ),
    );
}

function renderScheduleRow(s) {
    const socket = state.sockets.find(x => x.id === s.socket_id);
    const target = socket ? socket.name : `(missing socket: ${s.socket_id})`;
    const room = socket ? (socket.room || "Unassigned") : "—";

    return el("div", { class: "schedule-row" },
        el("div", { class: "schedule-time" }, s.time),
        el("div", { class: "schedule-info" },
            el("div", { class: "schedule-target" }, target),
            el("div", { class: "schedule-meta" }, `${room} · ${formatDays(s.days)}`),
        ),
        el("span", { class: "schedule-action", dataset: { action: s.action } }, s.action),
        el("label", { class: "switch", title: s.enabled ? "Enabled" : "Disabled" },
            (() => {
                const cb = el("input", { type: "checkbox" });
                cb.checked = s.enabled;
                cb.addEventListener("change", async () => {
                    try {
                        await api.updateSchedule(s.id, { ...s, enabled: cb.checked });
                        toasts.success(cb.checked ? "Schedule enabled" : "Schedule disabled");
                        await loadAll();
                    } catch (e) {
                        cb.checked = !cb.checked;
                        toasts.error("Failed", e.message);
                    }
                });
                return cb;
            })(),
            el("span", { class: "switch-track" }),
        ),
        el("div", { style: "display:flex; gap:4px" },
            el("button", {
                class: "icon-btn",
                "aria-label": "Edit schedule",
                onclick: () => openScheduleModal(s),
            }, iconSVG("edit")),
            el("button", {
                class: "icon-btn danger",
                "aria-label": "Delete schedule",
                onclick: () => withConfirm({
                    title: "Delete schedule?",
                    message: `${s.action.toUpperCase()} ${target} at ${s.time}.`,
                    confirmLabel: "Delete",
                    danger: true,
                }, async () => {
                    await api.deleteSchedule(s.id);
                    toasts.success("Schedule deleted");
                    await loadAll();
                }),
            }, iconSVG("trash")),
        ),
    );
}

// ---------- Socket modal ----------
function openSocketModal(existing = null) {
    const isEdit = !!existing;
    const body = el("form", { class: "modal-body", onsubmit: e => { e.preventDefault(); save(); } });

    const nameField = field("Socket name", el("input", {
        type: "text", required: true, autocomplete: "off",
        placeholder: "e.g. Living room lamp",
        value: existing?.name || "",
    }));
    const roomField = field("Room", el("input", {
        type: "text", autocomplete: "off",
        placeholder: "e.g. Living room",
        value: existing?.room || "",
    }), "Optional. Used to group sockets and for room-wide on/off.");
    const codeField = field("RF code", el("input", {
        type: "text", required: true, autocomplete: "off",
        placeholder: "e.g. 12345",
        value: existing?.code || "",
    }));
    const protoSel = el("select", null,
        ...PROTOCOLS.map(p => {
            const opt = el("option", { value: p.value }, p.label);
            if ((existing?.protocol || "nexa") === p.value) opt.selected = true;
            return opt;
        }),
    );
    const protoField = field("Protocol", protoSel);

    const row = el("div", { class: "field-row" }, codeField, protoField);
    body.append(nameField, roomField, row);

    async function save() {
        const payload = {
            name: $("input", nameField).value.trim(),
            room: $("input", roomField).value.trim(),
            code: $("input", codeField).value.trim(),
            protocol: protoSel.value,
        };
        if (!payload.name || !payload.code) {
            toasts.warn("Missing fields", "Name and RF code are required.");
            return;
        }
        try {
            if (isEdit) {
                await api.updateSocket(existing.id, payload);
                toasts.success("Socket updated", payload.name);
            } else {
                await api.createSocket(payload);
                toasts.success("Socket added", payload.name);
            }
            modal.close();
            await loadAll();
        } catch (e) {
            toasts.error("Save failed", e.message);
        }
    }

    modal.open({
        title: isEdit ? "Edit socket" : "Add socket",
        subtitle: isEdit ? "Update this socket's details." : "Configure a new 433MHz controllable socket.",
        body,
        actions: [
            { label: "Cancel", class: "btn-ghost", onClick: () => modal.close() },
            { label: isEdit ? "Save" : "Add socket", class: "btn-primary", onClick: save },
        ],
    });
}

// ---------- Schedule modal ----------
function openScheduleModal(existing = null) {
    const isEdit = !!existing;
    if (state.sockets.length === 0) {
        toasts.warn("No sockets", "Add a socket before creating schedules.");
        return;
    }

    const socketSel = el("select", { required: true },
        ...state.sockets.map(s => {
            const opt = el("option", { value: s.id }, `${s.name}${s.room ? ` · ${s.room}` : ""}`);
            if (existing?.socket_id === s.id) opt.selected = true;
            return opt;
        }),
    );
    const actionSel = el("select", { required: true },
        el("option", { value: "on" }, "Turn ON"),
        el("option", { value: "off" }, "Turn OFF"),
    );
    actionSel.value = existing?.action || "on";

    const timeInput = el("input", {
        type: "time", required: true,
        value: existing?.time || "08:00",
    });

    const selectedDays = new Set(existing?.days || []);
    const dayPicker = el("div", { class: "day-picker", role: "group", "aria-label": "Days of week" });
    for (let i = 0; i < 7; i++) {
        const chip = el("button", {
            type: "button",
            class: "day-chip",
            dataset: { day: i, selected: selectedDays.has(i) },
            "aria-pressed": selectedDays.has(i) ? "true" : "false",
            "aria-label": DAY_NAMES[i],
            title: DAY_NAMES[i],
            onclick: () => {
                if (selectedDays.has(i)) selectedDays.delete(i); else selectedDays.add(i);
                chip.dataset.selected = selectedDays.has(i);
                chip.setAttribute("aria-pressed", selectedDays.has(i));
            },
        }, DAYS_SHORT[i]);
        dayPicker.appendChild(chip);
    }
    function setDays(days) {
        selectedDays.clear();
        for (const d of days) selectedDays.add(d);
        for (const chip of $$(".day-chip", dayPicker)) {
            const d = Number(chip.dataset.day);
            chip.dataset.selected = selectedDays.has(d);
            chip.setAttribute("aria-pressed", selectedDays.has(d));
        }
    }
    const presets = el("div", { class: "day-presets" },
        el("button", { type: "button", class: "day-preset-btn", onclick: () => setDays([0,1,2,3,4,5,6]) }, "Every day"),
        el("button", { type: "button", class: "day-preset-btn", onclick: () => setDays([1,2,3,4,5]) }, "Weekdays"),
        el("button", { type: "button", class: "day-preset-btn", onclick: () => setDays([0,6]) }, "Weekends"),
        el("button", { type: "button", class: "day-preset-btn", onclick: () => setDays([]) }, "Clear"),
    );

    const enabledCb = el("input", { type: "checkbox" });
    enabledCb.checked = existing ? existing.enabled : true;
    const enabledRow = el("label", { class: "field", style: "flex-direction:row; align-items:center; gap:12px;" },
        el("span", { class: "switch" }, enabledCb, el("span", { class: "switch-track" })),
        el("span", null, "Enabled"),
    );

    const body = el("form", { onsubmit: e => { e.preventDefault(); save(); } },
        el("div", { class: "field-row" },
            field("Socket", socketSel),
            field("Action", actionSel),
        ),
        field("Time", timeInput, "24-hour HH:MM in the server's local time."),
        field("Days", el("div", null, dayPicker, presets), "Leave empty to fire every day."),
        enabledRow,
    );

    async function save() {
        const days = [...selectedDays].sort((a, b) => a - b);
        const payload = {
            socket_id: socketSel.value,
            action: actionSel.value,
            time: timeInput.value,
            days,
            enabled: enabledCb.checked,
        };
        if (!payload.time) {
            toasts.warn("Missing time", "Please pick a time.");
            return;
        }
        try {
            if (isEdit) {
                await api.updateSchedule(existing.id, payload);
                toasts.success("Schedule updated");
            } else {
                await api.createSchedule(payload);
                toasts.success("Schedule added");
            }
            modal.close();
            await loadAll();
        } catch (e) {
            toasts.error("Save failed", e.message);
        }
    }

    modal.open({
        title: isEdit ? "Edit schedule" : "Add schedule",
        subtitle: isEdit ? "Update when this schedule fires." : "Run a socket on or off at a chosen time.",
        body,
        actions: [
            { label: "Cancel", class: "btn-ghost", onClick: () => modal.close() },
            { label: isEdit ? "Save" : "Add schedule", class: "btn-primary", onClick: save },
        ],
    });
}

// ---------- Helpers ----------
function field(label, control, help) {
    return el("div", { class: "field" },
        el("label", null, label),
        control,
        help ? el("div", { class: "field-help" }, help) : null,
    );
}

function iconSVG(name) {
    const svgNS = "http://www.w3.org/2000/svg";
    const svg = document.createElementNS(svgNS, "svg");
    svg.setAttribute("viewBox", "0 0 24 24");
    svg.setAttribute("width", "16");
    svg.setAttribute("height", "16");
    const path = document.createElementNS(svgNS, "path");
    path.setAttribute("fill", "currentColor");
    path.setAttribute("d", {
        edit:  "M3 17.25V21h3.75L17.81 9.94l-3.75-3.75L3 17.25zM20.71 7.04a1 1 0 000-1.41l-2.34-2.34a1 1 0 00-1.41 0l-1.83 1.83 3.75 3.75 1.83-1.83z",
        trash: "M6 19a2 2 0 002 2h8a2 2 0 002-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z",
    }[name]);
    svg.appendChild(path);
    return svg;
}

async function action(fn, successMsg) {
    try {
        await fn();
        if (successMsg) toasts.success(successMsg);
        await loadAll();
    } catch (e) {
        toasts.error("Action failed", e.message);
    }
}

function withConfirm(opts, run) {
    return modal.confirm(opts).then(async ok => {
        if (!ok) return;
        try {
            await run();
        } catch (e) {
            toasts.error("Action failed", e.message || String(e));
        }
    });
}

// ---------- Routing ----------
function currentRoute() {
    const hash = window.location.hash || "#/dashboard";
    const m = hash.match(/^#\/([\w-]+)/);
    const name = m ? m[1] : "dashboard";
    return views[name] ? name : "dashboard";
}

function renderCurrentRoute() {
    const route = currentRoute();
    for (const item of $$(".nav-item")) {
        if (item.dataset.route === route) item.setAttribute("aria-current", "page");
        else item.removeAttribute("aria-current");
    }
    views[route]();
}

window.addEventListener("hashchange", renderCurrentRoute);

// ---------- Boot ----------
async function boot() {
    theme.init();
    if (!window.location.hash) window.location.hash = "#/dashboard";
    renderCurrentRoute();
    await Promise.all([loadAll(), refreshHealth()]);
    setInterval(loadAll, REFRESH_MS);
    setInterval(refreshHealth, REFRESH_MS);
}

document.addEventListener("DOMContentLoaded", boot);
