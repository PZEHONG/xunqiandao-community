"use strict";
(() => {
  const $ = id => document.getElementById(id), W = window.Workspace;
  const labels = {running:"执行中",success:"成功",failed:"失败",unknown:"待核对",skipped:"已跳过"};
  let tasks = [], schoolID = "", editing = null, place = null, pending = null, busy = false, sequence = 0, operation = 0;
  let mode = $("sign-panel").dataset.signMode === "daily" ? "daily" : "now", locked = [];
  const current = version => version === W.version();
  const isDaily = () => mode === "daily";
  function update() {
    const daily = isDaily(), saved = daily && !!place;
    $("sign-panel").dataset.signMode = mode; $("sign-panel").dataset.savedPlace = String(saved);
    $("sign-mode-now").setAttribute("aria-pressed",String(!daily)); $("sign-mode-daily").setAttribute("aria-pressed",String(daily));
    $("auto-editor").hidden = !daily; $("auto-name").disabled = $("auto-time").disabled = !daily || busy;
    $("auto-edit-note").hidden = $("auto-cancel").hidden = !editing || !daily;
    $("auto-saved-place").hidden = !saved;
    if (saved) { $("auto-saved-name").textContent = place.address_name || "已确认地点"; $("auto-saved-address").textContent = place.address; }
    $("sign-panel").querySelector(".sign-photo-section").hidden = daily;
    $("sign-purpose").textContent = daily ? "确认后按每日计划执行。" : "确认地点和信息后提交。";
    $("sign-save-hint").textContent = daily ? "确认启用后保存地点与内容，每天执行" : "地点仅用于本次签到";
    if (!W.isSigning()) $("sign-button").textContent = daily ? (editing ? "确认修改并启用" : "确认每日任务") : "提交签到";
    let valid = saved; if (!valid) { try { window.WorkspaceLocations.confirmedLocation(); valid = true; } catch {} }
    $("sign-button").disabled = busy || W.isSigning() || !valid;
  }
  function setMode(value) {
    cancelConfirmation(); mode = value === "daily" ? "daily" : "now";
    window.dispatchEvent(new CustomEvent("workspace-sign-details-changed")); update();
    if (isDaily()) { window.WorkspaceSign.load(); load(); }
  }
  function message(text = "", error = false) { $("auto-message").textContent = text; $("auto-message").hidden = !text; $("auto-message").classList.toggle("error",error); }
  function node(tag, text, className = "") { const n = document.createElement(tag); n.textContent = text; n.className = className; return n; }
  function cancelConfirmation() { pending = null; if ($("auto-confirm").open) $("auto-confirm").close("cancel"); }
  function controls(disabled) {
    if (disabled) { locked = [...$("sign-panel").querySelectorAll("button:enabled,input:enabled,select:enabled,textarea:enabled")]; locked.forEach(n => n.disabled = true); }
    else { locked.forEach(n => n.disabled = false); locked = []; $("auto-tasks").querySelectorAll("button").forEach(n => n.disabled = false); update(); }
  }
  async function working(action) {
    if (busy || W.isSigning()) return;
    const owner = W.version(), op = ++operation; busy = true; controls(true); message();
    try { await action(owner); }
    catch (e) { if (current(owner)) { message(e.message,true); if ($("sign-panel").hidden) W.toast(e.message,true); } }
    finally { if (op === operation) { busy = false; controls(false); } }
  }
  function localTime(value) { if (!value) return "—"; const date = new Date(value); return Number.isNaN(date.getTime()) ? "—" : new Intl.DateTimeFormat("zh-CN", {timeZone:"Asia/Shanghai",month:"2-digit",day:"2-digit",hour:"2-digit",minute:"2-digit",hour12:false}).format(date); }
  function button(text, action) { const n = node("button",text,"button secondary"); n.type = "button"; n.disabled = busy; n.addEventListener("click",action); return n; }
  function closeEditor() { cancelConfirmation(); editing = null; place = null; $("auto-name").value = "每日签到"; $("auto-time").value = "09:00"; update(); }
  function render(list, runs) {
    $("auto-task-count").textContent = `${list.length} 个任务 · ${list.filter(t => t.enabled).length} 个启用`;
    $("auto-tasks").replaceChildren(); $("auto-empty").hidden = !!list.length;
    $("auto-empty").textContent = "暂无每日任务。先确认地点，再创建每日计划。";
    for (const t of list) {
      const card = node("article","","auto-task"), heading = node("div","","auto-task-heading");
      heading.append(node("span",t.time,"auto-clock"),node("h3",t.name),node("span",t.enabled ? "已启用" : "已暂停",`auto-badge ${t.enabled ? "enabled" : ""}`));
      card.append(heading,node("p",`每天 · ${t.remark_label} · ${t.address_name || t.address}`),node("p",t.address),node("p",t.enabled ? `下次执行：${localTime(t.next_at)}（北京时间）` : "重新启用前需要核对已保存的信息"));
      if (t.last_run) card.append(node("p",`最近结果：${labels[t.last_run.status] || "待核对"} · ${t.last_run.message}`));
      const actions = node("div","","auto-task-actions");
      actions.append(button(t.enabled ? "暂停" : "重新启用",() => t.enabled ? pause(t) : confirm("resume",t)),button("编辑",() => edit(t)),button("删除",() => confirm("delete",t)));
      card.append(actions); $("auto-tasks").append(card);
    }
    $("auto-runs").replaceChildren(); $("auto-runs-empty").hidden = !!runs.length;
    for (const r of runs) {
      const row = node("article","","auto-run"), heading = node("div","","auto-run-heading");
      heading.append(node("strong",r.name),node("span",`${r.date} ${r.scheduled_time}`),node("span",labels[r.status] || "待核对",`auto-badge auto-result-${r.status}`));
      row.append(heading,node("p",r.message || "后台正在执行")); $("auto-runs").append(row);
    }
  }
  async function load() {
    const owner = W.version(), seq = ++sequence;
    let account; try { account = W.account(); } catch { return; }
    try {
      const query = `?account=${encodeURIComponent(account)}`;
      const [result, history] = await Promise.all([W.request("/api/auto-sign/tasks"+query),W.request("/api/auto-sign/runs"+query)]);
      if (!current(owner) || seq !== sequence) return;
      tasks = Array.isArray(result.tasks) ? result.tasks : []; schoolID = result.school_id || "";
      render(tasks, Array.isArray(history.runs) ? history.runs : []);
    } catch (e) { if (current(owner) && seq === sequence) message(e.message,true); }
  }
  async function prepare(owner, task = null) {
    if (!schoolID) throw new Error("请先连接账号，取得学校信息后再设置每日任务。");
    closeEditor(); setMode("daily"); editing = task;
    $("auto-name").value = task?.name || "每日签到"; $("auto-time").value = task?.time || "09:00";
    if (task) place = {address:task.address,address_name:task.address_name,latitude:task.latitude,longitude:task.longitude,location_confirmed:true,confirmation_token:""};
    update();
    if (task) await window.WorkspaceSign.configure(task.remark,task.comment);
    else await window.WorkspaceSign.load();
  }
  function summary(t) { return `账号：${t.account}\n学校 ID：${t.school_id || schoolID || "连接账号的学校"}\n任务：${t.name}\n时间：每天 ${t.time}（北京时间，包含周末）\n类型：${t.remark_label}\n地点：${t.address_name || ""}\n地址：${t.address}\n坐标：${t.latitude}, ${t.longitude}\n说明：${t.comment || "无"}\n照片：不附带`; }
  function confirm(action, task) {
    if (busy || W.isSigning()) return;
    pending = {action,task:structuredClone(task),owner:W.version()};
    $("auto-confirm-title").textContent = action === "delete" ? "确认删除每日任务" : "确认启用每日任务";
    $("auto-confirm-yes").textContent = action === "delete" ? "确认删除" : "确认启用";
    $("auto-confirm-text").textContent = summary(task);
    $("auto-confirm-note").textContent = action === "delete" ? "删除配置后不再按此计划执行，执行记录仍保留。已经发出的签到无法撤回。" : "确认后在本机保存以上信息并每天提交，从下一个计划时间开始执行。今天此任务已经执行过时，下一次为明天。";
    $("auto-confirm").returnValue = ""; $("auto-confirm").showModal();
  }
  function state(t, enabled, confirmed = false) { return W.request(`/api/auto-sign/tasks/${t.id}/state`, {method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({account:t.account,revision:t.revision,enabled,confirmed})}); }
  function pause(t) { working(async owner => { await state(t,false); if (!current(owner)) return; await load(); message("任务已暂停。已经发出的签到无法撤回。"); }); }
  function edit(t) {
    working(async owner => {
      let paused = t;
      if (t.enabled) { const response = await state(t,false); paused = response.task; }
      if (!current(owner)) return;
      await load(); if (!current(owner)) return;
      await prepare(owner,paused);
      if (current(owner)) $("sign-panel").scrollIntoView({block:"start"});
    });
  }
  $("auto-new").addEventListener("click",() => working(async owner => { W.account(); await load(); if (current(owner)) { await prepare(owner); if (current(owner)) $("sign-panel").scrollIntoView({block:"start"}); } }));
  $("auto-cancel").addEventListener("click",() => { closeEditor(); message("已取消编辑，原任务保持暂停。"); });
  $("auto-refresh").addEventListener("click",() => working(load));
  $("auto-change-place").addEventListener("click",() => { place = null; cancelConfirmation(); update(); $("location-search-panel").open = true; $("location-search").focus(); });
  for (const [id, value] of [["sign-mode-now","now"],["sign-mode-daily","daily"]]) $(id).addEventListener("click",() => { if (!busy && !W.isSigning()) setMode(value); });
  $("sign-form").addEventListener("input",() => { if (pending?.action === "save") cancelConfirmation(); });
  function submit() {
    if (!isDaily() || busy || W.isSigning() || !$("sign-form").reportValidity()) return;
    message();
    try {
      const location = place || window.WorkspaceLocations.confirmedLocation();
      const account = W.account(); if (editing && editing.account !== account) throw new Error("账号已变化，请重新编辑任务。");
      if (!schoolID) throw new Error("请先连接账号并刷新每日任务，核对学校信息。");
      const details = window.WorkspaceSign.snapshot();
      confirm("save",{id:editing?.id || 0,revision:editing?.revision || 0,account,school_id:schoolID,name:$("auto-name").value.trim(),time:$("auto-time").value,remark:details.remark,remark_label:window.WorkspaceSign.remarkLabel(),comment:details.comment,...location,enabled:true,confirmed:true});
    } catch (e) { message(e.message,true); }
  }
  $("auto-confirm").addEventListener("cancel",cancelConfirmation);
  $("auto-confirm").addEventListener("close",() => {
    const snapshot = pending; pending = null;
    if (!snapshot || $("auto-confirm").returnValue !== "confirm" || !current(snapshot.owner)) return;
    working(async owner => {
      const t = snapshot.task;
      if (snapshot.action === "save") await W.request(`/api/auto-sign/tasks${t.id ? "/"+t.id : ""}`,{method:t.id ? "PUT" : "POST",headers:{"Content-Type":"application/json"},body:JSON.stringify(t)});
      else if (snapshot.action === "resume") await state(t,true,true);
      else await W.request(`/api/auto-sign/tasks/${t.id}`,{method:"DELETE",headers:{"Content-Type":"application/json"},body:JSON.stringify({account:t.account,revision:t.revision})});
      if (!current(owner)) return;
      closeEditor(); await load();
      if (current(owner)) message(snapshot.action === "delete" ? "任务已删除，执行历史仍保留。" : "每日任务已启用。请保持电脑开机、联网及后台服务运行。");
    });
  });
  function reset() {
    sequence++; operation++; busy = false; tasks = []; schoolID = ""; closeEditor(); controls(false); message(); render([],[]);
    mode = "now"; update(); $("auto-panel").open = false;
    $("auto-empty").textContent = "连接账号后查看每日任务。";
  }
  window.addEventListener("workspace-account-changed",reset);
  window.addEventListener("workspace-config-cleared",reset);
  window.addEventListener("workspace-view-changed",event => { if (event.detail === "sign-panel") load(); });
  window.addEventListener("workspace-sign-mode-requested",event => setMode(event.detail));
  window.addEventListener("workspace-location-changed",() => { if (pending?.action === "save") cancelConfirmation(); update(); });
  window.addEventListener("workspace-sign-details-changed",() => { if (pending?.action === "save") cancelConfirmation(); });
  setInterval(() => { if (!document.hidden && !$("sign-panel").hidden && !busy && !$("auto-confirm").open) load(); },30000);
  window.WorkspaceAutoSign = {isDaily,submit,refresh:update};
  update();
  if (!$("sign-panel").hidden) { load(); if (isDaily()) window.WorkspaceSign.load(); }
})();
