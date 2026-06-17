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
  const statusPill = document.getElementById("status-pill");
  const formStatus = document.getElementById("form-status");
  const form = document.getElementById("lease-form");
  const timerStep = document.getElementById("timer-step");
  const pollStep = document.getElementById("poll-step");
  const submitButton = document.getElementById("lease-submit");
  const countdown = document.getElementById("countdown");
  const countdownValue = document.getElementById("countdown-value");
  const countdownCaption = document.getElementById("countdown-caption");
  const pollCopy = document.getElementById("poll-copy");
  const timerCopy = document.getElementById("timer-copy");
  const statusDetail = document.getElementById("status-detail");
  const leaseOptions = Array.from(document.querySelectorAll(".lease-option"));
  let pollTimer = null;
  let countdownTimer = null;
  let waitingForLease = true;

  function timerStartedKey() {
    return `reveille:${cfg.host || "unknown"}:timer-started`;
  }

  function browserStartedTimer() {
    try {
      return window.sessionStorage.getItem(timerStartedKey()) === "true";
    } catch (_) {
      return false;
    }
  }

  function rememberTimerStarted() {
    try {
      window.sessionStorage.setItem(timerStartedKey(), "true");
    } catch (_) {
      // Session storage is a convenience; the backend lease remains the source of truth.
    }
  }

  function forgetTimerStarted() {
    try {
      window.sessionStorage.removeItem(timerStartedKey());
    } catch (_) {
      // Ignore storage failures.
    }
  }

  function publicURL(path) {
    return `${cfg.publicPath || ""}${path}`;
  }

  function waitURL(params) {
    const query = new URLSearchParams(params);
    return `${cfg.waitPath || publicURL("/wait")}?${query}`;
  }

  function setPill(label, mode) {
    if (!statusPill) return;
    statusPill.textContent = label;
    statusPill.classList.toggle("error", mode === "error");
    statusPill.classList.toggle("ready", mode === "ready");
  }

  function setStatus(message, isError) {
    if (!status) return;
    status.textContent = message;
    status.classList.toggle("error", Boolean(isError));
    if (isError) {
      setPill("Needs attention", "error");
    }
  }

  function setFormStatus(message, isError) {
    if (!formStatus) return;
    formStatus.textContent = message;
    formStatus.classList.remove("hidden");
    formStatus.classList.toggle("error", Boolean(isError));
    if (isError) {
      setPill("Needs attention", "error");
    }
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

  function setSubmitState(disabled, label) {
    if (!submitButton) return;
    submitButton.disabled = disabled;
    submitButton.textContent = label;
  }

  function showPollStep() {
    if (timerStep) timerStep.classList.add("hidden");
    if (pollStep) pollStep.classList.remove("hidden");
  }

  function showTimerStep() {
    if (pollStep) pollStep.classList.add("hidden");
    if (timerStep) timerStep.classList.remove("hidden");
  }

  function updateSelectedLease() {
    leaseOptions.forEach((option) => {
      const input = option.querySelector("input");
      option.classList.toggle("selected", Boolean(input && input.checked));
    });
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
      if (countdownCaption) countdownCaption.textContent = "automatic stop disabled";
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
    if (countdownCaption) countdownCaption.textContent = "remaining";

    const updateCountdown = function () {
      const remaining = deadline.getTime() - Date.now();
      if (remaining <= 0) {
        countdownValue.textContent = "00:00";
        if (countdownCaption) countdownCaption.textContent = "expired";
        forgetTimerStarted();
        stopCountdown();
        return;
      }

      const totalSeconds = Math.ceil(remaining / 1000);
      const hours = Math.floor(totalSeconds / 3600);
      const minutes = Math.floor((totalSeconds % 3600) / 60);
      const seconds = totalSeconds % 60;
      countdownValue.textContent = hours > 0
        ? `${String(hours).padStart(2, "0")}:${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`
        : `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`;
    };

    updateCountdown();
    stopCountdown();
    countdownTimer = setInterval(updateCountdown, 1000);
  }

  async function poll() {
    const res = await fetch(waitURL({ host: cfg.host, returnTo: cfg.returnTo, format: "status" }), { headers: { Accept: "application/json" } });
    if (!res.ok) {
      throw new Error("status fetch failed");
    }
    return res.json();
  }

  async function readErrorMessage(res, fallback) {
    const prefix = `${res.status} ${res.statusText || "request failed"}`.trim();
    try {
      const text = (await res.text()).trim();
      if (text) return `${prefix}: ${text}`;
    } catch (_) {
      // Fall through to the fallback below.
    }
    return `${prefix}: ${fallback}`;
  }

  function applyLease(leaseData) {
    rememberTimerStarted();
    waitingForLease = false;
    showPollStep();
    setPill("Timer active", "ready");
    if (pollCopy) {
      pollCopy.textContent = leaseData.never
        ? "Timer saved. Reveille will keep watching readiness with automatic stop disabled."
        : "Timer saved. Reveille will send you through as soon as readiness checks pass.";
    }
    renderCountdown(leaseData.never ? null : leaseData.expiresAt);
    setStatus("Waiting for health check...", false);
    setStatusDetail("");
    setSubmitState(false, "Saved");
  }

  function showTimerChoice(data) {
    waitingForLease = true;
    renderCountdown(null);
    showTimerStep();
    setPill(data && data.healthy ? "Ready" : "Starting", data && data.healthy ? "ready" : "");
    setStatusDetail("");
    setSubmitState(false, "Start Timer");
    if (timerCopy) {
      timerCopy.textContent = data && data.healthy
        ? "The app is ready. Choose a run window to continue."
        : "The app is waking up. Pick how long it should stay available.";
    }
    setFormStatus("Choose a timer to continue.", false);
    return false;
  }

  function applyStatus(data, allowActiveLease) {
    if (!data) return false;
    waitingForLease = !data.leaseActive;

    if (data.leaseActive && !allowActiveLease) {
      return showTimerChoice(data);
    }

    if (data.healthy && data.leaseActive) {
      setPill("Ready", "ready");
      window.location.assign(data.returnTo || "/");
      return true;
    }

    const message = data.statusMessage || "Waiting for health check...";
    if (data.leaseActive) {
      showPollStep();
      setPill(data.never ? "No stop" : "Timer active", "ready");
      if (pollCopy) {
        pollCopy.textContent = pollCopyText(data);
      }
      renderCountdown(data.never ? null : data.expiresAt);
      setStatus(message, false);
      setStatusDetail(detailMessage(data));
      setSubmitState(false, "Saved");
      return true;
    }

    forgetTimerStarted();
    renderCountdown(null);
    showTimerChoice(data);
    setFormStatus(message, false);
    return false;
  }

  function startPolling(initialData) {
    if (pollTimer) return;
    if (initialData) {
      applyStatus(initialData, true);
    } else {
      poll()
        .then((data) => { applyStatus(data, true); })
        .catch(() => { setStatus("Unable to check app status yet.", true); });
    }
    pollTimer = setInterval(() => {
      poll()
        .then((data) => {
          if (!applyStatus(data, true)) {
            clearInterval(pollTimer);
            pollTimer = null;
          }
        })
        .catch(() => {
          if (waitingForLease) {
            setFormStatus("Unable to confirm readiness yet.", true);
            return;
          }
          setStatus("Unable to check app status yet.", true);
        });
    }, cfg.pollMillis || 5000);
  }

  async function reconcileLeaseFailure(fallbackMessage) {
    try {
      const data = await poll();
      if (data && data.leaseActive) {
        rememberTimerStarted();
        applyStatus(data, true);
        setFormStatus("Timer started. Waiting for health check...", false);
        if (!pollTimer) {
          startPolling(data);
        }
        return;
      }
    } catch (_) {
      // If status is unavailable too, fall back to the original error below.
    }

    setSubmitState(false, "Start Timer");
    setFormStatus(fallbackMessage, true);
  }

  leaseOptions.forEach((option) => {
    option.addEventListener("change", updateSelectedLease);
    option.addEventListener("click", updateSelectedLease);
  });
  updateSelectedLease();

  poll()
    .then((data) => {
      if (applyStatus(data, browserStartedTimer())) {
        startPolling(data);
      }
    })
    .catch(() => { setFormStatus("Unable to check app status yet.", true); });

  if (!form) return;

  form.addEventListener("submit", async function (event) {
    event.preventDefault();
    const body = new FormData(form);
    setSubmitState(true, "Saving...");
    setPill("Saving", "");
    setFormStatus("Saving timer...", false);

    try {
      const res = await fetch(waitURL({ host: cfg.host }), { method: "POST", body });
      if (res.ok) {
        const data = await res.json();
        if (data && (data.never || data.expiresAt)) {
          applyLease(data);
          setFormStatus("Timer saved. Waiting for health check...", false);
          if (!pollTimer) {
            startPolling();
          }
          return;
        }
        setSubmitState(false, "Start Timer");
        setFormStatus("Timer save response was missing lease details.", true);
        return;
      }
      const message = await readErrorMessage(res, "Timer update failed.");
      await reconcileLeaseFailure(`Timer update failed: ${message}`);
      return;
    } catch (_) {
      await reconcileLeaseFailure("Timer update failed because the browser could not confirm the server response.");
    }
  });

  function pollCopyText(data) {
    if (data.readinessState === "health_unreachable") {
      return data.never
        ? "No-stop timer saved. Reveille cannot reach the health endpoint yet."
        : "Timer saved. Reveille cannot reach the health endpoint yet.";
    }
    if (data.readinessState === "health_unhealthy") {
      return data.never
        ? "No-stop timer saved. The health endpoint is not healthy yet."
        : "Timer saved. The health endpoint is not healthy yet.";
    }
    return data.never
      ? "No-stop timer saved. Reveille will send you through when readiness checks pass."
      : "Timer saved. Reveille will send you through when readiness checks pass.";
  }

  function detailMessage(data) {
    if (data.readinessState === "health_unreachable") {
      return data.healthError ? `Health check error: ${data.healthError}` : "Reveille cannot reach the configured health endpoint yet.";
    }
    if (data.readinessState === "health_unhealthy") {
      return data.healthStatus ? `Health check returned HTTP ${data.healthStatus}.` : "Health check returned a non-healthy response.";
    }
    if (data.lastCheck) {
      return `Last health check: ${new Date(data.lastCheck).toLocaleTimeString()}.`;
    }
    return "";
  }
}());
