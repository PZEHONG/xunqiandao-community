"use strict";
(() => {
  const $ = id => document.getElementById(id), dialog = $("auth-dialog");
  let user = null, pane = "login", busy = false, generation = 0;
  const passwords = ["auth-password","auth-official-password"];
  let bindingSignature = "";
  let officialSchool = null;
  let schoolTimer, schoolController, schoolGeneration = 0, schoolComposing = false;
  const tabs = ["login","official"];
  const firstField = () => pane === "official" ? "auth-official-school" : "auth-identity";
  const agreementVersion = dialog.dataset.agreementVersion;
  const consentAccount = key => $(key === "official" ? "auth-official-account" : "auth-identity").value.trim().toLowerCase();
  const consentKey = account => "xun-agreement:" + account;
  function syncConsent(key = pane) {
    const account = consentAccount(key), field = $("auth-" + key + "-consent");
    let accepted = false;
    try { accepted = !!account && localStorage.getItem(consentKey(account)) === agreementVersion; } catch { /* Server remains authoritative. */ }
    field.required = !accepted; $("auth-" + key + "-consent-row").hidden = accepted;
  }
  function rememberConsent(result) {
    const account = (result.user?.student_number || result.user?.username || "").toLowerCase();
    if (!account) return;
    try {
      if (result.consent_accepted && result.agreement_version === agreementVersion) localStorage.setItem(consentKey(account),agreementVersion);
      else localStorage.removeItem(consentKey(account));
    } catch { /* Confirmation can be repeated if browser storage is unavailable. */ }
  }
  for (const key of tabs) {
    $(key === "official" ? "auth-official-account" : "auth-identity").addEventListener("input",() => {
      $("auth-" + key + "-consent").checked = false; syncConsent(key);
    });
  }
  function message(text = "", error = false) {
    $("auth-message").textContent = text; $("auth-message").hidden = !text;
    $("auth-message").classList.toggle("error", error);
  }
  function clearPasswords() {
    for (const id of passwords) { $(id).value = ""; $(id).type = "password"; }
    document.querySelectorAll("[data-auth-password]").forEach(button => { button.textContent = "显示"; button.setAttribute("aria-label","显示密码"); button.setAttribute("aria-pressed","false"); });
  }
  function selectPane(name) {
    if (busy) return;
    cancelSchoolLookup(); clearSchoolResults();
    clearPasswords(); pane = name; message();
    for (const key of tabs) $("auth-" + key + "-consent").checked = false;
    syncConsent();
    for (const key of tabs) $("auth-pane-" + key).hidden = key !== pane;
    for (const key of tabs) {
      const active = key === pane;
      $("auth-tab-" + key).setAttribute("aria-selected",String(active));
      $("auth-tab-" + key).tabIndex = active ? 0 : -1;
    }
    $("auth-title").textContent = pane === "official" ? "首次使用习讯云登录" : "登录讯签到";
    $("auth-footer").textContent = "首次通过习讯云验证后，系统将自动为您创建账号并完成登录。完成登录即表示您已阅读并同意《用户协议》和《隐私政策》。";
    $("auth-switch-text").textContent = pane === "login" ? "首次使用？" : "已完成首次登录？";
    $("auth-switch").textContent = pane === "login" ? "使用习讯云登录" : "返回学号登录";
    if (dialog.open) $(firstField()).focus();
    if (pane === "official" && !officialSchool) queueSchoolLookup();
  }
  function open(name = "login") {
    selectPane(name); if (!dialog.open) dialog.showModal();
    $(firstField()).focus();
  }
  function close() { if (!busy) dialog.close(); }
  async function api(path, data, signal) {
    const timeout = AbortSignal.timeout(30000);
    const response = await fetch("/api/auth" + path,{method:data ? "POST" : "GET",credentials:"same-origin",cache:"no-store",signal:signal ? AbortSignal.any([signal,timeout]) : timeout,...(data ? {headers:{"Content-Type":"application/json"},body:JSON.stringify(data)} : {})});
    let result;try {result = await response.json();} catch {throw new Error("账号服务暂时不可用，请稍后重试。");}
    if (!response.ok) {const error = new Error(result.error || "操作未成功，请重试。"); error.status = response.status; error.code = result.code; throw error;}
    return result;
  }
  function updateUser(value, bindings = [], force = false) {
    const signature = JSON.stringify(bindings);
    const changed = force || user?.id !== value?.id || bindingSignature !== signature;
    bindingSignature = signature;
    user = value; document.body.dataset.authState = user ? "authenticated" : "anonymous";
    $("auth-open").hidden = !!user; $("auth-profile").hidden = !user;
    $("system-account-email").textContent = user ? `学号 ${user.student_number || user.username}` : "登录后自动关联你的习讯云学号。";
    $("system-auth-open").hidden = !!user;
    if (changed) window.Workspace.setSession(user,bindings);
  }
  function expire() {
    if (user) { generation++; updateUser(null); }
    open(); message("登录已过期，请重新登录。",true);
  }
  function broadcast() {
    try { localStorage.setItem("xun-auth-change", String(Date.now()) + Math.random()); } catch { /* Current tab remains usable. */ }
  }
  async function restore(announce = false) {
    const current = ++generation;
    try {const result = await api("/self"); if (current !== generation) return; rememberConsent(result); updateUser(result.user,result.bindings); if (dialog.open) dialog.close();}
    catch (error) {
      if (current !== generation) return;
      updateUser(null); open();
      if (error.status !== 401) message("账号服务暂时无法连接，请稍后重试。",true);
      else if (announce) message("登录状态已更新，请重新登录。",true);
    }
  }
  async function submit(buttonID, label, task) {
    if (busy) return;
    busy = true; const button = $(buttonID), original = button.textContent;
    button.textContent = label;
    dialog.querySelectorAll("button,input").forEach(node => { node.disabled = true; });
    message();
    try { await task(); }
    catch (error) {
      if (error.code === "CONSENT_REQUIRED") {
        try { localStorage.removeItem(consentKey(consentAccount(pane))); } catch { /* Optional storage. */ }
        syncConsent(); $("auth-" + pane + "-consent").checked = false;
      }
      message(error.name === "TimeoutError" ? "请求超时，请稍后重试。" : error instanceof TypeError ? "无法连接账号服务，请稍后重试。" : error.message,true); }
    finally { busy = false; dialog.querySelectorAll("button,input").forEach(node => { node.disabled = false; }); button.textContent = original; }
  }
  $("auth-pane-login").addEventListener("submit",event => {
    event.preventDefault(); if (!event.target.reportValidity()) return;
    submit("auth-login-submit","登录中…",async () => {
      const current = ++generation;
      await api("/login",{identity:$("auth-identity").value.trim(),password:$("auth-password").value,accept_terms:$("auth-login-consent").checked});
      const result = await api("/self"); if (current !== generation) return;
      rememberConsent(result); updateUser(result.user,result.bindings,true); broadcast(); clearPasswords(); dialog.close();
      window.Workspace.toast("登录成功。");
      if (!result.bindings.length) { location.hash = "account-panel"; }
    });
  });
  function cancelSchoolLookup() {
    clearTimeout(schoolTimer); schoolController?.abort(); schoolController = null; schoolGeneration++;
  }
  function clearSchoolResults() {
    const box = $("auth-official-schools");
    box.replaceChildren(); box.hidden = true; box.setAttribute("aria-busy","false");
  }
  async function searchOfficialSchool() {
    cancelSchoolLookup();
    const name = $("auth-official-school").value.trim();
    if ([...name].length < 2 || schoolComposing || pane !== "official" || !dialog.open) return;
    const current = schoolGeneration, controller = new AbortController(); schoolController = controller;
    const box = $("auth-official-schools"), hint = $("auth-official-school-hint");
    const active = () => current === schoolGeneration && name === $("auth-official-school").value.trim() && pane === "official" && dialog.open;
    hint.textContent = "正在匹配学校…"; box.setAttribute("aria-busy","true");
    try {
      const result = await api("/schools?school_name=" + encodeURIComponent(name),undefined,controller.signal);
      if (!active()) return;
      const schools = (Array.isArray(result.schools) ? result.schools : []).filter(item => item && item.school_id != null && String(item.school_id).trim() && typeof item.school_name === "string" && item.school_name.trim());
      clearSchoolResults(); box.hidden = !schools.length;
      for (const school of schools) {
        const button = document.createElement("button"); button.type = "button"; button.textContent = school.school_name;
        button.addEventListener("click",() => {
          if (busy || !active()) return;
          cancelSchoolLookup(); clearSchoolResults();
          officialSchool = {school_id:String(school.school_id),school_name:school.school_name};
          $("auth-official-school").value = school.school_name;
          hint.textContent = "已选择：" + school.school_name;
          message(); $("auth-official-account").focus();
        });
        box.appendChild(button);
      }
      hint.textContent = schools.length ? "请选择下方匹配的学校。" : "未找到学校，请修改学校名称。";
    } catch (error) {
      if (active()) { clearSchoolResults(); hint.textContent = error.name === "TimeoutError" ? "匹配超时，请稍后重新输入学校名称。" : "暂时无法匹配学校，请稍后重新输入。"; }
    } finally {
      if (active()) { box.setAttribute("aria-busy","false"); schoolController = null; }
    }
  }
  function queueSchoolLookup() {
    cancelSchoolLookup(); clearSchoolResults();
    const name = $("auth-official-school").value.trim();
    $("auth-official-school-hint").textContent = [...name].length < 2 ? "输入至少 2 个字，自动显示匹配的学校。" : "输入后自动匹配，请从列表中选择学校。";
    if ([...name].length >= 2 && !schoolComposing) schoolTimer = setTimeout(searchOfficialSchool,350);
  }
  $("auth-official-school").addEventListener("input",event => {
    officialSchool = null; schoolComposing = event.isComposing || false; queueSchoolLookup();
  });
  $("auth-official-school").addEventListener("compositionstart",() => { schoolComposing = true; officialSchool = null; queueSchoolLookup(); });
  $("auth-official-school").addEventListener("compositionend",() => { schoolComposing = false; queueSchoolLookup(); });
  $("auth-official-school").addEventListener("keydown",event => {
    if (event.isComposing || schoolComposing || !["Enter","ArrowDown"].includes(event.key)) return;
    event.preventDefault();
    const first = $("auth-official-schools").querySelector("button");
    if (first) first.focus(); else if (!officialSchool) searchOfficialSchool();
  });
  $("auth-pane-official").addEventListener("submit",event => {
    event.preventDefault(); if (!event.target.reportValidity()) return;
    if (!officialSchool) {
      message("请先从匹配列表中选择学校。",true); $("auth-official-school").focus();
      if ($("auth-official-schools").hidden) searchOfficialSchool();
      return;
    }
    const school = officialSchool;
    submit("auth-official-submit","正在验证…",async () => {
      const current = ++generation;
      await api("/official",{account:$("auth-official-account").value.trim(),password:$("auth-official-password").value,school_id:school.school_id,school_name:school.school_name,accept_terms:$("auth-official-consent").checked});
      const result = await api("/self"); if (current !== generation) return;
      rememberConsent(result); updateUser(result.user,result.bindings,true); broadcast(); clearPasswords(); dialog.close();
      location.hash = "records-panel"; window.Workspace.toast("习讯云登录成功，已进入工作台。");
    });
  });
  $("auth-logout").addEventListener("click",async () => {
    if (busy) return; busy = true; $("auth-logout").disabled = true;
    try {
      await api("/logout",{}); generation++; updateUser(null); broadcast(); clearPasswords();
      busy = false; open(); window.Workspace.toast("已退出登录。");
    } catch (error) { if (error.status === 401) { busy = false; expire(); } else window.Workspace.toast("退出未成功，请重试。",true); }
    finally { busy = false; $("auth-logout").disabled = false; }
  });
  $("auth-open").addEventListener("click",() => open()); $("system-auth-open").addEventListener("click",() => open());
  $("auth-close").addEventListener("click",close);
  $("auth-forgot").addEventListener("click",() => message("请在习讯云官方 App 中找回密码，再用学号和新密码登录。"));
  $("auth-switch").addEventListener("click",() => selectPane(pane === "login" ? "official" : "login"));
  for (const key of tabs) {
    $("auth-tab-" + key).addEventListener("click",() => selectPane(key));
    $("auth-tab-" + key).addEventListener("keydown",event => {
      if (!["ArrowLeft","ArrowRight","Home","End"].includes(event.key)) return;
      event.preventDefault(); const next = event.key === "Home" ? tabs[0] : event.key === "End" ? tabs.at(-1) : tabs[(tabs.indexOf(key) + (event.key === "ArrowLeft" ? tabs.length - 1 : 1)) % tabs.length];
      selectPane(next); $("auth-tab-" + next).focus();
    });
  }
  document.querySelectorAll("[data-auth-password]").forEach(button => button.addEventListener("click",() => {
    const field = $(button.dataset.authPassword), visible = field.type === "password";
    field.type = visible ? "text" : "password"; button.textContent = visible ? "隐藏" : "显示";
    button.setAttribute("aria-label",visible ? "隐藏密码" : "显示密码"); button.setAttribute("aria-pressed",String(visible));
  }));
  dialog.addEventListener("cancel",event => { if (busy) event.preventDefault(); });
  dialog.addEventListener("close",() => { if (!dialog.open) { cancelSchoolLookup(); clearSchoolResults(); clearPasswords(); } });
  let backdrop = false;
  const outside = event => { const rect = dialog.getBoundingClientRect(); return event.clientX < rect.left || event.clientX > rect.right || event.clientY < rect.top || event.clientY > rect.bottom; };
  dialog.addEventListener("pointerdown",event => {backdrop = event.target === dialog && outside(event);});
  dialog.addEventListener("click",event => {if (backdrop && event.target === dialog && outside(event)) close(); backdrop = false;});
  window.addEventListener("storage",event => {if (event.key === "xun-auth-change") {generation++; updateUser(null); restore(true);}});
  window.addEventListener("pageshow",event => {if (event.persisted) restore(true);});
  document.addEventListener("visibilitychange",() => { if (!document.hidden && user && !busy) restore(); });
  setInterval(() => { if (user && !busy && !document.hidden) restore(); },60 * 60 * 1000);
  function reconnect() {
    if (dialog.open || busy) return;
    open();
    $("auth-identity").value = user?.student_number || user?.username || "";
    syncConsent();
    message("请登录一次以启用自动续连，之后无需反复登录。",true);
    $("auth-password").focus();
  }
  window.WorkspaceAuth = {open,expire,reconnect,user:() => user};
  document.body.dataset.authState = "loading"; restore();
})();
