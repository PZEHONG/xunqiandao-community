"use strict";
(() => {
  const $ = id => document.getElementById(id);
  let storageKey = "";
  let platformUserID = "";
  let platformStudentNumber = "";
  let platformSessionVersion = 0;
  let loggedInAccount = "";
  let selectedSchool = null;
  let accountVersion = 0;
  let pendingSign = null;
  let signSubmitting = false;
  let toastTimer;
  let queryTask = Promise.resolve();
  let calendarMonth = window.WorkspaceCalendar.currentMonth();
  let calendarSnapshot = null;
  let calendarBusy = false;
  const views = {"records-panel":"签到记录", "sign-panel":"签到", "account-panel":"账号设置", "week-panel":"实习周报", "month-panel":"实习月报"};

  function showView(focus = false) {
    let requested = location.hash.slice(1);
    const legacyDaily = requested === "auto-panel";
    if (legacyDaily) { requested = "sign-panel"; history.replaceState(null, "", "#sign-panel"); $("sign-panel").dataset.signMode = "daily"; }
    if (requested === "main-content") return;
    const active = Object.hasOwn(views, requested) ? requested : "records-panel";
    if (active !== "records-panel" && $("records-dialog").open) $("records-dialog").close();
    if (active !== "sign-panel" && $("sign-success").open) $("sign-success").close("reset");
    document.body.dataset.view = active;
    for (const id of Object.keys(views)) $(id).hidden = id !== active;
    document.querySelectorAll(".nav-link").forEach(link => {
      const selected = link.hash === `#${active}`;
      link.classList.toggle("active", selected);
      if (selected) link.setAttribute("aria-current", "page");
      else link.removeAttribute("aria-current");
    });
    document.title = `${views[active]} · 讯签到`;
    $("page-title").textContent = views[active];
    $("current-page").textContent = views[active];
    if (focus) $("main-content").focus({preventScroll:true});
    if (requested !== "main-content") window.scrollTo(0, 0);
    window.dispatchEvent(new CustomEvent("workspace-view-changed", {detail:active}));
    if (legacyDaily) window.dispatchEvent(new CustomEvent("workspace-sign-mode-requested", {detail:"daily"}));
  }
  function navigate(id) {
    if (location.hash !== `#${id}`) history.pushState(null, "", `#${id}`);
    showView(true);
  }
  document.querySelectorAll('a[href^="#"]').forEach(link => link.addEventListener("click", event => {
    const id = link.hash.slice(1);
    if (!Object.hasOwn(views, id) || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return;
    event.preventDefault();
    navigate(id);
  }));
  $("records-history").addEventListener("click", () => $("records-dialog").showModal());
  let recordsBackdropPressed = false;
  function outsideRecordsDialog(event) {
    const rect = $("records-dialog").getBoundingClientRect();
    return event.clientX < rect.left || event.clientX > rect.right || event.clientY < rect.top || event.clientY > rect.bottom;
  }
  $("records-dialog").addEventListener("pointerdown", event => { recordsBackdropPressed = event.target === $("records-dialog") && outsideRecordsDialog(event); });
  $("records-dialog").addEventListener("click", event => {
    if (recordsBackdropPressed && event.target === $("records-dialog") && outsideRecordsDialog(event)) $("records-dialog").close();
    recordsBackdropPressed = false;
  });
  window.addEventListener("hashchange", () => showView(true));
  window.addEventListener("popstate", () => showView(true));
  window.addEventListener("load", () => showView());
  showView();

  function showMessage(id, message, error = false) {
    const target = $(id);
    target.textContent = message;
    target.classList.toggle("error", error);
    target.hidden = !message;
  }
  function toast(message, error = false) {
    clearTimeout(toastTimer);
    $("toast").textContent = message;
    $("toast").classList.toggle("error", error);
    $("toast").hidden = false;
    toastTimer = setTimeout(() => { $("toast").hidden = true; }, 3500);
  }
  function persist() {
    if (!platformUserID) return;
    const config = {account:$("account").value.trim(),schoolId:selectedSchool?.school_id || "",schoolName:$("school-search").value.trim()};
    try { localStorage.setItem(storageKey, JSON.stringify(config)); }
    catch { toast("浏览器未允许保存配置，本次填写仍可使用。", true); }
  }
  function refreshSummary() {
    const account = $("account").value.trim();
    const school = $("school-search").value.trim() || "未设置学校";
    const status = loggedInAccount ? "已连接" : "待连接";
    const accountLink = $("account-summary").closest("a");
    $("account-summary").textContent = account || "账号设置";
    accountLink.title = account ? `${account} · ${status}\n${school}` : "查看账号设置";
    accountLink.setAttribute("aria-label", account ? `${account}，${status}，${school}，查看账号设置` : "查看账号设置");
    $("account-connection-description").textContent = loggedInAccount ? "已自动关联习讯云，可直接使用签到与报告功能。" : "首次使用请通过习讯云登录，验证成功后自动关联。";
    $("login-button").hidden = !!loggedInAccount;
  }
  async function request(path, options = {}) {
    const session = platformSessionVersion;
    const signing = path.split("?")[0] === "/api/sign";
    const uncertainSign = "签到提交结果暂时无法确认，请先刷新签到记录，再决定是否重新提交。";
    let response;
    try { response = await fetch(path, {...options, cache:"no-store", signal:AbortSignal.timeout(signing ? 60000 : 30000)}); }
    catch (error) { throw new Error(signing ? uncertainSign : (error.name === "TimeoutError" ? "请求超时，请稍后重试。" : "无法连接服务，请确认本地服务正在运行。")); }
    let result;
    try { result = await response.json(); }
    catch { throw new Error(signing ? uncertainSign : "服务返回了无法读取的结果，请稍后重试。"); }
    if (session !== platformSessionVersion && !path.startsWith("/api/auth/")) throw new Error("当前登录账号已变更，请重试。");
    if (result.code === "AUTH_REQUIRED") window.WorkspaceAuth?.expire();
    if (result.code === "UPSTREAM_LOGIN_REQUIRED") window.WorkspaceAuth?.reconnect();
    if (!response.ok) throw new Error(result.error || result.message || "操作失败，请重试。");
    return result;
  }
  async function withButton(id, label, task) {
    const button = $(id), original = button.innerHTML;
    button.disabled = true;
    button.textContent = label;
    try { await task(); }
    finally {
      button.disabled = false; button.innerHTML = original;
      if (id === "sign-button") {
        try { window.WorkspaceLocations.confirmedLocation(); }
        catch { button.disabled = true; }
      }
    }
  }
  function accountForAction() {
    if (!platformUserID) { window.WorkspaceAuth?.open(); throw new Error("请先登录讯签到。"); }
    const account = $("account").value.trim();
    if (account !== platformStudentNumber) throw new Error("习讯云学号须与登录学号一致。");
    if (!account) throw new Error("请先填写并连接习讯云账号。");
    return account;
  }
  function sanitize(value) {
    if (Array.isArray(value)) return value.map(sanitize);
    if (value && typeof value === "object") return Object.fromEntries(Object.entries(value).map(([key, val]) => [key, /token|password|secret|api_key/i.test(key) ? "已隐藏" : sanitize(val)]));
    return value;
  }
  function resetAccountState() {
    accountVersion++;
    loggedInAccount = "";
    pendingSign = null;
    if ($("sign-confirm").open) $("sign-confirm").close("cancel");
    if ($("sign-success").open) $("sign-success").close("reset");
    $("sign-success-message").textContent = "";
    for (const id of ["latitude", "longitude", "sign-address", "sign-name"]) $(id).value = "";
    $("records-content").hidden = true; $("records-empty").hidden = false;
    $("record-details").hidden = true; $("record-details").open = false;
    $("records-count").hidden = true;
    if ($("records-dialog").open) $("records-dialog").close();
    calendarSnapshot = null; calendarMonth = window.WorkspaceCalendar.currentMonth();
    renderCalendar();
    $("empty-title").textContent = "连接账号，查看签到记录";
    $("empty-description").textContent = "首次使用，请先设置学校和习讯云账号。";
    $("empty-account-link").hidden = false;
    $("query-data").replaceChildren(); $("record-table-body").replaceChildren(); $("query-raw").textContent = "";
    $("records-footnote").textContent = "连接账号后查看签到日历与明细。";
    for (const id of ["login-message", "query-message", "sign-message"]) showMessage(id, "");
    window.dispatchEvent(new CustomEvent("workspace-account-changed"));
  }
  function recordText(value) {
    return typeof value === "string" && value.trim() ? value.trim() : "";
  }
  function recordTime(record) {
    const raw = record.sign_time;
    if ((typeof raw === "string" || typeof raw === "number") && /^\d{10}(\d{3})?$/.test(String(raw))) {
      const date = new Date(Number(raw) * (String(raw).length === 10 ? 1000 : 1));
      if (!Number.isNaN(date.getTime())) {
        const parts = Object.fromEntries(new Intl.DateTimeFormat("zh-CN", {timeZone:"Asia/Shanghai",year:"numeric",month:"2-digit",day:"2-digit",hour:"2-digit",minute:"2-digit",second:"2-digit",hourCycle:"h23"}).formatToParts(date).map(part => [part.type, part.value]));
        return `${parts.year}-${parts.month}-${parts.day} ${parts.hour}:${parts.minute}:${parts.second}`;
      }
    }
    return recordText(record.sign_time_text) || "平台未返回时间";
  }
  function recordLines(primary, secondary = "") {
    const content = document.createElement("div"), main = document.createElement("span");
    main.className = "record-primary"; main.textContent = primary; content.append(main);
    if (secondary && secondary !== primary) {
      const extra = document.createElement("span"); extra.className = "record-secondary"; extra.textContent = secondary; content.append(extra);
    }
    return content;
  }
  function setCalendarBusy(busy) {
    calendarBusy = busy;
    $("sign-calendar").setAttribute("aria-busy", String(busy));
    $("calendar-prev").disabled = busy || !calendarSnapshot || calendarMonth === "1000-01";
    $("calendar-next").disabled = busy || !calendarSnapshot || calendarMonth >= window.WorkspaceCalendar.currentMonth();
    $("calendar-current").disabled = busy;
  }
  function renderCalendar() {
    const calendar = window.WorkspaceCalendar.buildMonth(calendarMonth, calendarSnapshot?.records || []);
    $("calendar-month").textContent = calendar.title;
    $("records-dialog-month").textContent = `${calendar.title} · 签到明细`;
    $("calendar-days").setAttribute("aria-label", `${calendar.title}每日签到情况`);
    $("calendar-current").hidden = calendarMonth === window.WorkspaceCalendar.currentMonth();
    const streak = calendarSnapshot?.continuous_sign_in ?? calendarSnapshot?.continuous_days;
    $("calendar-streak").textContent = /^\d+$/.test(String(streak)) ? String(streak) : "—";
    const fragment = document.createDocumentFragment();
    for (const day of calendar.days) {
      const cell = document.createElement("div");
      if (!day) {
        cell.className = "calendar-blank"; cell.setAttribute("aria-hidden", "true");
      } else {
        cell.className = "calendar-day";
        cell.setAttribute("role", "listitem");
        if (day.today) { cell.classList.add("is-today"); cell.setAttribute("aria-current", "date"); }
        if (day.normalCount) cell.classList.add("is-signed");
        if (day.abnormalCount) cell.classList.add("has-abnormal");
        if (day.unknownCount && !day.normalCount && !day.abnormalCount) cell.classList.add("is-unknown");
        const states = [];
        if (day.normalCount) states.push(`正常签到 ${day.normalCount} 次`);
        if (day.abnormalCount) states.push(`异常记录 ${day.abnormalCount} 条`);
        if (day.unknownCount) states.push(`状态待确认 ${day.unknownCount} 条`);
        cell.setAttribute("aria-label", `${day.date}${day.today ? "，今天" : ""}${day.lunar ? `，${day.lunar}` : ""}，${states.join("，") || (calendarSnapshot ? "无签到记录" : "尚未查询")}`);
        const disc = document.createElement("div"), date = document.createElement("time"), lunar = document.createElement("span");
        disc.className = "calendar-disc"; date.className = "calendar-date"; date.dateTime = day.date; date.textContent = String(day.day);
        lunar.className = "calendar-lunar"; lunar.textContent = day.lunar;
        disc.append(date, lunar); cell.append(disc);
        if (day.abnormalCount) { const indicator = document.createElement("span"); indicator.className = "calendar-status-note"; indicator.textContent = "异常"; cell.append(indicator); }
        else if (day.unknownCount && !day.normalCount) { const indicator = document.createElement("span"); indicator.className = "calendar-status-note"; indicator.textContent = "待确认"; cell.append(indicator); }
      }
      fragment.append(cell);
    }
    $("calendar-days").replaceChildren(fragment);
    $("calendar-abnormal-legend").hidden = !calendar.days.some(day => day?.abnormalCount);
    $("calendar-unknown-legend").hidden = !calendar.days.some(day => day?.unknownCount && !day.normalCount && !day.abnormalCount);
    if (calendarSnapshot) $("calendar-caption").textContent = `${calendar.normalDays} 天已签到 · ${calendarSnapshot.records.length} 条记录`;
    else {
      $("calendar-caption").textContent = "连接账号后查看签到情况。 ";
      const link = document.createElement("a"); link.href = "#account-panel"; link.textContent = "连接账号";
      link.addEventListener("click", event => { event.preventDefault(); navigate("account-panel"); });
      $("calendar-caption").append(link);
    }
    setCalendarBusy(calendarBusy);
  }
  renderCalendar();
  function renderData(data) {
    if (!data || !Array.isArray(data.records) || data.records.some(record => !record || typeof record !== "object" || Array.isArray(record))) throw new Error("查询结果不完整，暂时无法核实签到明细，请重新刷新。");
    // Validate the month before replacing any previously verified results.
    window.WorkspaceCalendar.buildMonth(data.month_date || calendarMonth);
    calendarMonth = data.month_date || calendarMonth; calendarSnapshot = data;
    renderCalendar();
    $("query-data").replaceChildren();
    const labels = {user_name:"姓名",school_name:"学校",class_name:"班级",continuous_sign_in:"连续签到天数",continuous_days:"连续签到天数",sign_days:"签到天数",signed_days:"签到天数",should_sign_days:"应签到天数",total_days:"总天数",sign_count:"签到次数",month_date:"查询月份"};
    const shownLabels = new Set();
    for (const [key, label] of Object.entries(labels)) {
      if (data[key] == null || typeof data[key] === "object" || shownLabels.has(label)) continue;
      shownLabels.add(label);
      const item = document.createElement("div"), name = document.createElement("span"), value = document.createElement("strong");
      item.className = "data-item"; name.textContent = label; value.textContent = String(data[key]); item.append(name, value); $("query-data").append(item);
    }
    const records = data.records;
    $("record-table-body").replaceChildren();
    for (const record of records) {
      const row = document.createElement("tr");
      const officialStatus = recordText(record.status_code_text);
      const status = officialStatus === "正常" ? "签到正常" : officialStatus === "异常" ? "签到异常" : officialStatus && !/^\d+$/.test(officialStatus) ? officialStatus : record.status_code != null ? "状态待确认" : "平台未返回状态";
      const state = recordLines(status, recordText(record.remark_text));
      state.className = officialStatus === "正常" ? "record-status is-normal" : officialStatus === "异常" ? "record-status is-abnormal" : "record-status";
      const name = recordText(record.address_name), address = recordText(record.address);
      const values = [recordTime(record), state, recordLines(name || address || "平台未返回地点", name ? address : "")];
      for (const [index, value] of values.entries()) {
        const cell = document.createElement("td"); cell.dataset.label = ["日期 / 时间", "签到状态", "签到地点"][index];
        if (typeof value === "string") cell.textContent = value; else cell.append(value);
        row.append(cell);
      }
      $("record-table-body").append(row);
    }
    $("record-table-wrap").hidden = records.length === 0;
    $("records-count").textContent = `${records.length} 条`;
    $("records-count").hidden = false;
    $("query-raw").textContent = JSON.stringify(sanitize(data), null, 2);
    $("record-details").hidden = false;
    $("records-empty").hidden = records.length > 0; $("records-content").hidden = false;
    $("empty-title").textContent = "本月暂无签到记录";
    $("empty-description").textContent = "习讯云返回的本月签到明细为空，可稍后刷新。";
    $("empty-account-link").hidden = true;
  }
  function setSession(user, bindings = []) {
    platformSessionVersion++;
    platformUserID = user ? String(user.id) : "";
    platformStudentNumber = user?.student_number || user?.username || "";
    storageKey = platformUserID ? `xixun-workspace-config:${platformUserID}` : "";
    $("login-form").reset(); $("sign-form").reset(); selectedSchool = null;
    resetAccountState();
    let config;
    try { config = JSON.parse(localStorage.getItem(storageKey) || "null"); } catch { /* Browser storage is optional. */ }
    $("account").value = platformStudentNumber;
    const binding = bindings.find(item => item.account === platformStudentNumber);
    if (binding) {
      $("account").value = binding.account;
      $("school-search").value = binding.school_name || (config?.account === binding.account ? config.schoolName || "" : "");
      selectedSchool = {school_id:binding.school_id,school_name:$("school-search").value};
      loggedInAccount = binding.account;
      showMessage("login-message", "已自动关联习讯云，无需重复连接。");
    }
    $("password").type = "password"; $("toggle-password").textContent = "显示";
    $("toggle-password").setAttribute("aria-label", "显示密码"); $("toggle-password").setAttribute("aria-pressed", "false");
    $("school-results").hidden = true; $("school-results").replaceChildren();
    refreshSummary();
    window.dispatchEvent(new CustomEvent("workspace-session-changed", {detail:user}));
    if (binding) { const version = accountVersion; queryTask.finally(() => { if (version === accountVersion) refreshRecords(); }); }
  }
  refreshSummary();
  for (const id of ["account", "school-search"]) $(id).addEventListener("input", () => {
    resetAccountState();
    if (id === "school-search") {
      selectedSchool = null;
      $("school-results").hidden = true;
      $("school-results").replaceChildren();
      $("school-search-message").textContent = "输入学校名称，连接时自动匹配；也可搜索后选择学校。";
    }
    refreshSummary();
  });
  $("toggle-password").addEventListener("click", () => {
    const visible = $("password").type === "password";
    $("password").type = visible ? "text" : "password";
    $("toggle-password").textContent = visible ? "隐藏" : "显示";
    $("toggle-password").setAttribute("aria-label", visible ? "隐藏密码" : "显示密码");
    $("toggle-password").setAttribute("aria-pressed", String(visible));
  });
  function selectSchool(school) {
    selectedSchool = {school_id:String(school.school_id),school_name:school.school_name};
    $("school-search").value = school.school_name;
    $("school-results").hidden = true;
    $("school-search-message").textContent = "已匹配学校，可以填写账号登录。";
    refreshSummary(); persist();
  }
  async function resolveSchool() {
    const name = $("school-search").value.trim();
    const version = accountVersion;
    $("school-results").hidden = true;
    if (!name) { $("school-search").focus(); throw new Error("请先输入学校名称。"); }
    $("school-search-message").textContent = "正在搜索学校…";
    try {
      const result = await request(`/api/search_school?school_name=${encodeURIComponent(name)}`);
      if (version !== accountVersion) return null;
      const schools = (Array.isArray(result.schools) ? result.schools : []).filter(school => school && school.school_id != null && String(school.school_id).trim() && typeof school.school_name === "string" && school.school_name.trim());
      $("school-results").replaceChildren();
      const exact = schools.filter(school => school.school_name.trim() === name);
      const match = exact.length === 1 ? exact[0] : (schools.length === 1 ? schools[0] : null);
      if (match) { selectSchool(match); return selectedSchool; }
      for (const school of schools) {
        const button = document.createElement("button"); button.type = "button"; button.textContent = school.school_name;
        button.addEventListener("click", () => { resetAccountState(); selectSchool(school); });
        $("school-results").append(button);
      }
      $("school-results").hidden = schools.length === 0;
      if (!schools.length) throw new Error("未找到学校，请确认学校名称后重试。");
      $("school-search-message").textContent = `找到 ${schools.length} 所学校，请点击选择。`;
      return null;
    } catch (error) { if (version !== accountVersion) return null; $("school-search-message").textContent = error.message; throw error; }
  }
  $("search-school").addEventListener("click", () => withButton("search-school", "搜索中", async () => {
    try { await resolveSchool(); }
    catch (error) { $("school-search-message").textContent = error.message; }
  }));
  $("school-search").addEventListener("keydown", event => { if (event.key === "Enter") { event.preventDefault(); $("search-school").click(); } });
  $("login-form").addEventListener("submit", event => {
    event.preventDefault();
    if (!$("login-form").reportValidity()) return;
    withButton("login-button", "正在连接…", async () => {
      const version = accountVersion;
      showMessage("login-message", "");
      try {
        const account = accountForAction(), password = $("password").value;
        if (!account || !$("school-search").value.trim() || !password) throw new Error("请完整填写学校名称、账号和密码。");
        const school = selectedSchool || await resolveSchool();
        if (version !== accountVersion) return;
        if (!school) throw new Error("请从搜索结果中选择学校，再连接账号。");
        await request("/api/login", {method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({account,password,school_id:school.school_id,school_name:school.school_name})});
        if (version !== accountVersion) return;
        loggedInAccount = account; $("password").value = ""; persist(); refreshSummary();
        showMessage("login-message", "账号连接成功。");
        navigate("records-panel");
        // A previous account's query may still own the disabled refresh button.
        await queryTask;
        if (version === accountVersion) $("query-button").click();
      } catch (error) { if (version !== accountVersion) return; loggedInAccount = ""; refreshSummary(); showMessage("login-message", error.message, true); }
    });
  });
  function refreshRecords(month = calendarMonth) {
    if ($("query-button").disabled) return queryTask;
    queryTask = withButton("query-button", "查询中…", async () => {
    const version = accountVersion;
    setCalendarBusy(true);
    showMessage("query-message", "");
    try {
      const account = accountForAction();
      const result = await request(`/api/query?account=${encodeURIComponent(account)}&month_date=${encodeURIComponent(month)}`);
      if (version !== accountVersion) return;
      if (result.data?.month_date && result.data.month_date !== month) throw new Error("平台返回的月份与查询不一致，请重新刷新。");
      renderData(result.data || {}); loggedInAccount = account; refreshSummary();
      const time = new Date().toLocaleTimeString("zh-CN", {timeZone:"Asia/Shanghai",hour:"2-digit",minute:"2-digit"});
      $("records-footnote").textContent = `${calendarMonth} 签到情况 · 更新于 ${time} · 来源：习讯云`;
      persist();
    } catch (error) {
      if (version === accountVersion) {
        showMessage("query-message", error.message, true);
        if (!$("records-content").hidden) $("records-footnote").textContent = `${calendarMonth} · 本次刷新未成功 · 以下为上次查询结果。`;
      }
    } finally { setCalendarBusy(false); }
    });
    return queryTask;
  }
  $("query-button").addEventListener("click", () => refreshRecords());
  $("calendar-prev").addEventListener("click", () => refreshRecords(window.WorkspaceCalendar.shiftMonth(calendarMonth, -1)));
  $("calendar-next").addEventListener("click", () => refreshRecords(window.WorkspaceCalendar.shiftMonth(calendarMonth, 1)));
  $("calendar-current").addEventListener("click", () => refreshRecords(window.WorkspaceCalendar.currentMonth()));
  $("sign-form").addEventListener("submit", event => {
    event.preventDefault();
    if (window.WorkspaceAutoSign?.isDaily()) { window.WorkspaceAutoSign.submit(); return; }
    if (signSubmitting) { showMessage("sign-message", "签到正在提交，请稍候。"); return; }
    if (!$("sign-form").reportValidity()) return;
    showMessage("sign-message", "");
    try {
      const account = accountForAction();
      if (!window.WorkspaceLocations) throw new Error("地点确认模块尚未加载，请刷新后重试。");
      if (!window.WorkspaceSign) throw new Error("签到表单尚未加载，请刷新后重试。");
      pendingSign = {account,...window.WorkspaceLocations.confirmedLocation(), details:window.WorkspaceSign.snapshot()};
      $("sign-confirm-text").textContent = `账号：${pendingSign.account}\n地点：${pendingSign.address}${pendingSign.address_name ? `\n名称：${pendingSign.address_name}` : ""}\n备注：${window.WorkspaceSign.remarkLabel()}${pendingSign.details.comment ? `\n说明：${pendingSign.details.comment}` : ""}\n照片：${pendingSign.details.photos.length} 张`;
      $("sign-confirm").returnValue = "";
      $("sign-confirm").showModal();
    } catch (error) { showMessage("sign-message", error.message, true); }
  });
  $("sign-confirm").addEventListener("cancel", () => { pendingSign = null; $("sign-confirm").returnValue = "cancel"; });
  function invalidateSign() {
    pendingSign = null;
    if ($("sign-confirm").open) $("sign-confirm").close("cancel");
  }
  window.addEventListener("workspace-location-changed", invalidateSign);
  window.addEventListener("workspace-sign-details-changed", invalidateSign);
  $("sign-success").addEventListener("close", () => {
    if ($("sign-success").returnValue !== "reset" && !$("sign-panel").hidden && platformUserID) $("sign-button").focus({preventScroll:true});
  });
  $("sign-confirm").addEventListener("close", () => {
    const data = pendingSign; pendingSign = null;
    if ($("sign-confirm").returnValue !== "confirm" || !data || signSubmitting) return;
    signSubmitting = true;
    window.WorkspaceSign.setBusy(true);
    withButton("sign-button", "提交中…", async () => {
      const version = accountVersion;
      try {
        const current = window.WorkspaceLocations.confirmedLocation();
        if (data.account !== accountForAction() || Object.entries(current).some(([key,value]) => data[key] !== value)) throw new Error("地点信息已变更，请重新确认后提交。");
        const details = await window.WorkspaceSign.upload(data.details, data.account, version);
        const checked = window.WorkspaceLocations.confirmedLocation();
        if (data.account !== accountForAction() || Object.entries(checked).some(([key,value]) => data[key] !== value)) throw new Error("地点信息已变更，请重新确认后提交。");
        window.WorkspaceSign.validate(data.details, version);
        const {details:localDetails, ...locationData} = data;
        const result = await request("/api/sign", {method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({...locationData,...details})});
        if (version !== accountVersion) return;
        persist(); window.WorkspaceSign.clearPhotos(); showMessage("sign-message", "");
        const message = result.message?.trim();
        $("sign-success-message").textContent = message && !["签到成功", "签到提交成功"].includes(message) ? message : "本次签到已完成，可在签到记录中查看。";
        $("sign-success").returnValue = "";
        $("sign-success").showModal();
      } catch (error) { if (version === accountVersion) showMessage("sign-message", error.message, true); }
      finally { signSubmitting = false; window.WorkspaceSign.setBusy(false); }
    });
  });
  $("clear-config").addEventListener("click", () => {
    resetAccountState();
    try { localStorage.removeItem(storageKey); } catch { /* Session fields can still be cleared. */ }
    $("login-form").reset(); $("sign-form").reset(); $("account").value = platformStudentNumber; selectedSchool = null; loggedInAccount = ""; refreshSummary(); $("school-results").hidden = true; $("school-results").replaceChildren();
    $("password").type = "password"; $("toggle-password").textContent = "显示";
    $("toggle-password").setAttribute("aria-label", "显示密码"); $("toggle-password").setAttribute("aria-pressed", "false");
    $("school-search-message").textContent = "输入学校名称，连接时自动匹配；也可搜索后选择学校。";
    for (const id of ["login-message","query-message","sign-message"]) showMessage(id, "");
    toast("已清除浏览器配置，服务器上的账号数据仍保留。");
    window.dispatchEvent(new CustomEvent("workspace-config-cleared"));
  });
  request("/api/health").then(() => { $("service-status").classList.add("online"); $("service-status").replaceChildren(Object.assign(document.createElement("i"), {ariaHidden:"true"}), document.createTextNode("服务正常")); }).catch(() => { $("service-status").classList.add("offline"); $("service-status").replaceChildren(Object.assign(document.createElement("i"), {ariaHidden:"true"}),document.createTextNode("服务未连接")); });
  window.Workspace = {request, account:accountForAction, version:() => accountVersion, isSigning:() => signSubmitting, toast, setSession, userID:() => platformUserID};
})();
