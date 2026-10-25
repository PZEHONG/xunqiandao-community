"use strict";
(() => {
  const $ = id => document.getElementById(id);
  let remarks = [], photos = [], version = 0, remarksTask = null, remarksSequence = 0;
  const imageTypes = new Set(["image/jpeg", "image/png", "image/gif", "image/webp"]);

  function changed() {
    version++;
    window.dispatchEvent(new CustomEvent("workspace-sign-details-changed"));
  }
  function photoStatus(message, error = false) {
    $("sign-photo-status").textContent = message;
    $("sign-photo-status").hidden = !message;
    $("sign-photo-status").classList.toggle("error", error);
  }
  function date() {
    const now = new Date();
    $("sign-date").textContent = new Intl.DateTimeFormat("zh-CN", {timeZone:"Asia/Shanghai",year:"numeric",month:"long",day:"numeric"}).format(now);
    $("sign-date").dateTime = new Intl.DateTimeFormat("en-CA", {timeZone:"Asia/Shanghai",year:"numeric",month:"2-digit",day:"2-digit"}).format(now);
  }
  function chooseRemark(mark) {
    $("sign-remark").value = mark.key;
    $("sign-remark-label").textContent = mark.value;
    changed();
    renderRemarks();
  }
  function renderRemarks() {
    $("sign-remark-options").replaceChildren();
    for (const mark of remarks) {
      const button = document.createElement("button"), label = document.createElement("span"), check = document.createElement("span");
      button.type = "button";
      button.className = "sign-remark-option";
      button.setAttribute("aria-pressed", String(mark.key === $("sign-remark").value));
      label.textContent = mark.value;
      check.textContent = mark.key === $("sign-remark").value ? "✓" : "";
      check.setAttribute("aria-hidden", "true");
      button.append(label, check);
      button.addEventListener("click", () => { if (window.Workspace.isSigning()) return; chooseRemark(mark); $("sign-remark-dialog").close(); });
      $("sign-remark-options").append(button);
    }
  }
  async function loadRemarks() {
    if (remarksTask) return remarksTask;
    const owner = window.Workspace.version(), sequence = ++remarksSequence;
    $("sign-remark-reload").disabled = true;
    $("sign-remark-message").textContent = "正在加载账号可用备注…";
    $("sign-remark-message").classList.remove("error");
    const task = (async () => {
      try {
        const account = window.Workspace.account();
        const data = await window.Workspace.request(`/api/sign/remarks?account=${encodeURIComponent(account)}`);
        if (owner !== window.Workspace.version() || sequence !== remarksSequence) return;
        const list = (Array.isArray(data.remarks) ? data.remarks : []).filter(mark => typeof mark.key === "string" && mark.key && typeof mark.value === "string" && mark.value);
        if (!list.length) throw new Error("账号暂未返回可用备注，请刷新重试。");
        remarks = list;
        const selected = remarks.find(mark => mark.key === $("sign-remark").value) || remarks.find(mark => mark.value === "上班") || remarks[0];
        if (selected.key !== $("sign-remark").value || selected.value !== $("sign-remark-label").textContent) chooseRemark(selected);
        else renderRemarks();
        $("sign-remark-message").textContent = "";
      } catch (error) {
        if (owner !== window.Workspace.version() || sequence !== remarksSequence) return;
        $("sign-remark-message").textContent = error.message;
        $("sign-remark-message").classList.add("error");
      } finally {
        if (sequence === remarksSequence) { remarksTask = null; $("sign-remark-reload").disabled = false; }
      }
    })();
    remarksTask = task;
    // A missing account can fail before the first await; keep that failure retryable.
    await task;
    if (remarksTask === task) remarksTask = null;
  }
  function renderPhotos() {
    $("sign-photo-list").querySelectorAll(".sign-photo").forEach(node => node.remove());
    photos.forEach(entry => {
      const item = document.createElement("div"), image = document.createElement("img"), remove = document.createElement("button");
      item.className = "sign-photo";
      image.src = entry.preview;
      image.alt = entry.file.name;
      remove.type = "button";
      remove.textContent = "×";
      remove.setAttribute("aria-label", `移除照片 ${entry.file.name}`);
      remove.addEventListener("click", () => {
        if (window.Workspace.isSigning()) return;
        photos = photos.filter(photo => photo !== entry);
        URL.revokeObjectURL(entry.preview);
        changed(); renderPhotos(); photoStatus("");
      });
      item.append(image, remove);
      $("sign-photo-list").insertBefore(item, $("sign-photo-add"));
    });
    $("sign-photo-add").hidden = photos.length >= 3;
  }
  function snapshot() {
    if (!remarks.some(mark => mark.key === $("sign-remark").value)) throw new Error("请先选择签到备注。");
    return {remark:$("sign-remark").value, comment:$("sign-comment").value, photos:photos.slice(), version};
  }
  function validate(snapshot, owner) {
    if (owner !== window.Workspace.version() || snapshot.version !== version) throw new Error("签到信息已变更，请重新核对后提交。");
  }
  async function upload(snapshot, account, owner) {
    validate(snapshot, owner);
    for (let index = 0; index < snapshot.photos.length; index++) {
      validate(snapshot, owner);
      const entry = snapshot.photos[index];
      if (entry.uri) continue;
      photoStatus(`正在上传照片 ${index + 1} / ${snapshot.photos.length}…`);
      const form = new FormData(); form.set("account", account); form.set("file", entry.file);
      const result = await window.Workspace.request("/api/sign/photos", {method:"POST", body:form});
      validate(snapshot, owner);
      if (typeof result.uri !== "string" || !result.uri) throw new Error("照片上传未返回有效地址，请重试。");
      entry.uri = result.uri;
    }
    photoStatus("");
    return {remark:snapshot.remark, comment:snapshot.comment, photos:snapshot.photos.map(photo => photo.uri)};
  }
  function clearPhotos() {
    photos.forEach(entry => URL.revokeObjectURL(entry.preview)); photos = [];
    $("sign-photos").value = ""; $("sign-comment").value = ""; $("sign-comment-count").textContent = "0 / 1000";
    changed(); renderPhotos(); photoStatus("");
  }
  function reset() {
    remarksSequence++; remarksTask = null; remarks = [];
    $("sign-remark").value = ""; $("sign-remark-label").textContent = "选择备注";
    $("sign-remark-message").textContent = ""; $("sign-remark-reload").disabled = false;
    $("sign-remark-dialog").close(); renderRemarks(); clearPhotos();
  }
  function setBusy(busy) {
    for (const id of ["sign-remark-open", "sign-comment", "sign-photo-add", "sign-photos"]) $(id).disabled = busy;
    $("sign-photo-list").querySelectorAll(".sign-photo button").forEach(button => button.disabled = busy);
  }

  $("sign-remark-open").addEventListener("click", () => { $("sign-remark-dialog").showModal(); if (!remarks.length) loadRemarks(); });
  $("sign-remark-reload").addEventListener("click", loadRemarks);
  $("sign-comment").addEventListener("input", () => { $("sign-comment-count").textContent = `${[...$("sign-comment").value].length} / 1000`; changed(); });
  $("sign-photo-add").addEventListener("click", () => $("sign-photos").click());
  $("sign-photos").addEventListener("change", () => {
    const files = [...$("sign-photos").files]; $("sign-photos").value = "";
    if (window.Workspace.isSigning() || !files.length) return;
    if (files.length + photos.length > 3) { photoStatus("最多添加 3 张照片。", true); return; }
    if (files.some(file => !imageTypes.has(file.type) || !file.size || file.size > 10 * 1024 * 1024)) { photoStatus("请选择 10 MB 以内的 JPG、PNG、GIF 或 WebP 图片。", true); return; }
    photos.push(...files.map(file => ({file,preview:URL.createObjectURL(file),uri:""})));
    changed(); renderPhotos(); photoStatus("");
  });
  window.addEventListener("workspace-account-changed", reset);
  window.addEventListener("workspace-config-cleared", reset);
  window.addEventListener("workspace-view-changed", event => { if (event.detail === "sign-panel") date(); });
  async function configure(remark, comment) {
    const owner = window.Workspace.version();
    await loadRemarks();
    if (owner !== window.Workspace.version()) return;
    const selected = remarks.find(mark => mark.key === remark);
    if (!selected) throw new Error("原签到类型已不可用，请重新选择。");
    chooseRemark(selected); $("sign-comment").value = comment || "";
    $("sign-comment-count").textContent = `${[...$("sign-comment").value].length} / 1000`; changed();
  }
  window.WorkspaceSign = {snapshot,validate,upload,setBusy,clearPhotos,load:loadRemarks,configure,remarkLabel:() => $("sign-remark-label").textContent};
  date();
})();
