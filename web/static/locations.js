"use strict";
(() => {
  const $ = id => document.getElementById(id);
  let map, marker, layers, SDK, sdkPromise, sdkKey = "", sdkSequence = 0;
  let applied = null, draft = null, previewPending = false, previewPoint = null, searchedPlace = false;
  let searchVersion = 0, selectionVersion = 0, connectionVersion = 0;
  let config = null;
  const zoomRange = [2, 20];
  let wheelDelta = 0, wheelTime = 0, wheelZoomTime = 0;

  function updateZoomControls() {
    const ready = !!map && $("map-empty").hidden;
    $("map-zoom-controls").hidden = !ready;
    const zoom = ready ? map.getZoom() : null;
    $("map-zoom-in").disabled = !ready || zoom >= zoomRange[1];
    $("map-zoom-out").disabled = !ready || zoom <= zoomRange[0];
  }
  function changeZoom(delta) {
    if (!map || !$("map-empty").hidden) return;
    const zoom = Math.min(zoomRange[1], Math.max(zoomRange[0], map.getZoom() + delta));
    map.setZoom(zoom, true);
    updateZoomControls();
  }

  function feedback(message, error = false) {
    $("location-search-message").textContent = message;
    $("location-search-message").classList.toggle("error", error);
  }
  function coordinates(lon, lat) {
    if (lon == null || lat == null || String(lon).trim() === "" || String(lat).trim() === "") return null;
    const pair = [Number(lon), Number(lat)];
    return pair.every(Number.isFinite) && Math.abs(pair[0]) <= 180 && Math.abs(pair[1]) <= 90 ? pair : null;
  }
  function validPlace(place) {
    return place && typeof place.address === "string" && place.address.trim() && typeof place.confirmation_token === "string" && place.confirmation_token && coordinates(place.longitude, place.latitude);
  }
  function status(label, confirmed = false) {
    $("location-status").textContent = label;
    $("location-status").classList.toggle("confirmed", confirmed);
  }
  function invalidateSubmission() { window.dispatchEvent(new CustomEvent("workspace-location-changed")); }
  function showMarker(point, center = true) {
    if (!map) return;
    if (!point) { marker?.hide(); $("map-recenter").disabled = true; return; }
    if (!marker) {
      marker = new SDK.Marker({position:point, draggable:true, title:"拖动标记查看参考地点", map});
      marker.on("dragend", event => reversePoint(event.lnglat || marker.getPosition()));
    } else { marker.setPosition(point); marker.show(); }
    $("map-recenter").disabled = false;
    if (center) map.setZoomAndCenter(16, point);
  }
  function currentPoint() { return coordinates($("longitude").value, $("latitude").value); }
  function displayPoint() { return previewPending ? previewPoint : currentPoint(); }
  function updateNote() {
    $("location-note-title").textContent = applied ? (applied.name || "所选地点") : "请选择地点";
    $("location-note-text").textContent = applied ? applied.address : "搜索并确认实际签到地点。";
    $("coordinate-source").textContent = applied ? "高德 GCJ-02" : "本次确认的地点";
    $("sign-button").disabled = window.Workspace.isSigning() || !applied || previewPending;
  }
  function dismissPreview() {
    selectionVersion++;
    draft = null; previewPending = false; previewPoint = null;
    $("location-preview").hidden = true;
    $("location-use").disabled = true;
    $("location-results").querySelectorAll("button").forEach(button => button.setAttribute("aria-pressed", "false"));
    showMarker(currentPoint());
    status(applied ? "已选择地点" : "待选择地点", !!applied);
    updateNote();
  }
  function preview(place, selectedButton = null) {
    if (!validPlace(place)) { feedback("此地点缺少完整地址或坐标，请选择其他结果。", true); return; }
    selectionVersion++;
    draft = {...place}; previewPending = true;
    previewPoint = coordinates(place.longitude, place.latitude);
    invalidateSubmission();
    $("location-preview-name").textContent = place.name || "所选地点";
    $("location-preview-address").textContent = place.address;
    $("location-preview").hidden = false;
    $("location-use").disabled = false;
    $("location-results").querySelectorAll("button").forEach(button => button.setAttribute("aria-pressed", String(button === selectedButton)));
    showMarker(coordinates(place.longitude, place.latitude));
    status("候选地点待确认");
    feedback("核对地图与地址后使用此地点。");
    updateNote();
  }
  function apply(place) {
    applied = {...place};
    $("sign-address").value = place.address;
    $("sign-name").value = place.name || "";
    $("latitude").value = place.latitude;
    $("longitude").value = place.longitude;
    dismissPreview(); updateNote();
    status("已选择地点", true);
    $("location-search-panel").open = false;
    $("location-results").hidden = true;
    invalidateSubmission();
    feedback("");
  }
  function clearState() {
    searchVersion++; selectionVersion++;
    applied = null; draft = null; previewPending = false; previewPoint = null; searchedPlace = false;
    for (const id of ["sign-address", "sign-name", "latitude", "longitude"]) $(id).value = "";
    $("location-results").replaceChildren(); $("location-results").hidden = true;
    $("location-preview").hidden = true; $("location-use").disabled = true;
    $("location-search").value = "";
    marker?.hide(); $("map-recenter").disabled = true;
    map?.setZoomAndCenter(5, [104.0,35.0]);
    $("location-search-panel").open = true;
    status("待选择地点"); updateNote(); feedback("搜索并选择实际签到地点。");
    invalidateSubmission();
  }
  function validateSubmission() {
    if (previewPending) throw new Error("请先确认使用候选地点，或取消候选。");
    if (!searchedPlace || !validPlace(applied)) throw new Error("请先搜索、选择并确认完整地点。");
    if ($("sign-address").value !== applied.address || $("sign-name").value !== (applied.name || "") || $("latitude").value !== String(applied.latitude) || $("longitude").value !== String(applied.longitude)) throw new Error("地点信息已变更，请重新搜索并确认。");
  }
  function confirmedLocation() {
    validateSubmission();
    return {address:applied.address, address_name:applied.name || "", latitude:String(applied.latitude), longitude:String(applied.longitude), location_confirmed:true, confirmation_token:applied.confirmation_token};
  }
  function sourceLabel() { return "用户搜索并确认的参考地点（GCJ-02）"; }
  async function search() {
    const keyword = $("location-search").value.trim(), city = $("location-city").value.trim();
    if (!keyword) { feedback("请输入地点名称或详细地址。", true); $("location-search").focus(); return; }
    if ($("location-search-button").disabled) return;
    const version = ++searchVersion, owner = window.Workspace.version();
    const button = $("location-search-button"); button.disabled = true;
    button.querySelector("span").textContent = "搜索中";
    $("location-results").hidden = true; $("location-results").replaceChildren();
    feedback("正在查找地点…");
    try {
      const data = await window.Workspace.request("/api/locations/search", {method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({keyword,city})});
      if (version !== searchVersion || owner !== window.Workspace.version()) return;
      const places = (Array.isArray(data.locations) ? data.locations : []).filter(validPlace);
      places.forEach((place, index) => {
        const button = document.createElement("button"), number = document.createElement("span"), detail = document.createElement("div"), name = document.createElement("strong"), address = document.createElement("p");
        button.type = "button"; button.className = "location-result"; button.setAttribute("aria-pressed", "false");
        number.textContent = index + 1; name.textContent = place.name || "所选地点"; address.textContent = place.address;
        detail.append(name, address); button.append(number, detail);
        button.addEventListener("click", () => { searchedPlace = true; preview(place, button); }); $("location-results").append(button);
      });
      $("location-results").hidden = !places.length;
      feedback(places.length ? `找到 ${places.length} 个地点，选择后可在地图上核对。` : "未找到匹配地点，请补充城市、街道或建筑名称。");
    } catch (error) { if (version === searchVersion && owner === window.Workspace.version()) feedback(error.message, true); }
    finally { button.disabled = false; button.querySelector("span").textContent = "搜索"; }
  }
  async function reversePoint(lnglat) {
    if (!searchedPlace) { feedback("请先搜索并选择地点，再用地图调整参考位置。", true); return; }
    const point = coordinates(lnglat?.getLng?.(), lnglat?.getLat?.());
    if (!point) return;
    const version = ++selectionVersion, owner = window.Workspace.version();
    draft = null; previewPending = true; previewPoint = point; invalidateSubmission();
    $("location-preview").hidden = false; $("location-use").disabled = true;
    $("location-preview-name").textContent = "正在识别所选位置";
    $("location-preview-address").textContent = "核对地址后即可使用此地点。";
    showMarker(point, false); status("候选地点待确认"); feedback("正在识别地图位置…");
    updateNote();
    try {
      const result = await window.Workspace.request("/api/locations/reverse", {method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({longitude:point[0].toFixed(6),latitude:point[1].toFixed(6)})});
      if (version !== selectionVersion || owner !== window.Workspace.version()) return;
      if (!validPlace(result.location)) throw new Error("未找到完整地址，请重新选点或搜索地点。");
      preview(result.location);
    } catch (error) {
      if (version !== selectionVersion || owner !== window.Workspace.version()) return;
      $("location-preview-name").textContent = "暂时无法识别此位置";
      $("location-preview-address").textContent = "请取消候选后重试，或通过地址搜索选择地点。";
      feedback(error.message, true);
    }
  }
  function loadSDK(settings) {
    window._AMapSecurityConfig = {serviceHost:location.origin + settings.security_proxy};
    if (sdkKey && sdkKey !== settings.js_key) return Promise.reject(new Error("地图 Key 已更新，请刷新页面后连接。"));
    if (window.AMap) return Promise.resolve(window.AMap);
    if (sdkPromise) return sdkPromise;
    sdkKey = settings.js_key;
    sdkPromise = new Promise((resolve, reject) => {
      const script = document.createElement("script");
      const callback = `xixunMapReady${++sdkSequence}`;
      let settled = false;
      const timer = setTimeout(() => finish(new Error("地图连接超时，请重新连接。")), 15000);
      function finish(error) {
        if (settled) return;
        settled = true;
        clearTimeout(timer); script.onerror = null; window[callback] = () => {};
        if (error) { script.remove(); sdkPromise = null; sdkKey = ""; reject(error); }
        else resolve(window.AMap);
      }
      window[callback] = () => finish(window.AMap ? null : new Error("地图暂时无法加载。"));
      script.onerror = () => finish(new Error("地图连接失败，请检查网络后重新连接。"));
      script.async = true;
      script.src = `https://webapi.amap.com/maps?v=2.0&key=${encodeURIComponent(settings.js_key)}&callback=${callback}`;
      document.head.append(script);
    });
    return sdkPromise;
  }
  function mapEmpty(title, description) {
    $("map-empty").hidden = false;
    $("map-empty-title").textContent = title; $("map-empty-description").textContent = description;
    $("map-state").textContent = title;
    for (const id of ["map-standard", "map-satellite", "map-recenter", "map-zoom-in", "map-zoom-out"]) $(id).disabled = true;
    $("map-zoom-controls").hidden = true;
  }
  async function connectMap() {
    if ($("map-retry").disabled) return;
    const version = ++connectionVersion;
    $("map-retry").disabled = true; $("map-frame").setAttribute("aria-busy", "true");
    mapEmpty("正在连接高德地图", "稍候即可查看地点与周边环境。");
    try {
      config = await window.Workspace.request("/api/maps/config");
      if (version !== connectionVersion) return;
      if (!$("location-city").value && config.default_city) $("location-city").value = config.default_city;
      if (!config.map_enabled) {
        map?.destroy(); map = marker = layers = null;
        mapEmpty("高德地图尚未连接", config.search_enabled ? "地址搜索仍可使用，选择并确认后即可填写签到信息。" : "请完成高德配置后搜索并确认地点。");
        return;
      }
      SDK = await loadSDK(config);
      if (version !== connectionVersion) return;
      map?.destroy(); marker = null;
      layers = {standard:new SDK.TileLayer(), satellite:new SDK.TileLayer.Satellite(), roads:new SDK.TileLayer.RoadNet()};
      const point = displayPoint();
      map = new SDK.Map("location-map", {viewMode:"2D",zoom:point?16:5,zooms:zoomRange,center:point || [104.0,35.0],layers:[layers.standard],zoomEnable:true,scrollWheel:false,touchZoom:true,doubleClickZoom:true});
      map.on("zoomchange", updateZoomControls);
      map.on("click", event => reversePoint(event.lnglat));
      const activeMap = map;
      await new Promise((resolve, reject) => {
        const timer = setTimeout(() => reject(new Error("底图加载超时，请检查网络与地图配置后重新连接。")), 15000);
        activeMap.on("complete", () => { clearTimeout(timer); resolve(); });
        activeMap.on("error", () => { clearTimeout(timer); reject(new Error("底图加载失败，请重新连接。")); });
        resizeMap();
      });
      if (version !== connectionVersion) return;
      $("map-empty").hidden = true;
      updateZoomControls();
      $("map-state").textContent = config.search_enabled ? "高德地图已连接 · 点击地图可选点" : "高德地图已连接 · 地点搜索待连接";
      for (const id of ["map-standard", "map-satellite"]) $(id).disabled = false;
      $("map-standard").setAttribute("aria-pressed", "true"); $("map-satellite").setAttribute("aria-pressed", "false");
      showMarker(displayPoint()); resizeMap();
    } catch (error) { map?.destroy(); map = marker = layers = null; mapEmpty("高德地图暂时不可用", error.message); }
    finally { $("map-retry").disabled = false; $("map-frame").setAttribute("aria-busy", "false"); }
  }
  function resizeMap() { if (map && !$("sign-panel").hidden) requestAnimationFrame(() => map?.resize()); }
  $("location-search-button").addEventListener("click", search);
  for (const id of ["location-search", "location-city"]) {
    $(id).addEventListener("keydown", event => { if (event.key === "Enter") { event.preventDefault(); search(); } });
    $(id).addEventListener("input", () => { searchVersion++; $("location-results").hidden = true; feedback("搜索并选择实际签到地点。"); });
  }
  $("location-use").addEventListener("click", () => { if (searchedPlace && validPlace(draft)) apply(draft); });
  $("location-cancel").addEventListener("click", () => { dismissPreview(); feedback("候选已取消，签到信息保持为当前填写的地点。"); });
  for (const id of ["sign-address", "sign-name", "latitude", "longitude"]) $(id).addEventListener("input", () => {
    applied = null; dismissPreview(); updateNote(); status("地点已变更 · 请重新确认"); invalidateSubmission();
  });
  for (const [id, satellite] of [["map-standard", false],["map-satellite",true]]) $(id).addEventListener("click", () => {
    if (!map) return;
    map.setLayers(satellite ? [layers.satellite,layers.roads] : [layers.standard]);
    $("map-standard").setAttribute("aria-pressed", String(!satellite)); $("map-satellite").setAttribute("aria-pressed", String(satellite));
  });
  $("map-recenter").addEventListener("click", () => showMarker(displayPoint()));
  $("map-zoom-in").addEventListener("click", () => changeZoom(1));
  $("map-zoom-out").addEventListener("click", () => changeZoom(-1));
  // Handle wheel input here to give mice and high-resolution trackpads one
  // consistent zoom path without competing with the SDK's wheel handler.
  $("map-frame").addEventListener("wheel", event => {
    if (!map || !$("map-empty").hidden || !event.deltaY) return;
    event.preventDefault();
    event.stopPropagation();
    const now = performance.now();
    if (now - wheelTime > 200 || Math.sign(wheelDelta) !== Math.sign(event.deltaY)) wheelDelta = 0;
    wheelTime = now;
    const unit = event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? $("map-frame").clientHeight : 1;
    wheelDelta += event.deltaY * unit;
    if (Math.abs(wheelDelta) < 40 || now - wheelZoomTime < 100) return;
    changeZoom(wheelDelta < 0 ? 1 : -1);
    wheelDelta = 0;
    wheelZoomTime = now;
  }, {capture:true, passive:false});
  $("map-retry").addEventListener("click", connectMap);
  $("location-adjust").addEventListener("click", () => {
    $("location-search-panel").open = !$("location-search-panel").open;
    if ($("location-search-panel").open) $("location-search").focus({preventScroll:true});
    resizeMap();
  });
  $("location-search-panel").addEventListener("toggle", () => { $("location-adjust").setAttribute("aria-expanded", String($("location-search-panel").open)); resizeMap(); });
  window.addEventListener("workspace-view-changed", event => { if (event.detail === "sign-panel") { if (!config) connectMap(); resizeMap(); } });
  window.addEventListener("resize", resizeMap);
  new ResizeObserver(resizeMap).observe($("map-frame"));
  window.addEventListener("workspace-account-changed", () => clearState());
  window.addEventListener("workspace-config-cleared", clearState);
  window.WorkspaceLocations = {confirmedLocation,validateSubmission,sourceLabel};
  clearState();
  if (!$("sign-panel").hidden) connectMap();
})();
