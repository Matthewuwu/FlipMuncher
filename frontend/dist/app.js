// Plain JS, no bundler: calls the Wails-generated globals directly.
const App = window.go.main.App;
const wr = window.runtime;
const $ = (id) => document.getElementById(id);

const els = {
  app: document.querySelector(".app"),
  urls: $("urls"), fieldBox: $("fieldBox"),
  linkHint: $("linkHint"), destBtn: $("destBtn"), destText: $("destText"),
  advancedLink: $("advancedLink"),
  previewList: $("previewList"), previewActions: $("previewActions"), addMoreBtn: $("addMoreBtn"),
  addBtn: $("addBtn"), addLabel: $("addLabel"), emptyQueue: $("emptyQueue"),
  settingsLayer: $("settingsLayer"), closeSettingsBtn: $("closeSettingsBtn"),
  outputDir: $("outputDir"),
  threads: $("threads"), retries: $("retries"), retryDelay: $("retryDelay"), chunkSize: $("chunkSize"),
  keepImages: $("keepImages"), compressImages: $("compressImages"),
  jpegQuality: $("jpegQuality"), jpegQualityVal: $("jpegQualityVal"), qualityRow: $("qualityRow"), sizeEst: $("sizeEst"), estHead: $("estHead"), estRows: $("estRows"), estNote: $("estNote"),
  saveConfigBtn: $("saveConfigBtn"), saveMsg: $("saveMsg"),
  queueSection: $("queueSection"), queue: $("queue"), counts: $("counts"),
  statusDot: $("statusDot"), statusText: $("statusText"),
  statQueue: $("statQueue"), statDone: $("statDone"), statSize: $("statSize"),
  themeBtn: $("themeBtn"), langBtn: $("langBtn"), langMenu: $("langMenu"),
  aboutBtn: $("aboutBtn"), aboutModal: $("aboutModal"), closeAboutBtn: $("closeAboutBtn"),
  aboutVersion: $("aboutVersion"), aboutLicense: $("aboutLicense"), aboutNotice: $("aboutNotice"),
  licenseText: $("licenseText"), aboutUpstreamLink: $("aboutUpstreamLink"), coffeeLink: $("coffeeLink"),
};

let lastQueueItems = [];
let savedSnapshot = "";

function openExternal(e, url) { e.preventDefault(); wr.BrowserOpenURL(url); }

// ---------- config ----------
async function loadConfig() {
  const cfg = await App.GetConfig();
  els.outputDir.value = cfg.outputDir;
  els.threads.value = cfg.threads;
  els.retries.value = cfg.retries;
  els.retryDelay.value = cfg.retryDelaySeconds;
  els.chunkSize.value = cfg.chunkSize;
  els.keepImages.checked = cfg.keepImages;
  els.compressImages.checked = cfg.compressImages;
  els.jpegQuality.value = cfg.jpegQuality;
  savedSnapshot = JSON.stringify(readConfigFromForm());
  refreshSettingsUI();
  updateDestText();
}

function readConfigFromForm() {
  return {
    outputDir: els.outputDir.value,
    threads: parseInt(els.threads.value, 10) || 1,
    retries: parseInt(els.retries.value, 10) || 0,
    retryDelaySeconds: parseFloat(els.retryDelay.value) || 0,
    chunkSize: parseInt(els.chunkSize.value, 10) || 10,
    keepImages: els.keepImages.checked,
    compressImages: els.compressImages.checked,
    jpegQuality: parseInt(els.jpegQuality.value, 10) || 60,
  };
}

function refreshSettingsUI(justSaved) {
  els.jpegQualityVal.textContent = els.jpegQuality.value;
  els.jpegQuality.disabled = !els.compressImages.checked;
  els.qualityRow.classList.toggle("off", !els.compressImages.checked);
  renderSizeEstimate();

  const dirty = JSON.stringify(readConfigFromForm()) !== savedSnapshot;
  els.saveMsg.className = "save-msg" + (justSaved ? " saved" : dirty ? " dirty" : "");
  els.saveMsg.textContent = justSaved ? t("saved") : dirty ? t("unsaved") : t("applies");
}

function updateDestText() {
  const short = els.outputDir.value.split(/[\\/]/).filter(Boolean).slice(-2).join("\\");
  els.destText.textContent = t("savesTo", { dir: short });
}

async function saveConfig() {
  await App.SaveConfig(readConfigFromForm());
  savedSnapshot = JSON.stringify(readConfigFromForm());
  refreshSettingsUI(true);
}

els.settingsLayer.addEventListener("input", () => refreshSettingsUI());
els.settingsLayer.addEventListener("change", () => refreshSettingsUI());

els.destBtn.addEventListener("click", async () => {
  const dir = await App.PickOutputFolder();
  if (dir) {
    els.outputDir.value = dir;
    await saveConfig();
    updateDestText();
  }
});

// ---------- overlays ----------
// Opening a layer remembers the opener and moves focus in; closing restores it.
let lastOpener = null;
function showLayer(layer, focusSel) {
  if (layer.classList.contains("hidden")) lastOpener = document.activeElement;
  layer.classList.remove("hidden");
  const f = layer.querySelector(focusSel);
  if (f) f.focus();
}
function hideLayer(layer) {
  if (layer.classList.contains("hidden")) return;
  layer.classList.add("hidden");
  const o = lastOpener;
  lastOpener = null;
  if (o && o.focus && document.contains(o)) o.focus();
}
function openSettings() {
  hideLayer(els.aboutModal);
  showLayer(els.settingsLayer, ".sheet-head button");
  scheduleMeasure(); // size estimate only runs while Advanced is open
}
function closeSettings() { hideLayer(els.settingsLayer); }
function closeAbout() { hideLayer(els.aboutModal); }
els.advancedLink.addEventListener("click", openSettings);
els.saveConfigBtn.addEventListener("click", saveConfig);
els.closeSettingsBtn.addEventListener("click", closeSettings);
els.settingsLayer.addEventListener("click", (e) => { if (e.target === els.settingsLayer) closeSettings(); });

els.aboutBtn.addEventListener("click", async () => {
  const info = await App.GetAboutInfo();
  els.aboutVersion.textContent = "v" + info.version;
  els.aboutLicense.textContent = info.license;
  els.aboutNotice.textContent = info.upstreamNotice;
  els.licenseText.textContent = info.licenseText;
  els.aboutUpstreamLink.href = info.upstreamUrl;
  els.aboutUpstreamLink.onclick = (e) => openExternal(e, info.upstreamUrl);
  els.coffeeLink.onclick = (e) => openExternal(e, els.coffeeLink.href);
  closeSettings();
  showLayer(els.aboutModal, "#closeAboutBtn");
});
els.closeAboutBtn.addEventListener("click", closeAbout);
els.aboutModal.addEventListener("click", (e) => { if (e.target === els.aboutModal) closeAbout(); });
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape") { closeSettings(); closeAbout(); }
  // Keep Tab inside the open sheet/dialog (the page behind stays reachable otherwise).
  if (e.key === "Tab") {
    const layer = [els.aboutModal, els.settingsLayer].find((l) => !l.classList.contains("hidden"));
    if (layer) {
      const f = Array.prototype.filter.call(layer.querySelectorAll("button, input, a[href], textarea"), (n) => !n.disabled && n.offsetParent !== null);
      if (f.length) {
        const i = f.indexOf(document.activeElement);
        if (i === -1 || (e.shiftKey && i === 0) || (!e.shiftKey && i === f.length - 1)) {
          e.preventDefault();
          f[i === -1 || !e.shiftKey ? 0 : f.length - 1].focus();
        }
      }
    }
  }
  // Ctrl+Enter (Cmd+Enter on macOS) toggles Advanced settings from anywhere,
  // including while typing in the link box.
  if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
    e.preventDefault();
    if (els.settingsLayer.classList.contains("hidden")) openSettings(); else closeSettings();
  }
});

// ---------- preview cards ----------
let pendingPreviews = [];
let previewIdSeq = 0;
let previewRunning = 0;
const previewQueue = [];
const MAX_CONCURRENT_PREVIEWS = 3;

function schedulePreview(entry) {
  previewQueue.push(entry);
  pumpPreviewQueue();
}

function pumpPreviewQueue() {
  while (previewRunning < MAX_CONCURRENT_PREVIEWS && previewQueue.length > 0) {
    const entry = previewQueue.shift();
    previewRunning++;
    App.PreviewBook(entry.url)
      .then((result) => {
        entry.status = "ready";
        entry.title = result.title;
        entry.pageCount = result.pageCount;
        entry.coverDataUrl = result.coverDataUrl;
      })
      .catch((err) => {
        entry.status = "error";
        entry.error = String(err && err.message ? err.message : err);
      })
      .finally(() => {
        previewRunning--;
        renderPreviewList();
        pumpPreviewQueue();
      });
  }
}

// Links are committed on paste, on Enter (a single typed link) and when the
// box loses focus. Never on a typing pause: a half-typed URL would be queued
// and the box emptied under the user's hands.
els.urls.addEventListener("paste", () => {
  // The pasted text is in the box only after the event completes.
  setTimeout(commitPastedUrls, 0);
});
els.urls.addEventListener("keydown", (e) => {
  if (e.key !== "Enter" || e.ctrlKey || e.metaKey || e.shiftKey || e.isComposing) return;
  const text = els.urls.value.trim();
  if (text && !text.includes("\n")) {
    e.preventDefault();
    commitPastedUrls();
  }
});
els.urls.addEventListener("blur", () => commitPastedUrls());

function commitPastedUrls() {
  const lines = els.urls.value.split("\n").map((u) => u.trim()).filter(Boolean);
  if (lines.length === 0) return;
  for (const url of lines) {
    const entry = {
      id: String(previewIdSeq++),
      url,
      status: "loading",
      title: "",
      pageCount: 0,
      coverDataUrl: "",
      error: "",
      buildPDF: true,
      extractText: false,
    };
    pendingPreviews.push(entry);
    schedulePreview(entry);
  }
  els.urls.value = "";
  els.fieldBox.classList.add("hidden");
  renderPreviewList();
}

els.addMoreBtn.addEventListener("click", () => {
  els.fieldBox.classList.remove("hidden");
  els.urls.focus();
});

function previewCardNode(entry) {
  const li = el("li", "preview-card state-" + entry.status);
  li.dataset.id = entry.id;

  const cover = el("div", "preview-cover");
  if (entry.status === "loading") {
    cover.innerHTML = '<span class="spinner"></span>';
  } else if (entry.status === "error") {
    cover.innerHTML = ICON.warn;
  } else {
    const img = document.createElement("img");
    img.src = entry.coverDataUrl;
    img.alt = "";
    cover.append(img);
  }
  li.append(cover);

  const body = el("div", "preview-body");
  if (entry.status === "error") {
    body.append(textNode("preview-error", entry.error || t("loadFail")));
  } else {
    const title = document.createElement("input");
    title.className = "preview-title input";
    title.type = "text";
    title.dataset.fk = "title";
    title.setAttribute("aria-label", t("bookTitle"));
    title.disabled = entry.status !== "ready";
    title.value = entry.title || "";
    title.placeholder = entry.status === "loading" ? t("loading") : "";
    title.addEventListener("input", () => { entry.title = title.value; });
    body.append(title);
    body.append(urlNode(entry.url));

    if (entry.status === "ready") {
      const toggles = el("div", "preview-toggles");
      const pdfBtn = el("button", "toggle" + (entry.buildPDF ? " on" : ""), t("pdf"));
      const textBtn = el("button", "toggle" + (entry.extractText ? " on" : ""), t("text"));
      pdfBtn.dataset.fk = "pdf"; textBtn.dataset.fk = "text";
      pdfBtn.setAttribute("aria-pressed", String(entry.buildPDF));
      textBtn.setAttribute("aria-pressed", String(entry.extractText));
      pdfBtn.type = "button";
      textBtn.type = "button";
      pdfBtn.addEventListener("click", () => {
        entry.buildPDF = !entry.buildPDF;
        if (!entry.buildPDF && !entry.extractText) entry.buildPDF = true;
        renderPreviewList();
      });
      textBtn.addEventListener("click", () => {
        entry.extractText = !entry.extractText;
        if (!entry.buildPDF && !entry.extractText) entry.buildPDF = true;
        renderPreviewList();
      });
      toggles.append(pdfBtn, textBtn);
      body.append(toggles);
    }
  }
  li.append(body);

  const remove = el("button", "icon-btn preview-remove", ICON.x);
  remove.title = t("remove");
  remove.setAttribute("aria-label", t("removeBook"));
  remove.addEventListener("click", () => {
    pendingPreviews = pendingPreviews.filter((e) => e.id !== entry.id);
    renderPreviewList();
  });
  li.append(remove);

  return li;
}

function renderPreviewList() {
  const active = document.activeElement;
  const activeCard = active && active.closest && active.closest(".preview-card");
  const activeId = activeCard ? activeCard.dataset.id : null;
  const activeKey = activeId != null && active.dataset ? active.dataset.fk : null;
  const selStart = active && "selectionStart" in active ? active.selectionStart : null;
  const selEnd = active && "selectionEnd" in active ? active.selectionEnd : null;

  els.previewList.innerHTML = "";
  const has = pendingPreviews.length > 0;
  els.previewList.classList.toggle("hidden", !has);
  els.previewActions.classList.toggle("hidden", !has);
  els.app.classList.toggle("has-previews", has);
  if (!has) els.fieldBox.classList.remove("hidden");

  for (const entry of pendingPreviews) {
    els.previewList.append(previewCardNode(entry));
  }

  renderSizeEstimate();
  const readyCount = pendingPreviews.filter((e) => e.status === "ready").length;
  els.addBtn.disabled = readyCount === 0;
  els.addLabel.textContent = t(readyCount === 1 ? "addOne" : "addMany", { n: readyCount });

  if (activeKey) {
    const node = els.previewList.querySelector(`[data-id="${activeId}"] [data-fk="${activeKey}"]`);
    if (node) {
      node.focus();
      if (activeKey === "title" && selStart != null) node.setSelectionRange(selStart, selEnd);
    }
  }
}

async function addReadyBooks() {
  const ready = pendingPreviews.filter((e) => e.status === "ready");
  if (ready.length === 0) return;
  const items = ready.map((e) => ({
    url: e.url,
    title: e.title.trim(),
    buildPDF: e.buildPDF,
    extractText: e.extractText,
  }));
  App.StartQueue(items);
  pendingPreviews = pendingPreviews.filter((e) => e.status !== "ready");
  renderPreviewList();
  // Always reveal the input area after adding, even if an errored card is
  // left behind — otherwise it (and #advancedLink inside it) stays
  // unreachable forever since only "ready" entries ever get filtered out here.
  els.fieldBox.classList.remove("hidden");
}
els.addBtn.addEventListener("click", addReadyBooks);

// ---------- queue ----------
const ICON = {
  check: '<svg width="12" height="12" viewBox="0 0 256 256" fill="currentColor"><path d="M229.66,77.66l-128,128a8,8,0,0,1-11.32,0l-56-56a8,8,0,0,1,11.32-11.32L96,188.69,218.34,66.34a8,8,0,0,1,11.32,11.32Z"/></svg>',
  warn: '<svg width="20" height="20" viewBox="0 0 256 256" fill="currentColor"><path d="M128,24A104,104,0,1,0,232,128,104.11,104.11,0,0,0,128,24Zm0,192a88,88,0,1,1,88-88A88.1,88.1,0,0,1,128,216Zm-8-80V80a8,8,0,0,1,16,0v56a8,8,0,0,1-16,0Zm20,36a12,12,0,1,1-12-12A12,12,0,0,1,140,172Z"/></svg>',
  x: '<svg width="14" height="14" viewBox="0 0 256 256" fill="currentColor"><path d="M205.66,194.34a8,8,0,0,1-11.32,11.32L128,139.31,61.66,205.66a8,8,0,0,1-11.32-11.32L116.69,128,50.34,61.66A8,8,0,0,1,61.66,50.34L128,116.69l66.34-66.35a8,8,0,0,1,11.32,11.32L139.31,128Z"/></svg>',
  stop: '<svg width="11" height="11" viewBox="0 0 256 256" fill="currentColor"><path d="M200,40H56A16,16,0,0,0,40,56V200a16,16,0,0,0,16,16H200a16,16,0,0,0,16-16V56A16,16,0,0,0,200,40Zm0,160H56V56H200V200Z"/></svg>',
  folder: '<svg width="13" height="13" viewBox="0 0 256 256" fill="currentColor"><path d="M216,72H131.31L104,44.69A15.86,15.86,0,0,0,92.69,40H40A16,16,0,0,0,24,56V200.62A15.4,15.4,0,0,0,39.38,216H216.89A15.13,15.13,0,0,0,232,200.89V88A16,16,0,0,0,216,72ZM40,56H92.69l16,16H40ZM216,200H40V88H216Z"/></svg>',
};
const phaseLabel = (phase) => (phase === "converting" ? t("phaseConverting") : phase === "ocr" ? t("phaseOcr") : t("phaseDownloading"));

function el(tag, cls, html) {
  const n = document.createElement(tag);
  if (cls) n.className = cls;
  if (html != null) n.innerHTML = html;
  return n;
}
function dirOf(path) {
  const idx = Math.max(path.lastIndexOf("/"), path.lastIndexOf("\\"));
  return idx !== -1 ? path.substring(0, idx) : path;
}
function urlNode(url) {
  const d = el("div", "url");
  d.title = url;
  const m = url.match(/^(?:https?:\/\/)?([^/]+\/)(.*)$/);
  const host = el("span", "host"); host.textContent = m ? m[1] : "";
  const rest = el("span"); rest.textContent = m ? m[2] : url;
  d.append(host, rest);
  return d;
}
function textNode(cls, text) { const n = el("div", cls); n.textContent = text; return n; }
function openBtn(label, path) {
  const b = el("button", "btn btn-secondary btn-sm", ICON.folder);
  b.append(label);
  b.title = t("openFolder");
  b.addEventListener("click", () => App.OpenFolder(dirOf(path)));
  return b;
}

// Progress ticks arrive for every page. Rebuilding the list on each one
// replaces the Stop button under the cursor, so a click that straddles a
// rebuild is lost and keyboard focus resets. When nothing but the active
// book's progress changed, update that row in place instead.
let queueShape = "";

function queueShapeOf(items) {
  return items.map((i) => [i.id, i.status, i.phase, i.total > 0, i.error, i.pdfPath, i.textPath].join("|")).join(";");
}

function updateActiveInPlace(item) {
  const li = els.queue.querySelector(".item.active");
  if (!li) return false;
  const pct = item.total > 0 ? Math.round((item.done / item.total) * 100) : 0;
  const phase = phaseLabel(item.phase);
  const sub = li.querySelector(".sub");
  const counter = li.querySelector(".counter");
  const bar = li.querySelector(".bar");
  if (!sub || !bar || (item.total > 0 && !counter)) return false;
  sub.textContent = item.total > 0 ? `${phase} \u00b7 ${pct}%` : t("starting");
  if (counter) counter.innerHTML = `${item.done}<small> / ${item.total}</small>`;
  bar.setAttribute("aria-valuemax", String(item.total || 1));
  bar.setAttribute("aria-valuenow", String(item.done || 0));
  bar.firstChild.style.width = pct + "%";
  return true;
}

function renderQueue(items) {
  const lastItems = items || [];
  lastQueueItems = lastItems;
  const shape = queueShapeOf(lastItems);
  if (shape === queueShape) {
    const active = lastItems.find((i) => i.status === "downloading");
    if (active && updateActiveInPlace(active)) return;
  }
  queueShape = shape;
  els.queue.innerHTML = "";
  const has = lastItems.length > 0;
  els.queueSection.classList.toggle("hidden", !has);

  let pos = 0;
  const c = { waiting: 0, downloading: 0, done: 0, error: 0 };

  for (const item of lastItems) {
    if (c[item.status] != null) c[item.status]++;
    const li = el("li", "item " + item.status);
    const main = el("div");
    main.style.minWidth = "0";
    main.append(urlNode(item.url));

    if (item.status === "downloading") {
      li.className = "item active";
      const pct = item.total > 0 ? Math.round((item.done / item.total) * 100) : 0;
      const phase = phaseLabel(item.phase);
      main.append(textNode("sub", item.total > 0 ? `${phase} · ${pct}%` : t("starting")));
      const row = el("div", "active-row");
      const glyph = el("span", "glyph", '<span class="spinner"></span>');
      const right = el("div", "counter-wrap");
      if (item.total > 0) right.append(el("div", "counter", `${item.done}<small> / ${item.total}</small>`));
      const stop = el("button", "btn btn-secondary btn-sm", ICON.stop);
      stop.append(t("stop"));
      stop.addEventListener("click", () => App.CancelCurrent());
      right.append(stop);
      row.append(glyph, main, right);
      const bar = el("div", "bar");
      bar.setAttribute("role", "progressbar");
      bar.setAttribute("aria-label", t("progress"));
      bar.setAttribute("aria-valuemin", "0");
      bar.setAttribute("aria-valuemax", String(item.total || 1));
      bar.setAttribute("aria-valuenow", String(item.done || 0));
      const fill = el("span"); fill.style.width = pct + "%";
      bar.append(fill);
      li.append(row, bar);
    } else if (item.status === "done") {
      main.append(textNode("sub", item.total ? t("donePages", { n: item.total }) : t("doneBook")));
      const actions = el("div", "actions");
      if (item.pdfPath) actions.append(openBtn(t("openPdf"), item.pdfPath));
      if (item.textPath) actions.append(openBtn(t("openText"), item.textPath));
      li.append(el("span", "glyph", ICON.check), main, actions);
    } else if (item.status === "error") {
      main.append(textNode("sub", item.error || t("error")));
      li.append(el("span", "glyph", ICON.warn), main);
    } else if (item.status === "cancelled") {
      main.append(textNode("sub", t("cancelled")));
      li.append(el("span", "glyph", ICON.x), main);
      li.style.gridTemplateColumns = "28px minmax(0,1fr)";
    } else {
      pos++;
      main.append(textNode("sub", t("waiting")));
      li.append(el("span", "glyph", String(pos)), main);
      li.style.gridTemplateColumns = "28px minmax(0,1fr)";
    }
    els.queue.append(li);
  }

  els.counts.textContent = [
    c.downloading && t("cActive", { n: c.downloading }), c.waiting && t("cWaiting", { n: c.waiting }),
    c.done && t("cDone", { n: c.done }), c.error && t("cFailed", { n: c.error }),
  ].filter(Boolean).join(" · ");

  const busy = c.downloading > 0;
  els.statusDot.classList.toggle("busy", busy);
  els.statusText.textContent = busy ? t("stWorking", { n: c.waiting }) : has ? t("stIdle") : t("stReady");
  els.statQueue.textContent = c.downloading + c.waiting;
  els.statDone.textContent = c.done;
}

wr.EventsOn("queue-updated", renderQueue);

// ---------- theme + language ----------
function setTheme(theme) {
  document.documentElement.dataset.theme = theme;
  try { localStorage.setItem("fm-theme", theme); } catch (_) {}
}
let savedTheme = "light";
try { savedTheme = localStorage.getItem("fm-theme") === "dark" ? "dark" : "light"; } catch (_) {}
setTheme(savedTheme);
els.themeBtn.addEventListener("click", () => setTheme(document.documentElement.dataset.theme === "dark" ? "light" : "dark"));

function applyLang() {
  applyStaticI18n();
  els.langBtn.textContent = lang.toUpperCase();
  els.langMenu.replaceChildren(...Object.keys(LANGS).map((code) => {
    const b = el("button", "");
    b.type = "button";
    b.setAttribute("role", "menuitemradio");
    b.setAttribute("aria-checked", String(code === lang));
    b.textContent = LANGS[code];
    b.addEventListener("click", () => {
      lang = code;
      try { localStorage.setItem("fm-lang", code); } catch (_) {}
      closeLangMenu();
      applyLang();
    });
    return b;
  }));
  queueShape = "";
  renderQueue(lastQueueItems);
  renderPreviewList();
  refreshSettingsUI();
  updateDestText();
}
function closeLangMenu() {
  els.langMenu.classList.add("hidden");
  els.langBtn.setAttribute("aria-expanded", "false");
}
els.langBtn.addEventListener("click", (e) => {
  e.stopPropagation();
  const open = els.langMenu.classList.toggle("hidden") === false;
  els.langBtn.setAttribute("aria-expanded", String(open));
  if (open) els.langMenu.querySelector('[aria-checked="true"]').focus();
});
document.addEventListener("click", closeLangMenu);
els.langMenu.addEventListener("keydown", (e) => { if (e.key === "Escape") { closeLangMenu(); els.langBtn.focus(); } });

loadConfig();
applyLang();
els.urls.focus();


// ---------- PDF size estimate ----------
// Relative JPEG size by quality, normalised to ~q85 (what AnyFlip pages are
// typically served at). Piecewise-linear between points; a rough model, not a promise.
const QUALITY_CURVE = [[10, 0.16], [20, 0.22], [30, 0.29], [40, 0.36], [50, 0.43], [60, 0.52], [75, 0.72], [85, 1], [90, 1.3], [100, 2.4]];
const RECOMMENDED_QUALITY = 60;

function qualityFactor(q) {
  const pts = QUALITY_CURVE;
  if (q <= pts[0][0]) return pts[0][1];
  for (let i = 1; i < pts.length; i++) {
    if (q <= pts[i][0]) {
      const [x0, y0] = pts[i - 1], [x1, y1] = pts[i];
      return y0 + ((y1 - y0) * (q - x0)) / (x1 - x0);
    }
  }
  return pts[pts.length - 1][1];
}

// The estimate is derived from the books currently previewed (pendingPreviews),
// so removing or queueing a book drops it, along with its cached measurements.
function sizeSamples() {
  const out = [];
  for (const e of pendingPreviews) {
    if (e.status !== "ready" || !e.pageCount || !e.coverDataUrl) continue;
    if (!e.sizeInfo) {
      const b64 = e.coverDataUrl.split(",")[1] || "";
      e.sizeInfo = { pageBytes: Math.floor((b64.length * 3) / 4), measured: new Map() };
    }
    if (e.sizeInfo.pageBytes) out.push(e);
  }
  return out;
}

// Re-encode the first page as JPEG in the webview for a real byte count at a
// quality, then scale by page count. Until every book has a measurement the
// rough curve above is used for all of them, so the numbers never mix models.
function measureJpeg(dataUrl, quality) {
  return new Promise((resolve, reject) => {
    const img = new Image();
    img.onload = () => {
      const c = document.createElement("canvas");
      c.width = img.naturalWidth; c.height = img.naturalHeight;
      const g = c.getContext("2d");
      g.fillStyle = "#fff"; g.fillRect(0, 0, c.width, c.height); // flatten transparency
      g.drawImage(img, 0, 0);
      c.toBlob((b) => (b ? resolve(b.size) : reject(new Error("encode failed"))), "image/jpeg", quality / 100);
    };
    img.onerror = () => reject(new Error("decode failed"));
    img.src = dataUrl;
  });
}

const PRESET_QUALITIES = [40, RECOMMENDED_QUALITY, 85];
let measureTimer = null;

function wantedQualities() {
  const q = parseInt(els.jpegQuality.value, 10) || RECOMMENDED_QUALITY;
  return els.compressImages.checked ? [...new Set([...PRESET_QUALITIES, q])] : [];
}

// Debounced and serial: dragging the slider only measures where it settles.
function scheduleMeasure() {
  clearTimeout(measureTimer);
  if (els.settingsLayer.classList.contains("hidden")) return; // re-armed by openSettings
  measureTimer = setTimeout(runMeasure, 150);
}

async function runMeasure() {
  let did = false;
  for (const e of sizeSamples()) {
    for (const q of wantedQualities()) {
      if (els.settingsLayer.classList.contains("hidden")) return; // closed mid-run
      if (e.sizeInfo.measured.has(q)) continue;
      did = true;
      try { e.sizeInfo.measured.set(q, await measureJpeg(e.coverDataUrl, q)); }
      catch { e.sizeInfo.measured.set(q, null); } // failed: keep the curve, never retry
    }
  }
  if (did) renderSizeEstimate();
}

// Returns { bytes, exact }. exact is true only when every book was measured.
function estimateBytes(quality, compress) {
  const samples = sizeSamples();
  let exact = compress && samples.length > 0 && samples.every((e) => typeof e.sizeInfo.measured.get(quality) === "number");
  const bytes = samples.reduce((sum, e) => {
    const perPage = !compress ? e.sizeInfo.pageBytes
      : exact ? e.sizeInfo.measured.get(quality)
      : e.sizeInfo.pageBytes * qualityFactor(quality);
    return sum + e.pageCount * perPage;
  }, 0);
  return { bytes, exact };
}

function fmtBytes(n) {
  return n >= 1e9 ? (n / 1e9).toFixed(2) + " GB" : n >= 1e6 ? (n / 1e6).toFixed(1) + " MB" : Math.max(1, Math.round(n / 1e3)) + " KB";
}

function renderSizeEstimate() {
  if (!els.estRows) return;
  const compress = els.compressImages.checked;
  const quality = parseInt(els.jpegQuality.value, 10) || RECOMMENDED_QUALITY;
  els.sizeEst.classList.toggle("off", !compress);
  const samples = sizeSamples();
  if (samples.length === 0) {
    els.estHead.textContent = t("estEmpty");
    els.estNote.textContent = "";
    els.statSize.textContent = "\u2014";
    els.estRows.replaceChildren();
    return;
  }
  scheduleMeasure();
  const pages = samples.reduce((s, e) => s + e.pageCount, 0);
  const books = `${samples.length} ${samples.length === 1 ? t("book") : t("books")}`;
  const cur = estimateBytes(quality, compress);
  els.statSize.textContent = "~" + fmtBytes(cur.bytes);
  els.estHead.textContent = compress
    ? t("estCompress", { p: cur.exact ? t("about_") : t("roughly"), size: fmtBytes(cur.bytes), q: quality, books, pages })
    : t("estOff", { size: fmtBytes(cur.bytes), books, pages });
  els.estNote.textContent = cur.exact || !compress
    ? t("noteExact")
    : samples.some((e) => e.sizeInfo.measured.get(quality) === null)
      ? t("noteFail")
      : t("noteWait");
  const presets = [
    { q: 40, name: t("pSmallest"), hint: t("pSmallestHint") },
    { q: RECOMMENDED_QUALITY, name: t("pRecommended"), hint: t("pRecommendedHint"), tag: true },
    { q: 85, name: t("pSharpest"), hint: t("pSharpestHint") },
  ];
  const ae = document.activeElement;
  const fi = ae && ae.classList && ae.classList.contains("est-row") ? Array.prototype.indexOf.call(els.estRows.children, ae) : -1;
  els.estRows.replaceChildren(...presets.map((p) => {
    const row = document.createElement("button");
    row.type = "button";
    row.className = "est-row" + (compress && quality === p.q ? " on" : "");
    const left = document.createElement("span");
    const b = document.createElement("b");
    b.textContent = `${p.name} (${p.q}%)`;
    if (p.tag) { const tg = document.createElement("span"); tg.className = "est-tag"; tg.textContent = t("suggested"); b.append(tg); }
    const small = document.createElement("small");
    small.textContent = p.hint;
    left.append(b, small);
    const size = document.createElement("span");
    size.className = "est-size";
    size.textContent = "~" + fmtBytes(estimateBytes(p.q, true).bytes);
    row.append(left, size);
    row.addEventListener("click", () => {
      els.compressImages.checked = true;
      els.jpegQuality.value = p.q;
      refreshSettingsUI();
    });
    return row;
  }));
  if (fi >= 0 && els.estRows.children[fi]) els.estRows.children[fi].focus();
}
