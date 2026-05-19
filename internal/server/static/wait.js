(function () {
  const configNode = document.getElementById("reveille-config");
  if (!configNode) return;

  let cfg;
  try {
    cfg = JSON.parse(configNode.textContent || "{}");
  } catch (_) {
    return;
  }

  const status = document.getElementById("status");
  const form = document.getElementById("lease-form");
  const timerStep = document.getElementById("timer-step");
  const pollStep = document.getElementById("poll-step");
  let pollTimer = null;

  function setStatus(message, isError) {
    if (!status) return;
    status.textContent = message;
    status.classList.toggle("error", Boolean(isError));
  }

  function showPollStep() {
    if (timerStep) timerStep.classList.add("hidden");
    if (pollStep) pollStep.classList.remove("hidden");
  }

  async function poll() {
    const params = new URLSearchParams({ host: cfg.host, returnTo: cfg.returnTo });
    const res = await fetch(`${cfg.publicPath}/api/status?${params}`, { headers: { Accept: "application/json" } });
    if (!res.ok) {
      setStatus("Unable to check status yet.", true);
      return;
    }
    const data = await res.json();
    if (data.healthy) {
      window.location.assign(data.returnTo || "/");
      return;
    }
    showPollStep();
    if (data.never) {
      setStatus("Starting with no automatic stop.", false);
      return;
    }
    setStatus("Starting and waiting for health check...", false);
  }

  function startPolling() {
    if (pollTimer) return;
    showPollStep();
    poll().catch(() => { setStatus("Unable to check status yet.", true); });
    pollTimer = setInterval(() => poll().catch(() => { setStatus("Unable to check status yet.", true); }), cfg.pollMillis || 5000);
  }

  if (!form) {
    startPolling();
    return;
  }

  form.addEventListener("submit", async function (event) {
    event.preventDefault();
    const body = new FormData(form);
    const params = new URLSearchParams({ host: cfg.host });
    const res = await fetch(`${cfg.publicPath}/api/lease?${params}`, { method: "POST", body });
    if (!res.ok) {
      setStatus("Timer update failed.", true);
      return;
    }
    setStatus("Timer set. Waiting for health check...", false);
    startPolling();
  });
}());
