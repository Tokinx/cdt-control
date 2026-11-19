const state = {
  authenticated: false,
  username: "",
  dashboard: null,
  accounts: [],
  activeInstanceId: null,
  detailLogTab: "realtime",
  detailLogLevel: "all",
  globalLogTab: "audit",
  globalLogLevel: "all",
  settingsCache: null,
  discoveredInstances: [],
  discoverSeq: 0,
  selects: {},
  regionCombo: null,
};

const $ = (sel, root = document) => root.querySelector(sel);
const $$ = (sel, root = document) => [...root.querySelectorAll(sel)];

const REGION_SUGGESTIONS = [
  { value: "cn-hongkong", label: "香港" },
  { value: "cn-shanghai", label: "上海" },
  { value: "cn-hangzhou", label: "杭州" },
  { value: "cn-beijing", label: "北京" },
  { value: "cn-shenzhen", label: "深圳" },
  { value: "cn-guangzhou", label: "广州" },
  { value: "cn-chengdu", label: "成都" },
  { value: "ap-southeast-1", label: "新加坡" },
  { value: "ap-northeast-1", label: "东京" },
  { value: "us-west-1", label: "硅谷" },
  { value: "eu-central-1", label: "法兰克福" },
];

const ICONS = {
  key: `<svg class="icon icon-xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" aria-hidden="true"><circle cx="7.5" cy="15.5" r="4.5"/><path d="m21 2-9.6 9.6"/><path d="m15.5 7.5 3 3L22 7l-3-3"/></svg>`,
  clock: `<svg class="icon icon-xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M12 7v5l3 2"/></svg>`,
  chevron: `<svg class="icon icon-xs chev" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="m9 6 6 6-6 6"/></svg>`,
  server: `<svg class="icon icon-xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" aria-hidden="true"><rect x="3" y="4" width="18" height="7"/><rect x="3" y="13" width="18" height="7"/><line x1="7" y1="7.5" x2="7.01" y2="7.5" stroke-width="2.5"/><line x1="7" y1="16.5" x2="7.01" y2="16.5" stroke-width="2.5"/></svg>`,
  cpu: `<svg class="icon icon-xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" aria-hidden="true"><rect x="6" y="6" width="12" height="12"/><rect x="9" y="9" width="6" height="6"/><path d="M9 2v4M15 2v4M9 18v4M15 18v4M2 9h4M2 15h4M18 9h4M18 15h4"/></svg>`,
  shield: `<svg class="icon icon-xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" aria-hidden="true"><path d="M12 2 4 5v6c0 5.2 3.4 9.8 8 11 4.6-1.2 8-5.8 8-11V5z"/></svg>`,
  play: `<svg class="icon icon-xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><polygon points="6 4 20 12 6 20 6 4"/></svg>`,
  power: `<svg class="icon icon-xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M12 3v9"/><path d="M18.36 6.64a9 9 0 1 1-12.73 0"/></svg>`,
  logs: `<svg class="icon icon-xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><rect x="3" y="4" width="18" height="16"/><path d="m7 9 3 3-3 3"/><line x1="12" y1="15" x2="17" y2="15"/></svg>`,
  edit: `<svg class="icon icon-xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M12 20h9"/><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z"/></svg>`,
  trash: `<svg class="icon icon-xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="M3 6h18"/><path d="M8 6V3h8v3"/><path d="M19 6v15H5V6"/></svg>`,
  test: `<svg class="icon icon-xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" aria-hidden="true"><path d="m5 12 5 5L20 7"/></svg>`,
  check: `<svg class="icon icon-xs" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="m5 12 5 5L20 7"/></svg>`,
  chevronDown: `<svg class="icon icon-xs cselect-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="m6 9 6 6 6-6"/></svg>`,
};

function esc(val) {
  return String(val ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function fmtTime(iso) {
  if (!iso) return "--";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  });
}

function fmtGB(v) {
  return Number(v || 0).toFixed(2);
}

function showToast(msg, kind = "info") {
  const region = $("#toast-region");
  const node = document.createElement("div");
  node.className = `toast ${kind === "error" ? "is-error" : kind === "success" ? "is-success" : ""}`;
  node.textContent = msg;
  region.append(node);
  setTimeout(() => node.remove(), 3500);
}

async function apiFetch(url, options = {}) {
  const opts = {
    credentials: "same-origin",
    headers: {
      ...(options.body ? { "Content-Type": "application/json" } : {}),
      ...(options.headers || {}),
    },
    ...options,
  };
  const res = await fetch(url, opts);
  let data = null;
  const ct = res.headers.get("content-type") || "";
  if (ct.includes("application/json")) {
    data = await res.json().catch(() => null);
  }
  if (res.status === 401 && !url.startsWith("/api/auth/")) {
    setLoggedOut();
    throw new Error("登录状态已过期，请重新登录");
  }
  if (!res.ok) {
    throw new Error(data?.error || `请求失败 (${res.status})`);
  }
  return data;
}

/* ==================== 浮层定位工具 ==================== */
const supportsPopover = Object.hasOwn(HTMLElement.prototype, "popover");

function positionFloatingMenu(anchorEl, menuEl) {
  const rect = anchorEl.getBoundingClientRect();
  const minWidth = Math.max(Math.round(rect.width), 132);
  const maxWidth = Math.min(window.innerWidth - 16, Math.max(minWidth, 440));
  menuEl.style.minWidth = `${minWidth}px`;
  menuEl.style.maxWidth = `${maxWidth}px`;

  const menuW = Math.max(Math.round(menuEl.offsetWidth), minWidth);
  const menuH = Math.min(Math.round(menuEl.scrollHeight || 240), 240);
  const spaceBelow = Math.floor(window.innerHeight - rect.bottom - 8);
  const spaceAbove = Math.floor(rect.top - 8);

  let top;
  let maxH;
  if (spaceBelow >= Math.min(menuH, 160) || spaceBelow >= spaceAbove) {
    top = Math.round(rect.bottom) + 2;
    maxH = Math.max(100, Math.min(240, spaceBelow));
  } else {
    maxH = Math.max(100, Math.min(240, spaceAbove));
    top = Math.max(8, Math.round(rect.top) - Math.min(menuH, maxH) - 2);
  }

  const left = Math.max(8, Math.min(Math.round(rect.left), window.innerWidth - menuW - 8));
  menuEl.style.left = `${left}px`;
  menuEl.style.top = `${top}px`;
  menuEl.style.maxHeight = `${maxH}px`;
}

/* ==================== 自定义 Select 组件 ==================== */
function createCustomSelect(rootEl, { options = [], value = "", placeholder = "请选择", onChange = null } = {}) {
  const trigger = $(".cselect-trigger", rootEl);
  const valueEl = $(".cselect-value", rootEl);
  const menu = $(".cselect-menu", rootEl);

  if (!supportsPopover) {
    menu.removeAttribute("popover");
  }

  let currentOptions = options;
  let currentValue = value;
  let isOpen = false;

  const isMenuOpen = () => (supportsPopover ? menu.matches(":popover-open") : isOpen);

  const renderOptions = () => {
    if (!currentOptions.length) {
      menu.innerHTML = `<div class="ccombo-empty">无可用选项</div>`;
      return;
    }
    menu.innerHTML = currentOptions.map((opt) => {
      const selected = String(opt.value) === String(currentValue);
      let contentHTML;
      if (opt.meta || opt.badge) {
        contentHTML = `
          <div class="cselect-opt-stack">
            <div class="cselect-opt-main">
              <span class="cselect-opt-title${opt.labelMono ? " mono" : ""}">${esc(opt.label)}</span>
            </div>
            <div class="cselect-opt-meta">${opt.meta ? `<span>${esc(opt.meta)}</span>` : ""}${opt.badge ? `<span class="cselect-opt-badge">${esc(opt.badge)}</span>` : ""}</div>
          </div>
        `;
      } else {
        contentHTML = `<span>${esc(opt.label)}${opt.sub ? ` <span class="cselect-option-sub">${esc(opt.sub)}</span>` : ""}</span>`;
      }
      return `
        <button type="button" class="cselect-option" role="option" data-val="${esc(opt.value)}" aria-selected="${selected}" ${opt.disabled ? "disabled" : ""}>
          ${contentHTML}
          <span>${selected ? ICONS.check : ""}</span>
        </button>
      `;
    }).join("");

    for (const btn of $$(".cselect-option:not(:disabled)", menu)) {
      btn.addEventListener("click", () => {
        setValue(btn.dataset.val, false);
        close();
        trigger.focus();
      });
    }
  };

  const syncDisplay = () => {
    const matched = currentOptions.find((o) => String(o.value) === String(currentValue));
    valueEl.textContent = matched ? matched.label : placeholder;
    for (const btn of $$(".cselect-option", menu)) {
      const sel = btn.dataset.val === String(currentValue);
      btn.setAttribute("aria-selected", String(sel));
      btn.lastElementChild.innerHTML = sel ? ICONS.check : "";
    }
  };

  const open = () => {
    if (isMenuOpen()) return;
    renderOptions();
    if (supportsPopover) {
      menu.showPopover();
    } else {
      menu.classList.add("is-open");
      isOpen = true;
    }
    trigger.setAttribute("aria-expanded", "true");
    positionFloatingMenu(trigger, menu);
    const activeOpt = $(`.cselect-option[aria-selected="true"]:not(:disabled)`, menu) || $(".cselect-option:not(:disabled)", menu);
    activeOpt?.focus();
  };

  const close = () => {
    if (!isMenuOpen()) return;
    if (supportsPopover) {
      menu.hidePopover();
    } else {
      menu.classList.remove("is-open");
      isOpen = false;
    }
    trigger.setAttribute("aria-expanded", "false");
  };

  const setOptions = (nextOptions, preserveValue = true) => {
    currentOptions = nextOptions || [];
    if (!preserveValue || !currentOptions.some((o) => String(o.value) === String(currentValue))) {
      currentValue = currentOptions.length ? String(currentOptions[0].value) : "";
    }
    renderOptions();
    syncDisplay();
  };

  const setValue = (nextVal, silent = true) => {
    const strVal = String(nextVal ?? "");
    const changed = strVal !== String(currentValue);
    currentValue = strVal;
    syncDisplay();
    if (!silent && changed && onChange) {
      onChange(currentValue);
    }
  };

  const getValue = () => currentValue;

  trigger.addEventListener("click", () => {
    if (isMenuOpen()) close();
    else open();
  });

  trigger.addEventListener("keydown", (e) => {
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      open();
    }
  });

  menu.addEventListener("keydown", (e) => {
    const items = $$(".cselect-option:not(:disabled)", menu);
    if (!items.length) return;
    const idx = items.indexOf(document.activeElement);
    if (e.key === "ArrowDown") {
      e.preventDefault();
      items[(idx + 1) % items.length].focus();
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      items[(idx - 1 + items.length) % items.length].focus();
    } else if (e.key === "Home") {
      e.preventDefault();
      items[0].focus();
    } else if (e.key === "End") {
      e.preventDefault();
      items[items.length - 1].focus();
    } else if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      close();
      trigger.focus();
    } else if (e.key === "Tab") {
      close();
    }
  });

  if (supportsPopover) {
    menu.addEventListener("toggle", (e) => {
      if (e.newState === "closed") {
        trigger.setAttribute("aria-expanded", "false");
      }
    });
  } else {
    document.addEventListener("pointerdown", (e) => {
      if (isOpen && !rootEl.contains(e.target)) close();
    });
  }

  window.addEventListener("resize", close);
  rootEl.closest(".modal-body")?.addEventListener("scroll", close);

  renderOptions();
  syncDisplay();

  return { setOptions, setValue, getValue, close };
}

/* ==================== 自定义 Combobox 组件（地域建议） ==================== */
function createRegionCombobox(rootEl, suggestions) {
  const input = $("input", rootEl);
  const toggleBtn = $(".ccombo-toggle", rootEl);
  const menu = $(".ccombo-menu", rootEl);

  if (!supportsPopover) {
    menu.removeAttribute("popover");
  }

  let open = false;
  let filtered = suggestions.slice();
  let highlightedIdx = -1;

  const renderList = (showAll = false) => {
    const q = showAll ? "" : input.value.trim().toLowerCase();
    filtered = q
      ? suggestions.filter((s) => s.value.toLowerCase().includes(q) || s.label.toLowerCase().includes(q))
      : suggestions.slice();
    highlightedIdx = filtered.length ? 0 : -1;

    if (!filtered.length) {
      menu.innerHTML = `<div class="ccombo-empty">无匹配预设，可直接输入自定义地域 ID</div>`;
      return;
    }

    menu.innerHTML = filtered.map((opt, idx) => `
      <button type="button" class="cselect-option ${idx === highlightedIdx ? "is-highlighted" : ""}" role="option" data-idx="${idx}" aria-selected="${opt.value === input.value}">
        <span class="mono">${esc(opt.value)}</span>
        <span class="cselect-option-sub">${esc(opt.label)}</span>
      </button>
    `).join("");

    for (const btn of $$(".cselect-option", menu)) {
      btn.addEventListener("mousedown", (e) => e.preventDefault());
      btn.addEventListener("click", () => {
        const chosen = filtered[Number(btn.dataset.idx)];
        if (chosen) {
          input.value = chosen.value;
          input.dispatchEvent(new Event("input", { bubbles: true }));
        }
        closeMenu();
        input.focus();
      });
    }
  };

  const openMenu = (showAll = false) => {
    renderList(showAll);
    if (supportsPopover) {
      if (!menu.matches(":popover-open")) menu.showPopover();
    } else {
      menu.classList.add("is-open");
    }
    open = true;
    input.setAttribute("aria-expanded", "true");
    positionFloatingMenu(input, menu);
  };

  const closeMenu = () => {
    if (!open) return;
    if (supportsPopover) {
      if (menu.matches(":popover-open")) menu.hidePopover();
    } else {
      menu.classList.remove("is-open");
    }
    open = false;
    highlightedIdx = -1;
    input.setAttribute("aria-expanded", "false");
  };

  const updateHighlight = () => {
    const items = $$(".cselect-option", menu);
    items.forEach((el, i) => el.classList.toggle("is-highlighted", i === highlightedIdx));
    if (highlightedIdx >= 0 && items[highlightedIdx]) {
      items[highlightedIdx].scrollIntoView({ block: "nearest" });
    }
  };

  input.addEventListener("focus", () => openMenu(false));
  input.addEventListener("input", () => openMenu(false));
  input.addEventListener("keydown", (e) => {
    if (!open && e.key === "ArrowDown") {
      e.preventDefault();
      openMenu(true);
      return;
    }
    if (!open) return;

    if (e.key === "ArrowDown") {
      e.preventDefault();
      if (filtered.length) {
        highlightedIdx = (highlightedIdx + 1) % filtered.length;
        updateHighlight();
      }
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      if (filtered.length) {
        highlightedIdx = (highlightedIdx - 1 + filtered.length) % filtered.length;
        updateHighlight();
      }
    } else if (e.key === "Enter" && highlightedIdx >= 0 && filtered[highlightedIdx]) {
      e.preventDefault();
      input.value = filtered[highlightedIdx].value;
      input.dispatchEvent(new Event("input", { bubbles: true }));
      closeMenu();
    } else if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      closeMenu();
    } else if (e.key === "Tab") {
      closeMenu();
    }
  });

  toggleBtn.addEventListener("mousedown", (e) => e.preventDefault());
  toggleBtn.addEventListener("click", () => {
    if (open) closeMenu();
    else {
      input.focus();
      openMenu(true);
    }
  });

  document.addEventListener("pointerdown", (e) => {
    if (open && !rootEl.contains(e.target) && !menu.contains(e.target)) {
      closeMenu();
    }
  });
  window.addEventListener("resize", closeMenu);
  rootEl.closest(".modal-body")?.addEventListener("scroll", closeMenu);

  return { close: closeMenu };
}

function initCustomSelects() {
  const logLevelOpts = [
    { value: "all", label: "全部级别" },
    { value: "action", label: "动作" },
    { value: "warn", label: "预警" },
    { value: "error", label: "错误" },
    { value: "info", label: "信息" },
  ];

  const modeOpts = [
    { value: "full", label: "自动开关机" },
    { value: "fuse_only", label: "仅超限关机" },
    { value: "paused", label: "仅监控流量" },
  ];

  const stoppedOpts = [
    { value: "StopCharging", label: "StopCharging（节省停机）" },
    { value: "KeepCharging", label: "KeepCharging（普通停机）" },
  ];

  const themeOpts = [
    { value: "system", label: "跟随系统" },
    { value: "light", label: "浅色" },
    { value: "dark", label: "深色" },
  ];

  state.selects.detailLogLevel = createCustomSelect($("#detail-log-level"), {
    options: logLevelOpts,
    value: "all",
    onChange: (val) => {
      state.detailLogLevel = val;
      if (state.activeInstanceId) refreshInstanceLogs(state.activeInstanceId);
    },
  });

  state.selects.globalLogLevel = createCustomSelect($("#glog-level"), {
    options: logLevelOpts,
    value: "all",
    onChange: (val) => {
      state.globalLogLevel = val;
      refreshGlobalLogs();
    },
  });

  state.selects.instAccount = createCustomSelect($("#inst-account-id"), {
    options: [],
    placeholder: "请选择账号",
    onChange: (val) => {
      if (!$("#inst-id").value) {
        onAddInstanceAccountChange(Number(val));
      }
    },
  });

  state.selects.instDiscover = createCustomSelect($("#inst-discover"), {
    options: [{ value: "__manual__", label: "手动录入" }],
    value: "__manual__",
    placeholder: "选择实例",
    onChange: (val) => applyDiscoveredInstanceSelection(val),
  });

  state.selects.instMode = createCustomSelect($("#inst-mode"), {
    options: modeOpts,
    value: "full",
  });

  state.selects.instStoppedMode = createCustomSelect($("#inst-stopped-mode"), {
    options: stoppedOpts,
    value: "StopCharging",
  });

  state.selects.setTheme = createCustomSelect($("#set-theme"), {
    options: themeOpts,
    value: getStoredTheme(),
  });

  state.regionCombo = createRegionCombobox($("#inst-region-combo"), REGION_SUGGESTIONS);
}

/* ==================== 主题管理 (dark-mode-toggle) ==================== */
function getStoredTheme() {
  try {
    const v = localStorage.getItem("color-scheme");
    return v === "light" || v === "dark" ? v : "system";
  } catch {
    return "system";
  }
}

function applyThemeOverride(mode) {
  const meta = $('meta[name="color-scheme"]');
  if (mode === "light" || mode === "dark") {
    meta.content = mode;
    try { localStorage.setItem("color-scheme", mode); } catch {}
  } else {
    meta.content = "light dark";
    try { localStorage.removeItem("color-scheme"); } catch {}
  }
}

window.addEventListener("storage", (e) => {
  if (e.key === "color-scheme") {
    const mode = e.newValue === "light" || e.newValue === "dark" ? e.newValue : "system";
    applyThemeOverride(mode);
    state.selects.setTheme?.setValue(mode, true);
  }
});

/* ==================== 原生 Dialog 增强 (dialog & light-dismiss) ==================== */
function setupDialogs() {
  for (const dialog of $$("dialog")) {
    dialog.addEventListener("close", () => {
      state.regionCombo?.close();
    });
  }

  for (const btn of $$("[data-close-dialog]")) {
    btn.addEventListener("click", () => {
      const target = document.getElementById(btn.dataset.closeDialog);
      if (target?.open) {
        if (typeof target.requestClose === "function") target.requestClose();
        else target.close("cancel");
      }
    });
  }

  if (!("closedBy" in HTMLDialogElement.prototype)) {
    const outside = (e, dialog) => {
      const r = dialog.getBoundingClientRect();
      return e.clientX < r.left || e.clientX > r.right || e.clientY < r.top || e.clientY > r.bottom;
    };
    for (const dialog of $$('dialog[closedby="any"]')) {
      let pressedOutside = false;
      dialog.addEventListener("pointerdown", (e) => {
        pressedOutside = e.target === dialog && outside(e, dialog);
      });
      dialog.addEventListener("pointerup", (e) => {
        if (pressedOutside && e.target === dialog && outside(e, dialog)) {
          dialog.close("dismiss");
        }
        pressedOutside = false;
      });
    }
  }
}

/* ==================== 表单交互验证 & ARIA (validate-input-after-interaction) ==================== */
function initFormValidation() {
  const linkError = (el, shown) => {
    const errorId = el.dataset?.error;
    if (!errorId) return;
    const ids = (el.getAttribute("aria-describedby") || "")
      .split(/\s+/)
      .filter((id) => id && id !== errorId);
    if (shown) ids.push(errorId);
    if (ids.length) el.setAttribute("aria-describedby", ids.join(" "));
    else el.removeAttribute("aria-describedby");
  };

  const announcer = document.createElement("div");
  announcer.setAttribute("aria-live", "polite");
  announcer.className = "visually-hidden";
  document.body.append(announcer);
  let timer;
  let announcedEl;

  const spokenText = (node) => {
    const copy = node.cloneNode(true);
    copy.querySelectorAll('[aria-hidden="true"]').forEach((n) => n.remove());
    copy.querySelectorAll('[role="img"][aria-label]').forEach((n) => {
      n.replaceWith(`${n.getAttribute("aria-label")}: `);
    });
    return copy.textContent.replace(/\s+/g, " ").trim();
  };

  const announceError = (el) => {
    const errNode = document.getElementById(el.dataset.error);
    if (!errNode) return;
    const label = el.labels?.[0] ? `${spokenText(el.labels[0])}: ` : "";
    clearTimeout(timer);
    announcer.textContent = "";
    announcedEl = el;
    timer = setTimeout(() => {
      announcer.textContent = label + spokenText(errNode);
    }, 500);
  };

  const supportsUserInvalid = CSS.supports("selector(:user-invalid)");
  const dirtyState = new WeakMap();
  const flagged = new WeakSet();
  const announced = new WeakSet();

  const isFieldInvalid = (el) => {
    if (supportsUserInvalid) return el.matches(":user-invalid");
    return el.classList.contains("user-invalid-fallback");
  };

  const syncAria = (el, shouldAnnounce = false) => {
    if (!el.matches?.("input, textarea")) return;
    const invalid = isFieldInvalid(el);
    if (invalid) {
      if (shouldAnnounce && !announced.has(el)) {
        announceError(el);
        announced.add(el);
      }
      el.setAttribute("aria-invalid", "true");
      linkError(el, true);
      flagged.add(el);
    } else if (flagged.has(el) || el.getAttribute("aria-invalid") !== "true") {
      el.setAttribute("aria-invalid", "false");
      linkError(el, false);
      flagged.delete(el);
      announced.delete(el);
      if (announcedEl === el) {
        clearTimeout(timer);
        announcer.textContent = "";
      }
    }
  };

  if (!supportsUserInvalid) {
    const updateFallback = (input) => {
      if (!input.checkValidity) return;
      const valid = input.validity.valid;
      input.classList.toggle("user-invalid-fallback", !valid);
      input.classList.toggle("user-valid-fallback", valid);
    };
    document.addEventListener("blur", (e) => {
      const st = dirtyState.get(e.target) || { touched: false, blurred: false };
      st.blurred = true;
      dirtyState.set(e.target, st);
      if (st.touched) updateFallback(e.target);
      syncAria(e.target, true);
    }, true);
    document.addEventListener("input", (e) => {
      const st = dirtyState.get(e.target) || { touched: false, blurred: false };
      st.touched = true;
      dirtyState.set(e.target, st);
      if (st.blurred) updateFallback(e.target);
      if (e.target.hasAttribute("aria-invalid")) syncAria(e.target);
    });
    document.addEventListener("invalid", (e) => {
      dirtyState.set(e.target, { touched: true, blurred: true });
      updateFallback(e.target);
      announced.add(e.target);
      syncAria(e.target);
    }, true);
  } else {
    document.addEventListener("blur", (e) => syncAria(e.target, true), true);
    document.addEventListener("input", (e) => {
      if (e.target.hasAttribute("aria-invalid")) syncAria(e.target);
    });
    document.addEventListener("change", (e) => syncAria(e.target, true), true);
    document.addEventListener("invalid", (e) => {
      announced.add(e.target);
      syncAria(e.target);
    }, true);
  }

  document.addEventListener("reset", (e) => {
    setTimeout(() => {
      for (const el of e.target.elements || []) {
        dirtyState.delete(el);
        el.classList?.remove("user-invalid-fallback", "user-valid-fallback");
        syncAria(el);
      }
    });
  }, true);

  $$("input, textarea").forEach((el) => syncAria(el));
}

/* ==================== 统一操作下拉菜单与二次确认弹窗 ==================== */
let actionMenuInitialized = false;
let activeActionTrigger = null;
let suppressedActionTrigger = null;
let fallbackActionMenuOpen = false;

function isActionPopoverOpen() {
  const menu = $("#action-popover");
  if (!menu) return false;
  return supportsPopover ? menu.matches(":popover-open") : fallbackActionMenuOpen;
}

function closeActionMenu(restoreFocus = false) {
  const menu = $("#action-popover");
  const prevTrigger = activeActionTrigger;
  activeActionTrigger = null;
  if (prevTrigger) {
    prevTrigger.setAttribute("aria-expanded", "false");
  }
  if (menu) {
    if (supportsPopover) {
      if (menu.matches(":popover-open")) {
        menu.hidePopover();
      }
    } else {
      menu.classList.remove("is-open");
      fallbackActionMenuOpen = false;
    }
  }
  if (restoreFocus && prevTrigger && prevTrigger.isConnected) {
    prevTrigger.focus();
  }
}

function initActionMenu() {
  if (actionMenuInitialized) return;
  const menu = $("#action-popover");
  if (!menu) return;
  actionMenuInitialized = true;

  if (!supportsPopover) {
    menu.removeAttribute("popover");
    document.addEventListener("pointerdown", (e) => {
      if (
        fallbackActionMenuOpen &&
        !menu.contains(e.target) &&
        !activeActionTrigger?.contains(e.target)
      ) {
        closeActionMenu(false);
      }
    });
  } else {
    document.addEventListener(
      "pointerdown",
      (e) => {
        if (
          activeActionTrigger &&
          activeActionTrigger.contains(e.target) &&
          menu.matches(":popover-open")
        ) {
          suppressedActionTrigger = activeActionTrigger;
        } else {
          suppressedActionTrigger = null;
        }
      },
      true
    );

    menu.addEventListener("toggle", (e) => {
      if (e.newState === "closed" && !menu.matches(":popover-open")) {
        if (activeActionTrigger) {
          activeActionTrigger.setAttribute("aria-expanded", "false");
          activeActionTrigger = null;
        }
      }
    });
  }

  menu.addEventListener("keydown", (e) => {
    const items = $$(".cselect-option:not(:disabled)", menu);
    if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      closeActionMenu(true);
      return;
    }
    if (e.key === "Tab") {
      closeActionMenu(false);
      return;
    }
    if (!items.length) return;
    const idx = items.indexOf(document.activeElement);
    if (e.key === "ArrowDown") {
      e.preventDefault();
      items[(idx + 1) % items.length].focus();
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      items[(idx - 1 + items.length) % items.length].focus();
    } else if (e.key === "Home") {
      e.preventDefault();
      items[0].focus();
    } else if (e.key === "End") {
      e.preventDefault();
      items[items.length - 1].focus();
    }
  });

  window.addEventListener("resize", () => {
    if (isActionPopoverOpen()) closeActionMenu(false);
  });
}

function openActionMenu(triggerBtn, items = []) {
  initActionMenu();
  const menu = $("#action-popover");
  if (!menu || !triggerBtn) return;

  if (suppressedActionTrigger === triggerBtn) {
    suppressedActionTrigger = null;
    return;
  }
  suppressedActionTrigger = null;

  if (isActionPopoverOpen() && activeActionTrigger === triggerBtn) {
    closeActionMenu(false);
    return;
  }

  if (activeActionTrigger && activeActionTrigger !== triggerBtn) {
    activeActionTrigger.setAttribute("aria-expanded", "false");
  }
  activeActionTrigger = triggerBtn;

  menu.innerHTML = items
    .map((item, idx) => {
      if (item.type === "divider") {
        return `<div class="cselect-divider" role="separator"></div>`;
      }
      const cls = `cselect-option${item.danger ? " cselect-option-danger" : ""}`;
      return `
        <button type="button" class="${cls}" data-action-idx="${idx}" ${item.disabled ? "disabled" : ""}>
          <span class="cselect-option-item">${item.icon || ""}<span>${esc(item.label)}</span></span>
        </button>
      `;
    })
    .join("");

  for (const btn of $$("[data-action-idx]", menu)) {
    btn.addEventListener("click", () => {
      const idx = Number(btn.dataset.actionIdx);
      const picked = items[idx];
      closeActionMenu(false);
      if (picked && !picked.disabled && picked.onClick) {
        picked.onClick();
      }
    });
  }

  if (supportsPopover) {
    if (!menu.matches(":popover-open")) {
      menu.showPopover();
    }
  } else {
    menu.classList.add("is-open");
    fallbackActionMenuOpen = true;
  }
  triggerBtn.setAttribute("aria-expanded", "true");
  positionFloatingMenu(triggerBtn, menu);
  const firstEnabled = $(".cselect-option:not(:disabled)", menu);
  firstEnabled?.focus();
}

function openActionConfirmModal({
  title = "确认操作",
  desc = "",
  summaryRows = [],
  warningText = "",
  showStopOptions = false,
  defaultForceStop = false,
  optionLabel = "",
  optionDefault = false,
  confirmLabel = "确认",
  danger = false,
} = {}) {
  return new Promise((resolve) => {
    const dialog = $("#action-confirm-dialog");
    $("#act-confirm-title").textContent = title;
    $("#act-confirm-desc").textContent = desc;

    const summaryEl = $("#act-confirm-summary");
    if (summaryRows.length) {
      summaryEl.innerHTML = summaryRows
        .map(
          ([label, value, mono]) =>
            `<dt>${esc(label)}</dt><dd${mono ? ' class="mono"' : ""}>${esc(value)}</dd>`
        )
        .join("");
      summaryEl.hidden = false;
    } else {
      summaryEl.innerHTML = "";
      summaryEl.hidden = true;
    }

    const warnEl = $("#act-confirm-warn");
    if (warningText) {
      warnEl.textContent = warningText;
      warnEl.hidden = false;
    } else {
      warnEl.textContent = "";
      warnEl.hidden = true;
    }

    const optsEl = $("#act-confirm-options");
    const optCheck = $("#act-confirm-opt-check");
    const optLabelEl = $("#act-confirm-opt-label");
    const resolvedOptLabel = optionLabel || (showStopOptions ? "强制关机 (ForceStop)" : "");
    const resolvedOptDefault = optionLabel ? Boolean(optionDefault) : Boolean(defaultForceStop);
    optsEl.hidden = !resolvedOptLabel;
    if (resolvedOptLabel) {
      optLabelEl.textContent = resolvedOptLabel;
      optCheck.checked = resolvedOptDefault;
    } else {
      optCheck.checked = false;
    }

    const submitBtn = $("#act-confirm-btn");
    submitBtn.textContent = confirmLabel;
    submitBtn.className = danger ? "btn btn-danger" : "btn btn-primary";

    const onClose = () => {
      const confirmed = dialog.returnValue === "confirm";
      dialog.returnValue = "";
      dialog.removeEventListener("close", onClose);
      const checked = Boolean(resolvedOptLabel) && optCheck.checked;
      resolve({
        confirmed,
        option_checked: checked,
        force_stop: showStopOptions && checked,
      });
    };
    dialog.returnValue = "";
    dialog.addEventListener("close", onClose);
    dialog.showModal();
  });
}

/* ==================== 登录与路由控制 ==================== */
function setLoggedOut() {
  state.authenticated = false;
  state.dashboard = null;
  $("#app-shell").hidden = true;
  $("#login-view").hidden = false;
  $("#login-username")?.focus();
}

async function setLoggedIn(username) {
  state.authenticated = true;
  state.username = username;
  $("#login-view").hidden = true;
  $("#app-shell").hidden = false;
  await refreshAll();
  handleHashRoute();
}

function handleHashRoute() {
  if (!state.authenticated) return;
  const hash = location.hash || "";
  const match = hash.match(/^#instance\/(\d+)$/);
  if (match) {
    openInstanceDetail(Number(match[1]), false);
  } else {
    showDashboardView(false);
  }
}

function showDashboardView(pushHash = true) {
  state.activeInstanceId = null;
  $("#detail-view").hidden = true;
  $("#dashboard-view").hidden = false;
  if (pushHash && location.hash) {
    history.pushState(null, "", "#");
  }
}

/* ==================== 数据加载与渲染 ==================== */
async function refreshAll() {
  if (!state.authenticated) return;
  const [dash, accRes] = await Promise.all([
    apiFetch("/api/dashboard"),
    apiFetch("/api/accounts"),
  ]);
  state.dashboard = dash;
  state.accounts = accRes.accounts || [];

  renderDashboard();
  if (state.activeInstanceId) {
    await refreshDetailView(state.activeInstanceId);
  }
}

function renderDashboard() {
  const dash = state.dashboard;
  if (!dash) return;
  renderCDTCards(dash.cdt_cards || [], dash.scheduler);
  renderInstancesTable(dash.instances || []);
}

function renderCDTCards(cards, scheduler) {
  const grid = $("#cdt-grid");
  $("#cdt-count-label").textContent = cards.length ? `${cards.length} 个账号` : "";

  const syncEl = $("#cdt-sync-label");
  const syncAt = scheduler?.last_run_at || cards[0]?.updated_at || "";
  syncEl.innerHTML = syncAt ? `${ICONS.clock}<span>上次巡检 ${esc(fmtTime(syncAt))}</span>` : "";

  if (!cards.length) {
    grid.innerHTML = `<p class="muted-text">尚未配置阿里云账号，请先在右上角「账号」中添加。</p>`;
    return;
  }

  grid.innerHTML = cards.map((card) => {
    const domPct = card.domestic_quota_gb > 0 ? Math.min(100, (card.domestic_gb / card.domestic_quota_gb) * 100) : 0;
    const ovsPct = card.overseas_quota_gb > 0 ? Math.min(100, (card.overseas_gb / card.overseas_quota_gb) * 100) : 0;
    const domCls = domPct >= 95 ? "is-danger" : domPct >= 80 ? "is-warn" : "";
    const ovsCls = ovsPct >= 95 ? "is-danger" : ovsPct >= 80 ? "is-warn" : "";

    const chips = (card.details || []).length
      ? card.details.map((d) => `<span class="region-chip mono">${esc(d.business_region_id)} ${fmtGB(d.traffic_gb)}G</span>`).join("")
      : `<span class="muted-text">无明细</span>`;

    return `
      <article class="cdt-card" data-fp="${esc(card.key_fingerprint)}">
        <div class="cdt-card-head">
          <div class="cdt-card-title">
            ${ICONS.key}
            <span class="cdt-card-label">${esc(card.label || card.access_key_masked)}</span>
          </div>
          <span class="muted-text mono">${esc(card.access_key_masked)}</span>
        </div>

        <div class="cdt-card-body">
          ${card.last_error ? `<div class="alert alert-danger">${esc(card.last_error)}</div>` : ""}

          <div class="cdt-pools">
            <div class="pool-item">
              <div class="pool-head"><span>境内</span><span class="tabular">${fmtGB(card.domestic_quota_gb)} GB</span></div>
              <div class="pool-val tabular">${fmtGB(card.domestic_gb)} <span class="pool-unit">GB</span></div>
              <div class="meter-track"><div class="meter-fill ${domCls}" style="width:${domPct.toFixed(1)}%"></div></div>
            </div>
            <div class="pool-item">
              <div class="pool-head"><span>境外</span><span class="tabular">${fmtGB(card.overseas_quota_gb)} GB</span></div>
              <div class="pool-val tabular">${fmtGB(card.overseas_gb)} <span class="pool-unit">GB</span></div>
              <div class="meter-track"><div class="meter-fill ${ovsCls}" style="width:${ovsPct.toFixed(1)}%"></div></div>
            </div>
          </div>
        </div>

        <div class="cdt-foot">
          <details class="region-details">
            <summary>${ICONS.chevron}<span>地域明细 (${(card.details || []).length})</span></summary>
            <div class="region-chips">${chips}</div>
          </details>
        </div>
      </article>
    `;
  }).join("");
}

function statusDotHTML(status) {
  switch (status) {
    case "Running":
      return `<span class="status-dot is-running">运行中</span>`;
    case "Stopped":
      return `<span class="status-dot is-stopped">已停止</span>`;
    case "Starting":
    case "Stopping":
      return `<span class="status-dot is-pending">${esc(status)}</span>`;
    case "NotFound":
      return `<span class="status-dot is-error">未找到</span>`;
    default:
      return `<span class="status-dot">${esc(status || "待检查")}</span>`;
  }
}

function modeLabel(mode) {
  if (mode === "fuse_only") return "仅超限关机";
  if (mode === "paused") return "仅监控流量";
  return "自动开关机";
}

function renderInstancesTable(instances) {
  const box = $("#instances-container");
  $("#ecs-count-label").textContent = instances.length ? `${instances.length} 台` : "";

  if (!instances.length) {
    box.innerHTML = `
      <div class="empty-state">
        <span class="muted-text">暂无受管实例，请先添加账号后点击右上角「添加实例」。</span>
      </div>
    `;
    return;
  }

  const rows = instances.map((inst) => {
    const pct = inst.traffic_threshold_gb > 0 ? Math.min(100, (inst.last_traffic_gb / inst.traffic_threshold_gb) * 100) : 0;
    const fillCls = pct >= 100 ? "is-danger" : pct >= 85 ? "is-warn" : "";
    const ipText = inst.last_public_ip || inst.last_private_ip || "";
    const subParts = [inst.region_id, ipText].filter(Boolean).join(" · ");
    const schedText = inst.enable_schedule_control
      ? `${inst.schedule_start}–${inst.schedule_stop}`
      : "";

    return `
      <tr>
        <th scope="row">
          <button type="button" class="inst-title-btn" data-detail-id="${inst.id}">
            ${ICONS.server}
            <span>${esc(inst.name || inst.instance_id)}</span>
          </button>
          <div class="muted-text mono inst-sub-line">${esc(subParts || inst.instance_id)}</div>
        </th>
        <td data-label="账号">
          <span>${esc(inst.account_name || "账号")}</span>
          ${!inst.enabled ? `<div class="muted-text">已禁用</div>` : ""}
        </td>
        <td data-label="状态">
          <button type="button" class="status-trigger" data-status-menu="${inst.id}" aria-expanded="false">
            ${statusDotHTML(inst.last_status)}
            ${ICONS.chevronDown}
          </button>
          ${inst.last_error ? `<div class="inst-error-hint">异常（点详情查看）</div>` : ""}
        </td>
        <td data-label="流量 / 阈值">
          <div class="traffic-cell">
            <div class="label-row tabular muted-text">
              <span>${esc(inst.last_flow_type || "--")}</span>
              <span>${fmtGB(inst.last_traffic_gb)} / ${fmtGB(inst.traffic_threshold_gb)} GB</span>
            </div>
            <div class="meter-track"><div class="meter-fill ${fillCls}" style="width:${pct.toFixed(1)}%"></div></div>
          </div>
        </td>
        <td data-label="策略">
          <span>${esc(modeLabel(inst.automation_mode))}${inst.automation_mode === "full" && schedText ? ` · ${esc(schedText)}` : ""}</span>
        </td>
        <td data-label="操作">
          <div class="row-actions">
            <button type="button" class="row-link" data-detail-id="${inst.id}">${ICONS.logs}<span>详情</span></button>
            <button type="button" class="row-link" data-action="edit" data-id="${inst.id}">${ICONS.edit}<span>编辑</span></button>
          </div>
        </td>
      </tr>
    `;
  }).join("");

  box.innerHTML = `
    <div tabindex="0" role="region" aria-label="受管 ECS 实例列表" class="table-wrapper">
      <table class="data-table">
        <thead>
          <tr>
            <th>实例</th>
            <th>账号</th>
            <th>状态</th>
            <th>流量 / 阈值</th>
            <th>策略</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>${rows}</tbody>
      </table>
    </div>
  `;

  for (const btn of $$("[data-status-menu]", box)) {
    btn.addEventListener("click", () => {
      const inst = (state.dashboard?.instances || []).find((x) => x.id === Number(btn.dataset.statusMenu));
      if (!inst) return;
      openActionMenu(btn, buildInstanceMenuItems(inst, { includeDelete: false }));
    });
  }
  for (const btn of $$("[data-detail-id]", box)) {
    btn.addEventListener("click", () => openInstanceDetail(Number(btn.dataset.detailId), true));
  }
  for (const btn of $$("[data-action]", box)) {
    btn.addEventListener("click", () => handleInstanceRowAction(btn.dataset.action, Number(btn.dataset.id)));
  }
}

function statusLabelText(status) {
  switch (status) {
    case "Running":
      return "运行中";
    case "Stopped":
      return "已停止";
    case "Starting":
      return "启动中";
    case "Stopping":
      return "停止中";
    case "NotFound":
      return "未找到";
    default:
      return status || "待检查";
  }
}

function buildInstanceMenuItems(inst, { includeDelete = false } = {}) {
  const items = [
    {
      label: "开机",
      icon: ICONS.play,
      disabled: inst.last_status === "Running",
      onClick: () => runConfirmedInstanceAction("start", inst),
    },
    {
      label: "普通停机",
      icon: ICONS.power,
      disabled: inst.last_status === "Stopped",
      onClick: () => runConfirmedInstanceAction("stop_keep", inst),
    },
    {
      label: "节省停机",
      icon: ICONS.power,
      disabled: inst.last_status === "Stopped",
      onClick: () => runConfirmedInstanceAction("stop_saving", inst),
    },
  ];
  if (includeDelete) {
    items.push({ type: "divider" });
    items.push({
      label: "删除实例",
      icon: ICONS.trash,
      danger: true,
      onClick: () => runConfirmedInstanceAction("delete", inst),
    });
  }
  return items;
}

async function runConfirmedInstanceAction(action, inst) {
  if (!inst) return;
  const instName = inst.name || inst.instance_id;
  const targetLabel = inst.name ? `${inst.name} (${inst.instance_id})` : inst.instance_id;
  const regionLabel = `${inst.region_id} · ${inst.account_name || "账号"}`;
  const schedLabel = inst.enable_schedule_control ? `${inst.schedule_start} – ${inst.schedule_stop}` : "全天";
  const modeSummary = inst.automation_mode === "full"
    ? `${modeLabel(inst.automation_mode)} · ${schedLabel}`
    : modeLabel(inst.automation_mode);
  const overThreshold = inst.traffic_threshold_gb > 0 && inst.last_traffic_gb >= inst.traffic_threshold_gb;

  try {
    if (action === "edit") {
      openInstanceDialog(inst);
      return;
    }

    if (action === "start") {
      let warningText = "";
      if (overThreshold) {
        warningText = inst.automation_mode === "paused"
          ? `当前累计流量（${fmtGB(inst.last_traffic_gb)} GB）已达到阈值（${fmtGB(inst.traffic_threshold_gb)} GB）；因策略为「仅监控流量」，定时巡检不会自动关机。`
          : `当前累计流量（${fmtGB(inst.last_traffic_gb)} GB）已达到阈值（${fmtGB(inst.traffic_threshold_gb)} GB），下次巡检可能自动触发超限关机；如需保持开机请在「编辑」中切换为「仅监控流量」。`;
      } else if (inst.automation_mode === "full" && inst.enable_schedule_control) {
        warningText = `当前策略处于「自动开关机」定时窗口（${schedLabel}），若当前不在允许运行时段内，下次巡检可能自动关机。`;
      }

      const res = await openActionConfirmModal({
        title: "确认开机实例",
        desc: "将调用阿里云 StartInstances 接口启动该 ECS 实例。",
        summaryRows: [
          ["目标实例", targetLabel, true],
          ["地域账号", regionLabel],
          ["状态变更", `${statusLabelText(inst.last_status)} → 运行中（Starting）`],
          ["CDT 流量", `${fmtGB(inst.last_traffic_gb)} / ${fmtGB(inst.traffic_threshold_gb)} GB（${inst.last_flow_type || "对应地域"}）`],
          ["策略模式", modeSummary],
        ],
        warningText,
        confirmLabel: "确认开机",
        danger: false,
      });
      if (!res.confirmed) return;

      await apiFetch(`/api/instances/${inst.id}/start`, { method: "POST" });
      showToast(`已发送开机指令：${instName}`, "success");
      await refreshAll();
    } else if (action === "stop" || action === "stop_keep" || action === "stop_saving") {
      const stopMode =
        action === "stop_keep"
          ? "KeepCharging"
          : action === "stop_saving"
            ? "StopCharging"
            : (inst.stopped_mode || "StopCharging");
      const isKeep = stopMode === "KeepCharging";
      const stopModeText = isKeep
        ? "KeepCharging（普通停机 · 停机期间保留资源并正常计费）"
        : "StopCharging（节省停机 · 回收算力并暂停计算资源计费）";
      const baseWarn = isKeep
        ? "普通停机期间实例规格与公网 IP 等资源继续计费，适合短时维护；再次开机不受节省停机库存回收影响。"
        : "仅按量付费 VPC 实例支持节省停机；停机期间云盘与固定公网 IP 仍继续计费，再次开机时可能受可用区库存影响。";
      const warningText = inst.automation_mode === "full"
        ? `${baseWarn} 注意当前策略为「自动开关机」，若处于允许运行时段且未超阈值，下次巡检可能自动开机，如需保持关机请在「编辑」中切换策略。`
        : baseWarn;

      const res = await openActionConfirmModal({
        title: isKeep ? "确认普通停机" : "确认节省停机",
        desc: isKeep
          ? "将调用阿里云 StopInstances 接口执行普通停机（StoppedMode=KeepCharging）。"
          : "将调用阿里云 StopInstances 接口执行节省停机（StoppedMode=StopCharging）。",
        summaryRows: [
          ["目标实例", targetLabel, true],
          ["地域账号", regionLabel],
          ["状态变更", `${statusLabelText(inst.last_status)} → 已停止（Stopping）`],
          ["停机模式", stopModeText],
          ["策略模式", modeSummary],
        ],
        warningText,
        showStopOptions: true,
        defaultForceStop: Boolean(inst.force_stop),
        confirmLabel: isKeep ? "确认普通停机" : "确认节省停机",
        danger: true,
      });
      if (!res.confirmed) return;

      await apiFetch(`/api/instances/${inst.id}/stop`, {
        method: "POST",
        body: JSON.stringify({
          force_stop: res.force_stop,
          stopped_mode: stopMode,
        }),
      });
      showToast(`已发送${isKeep ? "普通停机" : "节省停机"}指令：${instName}`, "success");
      await refreshAll();
    } else if (action === "delete") {
      const res = await openActionConfirmModal({
        title: "确认删除实例",
        desc: "仅从本监控面板移除该实例记录，不会停止、重启或释放阿里云上的 ECS 实例。",
        summaryRows: [
          ["目标实例", targetLabel, true],
          ["地域账号", regionLabel],
          ["当前状态", statusLabelText(inst.last_status)],
          ["策略模式", modeSummary],
        ],
        warningText: "删除后将同步清除该实例在本面板中的策略配置及历史审计日志；如仅需保留监控而停止自动开关机，可在「编辑」中切换为「仅监控流量」。",
        confirmLabel: "确认移除",
        danger: true,
      });
      if (!res.confirmed) return;

      await apiFetch(`/api/instances/${inst.id}`, { method: "DELETE" });
      showToast("已删除实例", "success");
      if (!$("#detail-view").hidden && state.activeInstanceId === inst.id) {
        showDashboardView(true);
      }
      await refreshAll();
    }
  } catch (err) {
    showToast(err.message, "error");
  }
}

async function handleInstanceRowAction(action, id) {
  const inst = (state.dashboard?.instances || []).find((x) => x.id === id);
  if (!inst) return;
  await runConfirmedInstanceAction(action, inst);
}

/* ==================== 实例详情与日志视图 ==================== */
async function openInstanceDetail(id, pushHash = true) {
  state.activeInstanceId = id;
  if (pushHash) {
    history.pushState(null, "", `#instance/${id}`);
  }
  $("#dashboard-view").hidden = true;
  $("#detail-view").hidden = false;
  await refreshDetailView(id);
}

async function refreshDetailView(id) {
  try {
    const data = await apiFetch(`/api/instances/${id}`);
    const inst = data.instance;
    const cdt = data.cdt;

    $("#detail-title").textContent = inst.name || inst.instance_id;
    $("#detail-status-slot").innerHTML = statusDotHTML(inst.last_status);
    $("#detail-subtitle").textContent = `${inst.instance_id} · ${inst.region_id} · 上次巡检 ${fmtTime(inst.last_checked_at)}`;

    const credLabel = `${inst.account_name || "账号"} (${inst.access_key_masked})`;
    $("#detail-badges").textContent = `${modeLabel(inst.automation_mode)} · ${credLabel}`;

    const quickBox = $("#detail-quick-actions");
    quickBox.innerHTML = `
      <button type="button" class="btn btn-subtle btn-sm" id="det-actions-btn" aria-expanded="false">
        <span>操作</span>
        ${ICONS.chevronDown}
      </button>
      <button type="button" class="btn btn-subtle btn-sm" id="det-edit">${ICONS.edit}<span>编辑</span></button>
    `;
    $("#det-actions-btn").addEventListener("click", (e) => {
      openActionMenu(e.currentTarget, buildInstanceMenuItems(inst, { includeDelete: true }));
    });
    $("#det-edit").addEventListener("click", () => openInstanceDialog(inst));

    const cdtText = cdt
      ? `境内 ${fmtGB(cdt.domestic_gb)} GB / 境外 ${fmtGB(cdt.overseas_gb)} GB`
      : `${inst.last_flow_type || "--"} ${fmtGB(inst.last_traffic_gb)} GB`;

    $("#detail-meta-list").innerHTML = `
      <dt>实例 ID</dt><dd class="mono">${esc(inst.instance_id)}</dd>
      <dt>地域</dt><dd class="mono">${esc(inst.region_id)}</dd>
      <dt>所属账号</dt><dd>${esc(credLabel)}</dd>
      <dt>公网 IP</dt><dd class="mono">${esc(inst.last_public_ip || "--")}</dd>
      <dt>私网 IP</dt><dd class="mono">${esc(inst.last_private_ip || "--")}</dd>
      <dt>规格</dt><dd>${esc(inst.last_spec || "--")}</dd>
      <dt>系统</dt><dd>${esc(inst.last_os || "--")}</dd>
      <dt>计费方式</dt><dd>${esc(inst.last_charge_type || "--")}</dd>
      <dt>账户 CDT</dt><dd class="tabular">${esc(cdtText)}</dd>
    `;

    $("#detail-policy-list").innerHTML = `
      <dt>策略模式</dt><dd>${esc(modeLabel(inst.automation_mode))}</dd>
      <dt>熔断阈值</dt><dd class="tabular">${fmtGB(inst.traffic_threshold_gb)} GB（${esc(inst.last_flow_type || "对应地域")}）</dd>
      <dt>定时窗口</dt><dd>${inst.automation_mode !== "full" ? (inst.enable_schedule_control ? `${esc(inst.schedule_start)} – ${esc(inst.schedule_stop)}（仅自动开关机生效）` : "未启用（当前策略非自动开关机）") : (inst.enable_schedule_control ? `${esc(inst.schedule_start)} – ${esc(inst.schedule_stop)}` : "全天")}</dd>
      <dt>停机模式</dt><dd>${esc(inst.stopped_mode === "KeepCharging" ? "KeepCharging（普通停机）" : "StopCharging（节省停机）")}${inst.force_stop ? " · 强制" : ""}</dd>
      <dt>监控开关</dt><dd>${inst.enabled ? "启用" : "停用"}</dd>
    `;

    const errBox = $("#detail-error-box");
    if (inst.last_error) {
      errBox.textContent = inst.last_error;
      errBox.hidden = false;
    } else {
      errBox.hidden = true;
    }

    await refreshInstanceLogs(id);
  } catch (err) {
    showToast(err.message, "error");
    showDashboardView(true);
  }
}

function logLevelText(level) {
  switch (level) {
    case "action":
      return "动作";
    case "warn":
      return "预警";
    case "error":
      return "错误";
    case "info":
      return "信息";
    default:
      return "全部级别";
  }
}

function syncLogClearButton(btn, items) {
  if (!btn) return;
  btn.disabled = items.length === 0;
}

async function refreshInstanceLogs(id) {
  const listEl = $("#detail-logs-list");
  const clearBtn = $("#btn-clear-inst-logs");
  try {
    const res = await apiFetch(`/api/instances/${id}/logs?level=${encodeURIComponent(state.detailLogLevel)}`);
    const items = state.detailLogTab === "realtime"
      ? filterLogsByLevel(res.realtime || [], state.detailLogLevel)
      : (res.audit || []);
    syncLogClearButton(clearBtn, items);
    renderLogItems(listEl, items);
  } catch (err) {
    syncLogClearButton(clearBtn, []);
    listEl.innerHTML = `<div class="muted-text log-empty">加载日志失败：${esc(err.message)}</div>`;
  }
}

function filterLogsByLevel(items, level) {
  if (!level || level === "all") return items;
  return items.filter((item) => item.level === level);
}

function renderLogItems(container, items) {
  if (!items.length) {
    container.innerHTML = `<div class="muted-text log-empty">暂无日志</div>`;
    return;
  }
  const levelLabel = (lv) => {
    if (lv === "error") return `<span class="status-dot is-error">错误</span>`;
    if (lv === "warn") return `<span class="status-dot is-pending">预警</span>`;
    if (lv === "action") return `<span class="status-dot is-running">动作</span>`;
    return `<span class="status-dot is-stopped">信息</span>`;
  };
  container.innerHTML = items.map((item) => `
    <div class="log-row">
      <span class="mono muted-text tabular">${esc(fmtTime(item.created_at))}</span>
      <span>${levelLabel(item.level)}</span>
      <span class="log-msg">${item.aliyun_instance_id ? `<span class="mono muted-text">[${esc(item.aliyun_instance_id)}]</span> ` : ""}${esc(item.message)}</span>
    </div>
  `).join("");
}

async function clearLogsWithConfirm({ tab, level, title, desc, summaryRows, endpoint }) {
  const isRealtime = tab === "realtime";
  const tabLabel = isRealtime ? "实时追踪" : "审计记录";
  const levelLabel = logLevelText(level);
  const res = await openActionConfirmModal({
    title,
    desc,
    summaryRows: [
      ...summaryRows,
      ["日志分类", tabLabel],
      ["日志级别", levelLabel],
    ],
    confirmLabel: "确认清除",
    danger: true,
  });
  if (!res.confirmed) return false;

  const scope = isRealtime ? "realtime" : "audit";
  const normalizedLevel = level || "all";
  await apiFetch(
    `${endpoint}?scope=${encodeURIComponent(scope)}&level=${encodeURIComponent(normalizedLevel)}`,
    { method: "DELETE" },
  );
  showToast(
    normalizedLevel === "all" ? `已清除${tabLabel}` : `已清除${tabLabel}（${levelLabel}）`,
    "success",
  );
  return true;
}

async function clearInstanceLogs(id) {
  const inst = (state.dashboard?.instances || []).find((x) => x.id === id);
  const targetLabel = inst ? (inst.name ? `${inst.name} (${inst.instance_id})` : inst.instance_id) : `#${id}`;
  try {
    const cleared = await clearLogsWithConfirm({
      tab: state.detailLogTab,
      level: state.detailLogLevel,
      title: "确认清除当前日志",
      desc: "仅清除当前实例在当前分类与筛选级别下显示的日志，清除后不可恢复。",
      summaryRows: [["目标实例", targetLabel, true]],
      endpoint: `/api/instances/${id}/logs`,
    });
    if (cleared) {
      await refreshInstanceLogs(id);
    }
  } catch (err) {
    showToast(err.message, "error");
  }
}

async function clearGlobalLogs() {
  try {
    const cleared = await clearLogsWithConfirm({
      tab: state.globalLogTab,
      level: state.globalLogLevel,
      title: "确认清除当前日志",
      desc: "仅清除全局日志在当前分类与筛选级别下显示的日志，清除后不可恢复。",
      summaryRows: [["作用范围", "全局（所有账号与实例）"]],
      endpoint: "/api/logs",
    });
    if (cleared) {
      await refreshGlobalLogs();
      if (state.activeInstanceId) {
        await refreshInstanceLogs(state.activeInstanceId);
      }
    }
  } catch (err) {
    showToast(err.message, "error");
  }
}

/* ==================== 弹窗：账号管理 ==================== */
async function openAccountsDialog() {
  resetAccountForm();
  await reloadAccountsInDialog();
  $("#accounts-dialog").showModal();
}

async function reloadAccountsInDialog() {
  const res = await apiFetch("/api/accounts");
  state.accounts = res.accounts || [];
  const box = $("#accounts-list");
  if (!state.accounts.length) {
    box.innerHTML = `<div class="muted-text">暂未保存阿里云账号，请在下方添加。</div>`;
    return;
  }
  box.innerHTML = state.accounts.map((acc) => `
    <div class="account-row">
      <div>
        <div class="meta-item">${ICONS.key}<strong>${esc(acc.name)}</strong> ${acc.enabled ? "" : `<span class="muted-text">（已停用）</span>`}</div>
        <div class="muted-text mono">${esc(acc.access_key_masked)} · 境内 ${fmtGB(acc.domestic_quota_gb)}G / 境外 ${fmtGB(acc.overseas_quota_gb)}G</div>
      </div>
      <div class="row-actions">
        <button type="button" class="row-link" data-acc-test="${acc.id}">${ICONS.test}<span>测试</span></button>
        <button type="button" class="row-link" data-acc-edit="${acc.id}">${ICONS.edit}<span>编辑</span></button>
        <button type="button" class="row-link row-icon-only row-link-danger" data-acc-del="${acc.id}" aria-label="删除账号" title="删除账号">${ICONS.trash}</button>
      </div>
    </div>
  `).join("");

  for (const b of $$("[data-acc-test]", box)) {
    b.addEventListener("click", async () => {
      try {
        const r = await apiFetch(`/api/accounts/${b.dataset.accTest}/test`, { method: "POST" });
        showToast(`${r.name}：境内 ${fmtGB(r.domestic_gb)} GB / 境外 ${fmtGB(r.overseas_gb)} GB`, "success");
      } catch (err) {
        showToast(err.message, "error");
      }
    });
  }
  for (const b of $$("[data-acc-edit]", box)) {
    b.addEventListener("click", () => {
      const acc = state.accounts.find((x) => x.id === Number(b.dataset.accEdit));
      if (acc) fillAccountForm(acc);
    });
  }
  for (const b of $$("[data-acc-del]", box)) {
    b.addEventListener("click", async () => {
      const acc = state.accounts.find((x) => x.id === Number(b.dataset.accDel));
      const res = await openActionConfirmModal({
        title: "确认删除账号",
        desc: "将从本面板移除该阿里云账号及加密保存的 AccessKey 凭证（若仍有关联实例需先解除）。",
        summaryRows: acc
          ? [
              ["账号名称", acc.name],
              ["AccessKey", acc.access_key_masked, true],
              ["CDT 配额", `境内 ${fmtGB(acc.domestic_quota_gb)} GB / 境外 ${fmtGB(acc.overseas_quota_gb)} GB`],
            ]
          : [["账号 ID", `#${b.dataset.accDel}`, true]],
        warningText: "此操作仅清理本面板保存的账号信息，不会影响阿里云账户本身；如仍有实例绑定此账号将被拒绝删除。",
        confirmLabel: "确认删除",
        danger: true,
      });
      if (!res.confirmed) return;
      try {
        await apiFetch(`/api/accounts/${b.dataset.accDel}`, { method: "DELETE" });
        showToast("已删除账号", "success");
        await reloadAccountsInDialog();
        await refreshAll();
      } catch (err) {
        showToast(err.message, "error");
      }
    });
  }
}

function resetAccountForm() {
  $("#account-form").reset();
  $("#acc-id").value = "";
  $("#account-form-title").textContent = "新增阿里云账号";
  $("#btn-account-reset").hidden = true;
  $("#acc-secret-req").hidden = false;
  $("#account-form-msg").hidden = true;
  $("#acc-dom-quota").value = "20";
  $("#acc-ovs-quota").value = "180";
  $("#acc-enabled").checked = true;
}

function fillAccountForm(acc) {
  $("#acc-id").value = acc.id;
  $("#acc-name").value = acc.name;
  $("#acc-ak-id").value = acc.access_key_id;
  $("#acc-ak-secret").value = "";
  $("#acc-dom-quota").value = acc.domestic_quota_gb;
  $("#acc-ovs-quota").value = acc.overseas_quota_gb;
  $("#acc-enabled").checked = Boolean(acc.enabled);
  $("#account-form-title").textContent = `编辑：${acc.name}`;
  $("#btn-account-reset").hidden = false;
  $("#acc-secret-req").hidden = true;
  $("#account-form-msg").hidden = true;
}

/* ==================== 弹窗：添加 / 编辑实例 ==================== */
function syncScheduleUI() {
  $("#inst-sched-times").hidden = !$("#inst-sched-enable").checked;
}

function setInstanceMetaInputs(region, instanceId, name) {
  $("#inst-region").value = region || "";
  $("#inst-aliyun-id").value = instanceId || "";
  $("#inst-name").value = name || "";
  for (const el of [$("#inst-region"), $("#inst-aliyun-id")]) {
    el.classList.remove("user-invalid-fallback");
  }
}

function applyDiscoveredInstanceSelection(val) {
  if (!val || val === "__loading__") return;
  if (val === "__manual__") {
    setInstanceMetaInputs("", "", "");
    $("#inst-region").focus();
    return;
  }
  const picked = state.discoveredInstances.find((it) => `${it.region_id}:${it.instance_id}` === val);
  if (!picked) return;
  const autoName = picked.instance_name && !/^launch-advisor-/i.test(picked.instance_name)
    ? picked.instance_name
    : "";
  setInstanceMetaInputs(picked.region_id, picked.instance_id, autoName);
}

function onAddInstanceAccountChange(accountId) {
  state.discoveredInstances = [];
  setInstanceMetaInputs("", "", "");
  discoverInstancesForForm(accountId);
}

async function discoverInstancesForForm(accountId) {
  if (!accountId) return;
  const seq = ++state.discoverSeq;
  const refreshBtn = $("#btn-inst-discover-refresh");

  state.discoveredInstances = [];
  refreshBtn.disabled = true;
  refreshBtn.textContent = "获取中...";
  state.selects.instDiscover.setOptions(
    [{ value: "__loading__", label: "正在获取实例..." }],
    false,
  );

  try {
    const res = await apiFetch(`/api/accounts/${accountId}/discover`);
    if (seq !== state.discoverSeq || !$("#instance-dialog").open || $("#inst-id").value) {
      return;
    }

    const list = res.instances || [];
    state.discoveredInstances = list;
    const existingSet = new Set(
      (state.dashboard?.instances || [])
        .filter((x) => Number(x.account_id) === Number(accountId))
        .map((x) => `${x.region_id}:${x.instance_id}`),
    );

    if (!list.length) {
      state.selects.instDiscover.setOptions(
        [{ value: "__manual__", label: "未获取到实例（手动录入）" }],
        false,
      );
      return;
    }

    const opts = list.map((it) => {
      const key = `${it.region_id}:${it.instance_id}`;
      const exists = existingSet.has(key);
      const ip = (it.public_ips || [])[0] || (it.private_ips || [])[0] || "";
      return {
        value: key,
        label: it.instance_id,
        labelMono: true,
        meta: [it.region_id, ip].filter(Boolean).join(" · "),
        badge: exists ? "已纳管" : "",
        disabled: exists,
      };
    });
    opts.push({ value: "__manual__", label: "手动录入", meta: "自行填写地域与实例 ID" });

    state.selects.instDiscover.setOptions(opts, false);
    const firstAvail = opts.find((o) => !o.disabled && o.value !== "__manual__");
    const userTypedManual = Boolean($("#inst-region").value.trim() || $("#inst-aliyun-id").value.trim());

    if (firstAvail && !userTypedManual) {
      state.selects.instDiscover.setValue(firstAvail.value, true);
      applyDiscoveredInstanceSelection(firstAvail.value);
    } else {
      state.selects.instDiscover.setValue("__manual__", true);
    }
  } catch {
    if (seq !== state.discoverSeq || !$("#instance-dialog").open || $("#inst-id").value) {
      return;
    }
    state.selects.instDiscover.setOptions(
      [{ value: "__manual__", label: "未获取到实例（手动录入）" }],
      false,
    );
  } finally {
    if (seq === state.discoverSeq) {
      refreshBtn.disabled = false;
      refreshBtn.textContent = "刷新";
    }
  }
}

function openInstanceDialog(inst) {
  if (!state.accounts.length) {
    showToast("请先在「账号」中添加至少一个阿里云账号", "error");
    openAccountsDialog();
    return;
  }

  const form = $("#instance-form");
  form.reset();
  $("#instance-form-msg").hidden = true;
  state.discoveredInstances = [];

  state.selects.instAccount.setOptions(
    state.accounts.map((a) => ({
      value: String(a.id),
      label: a.name,
      sub: a.access_key_masked,
    })),
    false,
  );

  if (!inst) {
    $("#instance-dialog-title").textContent = "添加监控实例";
    $("#inst-id").value = "";
    $("#inst-discover-field").hidden = false;
    const defaultAccId = String(state.accounts[0].id);
    state.selects.instAccount.setValue(defaultAccId, true);
    $("#inst-threshold").value = "190";
    state.selects.instMode.setValue("full", true);
    $("#inst-sched-enable").checked = false;
    $("#inst-sched-start").value = "08:00";
    $("#inst-sched-stop").value = "00:00";
    state.selects.instStoppedMode.setValue("StopCharging", true);
    $("#inst-force-stop").checked = false;
    $("#inst-enabled").checked = true;

    syncScheduleUI();
    $("#instance-dialog").showModal();
    discoverInstancesForForm(Number(defaultAccId));
  } else {
    $("#instance-dialog-title").textContent = `编辑：${inst.name || inst.instance_id}`;
    $("#inst-id").value = inst.id;
    $("#inst-discover-field").hidden = true;
    state.discoverSeq++;
    state.selects.instAccount.setValue(inst.account_id ? String(inst.account_id) : String(state.accounts[0].id), true);
    $("#inst-region").value = inst.region_id;
    $("#inst-aliyun-id").value = inst.instance_id;
    $("#inst-name").value = inst.name;
    $("#inst-threshold").value = inst.traffic_threshold_gb;
    state.selects.instMode.setValue(inst.automation_mode || "full", true);
    $("#inst-sched-enable").checked = Boolean(inst.enable_schedule_control);
    $("#inst-sched-start").value = inst.schedule_start || "08:00";
    $("#inst-sched-stop").value = inst.schedule_stop || "00:00";
    state.selects.instStoppedMode.setValue(inst.stopped_mode || "StopCharging", true);
    $("#inst-force-stop").checked = Boolean(inst.force_stop);
    $("#inst-enabled").checked = Boolean(inst.enabled);

    syncScheduleUI();
    $("#instance-dialog").showModal();
  }
}

/* ==================== 弹窗：设置与通知 ==================== */
async function openSettingsDialog() {
  $("#settings-msg").hidden = true;
  const res = await apiFetch("/api/settings");
  state.settingsCache = res;

  state.selects.setTheme.setValue(getStoredTheme(), true);
  $("#set-interval").value = res.scheduler?.interval_seconds || 60;
  $("#set-sched-enabled").checked = Boolean(res.scheduler?.enabled);

  const n = res.notifications || {};
  $("#set-bark-enabled").checked = Boolean(n.bark_enabled);
  $("#set-bark-endpoint").value = "";
  $("#set-bark-hint").textContent = n.has_bark ? `已配置：${n.bark_masked}` : "未配置";
  $("#set-bark-group").value = n.bark_group || "DevOps";
  $("#set-bark-sound").value = n.bark_sound || "";

  $("#set-webhook-enabled").checked = Boolean(n.webhook_enabled);
  $("#set-webhook-url").value = "";
  $("#set-webhook-hint").textContent = n.has_webhook ? `已配置：${n.webhook_masked}` : "未配置";

  $("#ev-start").checked = Boolean(n.notify_on_start);
  $("#ev-stop").checked = Boolean(n.notify_on_stop);
  $("#ev-fuse").checked = Boolean(n.notify_on_fuse);
  $("#ev-error").checked = Boolean(n.notify_on_error);

  $("#settings-dialog").showModal();
}

async function saveSettingsFromForm() {
  applyThemeOverride(state.selects.setTheme.getValue());

  const interval = Math.max(10, Number($("#set-interval").value || 60));
  const schedEnabled = $("#set-sched-enabled").checked;
  await apiFetch("/api/settings/scheduler", {
    method: "PUT",
    body: JSON.stringify({ interval_seconds: interval, enabled: schedEnabled }),
  });

  const cachedNotify = state.settingsCache?.notifications || {};
  const notifyPayload = {
    bark_enabled: $("#set-bark-enabled").checked,
    bark_endpoint: $("#set-bark-endpoint").value.trim(),
    has_bark: Boolean(cachedNotify.has_bark),
    bark_group: $("#set-bark-group").value.trim() || "DevOps",
    bark_sound: $("#set-bark-sound").value.trim(),
    webhook_enabled: $("#set-webhook-enabled").checked,
    webhook_url: $("#set-webhook-url").value.trim(),
    has_webhook: Boolean(cachedNotify.has_webhook),
    notify_on_start: $("#ev-start").checked,
    notify_on_stop: $("#ev-stop").checked,
    notify_on_warn: false,
    notify_on_fuse: $("#ev-fuse").checked,
    notify_on_error: $("#ev-error").checked,
  };
  const saved = await apiFetch("/api/settings/notifications", {
    method: "PUT",
    body: JSON.stringify(notifyPayload),
  });
  state.settingsCache.notifications = saved.notifications;
}

/* ==================== 弹窗：全局日志 ==================== */
async function openGlobalLogsDialog() {
  $("#global-logs-dialog").showModal();
  await refreshGlobalLogs();
}

async function refreshGlobalLogs() {
  const box = $("#global-logs-list");
  const clearBtn = $("#btn-clear-glogs");
  try {
    const res = await apiFetch(`/api/logs?level=${encodeURIComponent(state.globalLogLevel)}`);
    const items = state.globalLogTab === "realtime"
      ? filterLogsByLevel(res.realtime || [], state.globalLogLevel)
      : (res.audit || []);
    syncLogClearButton(clearBtn, items);
    renderLogItems(box, items);
  } catch (err) {
    syncLogClearButton(clearBtn, []);
    box.innerHTML = `<div class="muted-text log-empty">加载失败：${esc(err.message)}</div>`;
  }
}

/* ==================== 事件绑定与启动 ==================== */
function bindEvents() {
  $("#toggle-login-password").addEventListener("click", (e) => {
    const pw = $("#login-password");
    const btn = e.currentTarget;
    const show = pw.type === "password";
    pw.type = show ? "text" : "password";
    btn.setAttribute("aria-pressed", String(show));
    btn.setAttribute("aria-label", show ? "隐藏密码" : "显示密码");
    $("#eye-icon-show").hidden = show;
    $("#eye-icon-hide").hidden = !show;
    $("#eye-toggle-text").textContent = show ? "隐藏" : "显示";
  });

  $("#login-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const form = e.currentTarget;
    const errBox = $("#login-server-error");
    errBox.hidden = true;
    if (!form.reportValidity()) {
      form.querySelector(":invalid")?.focus();
      return;
    }
    const btn = $("#login-submit-btn");
    btn.disabled = true;
    try {
      const res = await apiFetch("/api/auth/login", {
        method: "POST",
        body: JSON.stringify({
          username: $("#login-username").value.trim(),
          password: $("#login-password").value,
        }),
      });
      await setLoggedIn(res.username);
    } catch (err) {
      errBox.textContent = err.message;
      errBox.hidden = false;
    } finally {
      btn.disabled = false;
    }
  });

  $("#brand-home-btn").addEventListener("click", () => showDashboardView(true));
  $("#btn-back-dashboard").addEventListener("click", () => showDashboardView(true));
  $("#btn-logout").addEventListener("click", async () => {
    await apiFetch("/api/auth/logout", { method: "POST" }).catch(() => {});
    setLoggedOut();
  });
  $("#btn-trigger-now").addEventListener("click", async (e) => {
    const btn = e.currentTarget;
    btn.disabled = true;
    btn.classList.add("is-loading");
    btn.title = "巡检中...";
    btn.setAttribute("aria-label", "巡检中");
    try {
      await apiFetch("/api/scheduler/trigger?sync=1", { method: "POST" });
      await refreshAll();
      showToast("巡检已完成", "success");
    } catch (err) {
      showToast(err.message, "error");
    } finally {
      btn.disabled = false;
      btn.classList.remove("is-loading");
      btn.title = "立即巡检";
      btn.setAttribute("aria-label", "立即巡检");
    }
  });
  $("#btn-open-accounts").addEventListener("click", openAccountsDialog);
  $("#btn-open-settings").addEventListener("click", openSettingsDialog);
  $("#btn-open-global-logs").addEventListener("click", openGlobalLogsDialog);
  $("#btn-add-instance").addEventListener("click", () => openInstanceDialog(null));
  $("#btn-inst-discover-refresh").addEventListener("click", () => {
    const accId = Number(state.selects.instAccount.getValue());
    if (accId) discoverInstancesForForm(accId);
  });
  const syncManualSelection = () => {
    if ($("#inst-id").value || !state.discoveredInstances.length) return;
    const key = `${$("#inst-region").value.trim()}:${$("#inst-aliyun-id").value.trim()}`;
    if (key !== state.selects.instDiscover.getValue()) {
      state.selects.instDiscover.setValue("__manual__", true);
    }
  };
  $("#inst-region").addEventListener("input", syncManualSelection);
  $("#inst-aliyun-id").addEventListener("input", syncManualSelection);

  $("#btn-account-reset").addEventListener("click", resetAccountForm);
  $("#btn-test-account-draft").addEventListener("click", async () => {
    const msgBox = $("#account-form-msg");
    msgBox.hidden = true;
    try {
      const res = await apiFetch("/api/credentials/test", {
        method: "POST",
        body: JSON.stringify({
          access_key_id: $("#acc-ak-id").value.trim(),
          access_key_secret: $("#acc-ak-secret").value.trim(),
        }),
      });
      msgBox.className = "alert alert-success";
      msgBox.textContent = `连通成功 · 境内 ${fmtGB(res.domestic_gb)} GB / 境外 ${fmtGB(res.overseas_gb)} GB`;
      msgBox.hidden = false;
    } catch (err) {
      msgBox.className = "alert alert-danger";
      msgBox.textContent = err.message;
      msgBox.hidden = false;
    }
  });
  $("#account-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    if (!e.currentTarget.reportValidity()) {
      e.currentTarget.querySelector(":invalid")?.focus();
      return;
    }
    const id = $("#acc-id").value;
    const msgBox = $("#account-form-msg");
    const submitBtn = e.currentTarget.querySelector('button[type="submit"]');
    msgBox.hidden = true;
    submitBtn.disabled = true;
    try {
      const payload = {
        name: $("#acc-name").value.trim(),
        access_key_id: $("#acc-ak-id").value.trim(),
        access_key_secret: $("#acc-ak-secret").value.trim(),
        domestic_quota_gb: Number($("#acc-dom-quota").value || 20),
        overseas_quota_gb: Number($("#acc-ovs-quota").value || 180),
        enabled: $("#acc-enabled").checked,
      };
      if (id) {
        await apiFetch(`/api/accounts/${id}`, { method: "PUT", body: JSON.stringify(payload) });
        showToast("账号已更新并完成巡检", "success");
      } else {
        await apiFetch("/api/accounts", { method: "POST", body: JSON.stringify(payload) });
        showToast("账号已添加并完成巡检", "success");
      }
      resetAccountForm();
      await reloadAccountsInDialog();
      await refreshAll();
    } catch (err) {
      msgBox.className = "alert alert-danger";
      msgBox.textContent = err.message;
      msgBox.hidden = false;
    } finally {
      submitBtn.disabled = false;
    }
  });

  $("#inst-sched-enable").addEventListener("change", syncScheduleUI);
  $("#btn-test-instance-cred").addEventListener("click", async () => {
    const msgBox = $("#instance-form-msg");
    msgBox.hidden = true;
    try {
      const res = await apiFetch("/api/credentials/test", {
        method: "POST",
        body: JSON.stringify({
          account_id: Number(state.selects.instAccount.getValue() || 0),
          region_id: $("#inst-region").value.trim(),
          instance_id: $("#inst-aliyun-id").value.trim(),
        }),
      });
      if (res.instance && !$("#inst-name").value.trim()) {
        $("#inst-name").value = res.instance.instance_name || res.instance.instance_id;
      }
      msgBox.className = "alert alert-success";
      msgBox.textContent = res.instance
        ? `验证成功 · ${res.instance.instance_name} (${res.instance.status}) · 境内 ${fmtGB(res.domestic_gb)}G / 境外 ${fmtGB(res.overseas_gb)}G`
        : `凭证有效 · 境内 ${fmtGB(res.domestic_gb)}G / 境外 ${fmtGB(res.overseas_gb)}G`;
      msgBox.hidden = false;
    } catch (err) {
      msgBox.className = "alert alert-danger";
      msgBox.textContent = err.message;
      msgBox.hidden = false;
    }
  });
  $("#instance-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    if (!e.currentTarget.reportValidity()) {
      e.currentTarget.querySelector(":invalid")?.focus();
      return;
    }
    const id = $("#inst-id").value;
    const msgBox = $("#instance-form-msg");
    const submitBtn = e.currentTarget.querySelector('button[type="submit"]');
    msgBox.hidden = true;
    submitBtn.disabled = true;

    const payload = {
      name: $("#inst-name").value.trim(),
      account_id: Number(state.selects.instAccount.getValue()),
      region_id: $("#inst-region").value.trim(),
      instance_id: $("#inst-aliyun-id").value.trim(),
      traffic_threshold_gb: Number($("#inst-threshold").value || 0),
      warn_percent: 0,
      automation_mode: state.selects.instMode.getValue(),
      enable_schedule_control: $("#inst-sched-enable").checked,
      schedule_start: $("#inst-sched-start").value.trim(),
      schedule_stop: $("#inst-sched-stop").value.trim(),
      stopped_mode: state.selects.instStoppedMode.getValue(),
      force_stop: $("#inst-force-stop").checked,
      enabled: $("#inst-enabled").checked,
    };

    try {
      if (id) {
        await apiFetch(`/api/instances/${id}`, { method: "PUT", body: JSON.stringify(payload) });
        showToast("实例已更新并完成巡检", "success");
      } else {
        await apiFetch("/api/instances", { method: "POST", body: JSON.stringify(payload) });
        showToast("实例已添加并完成巡检", "success");
      }
      $("#instance-dialog").close("saved");
      await refreshAll();
    } catch (err) {
      msgBox.className = "alert alert-danger";
      msgBox.textContent = err.message;
      msgBox.hidden = false;
    } finally {
      submitBtn.disabled = false;
    }
  });

  $("#settings-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const msg = $("#settings-msg");
    msg.hidden = true;
    try {
      await saveSettingsFromForm();
      showToast("设置已保存", "success");
      $("#settings-dialog").close("saved");
      await refreshAll();
    } catch (err) {
      msg.className = "alert alert-danger";
      msg.textContent = err.message;
      msg.hidden = false;
    }
  });

  $("#btn-test-notify").addEventListener("click", async () => {
    const msg = $("#settings-msg");
    msg.hidden = true;
    try {
      await saveSettingsFromForm();
      await apiFetch("/api/settings/notifications/test", { method: "POST" });
      msg.className = "alert alert-success";
      msg.textContent = "测试通知已发送";
      msg.hidden = false;
    } catch (err) {
      msg.className = "alert alert-danger";
      msg.textContent = err.message;
      msg.hidden = false;
    }
  });

  for (const tab of $$("[data-log-tab]")) {
    tab.addEventListener("click", () => {
      state.detailLogTab = tab.dataset.logTab;
      for (const t of $$("[data-log-tab]")) {
        const active = t === tab;
        t.classList.toggle("is-active", active);
        t.setAttribute("aria-selected", String(active));
      }
      if (state.activeInstanceId) refreshInstanceLogs(state.activeInstanceId);
    });
  }
  $("#btn-refresh-inst-logs").addEventListener("click", () => {
    if (state.activeInstanceId) refreshInstanceLogs(state.activeInstanceId);
  });
  $("#btn-clear-inst-logs").addEventListener("click", () => {
    if (state.activeInstanceId) clearInstanceLogs(state.activeInstanceId);
  });

  for (const tab of $$("[data-glog-tab]")) {
    tab.addEventListener("click", () => {
      state.globalLogTab = tab.dataset.glogTab;
      for (const t of $$("[data-glog-tab]")) {
        const active = t === tab;
        t.classList.toggle("is-active", active);
        t.setAttribute("aria-selected", String(active));
      }
      refreshGlobalLogs();
    });
  }
  $("#btn-refresh-glogs").addEventListener("click", refreshGlobalLogs);
  $("#btn-clear-glogs").addEventListener("click", clearGlobalLogs);

  window.addEventListener("hashchange", handleHashRoute);
}

async function boot() {
  setupDialogs();
  initCustomSelects();
  initFormValidation();
  bindEvents();

  setInterval(() => {
    if (state.authenticated && document.visibilityState === "visible") {
      refreshAll().catch(() => {});
    }
  }, 15_000);

  try {
    const me = await apiFetch("/api/auth/me");
    if (me?.authenticated) {
      await setLoggedIn(me.username);
      return;
    }
  } catch {
    // 未登录
  }
  setLoggedOut();
}

boot();
