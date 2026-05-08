#!/usr/bin/env bash

ROOT_DIR="/Users/denis/Projects/service-mesh"
ARTIFACTS_DIR="${ROOT_DIR}/k&s/test/artifacts"
RAMP_DIR="${ARTIFACTS_DIR}/ramp"
OUTPUT_DIR="${RAMP_DIR}/plots"

mkdir -p "${OUTPUT_DIR}"

echo "[ramp-plot] Generating ramp comparison plots..."

generate_html_report() {
  local nosidecar_summary="${RAMP_DIR}/nosidecar/summary.json"
  local sidecar_summary="${RAMP_DIR}/sidecar/summary.json"

  if [[ ! -f "${nosidecar_summary}" ]] || [[ ! -f "${sidecar_summary}" ]]; then
    echo "[ramp-plot] Error: missing ramp test results"
    echo "  Run: make vegeta-ramp-nosidecar && make vegeta-ramp-sidecar"
    exit 1
  fi

  local ns_rates sc_rates ns_throughput sc_throughput
  local ns_error sc_error ns_p95 sc_p95 ns_p99 sc_p99 ns_mean sc_mean

  ns_rates=$(jq -r '.rates | join(",")' "${nosidecar_summary}")
  sc_rates=$(jq -r '.rates | join(",")' "${sidecar_summary}")

  ns_throughput=$(jq -r '.throughputs | join(",")' "${nosidecar_summary}")
  sc_throughput=$(jq -r '.throughputs | join(",")' "${sidecar_summary}")

  ns_error=$(jq -r '.error_rates | join(",")' "${nosidecar_summary}")
  sc_error=$(jq -r '.error_rates | join(",")' "${sidecar_summary}")

  ns_p95=$(jq -r '.p95_ms | join(",")' "${nosidecar_summary}")
  sc_p95=$(jq -r '.p95_ms | join(",")' "${sidecar_summary}")

  ns_p99=$(jq -r '.p99_ms | join(",")' "${nosidecar_summary}")
  sc_p99=$(jq -r '.p99_ms | join(",")' "${sidecar_summary}")

  ns_mean=$(jq -r '.mean_ms | join(",")' "${nosidecar_summary}")
  sc_mean=$(jq -r '.mean_ms | join(",")' "${sidecar_summary}")

  cat > "${OUTPUT_DIR}/ramp-comparison.html" <<HTML
<!DOCTYPE html>
<html>
<head>
    <title>Sidecar Ramp Test Comparison</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.min.js"></script>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 40px; background: #f5f5f5; }
        .container { max-width: 1400px; margin: 0 auto; background: white; padding: 30px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        h1 { color: #333; text-align: center; }
        h2 { color: #555; margin-top: 40px; }
        .charts { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; margin-top: 20px; }
        .chart-card { background: #fafafa; padding: 20px; border-radius: 8px; }
        table { width: 100%; border-collapse: collapse; margin-top: 20px; }
        th, td { padding: 10px; text-align: center; border-bottom: 1px solid #ddd; }
        th { background: #4A90D9; color: white; }
        tr:hover { background: #f5f5f5; }
        .delta-positive { color: green; }
        .delta-negative { color: red; }
        .threshold-exceeded { color: red; font-weight: bold; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Load Ramp Test: No Sidecar vs With Sidecar</h1>
        <p style="text-align: center; color: #666;">Stop conditions: error_rate > 1% OR P95 > 1000ms</p>

        <h2>Throughput vs Load</h2>
        <div class="chart-card">
            <canvas id="throughputChart"></canvas>
        </div>

        <h2>Error Rate vs Load</h2>
        <div class="chart-card">
            <canvas id="errorChart"></canvas>
        </div>

        <h2>Latency (P95) vs Load</h2>
        <div class="chart-card">
            <canvas id="p95Chart"></canvas>
        </div>

        <h2>Latency (Mean) vs Load</h2>
        <div class="chart-card">
            <canvas id="meanChart"></canvas>
        </div>

        <h2>Comparison Table</h2>
        <table id="comparisonTable">
            <tr>
                <th>Rate (rps)</th>
                <th>No Sidecar P95 (ms)</th>
                <th>With Sidecar P95 (ms)</th>
                <th>Delta</th>
                <th>No Sidecar Error %</th>
                <th>With Sidecar Error %</th>
            </tr>
        </table>

        <script>
            const rates = [${ns_rates}];

            const nsThroughput = [${ns_throughput}];
            const scThroughput = [${sc_throughput}];

            const nsError = [${ns_error}];
            const scError = [${sc_error}];

            const nsP95 = [${ns_p95}];
            const scP95 = [${sc_p95}];

            const nsMean = [${ns_mean}];
            const scMean = [${sc_mean}];

            new Chart(document.getElementById('throughputChart'), {
                type: 'line',
                data: {
                    labels: rates,
                    datasets: [
                        { label: 'No Sidecar', data: nsThroughput, borderColor: '#4A90D9', backgroundColor: '#4A90D9', tension: 0.3 },
                        { label: 'With Sidecar', data: scThroughput, borderColor: '#E74C3C', backgroundColor: '#E74C3C', tension: 0.3 }
                    ]
                },
                options: {
                    responsive: true,
                    plugins: { title: { display: true, text: 'Throughput vs Load' } },
                    scales: { x: { title: { display: true, text: 'Request Rate (rps)' } }, y: { title: { display: true, text: 'Throughput (req/s)' } } }
                }
            });

            new Chart(document.getElementById('errorChart'), {
                type: 'line',
                data: {
                    labels: rates,
                    datasets: [
                        { label: 'No Sidecar', data: nsError, borderColor: '#4A90D9', backgroundColor: '#4A90D9', tension: 0.3 },
                        { label: 'With Sidecar', data: scError, borderColor: '#E74C3C', backgroundColor: '#E74C3C', tension: 0.3 }
                    ]
                },
                options: {
                    responsive: true,
                    plugins: { title: { display: true, text: 'Error Rate vs Load' } },
                    scales: { x: { title: { display: true, text: 'Request Rate (rps)' } }, y: { title: { display: true, text: 'Error Rate (%)' } } }
                }
            });

            new Chart(document.getElementById('p95Chart'), {
                type: 'line',
                data: {
                    labels: rates,
                    datasets: [
                        { label: 'No Sidecar', data: nsP95, borderColor: '#4A90D9', backgroundColor: '#4A90D9', tension: 0.3 },
                        { label: 'With Sidecar', data: scP95, borderColor: '#E74C3C', backgroundColor: '#E74C3C', tension: 0.3 }
                    ]
                },
                options: {
                    responsive: true,
                    plugins: { title: { display: true, text: 'P95 Latency vs Load' } },
                    scales: { x: { title: { display: true, text: 'Request Rate (rps)' } }, y: { title: { display: true, text: 'Latency (ms)' } } }
                }
            });

            new Chart(document.getElementById('meanChart'), {
                type: 'line',
                data: {
                    labels: rates,
                    datasets: [
                        { label: 'No Sidecar', data: nsMean, borderColor: '#4A90D9', backgroundColor: '#4A90D9', tension: 0.3 },
                        { label: 'With Sidecar', data: scMean, borderColor: '#E74C3C', backgroundColor: '#E74C3C', tension: 0.3 }
                    ]
                },
                options: {
                    responsive: true,
                    plugins: { title: { display: true, text: 'Mean Latency vs Load' } },
                    scales: { x: { title: { display: true, text: 'Request Rate (rps)' } }, y: { title: { display: true, text: 'Latency (ms)' } } }
                }
            });

            const table = document.getElementById('comparisonTable');
            for (let i = 0; i < rates.length; i++) {
                const row = table.insertRow(-1);
                const delta = ((scP95[i] - nsP95[i]) / nsP95[i] * 100).toFixed(1);
                const deltaClass = delta > 0 ? 'delta-negative' : 'delta-positive';
                const p95Warning = scP95[i] > 1000 ? 'threshold-exceeded' : '';
                row.innerHTML = \`
                    <td>\${rates[i]}</td>
                    <td>\${nsP95[i].toFixed(2)}</td>
                    <td class="\${p95Warning}">\${scP95[i].toFixed(2)}</td>
                    <td class="\${deltaClass}">\${delta > 0 ? '+' : ''}\${delta}%</td>
                    <td>\${nsError[i].toFixed(2)}%</td>
                    <td>\${scError[i].toFixed(2)}%</td>
                \`;
            }
        </script>
    </div>
</body>
</html>
HTML
}

generate_html_report
echo "[ramp-plot] Generated: ${OUTPUT_DIR}/ramp-comparison.html"