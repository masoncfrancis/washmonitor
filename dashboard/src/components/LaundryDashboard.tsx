import { useState, useEffect, useRef } from "react";

const API_URL = (import.meta.env.VITE_API_URL as string) || "";

type UserInfo = { name: string; color: string };

const LaundryDashboard = () => {
  const [washerUser, setWasherUser] = useState<string | null>(null);
  const [dryerUser, setDryerUser] = useState<string | null>(null);
  const [loading, setLoading] = useState<null | "washer" | "dryer">(null);
  const [userInfo, setUserInfo] = useState<{
    user1: UserInfo;
    user2: UserInfo;
  }>({
    user1: { name: "User1", color: "#3b82f6" },
    user2: { name: "User2", color: "#22c55e" },
  });
  const [userNamesError, setUserNamesError] = useState(false);
  const [apiHealthy, setApiHealthy] = useState(true);
  const [washerOnline, setWasherOnline] = useState<boolean | null>(null);
  const [dryerOnline, setDryerOnline] = useState<boolean | null>(null);
  const [washerLastSeen, setWasherLastSeen] = useState<string | null>(null);
  const [dryerLastSeen, setDryerLastSeen] = useState<string | null>(null);

  const mountedRef = useRef(false);
  const controllersRef = useRef<AbortController[]>([]);
  const timeoutsRef = useRef<number[]>([]);

  const formatRelativeTime = (iso: string | null) => {
    if (!iso) return "";
    const then = new Date(iso).getTime();
    if (Number.isNaN(then)) return "";
    const diff = Date.now() - then;
    if (diff < 0) return "just now";
    const seconds = Math.floor(diff / 1000);
    if (seconds < 10) return "just now";
    if (seconds < 60) return `${seconds}s ago`;
    const minutes = Math.floor(seconds / 60);
    if (minutes < 60) return `${minutes}m ago`;
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return `${hours}h ago`;
    const days = Math.floor(hours / 24);
    return `${days}d ago`;
  };

  const [stage, setStage] = useState<"main" | "select-user">("main");
  const [selectedAppliance, setSelectedAppliance] = useState<
    null | "washer" | "dryer"
  >(null);

  useEffect(() => {
    mountedRef.current = true;

    const abortAllControllers = () => {
      controllersRef.current.forEach((c) => c.abort());
      controllersRef.current = [];
    };

    const clearAllTimeouts = () => {
      timeoutsRef.current.forEach((t) => clearTimeout(t));
      timeoutsRef.current = [];
    };

    const fetchStatus = async () => {
      const controller = new AbortController();
      controllersRef.current.push(controller);
      try {
        const [washerRes, dryerRes] = await Promise.all([
          fetch(`${API_URL}/washer/getAgentStatus`, { signal: controller.signal }),
          fetch(`${API_URL}/dryer/getAgentStatus`, { signal: controller.signal }),
        ]);
        if (!mountedRef.current) return;

        if (washerRes.ok && dryerRes.ok) {
          setApiHealthy(true);
        } else {
          setApiHealthy(false);
        }
        if (washerRes.ok) {
          const washerData = await washerRes.json();
          if (!mountedRef.current) return;
          if (washerData.status === "monitor" && washerData.user) {
            setWasherUser(washerData.user);
          } else {
            setWasherUser(null);
          }
        }
        if (dryerRes.ok) {
          const dryerData = await dryerRes.json();
          if (!mountedRef.current) return;
          if (dryerData.status === "monitor" && dryerData.user) {
            setDryerUser(dryerData.user);
          } else {
            setDryerUser(null);
          }
        }
        try {
          const healthRes = await fetch(`${API_URL}/health`, { signal: controller.signal });
          if (!mountedRef.current) return;
          if (healthRes.ok) {
            const health = await healthRes.json();
            if (health?.api && typeof health.api.healthy === "boolean") {
              setApiHealthy(health.api.healthy);
            }
            if (health?.washer) {
              setWasherOnline(Boolean(health.washer.online));
              setWasherLastSeen(health.washer.lastSeen || null);
            }
            if (health?.dryer) {
              setDryerOnline(Boolean(health.dryer.online));
              setDryerLastSeen(health.dryer.lastSeen || null);
            }
          } else {
            setApiHealthy(false);
          }
        } catch (e: any) {
          if (e?.name === "AbortError") return;
          console.log("Error fetching health:", e);
          setApiHealthy(false);
        }
      } catch (e: any) {
        if (e?.name === "AbortError") return;
        console.log("Error fetching status:", e);
        setApiHealthy(false);
      } finally {
        // remove controller from list
        controllersRef.current = controllersRef.current.filter((c) => c !== controller);
      }
    };

    const fetchNames = async () => {
      const controller = new AbortController();
      controllersRef.current.push(controller);
      try {
        const res = await fetch(`${API_URL}/users/names`, { signal: controller.signal });
        if (!mountedRef.current) return;
        if (!res.ok) {
          setApiHealthy(false);
          setUserNamesError(true);
          setUserInfo({
            user1: { name: "User1", color: "#3b82f6" },
            user2: { name: "User2", color: "#22c55e" },
          });
          return;
        }
        const data = await res.json();
        if (!mountedRef.current) return;
        setApiHealthy(true);
        if (
          data.user1 &&
          data.user2 &&
          typeof data.user1.name === "string" &&
          typeof data.user2.name === "string" &&
          typeof data.user1.color === "string" &&
          typeof data.user2.color === "string"
        ) {
          setUserInfo({
            user1: { name: data.user1.name, color: data.user1.color },
            user2: { name: data.user2.name, color: data.user2.color },
          });
          setUserNamesError(false);
        } else {
          setUserNamesError(true);
          setUserInfo({
            user1: { name: "User1", color: "#3b82f6" },
            user2: { name: "User2", color: "#22c55e" },
          });
        }
      } catch (e: any) {
        if (e?.name === "AbortError") return;
        setApiHealthy(false);
        setUserNamesError(true);
        setUserInfo({
          user1: { name: "User1", color: "#3b82f6" },
          user2: { name: "User2", color: "#22c55e" },
        });
        console.log("Error fetching user names:", e);
      } finally {
        controllersRef.current = controllersRef.current.filter((c) => c.signal !== controller.signal);
      }
    };

    fetchNames();
    fetchStatus();
    const intervalId = window.setInterval(fetchStatus, 5000);
    return () => {
      mountedRef.current = false;
      clearInterval(intervalId);
      abortAllControllers();
      clearAllTimeouts();
    };
  }, []);

  const handleApplianceClick = (appliance: "washer" | "dryer") => {
    if (
      (appliance === "washer" && washerUser) ||
      (appliance === "dryer" && dryerUser)
    ) {
      setLoading(appliance);
      const apiPath = appliance === "washer" ? "washer" : "dryer";
      // create a controller for this user action request so it can be aborted if needed
      const controller = new AbortController();
      controllersRef.current.push(controller);
      fetch(`${API_URL}/${apiPath}/setAgentStatus`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ status: "idle" }),
        signal: controller.signal,
      }).catch((e) => {
        if ((e as any)?.name === "AbortError") return;
        console.log("Error setting status:", e);
      }).finally(() => {
        // simulate small delay for UI; store timeout so we can clear if unmounted
        const t = window.setTimeout(() => {
          if (!mountedRef.current) return;
          if (appliance === "washer") {
            setWasherUser(null);
          } else {
            setDryerUser(null);
          }
          setLoading(null);
        }, 300);
        timeoutsRef.current.push(t);
        controllersRef.current = controllersRef.current.filter((c) => c !== controller);
      });
      return;
    }
    setSelectedAppliance(appliance);
    setStage("select-user");
  };

  const handleUserClick = async (person: "user1" | "user2") => {
    if (!selectedAppliance) return;
    setLoading(selectedAppliance);
    const apiPath = selectedAppliance === "washer" ? "washer" : "dryer";
    const controller = new AbortController();
    controllersRef.current.push(controller);
    try {
      await fetch(`${API_URL}/${apiPath}/setAgentStatus`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ status: "monitor", user: person }),
        signal: controller.signal,
      });
    } catch (e: any) {
      if (e?.name !== "AbortError") {
        console.log("Error setting status:", e);
      }
    } finally {
      const t = window.setTimeout(() => {
        if (!mountedRef.current) return;
        if (selectedAppliance === "washer") {
          setWasherUser(person);
        } else {
          setDryerUser(person);
        }
        setLoading(null);
        setStage("main");
        setSelectedAppliance(null);
      }, 300);
      timeoutsRef.current.push(t);
      controllersRef.current = controllersRef.current.filter((c) => c !== controller);
    }
  };

  return (
    <div className="app-root">
      {/* Error banner */}
      {(() => {
        const issues: string[] = [];
        if (!apiHealthy) issues.push("Cannot reach API server");
        if (userNamesError) issues.push("Could not obtain user names");
        if (washerOnline === false) {
          issues.push(
            washerLastSeen
              ? `Washer Sensor Offline (${formatRelativeTime(washerLastSeen)})`
              : "Washer Sensor Offline",
          );
        }
        if (dryerOnline === false) {
          issues.push(
            dryerLastSeen
              ? `Dryer Sensor Offline (${formatRelativeTime(dryerLastSeen)})`
              : "Dryer Sensor Offline",
          );
        }
        if (issues.length === 0) return null;
        return <div className="error-banner">{issues.join(" • ")}</div>;
      })()}

      {stage === "main" && (
        <div className="appliances-row">
          <div
            className="appliance"
            style={{
              backgroundColor: washerUser
                ? userInfo[washerUser as "user1" | "user2"]?.color
                : "#3b82f6",
            }}
            onClick={() => handleApplianceClick("washer")}
          >
            {washerUser ? (
              <>
                <div className="appliance-sub">
                  {userInfo[washerUser as "user1" | "user2"]?.name} is using the
                </div>
                <div className="appliance-title">Washer</div>
                <div className="loader" />
              </>
            ) : (
              <>
                <div className="appliance-title">Washer</div>
                <div className="appliance-muted">Tap to use</div>
              </>
            )}
          </div>

          <div
            className="appliance"
            style={{
              backgroundColor: dryerUser
                ? userInfo[dryerUser as "user1" | "user2"]?.color
                : "#0c3a84",
            }}
            onClick={() => handleApplianceClick("dryer")}
          >
            {dryerUser ? (
              <>
                <div className="appliance-sub">
                  {userInfo[dryerUser as "user1" | "user2"]?.name} is using the
                </div>
                <div className="appliance-title">Dryer</div>
                <div className="loader" />
              </>
            ) : (
              <>
                <div className="appliance-title">Dryer</div>
                <div className="appliance-muted">Tap to use</div>
              </>
            )}
          </div>
        </div>
      )}

      {stage === "select-user" && selectedAppliance && (
        <div className="select-user-stage">
          <div className="select-header">
            Who is using the {selectedAppliance}?
          </div>
          <div className="select-row">
            <div
              className="select-person"
              style={{ backgroundColor: userInfo.user1.color }}
              onClick={() => handleUserClick("user1")}
            >
              {userInfo.user1.name}
            </div>
            <div
              className="select-person"
              style={{ backgroundColor: userInfo.user2.color }}
              onClick={() => handleUserClick("user2")}
            >
              {userInfo.user2.name}
            </div>
          </div>
          <div
            className="select-cancel"
            onClick={() => {
              setStage("main");
              setSelectedAppliance(null);
            }}
          >
            Cancel
          </div>
        </div>
      )}

      {loading && (
        <div className="loading-overlay">
          <div className="spinner" />
        </div>
      )}
    </div>
  );
};

export default LaundryDashboard;
