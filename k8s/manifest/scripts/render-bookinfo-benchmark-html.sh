#!/usr/bin/env bash
set -euo pipefail

RUN_DIR="${1:-}"
if [[ -z "${RUN_DIR}" ]]; then
  echo "Usage: $0 <run-dir>"
  exit 1
fi

if [[ ! -d "${RUN_DIR}" ]]; then
  echo "Run directory does not exist: ${RUN_DIR}"
  exit 1
fi

node - "${RUN_DIR}" <<'NODE'
const fs = require("fs");
const path = require("path");

const runDir = process.argv[2];
const readJSON = (file, fallback) => {
  try {
    return JSON.parse(fs.readFileSync(file, "utf8"));
  } catch {
    return fallback;
  }
};

const summaryPath = path.join(runDir, "summary.json");
const samplesPath = path.join(runDir, "samples.json");
let summary = readJSON(summaryPath, null);
let samples = readJSON(samplesPath, []);

if (!summary) {
  const modes = fs.readdirSync(runDir).filter((name) => fs.statSync(path.join(runDir, name)).isDirectory());
  samples = modes.flatMap((mode) => {
    const modeDir = path.join(runDir, mode);
    const sampleDirs = fs.readdirSync(modeDir).filter((name) => fs.statSync(path.join(modeDir, name)).isDirectory());
    if (sampleDirs.length === 0 && fs.existsSync(path.join(modeDir, "report.json"))) {
      sampleDirs.push(".");
    }
    return sampleDirs.map((sample) => {
      const report = readJSON(path.join(modeDir, sample, "report.json"), readJSON(path.join(modeDir, "report.json"), {}));
      return {
        mode,
        sample: sample === "." ? "sample-1" : sample,
        throughput: report.throughput || 0,
        success: report.success || 0,
        mean_ms: ((report.latencies || {}).mean || 0) / 1e6,
        p50_ms: ((report.latencies || {})["50th"] || 0) / 1e6,
        p95_ms: ((report.latencies || {})["95th"] || 0) / 1e6,
        p99_ms: ((report.latencies || {})["99th"] || 0) / 1e6,
      };
    });
  });
}

const modes = [...new Set(samples.map((s) => s.mode))];
const median = (values) => {
  const sorted = values.slice().sort((a, b) => a - b);
  if (sorted.length === 0) return 0;
  const mid = Math.floor(sorted.length / 2);
  return sorted.length % 2 ? sorted[mid] : (sorted[mid - 1] + sorted[mid]) / 2;
};
summary ||= modes.map((mode) => {
  const rows = samples.filter((s) => s.mode === mode);
  return {
    mode,
    samples: rows.length,
    throughput: rows.reduce((acc, row) => acc + row.throughput, 0) / rows.length,
    success: rows.reduce((acc, row) => acc + row.success, 0) / rows.length,
    mean_ms: median(rows.map((row) => row.mean_ms)),
    p50_ms: median(rows.map((row) => row.p50_ms)),
    p95_ms: median(rows.map((row) => row.p95_ms)),
    p99_ms: median(rows.map((row) => row.p99_ms)),
  };
});

const fmt = (n, digits = 2) => Number(n || 0).toFixed(digits);
const palette = {
  nosidecar: "#2563eb",
  "sidecar-buffered": "#d97706",
  "sidecar-zero-copy": "#059669",
  istio: "#7c3aed",
};

const dataScript = JSON.stringify({ summary, samples }, null, 2).replace(/</g, "\\u003c");
const html = `<!doctype html>
<html lang="ru">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Bookinfo Service Mesh Benchmark</title>
  <style>
    :root { color-scheme: light; --ink:#172033; --muted:#697386; --line:#d9dee8; --panel:#ffffff; --bg:#f5f7fb; }
    * { box-sizing: border-box; }
    body { margin:0; font:14px/1.45 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif; background:var(--bg); color:var(--ink); }
    header { padding:34px 40px 22px; background:#111827; color:white; }
    h1 { margin:0; font-size:30px; letter-spacing:0; }
    .subtitle { margin-top:8px; color:#cbd5e1; max-width:980px; }
    main { padding:28px 40px 42px; max-width:1280px; margin:0 auto; }
    .grid { display:grid; gap:16px; }
    .cards { grid-template-columns:repeat(4,minmax(0,1fr)); margin-bottom:22px; }
    .card, .panel { background:var(--panel); border:1px solid var(--line); border-radius:8px; box-shadow:0 8px 24px rgba(15,23,42,.06); }
    .card { padding:18px; }
    .label { color:var(--muted); font-size:12px; text-transform:uppercase; letter-spacing:.04em; }
    .value { margin-top:8px; font-size:28px; font-weight:760; }
    .hint { margin-top:4px; color:var(--muted); }
    .panel { padding:20px; margin-top:18px; }
    .panel h2 { margin:0 0 14px; font-size:18px; }
    canvas { width:100%; height:320px; display:block; }
    table { width:100%; border-collapse:collapse; overflow:hidden; }
    th, td { padding:11px 12px; border-bottom:1px solid var(--line); text-align:right; }
    th:first-child, td:first-child { text-align:left; }
    th { color:var(--muted); font-size:12px; text-transform:uppercase; letter-spacing:.04em; }
    .chip { display:inline-flex; align-items:center; gap:8px; }
    .dot { width:10px; height:10px; border-radius:50%; display:inline-block; }
    .notes { color:var(--muted); margin-top:12px; }
    @media (max-width: 900px) { main, header { padding-left:18px; padding-right:18px; } .cards { grid-template-columns:1fr 1fr; } }
  </style>
</head>
<body>
  <header>
    <h1>Bookinfo Service Mesh Benchmark</h1>
    <div class="subtitle">120 rps, steady-state замер после HTTP readiness и warm-up. Таблица показывает агрегат по samples; latency в миллисекундах.</div>
  </header>
  <main>
    <section class="grid cards" id="cards"></section>
    <section class="panel"><h2>Latency Percentiles</h2><canvas id="latency"></canvas></section>
    <section class="panel"><h2>Throughput And Success</h2><canvas id="throughput"></canvas></section>
    <section class="panel"><h2>Sample Variability: p95</h2><canvas id="samples"></canvas></section>
    <section class="panel">
      <h2>Summary</h2>
      <table id="summary"></table>
      <div class="notes">Generated from ${path.basename(runDir)}. Raw artifacts remain in the same run directory.</div>
    </section>
  </main>
  <script>const BENCH = ${dataScript}; const COLORS = ${JSON.stringify(palette)};</script>
  <script>
    const fmt = (n, d=2) => Number(n || 0).toFixed(d);
    const modes = BENCH.summary.map(x => x.mode);
    const color = mode => COLORS[mode] || "#334155";
    function drawGroupedBars(canvas, groups, series, maxValue, suffix="") {
      const ctx = canvas.getContext("2d");
      const ratio = devicePixelRatio || 1;
      const w = canvas.clientWidth * ratio, h = canvas.clientHeight * ratio;
      canvas.width = w; canvas.height = h; ctx.scale(ratio, ratio);
      const W = canvas.clientWidth, H = canvas.clientHeight, pad = {l:54,r:18,t:18,b:58};
      ctx.clearRect(0,0,W,H); ctx.font = "12px -apple-system, BlinkMacSystemFont, Segoe UI, sans-serif";
      ctx.strokeStyle = "#d9dee8"; ctx.fillStyle = "#697386";
      for (let i=0;i<=4;i++) { const y = pad.t + (H-pad.t-pad.b)*i/4; ctx.beginPath(); ctx.moveTo(pad.l,y); ctx.lineTo(W-pad.r,y); ctx.stroke(); const v=maxValue*(1-i/4); ctx.fillText(fmt(v,0)+suffix, 8, y+4); }
      const plotW = W-pad.l-pad.r, groupW = plotW/groups.length, barGap = 5, barW = Math.max(6, (groupW-22)/series.length - barGap);
      groups.forEach((g, gi) => {
        const x0 = pad.l + gi*groupW + 11;
        series.forEach((s, si) => {
          const v = s.values[gi] || 0; const bh = (H-pad.t-pad.b)*(v/maxValue);
          const x = x0 + si*(barW+barGap), y = H-pad.b-bh;
          ctx.fillStyle = s.color; ctx.fillRect(x,y,barW,bh);
        });
        ctx.fillStyle = "#172033"; ctx.textAlign = "center"; ctx.fillText(g, x0 + (series.length*(barW+barGap))/2 - barGap, H-28);
      });
      ctx.textAlign = "left";
      series.forEach((s, i) => { ctx.fillStyle=s.color; ctx.fillRect(pad.l+i*130, H-14, 10, 10); ctx.fillStyle="#697386"; ctx.fillText(s.name, pad.l+14+i*130, H-5); });
    }
    function drawLine(canvas, rows, field, maxValue) {
      const ctx = canvas.getContext("2d");
      const ratio = devicePixelRatio || 1, W = canvas.clientWidth, H = canvas.clientHeight;
      canvas.width = W*ratio; canvas.height = H*ratio; ctx.scale(ratio, ratio); ctx.clearRect(0,0,W,H);
      const pad = {l:54,r:18,t:18,b:54}; ctx.strokeStyle = "#d9dee8"; ctx.fillStyle = "#697386"; ctx.font="12px -apple-system,BlinkMacSystemFont,Segoe UI,sans-serif";
      for (let i=0;i<=4;i++) { const y=pad.t+(H-pad.t-pad.b)*i/4; ctx.beginPath(); ctx.moveTo(pad.l,y); ctx.lineTo(W-pad.r,y); ctx.stroke(); ctx.fillText(fmt(maxValue*(1-i/4),0),8,y+4); }
      modes.forEach((mode) => {
        const points = rows.filter(r => r.mode === mode);
        if (!points.length) return;
        ctx.strokeStyle = color(mode); ctx.lineWidth = 2; ctx.beginPath();
        points.forEach((p, i) => {
          const x = pad.l + ((W-pad.l-pad.r) * i / Math.max(1, points.length-1));
          const y = H-pad.b - (H-pad.t-pad.b) * ((p[field] || 0) / maxValue);
          if (i === 0) ctx.moveTo(x,y); else ctx.lineTo(x,y);
        });
        ctx.stroke();
        points.forEach((p, i) => {
          const x = pad.l + ((W-pad.l-pad.r) * i / Math.max(1, points.length-1));
          const y = H-pad.b - (H-pad.t-pad.b) * ((p[field] || 0) / maxValue);
          ctx.fillStyle = color(mode); ctx.beginPath(); ctx.arc(x,y,4,0,Math.PI*2); ctx.fill();
        });
      });
      modes.forEach((m,i)=>{ ctx.fillStyle=color(m); ctx.fillRect(pad.l+i*150,H-14,10,10); ctx.fillStyle="#697386"; ctx.fillText(m,pad.l+14+i*150,H-5); });
    }
    const best = BENCH.summary.slice().sort((a,b)=>a.p95_ms-b.p95_ms)[0];
    document.getElementById("cards").innerHTML = BENCH.summary.map(row => \`
      <div class="card"><div class="label">\${row.mode}</div><div class="value" style="color:\${color(row.mode)}">\${fmt(row.p95_ms)} ms</div><div class="hint">p95, \${row.samples || 1} sample(s), success \${fmt(row.success*100,2)}%</div></div>\`).join("");
    const maxLatency = Math.ceil(Math.max(...BENCH.summary.flatMap(r => [r.mean_ms,r.p50_ms,r.p95_ms,r.p99_ms])) * 1.15 / 10) * 10 || 10;
    drawGroupedBars(document.getElementById("latency"), modes, [
      {name:"mean", color:"#94a3b8", values:BENCH.summary.map(r=>r.mean_ms)},
      {name:"p50", color:"#38bdf8", values:BENCH.summary.map(r=>r.p50_ms)},
      {name:"p95", color:"#f59e0b", values:BENCH.summary.map(r=>r.p95_ms)},
      {name:"p99", color:"#ef4444", values:BENCH.summary.map(r=>r.p99_ms)}
    ], maxLatency, "ms");
    drawGroupedBars(document.getElementById("throughput"), modes, [
      {name:"rps", color:"#10b981", values:BENCH.summary.map(r=>r.throughput)},
      {name:"success %", color:"#6366f1", values:BENCH.summary.map(r=>r.success*120)}
    ], 130, "");
    drawLine(document.getElementById("samples"), BENCH.samples, "p95_ms", Math.ceil(Math.max(...BENCH.samples.map(s=>s.p95_ms))*1.15/10)*10 || 10);
    document.getElementById("summary").innerHTML = \`
      <thead><tr><th>mode</th><th>samples</th><th>rps</th><th>success</th><th>mean</th><th>p50</th><th>p95</th><th>p99</th></tr></thead>
      <tbody>\${BENCH.summary.map(r => \`<tr><td><span class="chip"><span class="dot" style="background:\${color(r.mode)}"></span>\${r.mode}</span></td><td>\${r.samples||1}</td><td>\${fmt(r.throughput)}</td><td>\${fmt(r.success*100)}%</td><td>\${fmt(r.mean_ms)}</td><td>\${fmt(r.p50_ms)}</td><td>\${fmt(r.p95_ms)}</td><td>\${fmt(r.p99_ms)}</td></tr>\`).join("")}</tbody>\`;
  </script>
</body>
</html>`;

fs.writeFileSync(path.join(runDir, "report.html"), html);
console.log(path.join(runDir, "report.html"));
NODE
