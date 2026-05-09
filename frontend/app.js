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
    socketTimer(id, body)    { return this.req(`/sockets/${encodeURIComponent(id)}/timer`, { method: "POST", body }); },
    allOn()                  { return this.req("/sockets/all/on", { method: "POST" }); },
    allOff()                 { return this.req("/sockets/all/off", { method: "POST" }); },
    roomOn(room)             { return this.req(`/rooms/${encodeURIComponent(room)}/on`, { method: "POST" }); },
    roomOff(room)            { return this.req(`/rooms/${encodeURIComponent(room)}/off`, { method: "POST" }); },
    listRooms()              { return this.req("/rooms"); },
    listSchedules()          { return this.req("/schedules"); },
    createSchedule(body)     { return this.req("/schedules", { method: "POST", body }); },
    updateSchedule(id, body) { return this.req(`/schedules/${encodeURIComponent(id)}`, { method: "PUT", body }); },
    deleteSchedule(id)       { return this.req(`/schedules/${encodeURIComponent(id)}`, { method: "DELETE" }); },
    listGroups()             { return this.req("/groups"); },
    createGroup(body)        { return this.req("/groups", { method: "POST", body }); },
    updateGroup(id, body)    { return this.req(`/groups/${encodeURIComponent(id)}`, { method: "PUT", body }); },
    deleteGroup(id)          { return this.req(`/groups/${encodeURIComponent(id)}`, { method: "DELETE" }); },
    groupAction(id, action)  { return this.req(`/groups/${encodeURIComponent(id)}/${action}`, { method: "POST" }); },
    listScenes()             { return this.req("/scenes"); },
    createScene(body)        { return this.req("/scenes", { method: "POST", body }); },
    updateScene(id, body)    { return this.req(`/scenes/${encodeURIComponent(id)}`, { method: "PUT", body }); },
    deleteScene(id)          { return this.req(`/scenes/${encodeURIComponent(id)}`, { method: "DELETE" }); },
    activateScene(id)        { return this.req(`/scenes/${encodeURIComponent(id)}/activate`, { method: "POST" }); },
    listTimers()             { return this.req("/timers"); },
    createTimer(body)        { return this.req("/timers", { method: "POST", body }); },
    deleteTimer(id)          { return this.req(`/timers/${encodeURIComponent(id)}`, { method: "DELETE" }); },
};

// ---------- App state ----------
const state = {
    sockets: [],
    schedules: [],
    rooms: [],
    groups: [],
    scenes: [],
    timers: [],
    search: "",
    roomFilter: "",
    loadedOnce: false,
};

function socketById(id)  { return state.sockets.find(s => s.id === id); }
function groupById(id)   { return state.groups.find(g => g.id === id); }
function sceneById(id)   { return state.scenes.find(s => s.id === id); }

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
        const [sockets, schedules, rooms, groups, scenes, timers] = await Promise.all([
            api.listSockets(),
            api.listSchedules(),
            api.listRooms(),
            api.listGroups(),
            api.listScenes(),
            api.listTimers(),
        ]);
        state.sockets = sockets || [];
        state.schedules = schedules || [];
        state.rooms = rooms || [];
        state.groups = groups || [];
        state.scenes = scenes || [];
        state.timers = timers || [];
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
        const groupSceneCount = state.groups.length + state.scenes.length;
        $("[data-stat=total]", root).textContent = total;
        $("[data-stat=on]", root).textContent = on;
        $("[data-stat=schedules]", root).textContent = enabledSchedules;
        $("[data-stat=groups]", root).textContent = groupSceneCount;

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

        // Scenes (compact tiles)
        $("[data-action=add-scene]", root).addEventListener("click", () => openSceneModal());
        const scenesRoot = $("[data-scenes]", root);
        if (state.scenes.length === 0) {
            scenesRoot.appendChild(el("p", { class: "field-help" }, "No scenes yet. Click “New scene” to combine a few sockets into a one-tap action."));
        } else {
            for (const s of state.scenes) scenesRoot.appendChild(renderSceneTile(s));
        }

        // Pending timers
        const timersCard = $("[data-section=timers]", root);
        const timersRoot = $("[data-timers]", root);
        if (state.timers.length === 0) {
            timersCard.hidden = true;
        } else {
            timersCard.hidden = false;
            timersRoot.innerHTML = "";
            for (const t of state.timers) timersRoot.appendChild(renderTimerRow(t));
        }

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

    groups() {
        setTopbar({
            title: "Groups",
            subtitle: `${state.groups.length} configured`,
            actions: [{
                label: "Add group",
                class: "btn-primary",
                onClick: () => openGroupModal(),
            }],
        });

        const root = $("#view-root");
        root.innerHTML = "";
        const view = tpl("tpl-groups");
        root.appendChild(view);

        const list = $("[data-list]", view);
        if (state.groups.length === 0) {
            const empty = tpl("tpl-empty-groups");
            $("[data-action=add-group]", empty).addEventListener("click", () => openGroupModal());
            list.appendChild(empty);
            return;
        }
        const wrap = el("div", { class: "entity-list" });
        for (const g of state.groups) wrap.appendChild(renderGroupCard(g));
        list.appendChild(wrap);
    },

    scenes() {
        setTopbar({
            title: "Scenes",
            subtitle: `${state.scenes.length} configured`,
            actions: [{
                label: "Add scene",
                class: "btn-primary",
                onClick: () => openSceneModal(),
            }],
        });

        const root = $("#view-root");
        root.innerHTML = "";
        const view = tpl("tpl-scenes");
        root.appendChild(view);

        const list = $("[data-list]", view);
        if (state.scenes.length === 0) {
            const empty = tpl("tpl-empty-scenes");
            $("[data-action=add-scene]", empty).addEventListener("click", () => openSceneModal());
            list.appendChild(empty);
            return;
        }
        const wrap = el("div", { class: "entity-list" });
        for (const s of state.scenes) wrap.appendChild(renderSceneCard(s));
        list.appendChild(wrap);
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
                    "aria-label": "Set timer",
                    title: "Set timer",
                    onclick: () => openTimerModal(s),
                }, iconSVG("timer")),
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

function describeTarget(targetType, targetId, fallbackSocketId) {
    const tt = targetType || (fallbackSocketId ? "socket" : "");
    const tid = targetId || fallbackSocketId;
    if (tt === "socket") {
        const s = socketById(tid);
        return s ? { kind: "Socket", label: s.name, sub: s.room || "Unassigned" }
                 : { kind: "Socket", label: `(missing socket: ${tid})`, sub: "—" };
    }
    if (tt === "group") {
        const g = groupById(tid);
        return g ? { kind: "Group", label: g.name, sub: `${g.socket_ids.length} sockets` }
                 : { kind: "Group", label: `(missing group: ${tid})`, sub: "—" };
    }
    if (tt === "scene") {
        const sc = sceneById(tid);
        return sc ? { kind: "Scene", label: sc.name, sub: `${sc.actions.length} actions` }
                  : { kind: "Scene", label: `(missing scene: ${tid})`, sub: "—" };
    }
    return { kind: "?", label: "Unknown target", sub: "" };
}

function renderScheduleRow(s) {
    const t = describeTarget(s.target_type, s.target_id, s.socket_id);

    return el("div", { class: "schedule-row" },
        el("div", { class: "schedule-time" }, s.time),
        el("div", { class: "schedule-info" },
            el("div", { class: "schedule-target" }, `${t.kind}: ${t.label}`),
            el("div", { class: "schedule-meta" }, `${t.sub} · ${formatDays(s.days)}`),
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
    if (state.sockets.length === 0 && state.groups.length === 0 && state.scenes.length === 0) {
        toasts.warn("Nothing to schedule", "Add a socket, group, or scene first.");
        return;
    }

    // Initial target type: prefer existing target_type, else fall back to
    // socket_id, else first available kind.
    const initialType = existing?.target_type
        || (existing?.socket_id ? "socket" : null)
        || (state.sockets.length ? "socket" : state.groups.length ? "group" : "scene");

    const targetTypeSeg = renderSegmented("schedule-target-type", initialType, [
        { value: "socket", label: "Socket", disabled: state.sockets.length === 0 },
        { value: "group",  label: "Group",  disabled: state.groups.length === 0 },
        { value: "scene",  label: "Scene",  disabled: state.scenes.length === 0 },
    ]);

    const targetSel = el("select", { required: true });
    const actionSel = el("select", { required: true });
    const actionField = field("Action", actionSel);

    function rebuildTarget() {
        const tt = $("input:checked", targetTypeSeg).value;
        targetSel.innerHTML = "";
        if (tt === "socket") {
            for (const s of state.sockets) {
                targetSel.appendChild(el("option", { value: s.id }, `${s.name}${s.room ? ` · ${s.room}` : ""}`));
            }
        } else if (tt === "group") {
            for (const g of state.groups) {
                targetSel.appendChild(el("option", { value: g.id }, `${g.name} · ${g.socket_ids.length} sockets`));
            }
        } else {
            for (const sc of state.scenes) {
                targetSel.appendChild(el("option", { value: sc.id }, `${sc.name} · ${sc.actions.length} actions`));
            }
        }
        const wantId = existing?.target_id || existing?.socket_id;
        if (wantId && targetSel.querySelector(`option[value="${CSS.escape(wantId)}"]`)) {
            targetSel.value = wantId;
        }

        // Action options depend on target type. Scenes have no choice — they
        // always activate.
        actionSel.innerHTML = "";
        if (tt === "scene") {
            actionSel.appendChild(el("option", { value: "activate" }, "Activate"));
            actionField.style.opacity = "0.6";
            actionSel.disabled = true;
        } else {
            actionSel.appendChild(el("option", { value: "on" }, "Turn ON"));
            actionSel.appendChild(el("option", { value: "off" }, "Turn OFF"));
            actionSel.appendChild(el("option", { value: "toggle" }, "Toggle"));
            actionField.style.opacity = "1";
            actionSel.disabled = false;
            actionSel.value = (existing?.action && ["on","off","toggle"].includes(existing.action))
                ? existing.action : "on";
        }
    }
    for (const r of $$("input", targetTypeSeg)) r.addEventListener("change", rebuildTarget);
    rebuildTarget();

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
        field("Target type", targetTypeSeg),
        el("div", { class: "field-row" },
            field("Target", targetSel),
            actionField,
        ),
        field("Time", timeInput, "24-hour HH:MM in the server's local time."),
        field("Days", el("div", null, dayPicker, presets), "Leave empty to fire every day."),
        enabledRow,
    );

    async function save() {
        const days = [...selectedDays].sort((a, b) => a - b);
        const tt = $("input:checked", targetTypeSeg).value;
        const payload = {
            target_type: tt,
            target_id: targetSel.value,
            action: actionSel.value,
            time: timeInput.value,
            days,
            enabled: enabledCb.checked,
        };
        if (!payload.target_id) {
            toasts.warn("Missing target", "Pick something to schedule.");
            return;
        }
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
        timer: "M15 1H9v2h6V1zm-4 13h2V8h-2v6zm8.03-6.61l1.42-1.42a10 10 0 00-1.41-1.41l-1.42 1.42A8 8 0 0012 4a8 8 0 100 16 8 8 0 007.03-12.61z",
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

// ---------- Group rendering & modal ----------
function renderGroupCard(g) {
    const memberChips = g.socket_ids.map(id => {
        const s = socketById(id);
        return el("span", { class: "member-chip" }, s ? s.name : `(missing: ${id})`);
    });
    return el("article", { class: "entity-card" },
        el("div", { class: "entity-head" },
            el("div", { class: "entity-name" }, g.name),
            el("div", { class: "entity-meta" }, `${g.socket_ids.length} socket${g.socket_ids.length === 1 ? "" : "s"}`),
            el("div", { class: "entity-members" }, ...memberChips),
        ),
        el("div", { class: "entity-actions" },
            el("button", { class: "btn btn-success", onclick: () => action(() => api.groupAction(g.id, "on"), `${g.name} on`) }, "On"),
            el("button", { class: "btn btn-danger", onclick: () => action(() => api.groupAction(g.id, "off"), `${g.name} off`) }, "Off"),
            el("button", { class: "btn", onclick: () => action(() => api.groupAction(g.id, "toggle"), `${g.name} toggled`) }, "Toggle"),
            el("button", { class: "icon-btn", "aria-label": "Edit", onclick: () => openGroupModal(g) }, iconSVG("edit")),
            el("button", {
                class: "icon-btn danger",
                "aria-label": "Delete",
                onclick: () => withConfirm({
                    title: "Delete group?",
                    message: `“${g.name}” and any schedules pointing at it will be removed. The sockets themselves are not affected.`,
                    confirmLabel: "Delete",
                    danger: true,
                }, async () => {
                    await api.deleteGroup(g.id);
                    toasts.success("Group deleted", g.name);
                    await loadAll();
                }),
            }, iconSVG("trash")),
        ),
    );
}

function openGroupModal(existing = null) {
    const isEdit = !!existing;
    if (state.sockets.length === 0) {
        toasts.warn("No sockets", "Add a socket before creating a group.");
        return;
    }
    const nameField = field("Name", el("input", {
        type: "text", required: true, autocomplete: "off",
        placeholder: "e.g. Living room lights",
        value: existing?.name || "",
    }));
    const selectedIds = new Set(existing?.socket_ids || []);
    const picker = el("div", { class: "member-picker", role: "group", "aria-label": "Members" });
    const sortedSockets = [...state.sockets].sort((a, b) => {
        const ar = (a.room || "").toLowerCase(), br = (b.room || "").toLowerCase();
        if (ar !== br) return ar.localeCompare(br);
        return a.name.localeCompare(b.name);
    });
    for (const s of sortedSockets) {
        const cb = el("input", { type: "checkbox" });
        cb.checked = selectedIds.has(s.id);
        cb.addEventListener("change", () => {
            if (cb.checked) selectedIds.add(s.id); else selectedIds.delete(s.id);
        });
        picker.appendChild(el("label", { class: "member-picker-row" },
            cb,
            el("div", null,
                el("div", null, s.name),
                el("div", { class: "field-help" }, s.room || "Unassigned"),
            ),
            el("span", { class: "meta" }, s.code),
        ));
    }
    const membersField = field(
        `Members (${state.sockets.length} sockets)`,
        picker,
        "Toggle the sockets that belong to this group.",
    );

    const body = el("form", { onsubmit: e => { e.preventDefault(); save(); } },
        nameField, membersField,
    );

    async function save() {
        const payload = {
            name: $("input", nameField).value.trim(),
            socket_ids: [...selectedIds],
        };
        if (!payload.name) {
            toasts.warn("Missing name", "Give the group a name.");
            return;
        }
        try {
            if (isEdit) {
                await api.updateGroup(existing.id, payload);
                toasts.success("Group updated", payload.name);
            } else {
                await api.createGroup(payload);
                toasts.success("Group created", payload.name);
            }
            modal.close();
            await loadAll();
        } catch (e) {
            toasts.error("Save failed", e.message);
        }
    }

    modal.open({
        title: isEdit ? "Edit group" : "New group",
        subtitle: "Groups let you control multiple sockets in one tap.",
        body,
        actions: [
            { label: "Cancel", class: "btn-ghost", onClick: () => modal.close() },
            { label: isEdit ? "Save" : "Create group", class: "btn-primary", onClick: save },
        ],
    });
}

// ---------- Scene rendering & modal ----------
function renderSceneTile(sc) {
    return el("button", {
        class: "scene-tile",
        type: "button",
        onclick: () => action(() => api.activateScene(sc.id), `Scene activated: ${sc.name}`),
    },
        el("div", { class: "scene-tile-name" }, sc.name),
        el("div", { class: "scene-tile-meta" }, `${sc.actions.length} action${sc.actions.length === 1 ? "" : "s"}`),
    );
}

function renderSceneCard(sc) {
    const chips = sc.actions.map(a => {
        const socket = socketById(a.socket_id);
        const label = socket ? socket.name : `(missing)`;
        return el("span", { class: "member-chip", dataset: { action: a.action } },
            `${label} → ${a.action.toUpperCase()}`);
    });
    return el("article", { class: "entity-card" },
        el("div", { class: "entity-head" },
            el("div", { class: "entity-name" }, sc.name),
            el("div", { class: "entity-meta" }, `${sc.actions.length} action${sc.actions.length === 1 ? "" : "s"}`),
            el("div", { class: "entity-members" }, ...chips),
        ),
        el("div", { class: "entity-actions" },
            el("button", {
                class: "btn btn-primary",
                onclick: () => action(() => api.activateScene(sc.id), `Scene activated: ${sc.name}`),
            }, "Activate"),
            el("button", { class: "icon-btn", "aria-label": "Edit", onclick: () => openSceneModal(sc) }, iconSVG("edit")),
            el("button", {
                class: "icon-btn danger",
                "aria-label": "Delete",
                onclick: () => withConfirm({
                    title: "Delete scene?",
                    message: `“${sc.name}” and any schedules pointing at it will be removed.`,
                    confirmLabel: "Delete",
                    danger: true,
                }, async () => {
                    await api.deleteScene(sc.id);
                    toasts.success("Scene deleted", sc.name);
                    await loadAll();
                }),
            }, iconSVG("trash")),
        ),
    );
}

function openSceneModal(existing = null) {
    const isEdit = !!existing;
    if (state.sockets.length === 0) {
        toasts.warn("No sockets", "Add a socket before creating a scene.");
        return;
    }
    const nameField = field("Name", el("input", {
        type: "text", required: true, autocomplete: "off",
        placeholder: "e.g. Movie night",
        value: existing?.name || "",
    }));

    // Map socketId -> "ignore" | "on" | "off"
    const initial = new Map();
    if (existing) for (const a of existing.actions) initial.set(a.socket_id, a.action);

    const picker = el("div", { class: "member-picker", role: "group", "aria-label": "Scene actions" });
    const sortedSockets = [...state.sockets].sort((a, b) => {
        const ar = (a.room || "").toLowerCase(), br = (b.room || "").toLowerCase();
        if (ar !== br) return ar.localeCompare(br);
        return a.name.localeCompare(b.name);
    });
    const rowSelects = new Map();
    for (const s of sortedSockets) {
        const sel = el("select", null,
            el("option", { value: "ignore" }, "Ignore"),
            el("option", { value: "on" }, "Turn on"),
            el("option", { value: "off" }, "Turn off"),
        );
        sel.value = initial.get(s.id) || "ignore";
        rowSelects.set(s.id, sel);
        picker.appendChild(el("div", { class: "member-picker-row" },
            el("div", null,
                el("div", null, s.name),
                el("div", { class: "field-help" }, s.room || "Unassigned"),
            ),
            sel,
        ));
    }
    const membersField = field(
        "Per-socket actions",
        picker,
        "Set each socket to On, Off, or Ignore. Ignored sockets are not touched when the scene runs.",
    );

    const body = el("form", { onsubmit: e => { e.preventDefault(); save(); } },
        nameField, membersField,
    );

    async function save() {
        const actions = [];
        for (const [sid, sel] of rowSelects) {
            if (sel.value !== "ignore") actions.push({ socket_id: sid, action: sel.value });
        }
        const payload = {
            name: $("input", nameField).value.trim(),
            actions,
        };
        if (!payload.name) {
            toasts.warn("Missing name", "Give the scene a name.");
            return;
        }
        if (actions.length === 0) {
            toasts.warn("No actions", "Set at least one socket to On or Off.");
            return;
        }
        try {
            if (isEdit) {
                await api.updateScene(existing.id, payload);
                toasts.success("Scene updated", payload.name);
            } else {
                await api.createScene(payload);
                toasts.success("Scene created", payload.name);
            }
            modal.close();
            await loadAll();
        } catch (e) {
            toasts.error("Save failed", e.message);
        }
    }

    modal.open({
        title: isEdit ? "Edit scene" : "New scene",
        subtitle: "A scene drives selected sockets to specific states in one tap.",
        body,
        actions: [
            { label: "Cancel", class: "btn-ghost", onClick: () => modal.close() },
            { label: isEdit ? "Save" : "Create scene", class: "btn-primary", onClick: save },
        ],
    });
}

// ---------- Timer modal & rendering ----------
function renderTimerRow(t) {
    const target = describeTarget(t.target_type, t.target_id);
    const firesAt = new Date(t.fires_at);
    const countdownEl = el("span", { class: "countdown" }, formatCountdown(firesAt));
    // Live tick — update once per second while this node is in the DOM.
    const interval = setInterval(() => {
        if (!countdownEl.isConnected) {
            clearInterval(interval);
            return;
        }
        countdownEl.textContent = formatCountdown(firesAt);
    }, 1000);

    return el("div", { class: "timer-row" },
        el("span", { class: "schedule-action", dataset: { action: t.action === "on" ? "on" : "off" } },
            t.action),
        el("div", null,
            el("div", null, `${target.kind}: ${target.label}`),
            el("div", { class: "field-help" }, firesAt.toLocaleString()),
        ),
        countdownEl,
        el("button", {
            class: "icon-btn danger",
            "aria-label": "Cancel timer",
            onclick: async () => {
                try {
                    await api.deleteTimer(t.id);
                    toasts.success("Timer cancelled");
                    await loadAll();
                } catch (e) { toasts.error("Failed", e.message); }
            },
        }, iconSVG("trash")),
    );
}

function formatCountdown(when) {
    const ms = when.getTime() - Date.now();
    if (ms <= 0) return "now";
    const s = Math.floor(ms / 1000);
    if (s < 60) return `${s}s`;
    const m = Math.floor(s / 60);
    if (m < 60) return `${m}m ${s % 60}s`;
    const h = Math.floor(m / 60);
    return `${h}h ${m % 60}m`;
}

function openTimerModal(socket) {
    const presets = [
        { label: "1 min",  seconds: 60 },
        { label: "15 min", seconds: 15 * 60 },
        { label: "30 min", seconds: 30 * 60 },
        { label: "1 hour", seconds: 60 * 60 },
        { label: "2 hours", seconds: 2 * 60 * 60 },
        { label: "4 hours", seconds: 4 * 60 * 60 },
    ];

    const actionSeg = renderSegmented("timer-action", "off", [
        { value: "off", label: "Turn off" },
        { value: "on",  label: "Turn on" },
        { value: "toggle", label: "Toggle" },
    ]);

    const customMins = el("input", { type: "number", min: "1", placeholder: "Minutes", style: "max-width:160px" });

    const body = el("form", { onsubmit: e => { e.preventDefault(); submitCustom(); } },
        field("Action", actionSeg),
        field("Quick presets", el("div", { class: "preset-row" },
            ...presets.map(p => el("button", {
                type: "button",
                class: "btn btn-secondary",
                onclick: () => fire(p.seconds, p.label),
            }, p.label))),
            "Click a preset to set the timer immediately.",
        ),
        field("Custom", el("div", { style: "display:flex; gap:8px; align-items:center" },
            customMins,
            el("button", { type: "submit", class: "btn btn-primary" }, "Set custom timer"),
        ), "Pick any number of minutes."),
    );

    async function fire(seconds, label) {
        const action = $("input:checked", actionSeg).value;
        try {
            await api.socketTimer(socket.id, { action, in_seconds: seconds, note: `Quick: ${label}` });
            toasts.success("Timer set", `${socket.name}: ${action} in ${label}`);
            modal.close();
            await loadAll();
        } catch (e) { toasts.error("Failed", e.message); }
    }
    function submitCustom() {
        const mins = parseInt(customMins.value, 10);
        if (!Number.isFinite(mins) || mins <= 0) {
            toasts.warn("Pick a duration", "Enter a positive number of minutes.");
            return;
        }
        fire(mins * 60, `${mins} min`);
    }

    modal.open({
        title: `Set a timer · ${socket.name}`,
        subtitle: "Schedules a one-shot action and removes itself once it fires.",
        body,
        actions: [
            { label: "Close", class: "btn-ghost", onClick: () => modal.close() },
        ],
    });
}

// ---------- Segmented control helper ----------
function renderSegmented(name, defaultValue, options) {
    const container = el("div", { class: "segmented", role: "radiogroup" });
    for (const opt of options) {
        const id = `${name}_${opt.value}`;
        const input = el("input", { type: "radio", name, id, value: opt.value, disabled: opt.disabled || false });
        if (opt.value === defaultValue) input.checked = true;
        const label = el("label", { for: id, "aria-disabled": opt.disabled ? "true" : null }, opt.label);
        if (opt.disabled) label.style.opacity = "0.4";
        container.append(input, label);
    }
    // If the default was disabled, pick the first enabled one.
    if (!container.querySelector("input:checked")) {
        const first = container.querySelector("input:not([disabled])");
        if (first) first.checked = true;
    }
    return container;
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
