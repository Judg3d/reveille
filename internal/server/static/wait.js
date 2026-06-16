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
  const formStatus = document.getElementById("form-status");
  const form = document.getElementById("lease-form");
  const timerStep = document.getElementById("timer-step");
  const pollStep = document.getElementById("poll-step");
  const submitButton = document.getElementById("lease-submit");
  const countdown = document.getElementById("countdown");
  const countdownValue = document.getElementById("countdown-value");
  const pollCopy = document.getElementById("poll-copy");
  const statusDetail = document.getElementById("status-detail");
  let pollTimer = null;
  let countdownTimer = null;

  function setStatus(message, isError) {
    if (!status) return;
    status.textContent = message;
    status.classList.toggle("error", Boolean(isError));
  }

  function setFormStatus(message, isError) {
    if (!formStatus) return;
    formStatus.textContent = message;
    formStatus.classList.remove("hidden");
    formStatus.classList.toggle("error", Boolean(isError));
  }

  function setStatusDetail(message) {
    if (!statusDetail) return;
    if (!message) {
      statusDetail.textContent = "";
      statusDetail.classList.add("hidden");
      return;
    }
    statusDetail.textContent = message;
    statusDetail.classList.remove("hidden");
  }

  function showPollStep() {
    if (timerStep) timerStep.classList.add("hidden");
    if (pollStep) pollStep.classList.remove("hidden");
  }

  function showTimerStep() {
    if (pollStep) pollStep.classList.add("hidden");
    if (timerStep) timerStep.classList.remove("hidden");
  }

  function stopCountdown() {
    if (countdownTimer) {
      clearInterval(countdownTimer);
      countdownTimer = null;
    }
  }

  function renderCountdown(expiresAt) {
    if (!countdown || !countdownValue) return;
    if (!expiresAt) {
      stopCountdown();
      countdown.classList.add("hidden");
      countdownValue.textContent = "--:--";
      return;
    }

    const deadline = new Date(expiresAt);
    if (Number.isNaN(deadline.getTime())) {
      stopCountdown();
      countdown.classList.add("hidden");
      countdownValue.textContent = "--:--";
      return;
    }

    countdown.classList.remove("hidden");

    const updateCountdown = function () {
      const remaining = deadline.getTime() - Date.now();
      if (remaining <= 0) {
        countdownValue.textContent = "00:00";
        stopCountdown();
        return;
      }

      const totalSeconds = Math.ceil(remaining / 1000);
      const minutes = Math.floor(totalSeconds / 60);
      const seconds = totalSeconds % 60;
      countdownValue.textContent = `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
    };

    updateCountdown();
    stopCountdown();
    countdownTimer = setInterval(updateCountdown, 1000);
  }

  async function poll() {
    const params = new URLSearchParams({ host: cfg.host, returnTo: cfg.returnTo });
    const res = await fetch(`${cfg.publicPath}/api/status?${params}`, { headers: { Accept: "application/json" } });
    if (!res.ok) {
      throw new Error("status fetch failed");
    }
    return res.json();
  }

  function applyStatus(data) {
    if (!data) return false;
    if (data.healthy) {
      window.location.assign(data.returnTo || "/");
      return true;
    }

    const message = data.statusMessage || "App start was requested. Waiting for health check before redirect.";
    if (data.leaseActive) {
      showPollStep();
      if (pollCopy) {
        pollCopy.textContent = pollCopyText(data);
      }
      renderCountdown(data.never ? null : data.expiresAt);
      setStatus(message, false);
      setStatusDetail(detailMessage(data));
      return true;
    }

    renderCountdown(null);
    showTimerStep();
    setStatusDetail("");
    setFormStatus(message, false);
    return false;
  }

  function startPolling() {
    if (pollTimer) return;
    poll()
      .then(applyStatus)
      .catch(() => { setStatus("Unable to check app status yet.", true); });
    pollTimer = setInterval(() => {
      poll()
        .then((data) => {
          if (!applyStatus(data)) {
            clearInterval(pollTimer);
            pollTimer = null;
          }
        })
        .catch(() => { setStatus("Unable to check app status yet.", true); });
    }, cfg.pollMillis || 5000);
  }

  async function reconcileAfterLeaseAttempt(defaultMessage, errorMessage) {
    try {
      const data = await poll();
      const activeLease = applyStatus(data);
      if (activeLease) {
        setFormStatus(defaultMessage, false);
        startPolling();
        return;
      }
      if (submitButton) {
        submitButton.disabled = false;
        submitButton.textContent = "Start Timer";
      }
      setFormStatus(errorMessage, true);
    } catch (_) {
      if (submitButton) {
        submitButton.disabled = false;
        submitButton.textContent = "Start Timer";
      }
      setFormStatus(errorMessage, true);
    }
  }

  poll()
    .then((data) => {
      if (applyStatus(data)) {
        startPolling();
      }
    })
    .catch(() => { setFormStatus("Unable to check app status yet.", true); });

  if (!form) return;

  form.addEventListener("submit", async function (event) {
    event.preventDefault();
    const body = new FormData(form);
    const params = new URLSearchParams({ host: cfg.host });
    if (submitButton) {
      submitButton.disabled = true;
      submitButton.textContent = "Saving...";
    }
    setFormStatus("Saving timer and checking app status...", false);

    try {
      const res = await fetch(`${cfg.publicPath}/api/lease?${params}`, { method: "POST", body });
      if (res.ok) {
        if (submitButton) {
          submitButton.disabled = false;
          submitButton.textContent = "Saved";
        }
        await reconcileAfterLeaseAttempt("Timer saved. Waiting for health check...", "Timer save could not be confirmed on the server.");
        return;
      }
    } catch (_) {
      // Reconcile against backend state below after transient fetch failures.
    }

    if (submitButton) {
      submitButton.disabled = false;
      submitButton.textContent = "Start Timer";
    }
    await reconcileAfterLeaseAttempt("Timer already active on the server. Waiting for health check...", "Timer update failed and no active lease was found.");
  });

  function pollCopyText(data) {
    if (data.readinessState === "health_unreachable") {
      return data.never
        ? "Reveille saved a no-stop lease, but the health endpoint is not reachable yet."
        : "Reveille saved your timer, but the health endpoint is not reachable yet.";
    }
    if (data.readinessState === "health_unhealthy") {
      return data.never
        ? "Reveille saved a no-stop lease, but the health endpoint is returning a non-healthy status."
        : "Reveille saved your timer, but the health endpoint is returning a non-healthy status.";
    }
    return data.never
      ? "Reveille saved a no-stop lease. This page will redirect automatically as soon as readiness checks pass."
      : "Reveille saved your timer. This page will redirect automatically as soon as readiness checks pass.";
  }

  function detailMessage(data) {
    if (data.readinessState === "health_unreachable") {
      return data.healthError ? `Health check error: ${data.healthError}` : "Reveille cannot reach the configured health endpoint yet.";
    }
    if (data.readinessState === "health_unhealthy") {
      return data.healthStatus ? `Health check returned HTTP ${data.healthStatus}. Redirect will stay blocked until a healthy status is returned.` : "Health check returned a non-healthy response.";
    }
    if (data.lastCheck) {
      return `Last health check: ${new Date(data.lastCheck).toLocaleTimeString()}.`;
    }
    return "";
  }
}());
