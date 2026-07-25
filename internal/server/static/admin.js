(function () {
  const configNode = document.getElementById("reveille-admin-config");
  if (!configNode) return;

  let cfg;
  try {
    cfg = JSON.parse(configNode.textContent || "{}");
  } catch (_) {
    return;
  }

  if (cfg.token && window.history && window.history.replaceState) {
    try {
      const current = new URL(window.location.href);
      current.searchParams.delete("token");
      window.history.replaceState(null, "", current.toString());
    } catch (_) {
      // Keeping the token in memory is enough.
    }
  }

  const tbody = document.getElementById("hosts");
  const refreshState = document.getElementById("refresh-state");
  const errorNode = document.getElementById("error");

  function headers() {
    const h = { Accept: "application/json" };
    if (cfg.token) h["X-Reveille-Admin-Token"] = cfg.token;
    return h;
  }

  function showError(message) {
    if (!errorNode) return;
    errorNode.textContent = message || "";
    errorNode.classList.toggle("hidden", !message);
  }

  async function api(path, options) {
    const res = await fetch(path, Object.assign({ headers: headers() }, options || {}));
    if (!res.ok) {
      const text = (await res.text().catch(() => "")) || res.statusText;
      throw new Error(text.trim() || "request failed");
    }
    return res.json();
  }

  function leaseText(host) {
    if (!host.lease) return "—";
    if (host.lease.never) return "never stops";
    let text = host.lease.label || "timer";
    if (host.lease.expiresAt) {
      text += " · until " + new Date(host.lease.expiresAt).toLocaleTimeString();
    }
    if (host.lease.provisional) text += " (provisional)";
    if (host.lease.idle) text += " (idle)";
    return text;
  }

  function actionButton(label, path, refreshAfter) {
    const button = document.createElement("button");
    button.textContent = label;
    button.addEventListener("click", async () => {
      button.disabled = true;
      showError("");
      try {
        await api(path, { method: "POST" });
      } catch (err) {
        showError(label + " failed: " + err.message);
      } finally {
        button.disabled = false;
        refreshAfter();
      }
    });
    return button;
  }

  function leaseSelect(host, refreshAfter) {
    const select = document.createElement("select");
    const placeholder = document.createElement("option");
    placeholder.textContent = "set timer…";
    placeholder.value = "";
    select.appendChild(placeholder);
    (host.leaseLabels || []).forEach((label) => {
      const option = document.createElement("option");
      option.value = label;
      option.textContent = label;
      select.appendChild(option);
    });
    select.addEventListener("change", async () => {
      if (!select.value) return;
      showError("");
      const body = new URLSearchParams({ lease: select.value });
      try {
        await api("/api/hosts/" + encodeURIComponent(host.host) + "/lease", {
          method: "POST",
          body,
        });
      } catch (err) {
        showError("Timer update failed: " + err.message);
      } finally {
        select.value = "";
        refreshAfter();
      }
    });
    return select;
  }

  function render(hosts) {
    tbody.textContent = "";
    hosts.forEach((host) => {
      const row = document.createElement("tr");

      const hostCell = document.createElement("td");
      hostCell.textContent = host.host;
      row.appendChild(hostCell);

      const targetCell = document.createElement("td");
      targetCell.textContent = `${host.type}: ${host.name || host.id} (${host.environment})`;
      row.appendChild(targetCell);

      const stateCell = document.createElement("td");
      const pill = document.createElement("span");
      pill.className = "pill " + (host.healthy ? "ok" : "down");
      pill.textContent = host.healthy ? "running" : "stopped";
      pill.title = host.healthError || "";
      stateCell.appendChild(pill);
      row.appendChild(stateCell);

      const leaseCell = document.createElement("td");
      leaseCell.textContent = leaseText(host);
      row.appendChild(leaseCell);

      const actions = document.createElement("td");
      actions.className = "actions";
      const base = "/api/hosts/" + encodeURIComponent(host.host);
      actions.appendChild(actionButton("Start", base + "/start", refresh));
      actions.appendChild(actionButton("Stop", base + "/stop", refresh));
      actions.appendChild(leaseSelect(host, refresh));
      row.appendChild(actions);

      tbody.appendChild(row);
    });
  }

  async function refresh() {
    try {
      const hosts = await api("/api/hosts");
      render(hosts);
      if (refreshState) {
        refreshState.textContent = "Updated " + new Date().toLocaleTimeString();
        refreshState.classList.remove("down");
      }
      showError("");
    } catch (err) {
      if (refreshState) {
        refreshState.textContent = "Refresh failed";
        refreshState.classList.add("down");
      }
      showError("Could not load hosts: " + err.message);
    }
  }

  refresh();
  setInterval(refresh, cfg.pollMillis || 5000);
}());
