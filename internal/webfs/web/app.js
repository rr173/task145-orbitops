// OrbitOps front-end. Minimal vanilla JS that drives the real Go backend:
// register satellite -> register station -> forecast contacts -> plan maneuver
// -> reconcile and compare next-windows. No business math lives here; all
// computation is performed server-side.
(function () {
  "use strict";
  var lastSat = localStorage.getItem("orbitops.lastSat") || "";
  var lastSta = localStorage.getItem("orbitops.lastSta") || "";

  function $(id) { return document.getElementById(id); }
  function json(r) { return r.text().then(function (t) { try { return JSON.parse(t); } catch (e) { return { raw: t }; } }); }
  function show(id, v) { $(id).textContent = typeof v === "string" ? v : JSON.stringify(v, null, 2); }

  function post(path, body) {
    return fetch(path, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) }).then(json);
  }
  function get(path) { return fetch(path).then(json); }

  function val(id, dflt) { var v = $(id).value.trim(); return v === "" ? dflt : v; }

  // 1. Register satellite
  $("btnSat").addEventListener("click", function () {
    var body = {
      name: $("satName").value || "sat",
      catalog: $("satCatalog").value || "0",
      a: parseFloat($("satA").value), e: parseFloat($("satE").value),
      i: parseFloat($("satI").value), raan: parseFloat($("satRaan").value),
      argp: parseFloat($("satArgp").value), m: parseFloat($("satM").value),
      epoch: parseInt($("satEpoch").value, 10)
    };
    post("/api/satellites", body).then(function (r) {
      show("satResult", r);
      if (r && r.ID) { lastSat = r.ID; localStorage.setItem("orbitops.lastSat", lastSat); }
    });
  });

  // 2. Register station
  $("btnStation").addEventListener("click", function () {
    var body = {
      name: $("stName").value || "station",
      lat: parseFloat($("stLat").value), lon: parseFloat($("stLon").value),
      alt_m: parseFloat($("stAlt").value), min_elevation: parseFloat($("stElev").value)
    };
    post("/api/groundstations", body).then(function (r) {
      show("stResult", r);
      if (r && r.ID) { lastSta = r.ID; localStorage.setItem("orbitops.lastSta", lastSta); }
    });
  });

  // 3. Forecast contacts
  function renderContacts(list) {
    var wrap = $("contactList");
    if (!list || !list.length) { wrap.innerHTML = "<p class='result'>无有效过境</p>"; return; }
    var rows = list.map(function (c) {
      return "<tr><td>" + c.SatelliteID + "</td><td>" + c.StationID + "</td><td>" + c.AOS +
        "</td><td>" + c.Los + "</td><td>" + c.TCA + "</td><td>" + c.MaxElevationDeg.toFixed(2) +
        "</td><td>" + c.AOSAzDeg.toFixed(1) + "</td><td>" + c.LosAzDeg.toFixed(1) +
        "</td><td>" + (c.Sunlit ? "☀" : "🌑") + "</td><td>" + c.Source + "</td></tr>";
    }).join("");
    wrap.innerHTML = "<table><thead><tr><th>卫星</th><th>地面站</th><th>AOS</th><th>LOS</th><th>TCA</th><th>峰值仰角°</th><th>AOS方位°</th><th>LOS方位°</th><th>光照</th><th>来源</th></tr></thead><tbody>" + rows + "</tbody></table>";
  }
  $("btnForecast").addEventListener("click", function () {
    var body = {
      satellite_id: val("fcSat", lastSat), station_id: val("fcSta", lastSta),
      start: parseInt($("fcStart").value, 10), duration: parseInt($("fcDur").value, 10),
      step: parseInt($("fcStep").value, 10)
    };
    post("/api/contacts/forecast", body).then(function (r) {
      if (r && r.error) { renderContacts([]); $("contactList").innerHTML = "<p class='result'>错误: " + r.message + "</p>"; return; }
      renderContacts(r);
    });
  });

  // 4. Maneuver
  $("btnManeuver").addEventListener("click", function () {
    var body = { satellite_id: val("mvSat", lastSat), type: $("mvType").value, exec_at: parseInt($("mvExec").value, 10) };
    post("/api/maneuvers", body).then(function (r) { show("mvResult", r); });
  });

  // 5. Reconcile
  $("btnReconcile").addEventListener("click", function () {
    post("/api/recompute", {}).then(function (r) { show("nwResult", "reconcile: " + JSON.stringify(r)); });
  });
  $("btnSnapshot").addEventListener("click", function () {
    get("/api/nextwindows").then(function (r) { show("nwResult", r); });
  });

  // 6. Constellation
  $("btnConst").addEventListener("click", function () {
    get("/api/report/constellation").then(function (rows) {
      var wrap = $("constList");
      if (!rows || !rows.length) { wrap.innerHTML = "<p class='result'>无卫星</p>"; return; }
      var trs = rows.map(function (r) {
        return "<tr><td>" + r.Satellite.ID + "</td><td>" + r.Satellite.Name + "</td><td>" + r.Satellite.Status +
          "</td><td>" + r.NextContactEpoch + "</td><td>" + r.NextContactStation +
          "</td><td>" + r.NextManeuverEpoch + "</td><td>" + r.NextManeuverType +
          "</td><td>" + r.OpenAlerts + "</td></tr>";
      }).join("");
      wrap.innerHTML = "<table><thead><tr><th>ID</th><th>名称</th><th>状态</th><th>下一过境</th><th>地面站</th><th>下一机动</th><th>类型</th><th>未关闭告警</th></tr></thead><tbody>" + trs + "</tbody></table>";
    });
  });

  // initial load
  get("/api/report/constellation").then(function () {}).catch(function () {});
})();
