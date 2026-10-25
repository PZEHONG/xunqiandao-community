"use strict";
(() => {
  let scriptPromise;
  const scenes = new Map();
  const unavailable = () => new Error("人机验证暂时无法加载，请重试并检查网络。");
  function loadScript() {
    if (window.initGeetest4) return Promise.resolve();
    if (scriptPromise) return scriptPromise;
    scriptPromise = new Promise((resolve, reject) => {
      const script = document.createElement("script");
      const fail = () => { clearTimeout(timer); script.remove(); scriptPromise = null; reject(unavailable()); };
      const timer = setTimeout(fail, 12000);
      script.src = "https://static.geetest.com/v4/gt4.js";
      script.async = true;
      script.onload = () => { clearTimeout(timer); if (window.initGeetest4) resolve(); else fail(); };
      script.onerror = fail;
      document.head.appendChild(script);
    });
    return scriptPromise;
  }
  function instance(key, config) {
    const existing = scenes.get(key);
    if (existing?.id === config.captcha_id) return existing.ready;
    existing?.object?.destroy();
    const scene = {id:config.captcha_id, object:null, pending:null};
    scenes.set(key, scene);
    scene.ready = loadScript().then(() => new Promise((resolve, reject) => {
      let settled = false;
      const fail = () => {
        if (!settled) { settled = true; clearTimeout(timer); reject(unavailable()); }
        scene.pending?.reject(unavailable()); scene.pending = null;
        scene.object?.destroy(); scene.object = null;
        if (scenes.get(key) === scene) scenes.delete(key);
      };
      const timer = setTimeout(fail, 12000);
      window.initGeetest4({captchaId:scene.id,product:"bind",language:"zho",protocol:"https://",timeout:10000,onError:fail}, object => {
        if (scenes.get(key) !== scene) { object.destroy(); return; }
        scene.object = object;
        object.onReady(() => { if (!settled) { settled = true; clearTimeout(timer); resolve(scene); } });
        object.onSuccess(() => {
          const proof = object.getValidate(), pending = scene.pending;
          scene.pending = null;
          if (!pending) return;
          if (!proof) { pending.reject(new Error("请重新完成人机验证。")); return; }
          const {lot_number,captcha_output,pass_token,gen_time} = proof;
          pending.resolve({lot_number,captcha_output,pass_token,gen_time});
        });
        object.onClose(() => { scene.pending?.reject(new Error("已取消人机验证，点击发送验证码可重试。")); scene.pending = null; });
        object.onError(fail);
      });
    })).catch(error => { if (scenes.get(key) === scene) scenes.delete(key); throw error; });
    return scene.ready;
  }
  async function verify(key, config) {
    if (!config?.enabled) return undefined;
    if (!config.ready) throw new Error("人机验证尚未配置，请联系管理员。");
    const scene = await instance(key, config);
    let timer;
    try {
      return await new Promise((resolve, reject) => {
        timer = setTimeout(() => reject(new Error("人机验证已超时，请重试。")), 120000);
        scene.pending = {resolve:value => {clearTimeout(timer); resolve(value);},reject:error => {clearTimeout(timer); reject(error);}};
        scene.object.showCaptcha();
      });
    } finally { clearTimeout(timer); scene.pending = null; scene.object?.reset(); }
  }
  function prepare(config) {
    if (config?.enabled && config.ready) for (const key of ["register","reset"]) instance(key, config).catch(() => {});
  }
  window.GeeTestVerification = {verify, prepare};
})();
