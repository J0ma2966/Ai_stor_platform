// У сайта нет своего service worker. Если на этом origin зарегистрирован чужой
// (например, PWA другого проекта, открытого раньше на localhost:3000), он отдаёт
// старые JS-чанки из Cache Storage. Новые модули в них не находятся, и React падает
// с «Cannot read properties of undefined (reading 'call')» на обычном F5;
// Ctrl+F5 обходит service worker, поэтому помогает. Скрипт снимает такие
// регистрации, чистит их Cache Storage и один раз перезагружает страницу,
// если она была под их управлением. Без регистраций ничего не делает.
export const STALE_WORKER_SCRIPT =
  "(function(){var n=navigator.serviceWorker;if(!n||!n.getRegistrations)return;" +
  "n.getRegistrations().then(function(rs){if(!rs.length)return;" +
  "return Promise.all(rs.map(function(r){return r.unregister()}))" +
  ".then(function(){return window.caches?caches.keys().then(function(ks){return Promise.all(ks.map(function(k){return caches.delete(k)}))}):0})" +
  ".then(function(){if(n.controller)location.reload()})}).catch(function(){})})()";
