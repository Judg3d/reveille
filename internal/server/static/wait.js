(function () {
  const cfg = window.reveille;
  const status = document.getElementById("status");
  const form = document.getElementById("lease-form");

  async function poll() {
    const params = new URLSearchParams({ host: cfg.host, returnTo: cfg.returnTo });
    const res = await fetch(`${cfg.publicPath}/api/status?${params}`, { headers: { Accept: "application/json" } });
    if (!res.ok) {
      status.textContent = "Unable to check status yet.";
      return;
    }
    const data = await res.json();
    if (data.healthy) {
      window.location.assign(data.returnTo || "/");
      return;
    }
    status.textContent = data.never ? "Starting with no automatic stop." : "Starting and waiting for health check...";
  }

  form.addEventListener("submit", async function (event) {
    event.preventDefault();
    const body = new FormData(form);
    const params = new URLSearchParams({ host: cfg.host });
    const res = await fetch(`${cfg.publicPath}/api/lease?${params}`, { method: "POST", body });
    status.textContent = res.ok ? "Lease updated." : "Lease update failed.";
  });

  poll().catch(() => { status.textContent = "Unable to check status yet."; });
  setInterval(() => poll().catch(() => { status.textContent = "Unable to check status yet."; }), cfg.pollMillis || 5000);
}());
