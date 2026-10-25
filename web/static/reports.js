"use strict";
(() => {
  const workspace = window.Workspace;
  const names = {week:"周报", month:"月报"};
  const controllers = new Map();
  const clearDrafts = document.createElement("button");
  clearDrafts.id = "clear-report-drafts"; clearDrafts.type = "button"; clearDrafts.className = "text-button";
  clearDrafts.textContent = "清除本地报告草稿";
  document.getElementById("clear-config").after(clearDrafts);
  clearDrafts.addEventListener("click", () => {
    const userID = String(workspace.userID()), version = workspace.version();
    if (!/^[1-9]\d*$/.test(userID)) { workspace.toast("请先登录后清除当前用户的本地报告草稿。", true); return; }
    if ([...controllers.values()].some(state => state.submitting || state.creating)) { workspace.toast("请等待报告操作结束后再清除草稿。", true); return; }
    if (!window.confirm("清除当前用户在此浏览器所有账号与学校的周报、月报草稿及恢复副本，无法撤销。请先下载需要保留的草稿。已提交到服务器的报告、每日任务和浏览器配置不受影响。")) return;
    if (String(workspace.userID()) !== userID || workspace.version() !== version) return;
    try {
      const pattern = new RegExp(`^xixun-report-draft:${userID}:[^:]*:[^:]*:(week|month)(:recovery)?$`);
      const keys = Array.from({length: localStorage.length}, (_, index) => localStorage.key(index)).filter(key => key !== null && pattern.test(key));
      for (const key of keys) localStorage.removeItem(key);
      pending = null; if (confirm.open) confirm.close("cancel");
      for (const state of controllers.values()) {
        state.template = []; state.files = []; state.orphaned = [];
        state.$("form").reset(); state.$("fields").replaceChildren(); state.$("file-list").replaceChildren();
        state.$("draft-status").textContent = ""; state.$("editor").hidden = true; state.panel.querySelector(".report-list-view").hidden = false;
      }
      workspace.toast("已清除当前用户在此浏览器的报告草稿和恢复副本，已提交到服务器的报告不受影响。");
    } catch { workspace.toast("未能完整清除本地报告草稿，请检查浏览器存储权限后重试。", true); }
  });
  const confirm = document.getElementById("report-confirm");
  const detail = document.getElementById("report-detail");
  let pending = null, detailVersion = 0, downloadText = "";
  function message(node, text, error = false) {
    node.textContent = text; node.hidden = !text; node.classList.toggle("error", error);
  }
  function element(tag, text, className) {
    const node = document.createElement(tag); if (text != null) node.textContent = text;
    if (className) node.className = className; return node;
  }
  function timestamp(value, dateOnly = false) {
    if (!value || value === "0") return "—";
    const numeric = Number(value);
    const date = Number.isFinite(numeric) ? new Date(numeric < 1e12 ? numeric * 1000 : numeric) : new Date(value);
    if (Number.isNaN(date.getTime())) return String(value);
    return dateOnly ? date.toLocaleDateString("zh-CN") : date.toLocaleString("zh-CN", {hour12:false});
  }
  function rating(row) { return {1:"优秀",2:"良好",3:"及格",4:"不及格"}[row.score] || (String(row.state) === "1" ? "已批阅" : "未批阅"); }
  function draftKey(kind) { return `xixun-report-draft:${window.Workspace.userID()}:${encodeURIComponent(document.getElementById("account").value.trim())}:${encodeURIComponent(document.getElementById("school-search").value.trim())}:${kind}`; }
  function current(version) { return version === workspace.version(); }
  function storedDraft(key) {try {return localStorage.getItem(key);} catch {return null;}}
  function validDates(start, end, optional = false) {
    if (optional && !start && !end) return;
    if (!start || !end || end < start) throw new Error("请填写起止日期，结束日期必须不早于开始日期。");
  }
  function fileURL(uri) {
    try {const url = new URL(uri, "https://file.xixunyun.com/"); if (url.protocol === "https:" || url.protocol === "http:") return url.href;} catch {}
    return null;
  }

  async function openDetail(kind, id) {
    const version = workspace.version(), request = ++detailVersion;
    document.getElementById("report-detail-title").textContent = `实习${names[kind]}详情`;
    document.getElementById("report-detail-body").replaceChildren();
    document.getElementById("report-download").hidden = true; downloadText = "";
    message(document.getElementById("report-detail-message"), "正在读取报告…");
    if (!detail.open) detail.showModal();
    try {
      const params = new URLSearchParams({account:workspace.account(), id});
      const result = await workspace.request(`/api/reports/detail?${params}`);
      if (!current(version) || request !== detailVersion || !detail.open) return;
      const data = result.data || {}, body = document.getElementById("report-detail-body");
      const summary = element("dl", null, "report-detail-summary");
      const values = [["姓名",data.student_name || "—"],["报告日期",`${timestamp(data.start_date,true)} 至 ${timestamp(data.end_date,true)}`],["提交时间",timestamp(data.create_time)],["批阅结果",rating(data)],["批阅时间",timestamp(data.mark_time)],["批阅老师",data.teacher_name || "—"],["已打回",String(data.repulsed) === "1" ? "是" : "否"]];
      downloadText = `实习${names[kind]}\n`;
      for (const [label,value] of values) {summary.append(element("dt",label),element("dd",value)); downloadText += `${label}：${value}\n`;}
      body.append(summary);
      let content = data.content;
      if (typeof content === "string") {try {content = JSON.parse(content);} catch {content = [{title:"报告内容",content}];}}
      if (!Array.isArray(content)) content = [];
      for (const field of content) {
        if (!field || typeof field !== "object") continue;
        body.append(element("h3",field.title || "报告内容"),element("p",field.content || "未填写","report-detail-text"));
        downloadText += `\n${field.title || "报告内容"}\n${field.content || "未填写"}\n`;
      }
      body.append(element("h3","批阅评语"),element("p",data.mark_result || "暂无评语","report-detail-text"));
      downloadText += `\n批阅评语\n${data.mark_result || "暂无评语"}\n`;
      const attachments = Array.isArray(data.attachment) ? data.attachment : String(data.attachment || "").split(",");
      const list = element("ul",null,"report-files");
      for (const uri of attachments.filter(value => typeof value === "string" && value.trim())) {
        const href = fileURL(uri); if (!href) continue;
        const link = element("a", decodeName(uri)); link.href = href; link.target = "_blank"; link.rel = "noopener noreferrer";
        const item = element("li"); item.append(link); list.append(item); downloadText += `附件：${href}\n`;
      }
      if (list.children.length) body.append(element("h3","附件"),list);
      message(document.getElementById("report-detail-message"), "");
      document.getElementById("report-download").hidden = false;
    } catch (error) {if (current(version) && request === detailVersion) message(document.getElementById("report-detail-message"),error.message,true);}
  }
  function decodeName(uri) {try {return decodeURIComponent(uri.split("/").pop().split("?")[0]) || "查看附件";} catch {return "查看附件";}}
  detail.addEventListener("close", () => {detailVersion++; downloadText = "";});
  document.getElementById("report-download").addEventListener("click", () => {
    if (!downloadText) return;
    const url = URL.createObjectURL(new Blob([downloadText],{type:"text/plain;charset=utf-8"}));
    const link = element("a"); link.href = url; link.download = `${document.getElementById("report-detail-title").textContent}.txt`;
    link.click(); setTimeout(() => URL.revokeObjectURL(url),1000);
  });

  for (const kind of ["week","month"]) {
    const panel = document.getElementById(`${kind}-panel`);
    panel.querySelector(".report-workspace").append(document.getElementById("report-layout").content.cloneNode(true));
    const $ = role => panel.querySelector(`[data-role="${role}"]`);
    panel.querySelectorAll("[data-label]").forEach(label => {const input = $(label.dataset.label); input.id = `${kind}-${label.dataset.label}`; label.htmlFor = input.id;});
    $("new").textContent = `新建${names[kind]}`; $("submit").textContent = `提交${names[kind]}`; $("editor-title").textContent = `新建实习${names[kind]}`;
    const state = {kind, $, panel, loaded:false, page:1, pages:1, request:0, template:[], files:[], orphaned:[], creating:false, submitting:false};
    controllers.set(kind,state);
    function content() {return state.template.map((field,index) => ({...field, content:$("fields").querySelectorAll("textarea")[index]?.value.trim() || ""}));}
    function saveDraft() {
      if (!state.template.length || !document.getElementById("account").value.trim()) return;
      const draft = {start:$("start").value,end:$("end").value,content:content(),orphaned:state.orphaned,attachments:state.files.filter(file => file.uri).map(({name,uri}) => ({name,uri})),pendingNames:state.files.filter(file => !file.uri).map(file => file.name)};
      try {localStorage.setItem(draftKey(kind),JSON.stringify(draft)); $("draft-status").textContent = state.files.some(file => !file.uri) ? "草稿已保存；重新打开页面时需重新选择未上传的附件。" : "草稿已保存到此浏览器。";}
      catch {$("draft-status").textContent = "浏览器未允许保存草稿，请保持本页打开。";}
    }
    state.saveDraft = saveDraft;
    function renderFiles() {
      $("file-list").replaceChildren();
      state.files.forEach((file,index) => {
        const item = element("li"), text = element("span",file.name + (file.uri ? " · 已上传" : " · 待上传")), remove = element("button","移除","text-button"); remove.type = "button"; remove.disabled = state.submitting; remove.setAttribute("aria-label",`移除附件 ${file.name}`);
        remove.addEventListener("click", () => {state.files.splice(index,1);renderFiles();saveDraft();});
        item.append(text,remove); $("file-list").append(item);
      });
    }
    state.renderFiles = renderFiles;
    function renderList(data) {
      const list = Array.isArray(data.list) ? data.list.filter(row => row && typeof row === "object") : [];
      $("rows").replaceChildren();
      for (const row of list) {
        const tr = element("tr");
        const cells = [["提交时间",timestamp(row.create_time)],["批阅时间",timestamp(row.mark_time)],["批阅结果",rating(row)],["批阅老师",row.teacher_name || "—"],["已打回",String(row.repulsed) === "1" ? "是" : "否"]];
        for (const [label,value] of cells) {const td = element("td",value);td.dataset.label = label;tr.append(td);}
        const action = element("td"), button = element("button","查看","button secondary"); action.dataset.label = "操作"; button.type = "button";
        button.disabled = row.id == null || String(row.id).trim() === ""; button.addEventListener("click", () => openDetail(kind,String(row.id))); action.append(button); tr.append(action); $("rows").append(tr);
      }
      const count = Math.max(0,Number(data.count) || list.length); state.pages = Math.max(1,Math.ceil(count/15));
      $("empty").textContent = list.length ? "" : `暂无符合条件的${names[kind]}，可调整筛选条件或新建报告。`; $("empty").hidden = list.length > 0;
      $("table-wrap").hidden = !list.length; $("pagination").hidden = false; $("count").textContent = `共 ${count} 条`; $("page").textContent = `${state.page} / ${state.pages}`;
      $("previous").disabled = state.page <= 1; $("next").disabled = state.page >= state.pages;
    }
    async function load(page = 1) {
      const version = workspace.version(), request = ++state.request;
      $("search").disabled = true; $("previous").disabled = true; $("next").disabled = true;
      message($("list-message"),"正在查询报告…");
      try {
        validDates($("filter-start").value,$("filter-end").value,true);
        const params = new URLSearchParams({account:workspace.account(),business_type:kind,page:String(page),page_size:"15",start_time:$("filter-start").value,end_time:$("filter-end").value,mark_state:$("filter-status").value});
        const result = await workspace.request(`/api/reports?${params}`);
        if (!current(version) || request !== state.request) return;
        state.page = page; state.loaded = true; renderList(result.data || {}); message($("list-message"),"");
      } catch (error) {if (current(version) && request === state.request) message($("list-message"),error.message,true);}
      finally {if (request === state.request) {$("search").disabled = false; $("previous").disabled = state.page <= 1; $("next").disabled = state.page >= state.pages;}}
    }
    state.load = load;
    panel.querySelector(".report-filters").addEventListener("submit",event => {event.preventDefault();load();});
    $("previous").addEventListener("click", () => load(state.page-1)); $("next").addEventListener("click", () => load(state.page+1));
    $("new").addEventListener("click", async () => {
      if (state.creating || state.submitting) return;
      const version = workspace.version(); state.creating = true; $("new").disabled = true;
      message($("list-message"),"正在读取学校报告模板…");
      try {
        const params = new URLSearchParams({account:workspace.account(),business_type:kind});
        const result = await workspace.request(`/api/reports/template?${params}`);
        if (!current(version)) return;
        if (!Array.isArray(result.data) || !result.data.length) throw new Error("学校尚未配置报告模板，请在习讯云确认。");
        state.template = result.data; state.files = []; $("form").reset(); $("fields").replaceChildren();
        let draft = null;
        try {draft = JSON.parse(localStorage.getItem(draftKey(kind)) || "null");} catch {}
        const savedFields = (Array.isArray(draft?.content) ? draft.content : []).filter(field => field && typeof field.title === "string" && typeof field.content === "string");
        const used = new Set();
        state.template.forEach((field,index) => {
          const container = element("div",null,"field"), label = element("label",field.title), required = String(field.require) === "1", hint = element("span",required ? "（必填）" : "（选填）",required ? "required" : "optional"), input = element("textarea");
          input.id = `${kind}-content-${index}`; input.rows = 6; input.required = required; input.maxLength = 50000; input.placeholder = "填写实际实习工作与体会";label.htmlFor = input.id;label.append(hint);
          const candidates = savedFields.filter(item => item.title === field.title && !used.has(item));
          const saved = candidates.length === 1 ? candidates[0] : candidates.find(item => item.sort === field.sort);
          if (saved) {input.value = saved.content;used.add(saved);}
          container.append(label,input); $("fields").append(container);
        });
        state.orphaned = [...(Array.isArray(draft?.orphaned) ? draft.orphaned : []),...savedFields.filter(field => !used.has(field))].filter(field => field && typeof field.title === "string" && typeof field.content === "string" && field.content.trim());
        $("old-fields").replaceChildren();
        for (const field of state.orphaned) $("old-fields").append(element("h3",field.title),element("p",field.content,"report-detail-text"));
        $("old-content").hidden = !state.orphaned.length;
        if (draft && typeof draft === "object") {
          if (typeof draft.start === "string") $("start").value = draft.start;
          if (typeof draft.end === "string") $("end").value = draft.end;
          state.files = (Array.isArray(draft.attachments) ? draft.attachments : []).filter(file => file && typeof file.name === "string" && typeof file.uri === "string" && fileURL(file.uri)).slice(0,3);
        }
        renderFiles(); $("draft-status").textContent = draft?.pendingNames?.length ? "已恢复草稿；请重新选择尚未上传的附件。" : draft ? "已恢复此账号的报告草稿。" : "草稿保存在此浏览器，按账号与报告类型分开。";
        panel.querySelector(".report-list-view").hidden = true; $("editor").hidden = false; message($("editor-message"),"");message($("list-message"),""); $("start").focus({preventScroll:true});
      } catch (error) {if (current(version)) message($("list-message"),error.message,true);}
      finally {if (current(version)) {state.creating = false; $("new").disabled = false;}}
    });
    $("save").addEventListener("click",saveDraft);
    $("form").addEventListener("input",saveDraft);
    $("back").addEventListener("click", () => {if (state.submitting) return; saveDraft(); $("editor").hidden = true;panel.querySelector(".report-list-view").hidden = false;});
    $("files").addEventListener("change", () => {
      const files = Array.from($("files").files);
      if (files.length + state.files.length > 3 || files.some(file => !file.size || file.size > 10*1024*1024)) {message($("editor-message"),"最多 3 个附件，每个需为 1 字节至 10 MB。",true);$("files").value = "";return;}
      state.files.push(...files.map(file => ({name:file.name,file,uri:""}))); $("files").value = ""; message($("editor-message"),"");renderFiles();saveDraft();
    });
    $("form").addEventListener("submit",event => {
      event.preventDefault();if (state.submitting || confirm.open || !state.template.length) return;
      message($("editor-message"),"");
      try {
        if (!$("form").reportValidity()) return;
        validDates($("start").value,$("end").value);
        const fields = content(); if (fields.some(field => String(field.require) === "1" && !field.content)) throw new Error("请填写必填的报告内容。");
        const input = {account:workspace.account(),business_type:kind,start_date:$("start").value,end_date:$("end").value,content:fields};
        saveDraft(); pending = {state,input,version:workspace.version(),key:draftKey(kind)};
        document.getElementById("report-confirm-title").textContent = `确认提交实习${names[kind]}`;
        document.getElementById("report-confirm-text").textContent = `账号：${input.account}\n日期：${input.start_date} 至 ${input.end_date}\n附件：${state.files.length} 个`;
        confirm.returnValue = "";confirm.showModal();
      } catch (error) {message($("editor-message"),error.message,true);}
    });
  }
  confirm.addEventListener("cancel", () => {pending = null;confirm.returnValue = "cancel";});
  confirm.addEventListener("close", async () => {
    const task = pending;pending = null;
    if (!task || confirm.returnValue !== "confirm" || !current(task.version)) return;
    const {state,input,version,key} = task, $ = state.$;
    if (state.submitting) return;
    state.submitting = true;const controls = Array.from($("form").querySelectorAll("input,textarea,button"));controls.forEach(node => node.disabled = true);$("back").disabled = true;
    message($("editor-message"),"正在上传附件并提交报告，请稍候…");
    try {
      for (const file of state.files) {
        if (!current(version)) return;
        if (file.uri) continue;
        const form = new FormData();form.set("account",input.account);form.set("file",file.file,file.name);
        const result = await workspace.request("/api/reports/attachments",{method:"POST",body:form});
        if (!current(version)) return;
        if (!result.uri) throw new Error("附件上传未返回地址，请重试。");
        file.uri = result.uri;state.saveDraft();
      }
      if (!current(version)) return;
      const submittedDraft = storedDraft(key);
      await workspace.request("/api/reports",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({...input,attachments:state.files.map(file => file.uri)})});
      // The original report may succeed after the user changes accounts.
      // Clear only the submitted draft; keep a newer draft using the same key.
      try {if (submittedDraft !== null && storedDraft(key) === submittedDraft) localStorage.removeItem(key);} catch {}
      if (!current(version)) return;
      $("form").reset();state.files = [];state.template = [];$("fields").replaceChildren();state.renderFiles();$("editor").hidden = true;state.panel.querySelector(".report-list-view").hidden = false;
      await state.load();if (current(version)) workspace.toast(`实习${names[state.kind]}提交成功。`);
    } catch (error) {if (current(version)) {state.saveDraft();message($("editor-message"),error.message,true);}}
    finally {if (current(version)) {state.submitting = false;controls.forEach(node => node.disabled = false);$("back").disabled = false;state.renderFiles();}}
  });
  function activeView() {
    for (const state of controllers.values()) if (!state.panel.hidden && !state.loaded && !state.submitting) state.load();
  }
  window.addEventListener("workspace-view-changed",activeView);
  window.addEventListener("workspace-session-changed", event => { if (event.detail) activeView(); });
  window.addEventListener("workspace-account-changed", () => {
    pending = null;detailVersion++; if (confirm.open) confirm.close("cancel");if (detail.open) detail.close();
    for (const state of controllers.values()) {
      state.request++;state.loaded = false;state.template = [];state.files = [];state.orphaned = [];state.creating = false;state.submitting = false;state.page = 1;state.pages = 1;
      const $ = state.$;$("form").reset();state.panel.querySelector(".report-filters").reset();$("rows").replaceChildren();$("fields").replaceChildren();$("file-list").replaceChildren();$("editor").hidden = true;state.panel.querySelector(".report-list-view").hidden = false;
      $("table-wrap").hidden = true;$("pagination").hidden = true;$("empty").hidden = false;$("empty").textContent = "连接账号后，可查询和填写报告。";
      state.panel.querySelectorAll("input,textarea,button,select").forEach(node => node.disabled = false); message($("list-message"),"");message($("editor-message"),"");
    }
  });
  activeView();
})();
