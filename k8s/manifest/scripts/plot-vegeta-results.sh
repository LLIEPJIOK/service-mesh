#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
ARTIFACTS_DIR="${ROOT_DIR}/k8s/test/artifacts"
OUTPUT_DIR="${ARTIFACTS_DIR}/plots"
MODES=("nosidecar" "sidecar")

require_binary() {
  local bin="$1"
  if ! command -v "${bin}" >/dev/null 2>&1; then
    echo "Missing required binary: ${bin}"
    exit 1
  fi
}

check_python_deps() {
  if command -v python3 >/dev/null 2>&1; then
    return 0
  fi
  if command -v python >/dev/null 2>&1; then
    return 0
  fi
  echo "Warning: Python not found. Plots will be generated using gnuplot if available."
  return 0
}

mkdir -p "${OUTPUT_DIR}"

echo "[plot] Generating comparative plots..."

generate_plot_script() {
  cat > "${OUTPUT_DIR}/comparison.gp" <<'GPLOT'
set terminal png size 1200,800
set output 'comparison.png'
set title 'Sidecar Benchmark Comparison: No Sidecar vs With Sidecar'
set grid

set style data histogram
set style histogram cluster gap 1
set boxwidth 0.4
set xtics ("No Sidecar" 0, "With Sidecar" 1)

# Throughput comparison
set multiplot layout 2,2 title "Benchmark Results Comparison"

# Throughput
set title "Throughput (requests/sec)"
set ylabel "Req/sec"
unset key
plot '< echo "0 $(cat ../nosidecar/summary.json | jq -r .throughput); 1 $(cat ../sidecar/summary.json | jq -r .throughput)"' using 2:xtic(1) with boxes lc rgb "blue"

# Success Rate
set title "Success Rate (%)"
set ylabel "%"
plot '< echo "0 $(cat ../nosidecar/summary.json | jq -r .success_rate); 1 $(cat ../sidecar/summary.json | jq -r .success_rate)"' using 2:xtic(1) with boxes lc rgb "green"

# P95 Latency
set title "P95 Latency (ms)"
set ylabel "ms"
plot '< echo "0 $(cat ../nosidecar/summary.json | jq -r .latency_p95_ms); 1 $(cat ../sidecar/summary.json | jq -r .latency_p95_ms)"' using 2:xtic(1) with boxes lc rgb "red"

# P99 Latency
set title "P99 Latency (ms)"
set ylabel "ms"
plot '< echo "0 $(cat ../nosidecar/summary.json | jq -r .latency_p99_ms); 1 $(cat ../sidecar/summary.json | jq -r .latency_p99_ms)"' using 2:xtic(1) with boxes lc rgb "orange"

unset multiplot
GPLOT
}

generate_html_report() {
  local nosidecar_json="${ARTIFACTS_DIR}/nosidecar/summary.json"
  local sidecar_json="${ARTIFACTS_DIR}/sidecar/summary.json"

  local ns_throughput ns_success ns_p95 ns_p99 ns_mean
  local sc_throughput sc_success sc_p95 sc_p99 sc_mean

  ns_throughput=$(jq -r '.throughput' "${nosidecar_json}")
  ns_success=$(jq -r '.success_rate' "${nosidecar_json}")
  ns_p95=$(jq -r '.latency_p95_ms' "${nosidecar_json}")
  ns_p99=$(jq -r '.latency_p99_ms' "${nosidecar_json}")
  ns_mean=$(jq -r '.latency_mean_ms' "${nosidecar_json}")

  sc_throughput=$(jq -r '.throughput' "${sidecar_json}")
  sc_success=$(jq -r '.success_rate' "${sidecar_json}")
  sc_p95=$(jq -r '.latency_p95_ms' "${sidecar_json}")
  sc_p99=$(jq -r '.latency_p99_ms' "${sidecar_json}")
  sc_mean=$(jq -r '.latency_mean_ms' "${sidecar_json}")

  cat > "${OUTPUT_DIR}/comparison.html" <<HTML
<!DOCTYPE html>
<html>
<head>
    <title>Sidecar Benchmark Comparison</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.0/dist/chart.umd.min.js"></script>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 40px; background: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; background: white; padding: 30px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        h1 { color: #333; text-align: center; }
        .charts { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; margin-top: 30px; }
        .chart-card { background: #fafafa; padding: 20px; border-radius: 8px; }
        table { width: 100%; border-collapse: collapse; margin-top: 30px; }
        th, td { padding: 12px; text-align: left; border-bottom: 1px solid #ddd; }
        th { background: #4A90D9; color: white; }
        tr:hover { background: #f5f5f5; }
        .delta-positive { color: green; }
        .delta-negative { color: red; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Sidecar Benchmark Comparison</h1>

        <table>
            <tr>
                <th>Metric</th>
                <th>No Sidecar</th>
                <th>With Sidecar</th>
                <th>Delta</th>
            </tr>
            <tr>
                <td>Throughput (req/sec)</td>
                <td>${ns_throughput}</td>
                <td>${sc_throughput}</td>
                <td class="$([ $(echo "${sc_throughput} < ${ns_throughput}" | bc -l) -eq 1 ] && echo "delta-negative" || echo "delta-positive")">
                    $(echo "scale=2; (${sc_throughput} - ${ns_throughput}) / ${ns_throughput} * 100" | bc)%</td>
            </tr>
            <tr>
                <td>Success Rate (%)</td>
                <td>${ns_success}</td>
                <td>${sc_success}</td>
                <td>$(echo "scale=2; ${sc_success} - ${ns_success}" | bc)%</td>
            </tr>
            <tr>
                <td>P95 Latency (ms)</td>
                <td>${ns_p95}</td>
                <td>${sc_p95}</td>
                <td class="$([ $(echo "${sc_p95} > ${ns_p95}" | bc -l) -eq 1 ] && echo "delta-negative" || echo "delta-positive")">
                    $(echo "scale=2; (${sc_p95} - ${ns_p95}) / ${ns_p95} * 100" | bc)%</td>
            </tr>
            <tr>
                <td>P99 Latency (ms)</td>
                <td>${ns_p99}</td>
                <td>${sc_p99}</td>
                <td class="$([ $(echo "${sc_p99} > ${ns_p99}" | bc -l) -eq 1 ] && echo "delta-negative" || echo "delta-positive")">
                    $(echo "scale=2; (${sc_p99} - ${ns_p99}) / ${ns_p99} * 100" | bc)%</td>
            </tr>
            <tr>
                <td>Mean Latency (ms)</td>
                <td>${ns_mean}</td>
                <td>${sc_mean}</td>
                <td class="$([ $(echo "${sc_mean} > ${ns_mean}" | bc -l) -eq 1 ] && echo "delta-negative" || echo "delta-positive")">
                    $(echo "scale=2; (${sc_mean} - ${ns_mean}) / ${ns_mean} * 100" | bc)%</td>
            </tr>
        </table>

        <div class="charts">
            <div class="chart-card">
                <canvas id="throughputChart"></canvas>
            </div>
            <div class="chart-card">
                <canvas id="latencyChart"></canvas>
            </div>
        </div>

        <script>
            const metrics = {
                labels: ['No Sidecar', 'With Sidecar'],
                throughput: [${ns_throughput}, ${sc_throughput}],
                p95: [${ns_p95}, ${sc_p95}],
                p99: [${ns_p99}, ${sc_p99}],
                mean: [${ns_mean}, ${sc_mean}]
            };

            new Chart(document.getElementById('throughputChart'), {
                type: 'bar',
                data: {
                    labels: metrics.labels,
                    datasets: [{ label: 'Throughput (req/sec)', data: metrics.throughput, backgroundColor: '#4A90D9' }]
                },
                options: { responsive: true, plugins: { title: { display: true, text: 'Throughput Comparison' } } }
            });

            new Chart(document.getElementById('latencyChart'), {
                type: 'bar',
                data: {
                    labels: metrics.labels,
                    datasets: [
                        { label: 'P95', data: metrics.p95, backgroundColor: '#E74C3C' },
                        { label: 'P99', data: metrics.p99, backgroundColor: '#F39C12' },
                        { label: 'Mean', data: metrics.mean, backgroundColor: '#27AE60' }
                    ]
                },
                options: { responsive: true, plugins: { title: { display: true, text: 'Latency Comparison (ms)' } } }
            });
        </script>
    </div>
</body>
</html>
HTML
}

if [[ -d "${ARTIFACTS_DIR}/nosidecar" ]] && [[ -d "${ARTIFACTS_DIR}/sidecar" ]]; then
  generate_plot_script
  generate_html_report

  if command -v gnuplot >/dev/null 2>&1; then
    cd "${OUTPUT_DIR}"
    gnuplot comparison.gp 2>/dev/null || true
    cd - >/dev/null
  fi

  echo "[plot] Generated:"
  echo "  ${OUTPUT_DIR}/comparison.html"
  [[ -f "${OUTPUT_DIR}/comparison.png" ]] && echo "  ${OUTPUT_DIR}/comparison.png"
else
  echo "[plot] Error: missing benchmark results"
  echo "  Run 'make vegeta-bench' first"
  exit 1
fi