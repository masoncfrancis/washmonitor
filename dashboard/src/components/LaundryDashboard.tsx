import { useState, useEffect, useRef } from "react";

const API_URL = (import.meta.env.VITE_API_URL as string) || "";

type User = { id: number; name: string; color: string };
type Appliance = "washer" | "dryer";

const LaundryDashboard = () => {
  const [washerUser, setWasherUser] = useState<number | null>(null);
  const [dryerUser, setDryerUser] = useState<number | null>(null);
  const [loading, setLoading] = useState<null | Appliance>(null);
  const [users, setUsers] = useState<User[]>([]);
  const [userNamesError, setUserNamesError] = useState(false);
  const [apiHealthy, setApiHealthy] = useState(true);
  const [washerOnline, setWasherOnline] = useState<boolean | null>(null);
  const [dryerOnline, setDryerOnline] = useState<boolean | null>(null);
  const [washerLastSeen, setWasherLastSeen] = useState<string | null>(null);
  const [dryerLastSeen, setDryerLastSeen] = useState<string | null>(null);

  const [stage, setStage] = useState<"main" | "select-user">("main");
  const [selectedAppliance, setSelectedAppliance] = useState<null | Appliance>(
    null,
  );

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

  const getUserById = (id: number | null) => {
    if (id === null) return undefined;
    return users.find((u) => u.id === id);
  };

  const getGridCols = () => {
    const count = users.length;
    if (count <= 2) return 2;
    if (count <= 4) return 2;
    return 3;
  };

  const parseUsersResponse = (data: Record<string, unknown>): User[] => {
    const parsedUsers: User[] = [];

    Object.entries(data).forEach(([key, value]) => {
      const id = Number.parseInt(key, 10);
      if (Number.isNaN(id)) return;
      if (!value || typeof value !== "object") return;
      const v = value as { name?: unknown; color?: unknown };
      if (typeof v.name !== "string" || typeof v.color !== "string") return;
      parsedUsers.push({ id, name: v.name, color: v.color });
    });

    return parsedUsers.sort((a, b) => a.id - b.id);
  };

  const postAgentStatus = async (
    appliance: Appliance,
    payload: { status: "idle" } | { status: "monitor"; user: string },
    controller: AbortController,
  ) => {
    const apiPath = appliance === "washer" ? "washer" : "dryer";
    await fetch(`${API_URL}/${apiPath}/setAgentStatus`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
      signal: controller.signal,
    });
  };

  const setMonitoringUser = (appliance: Appliance, userId: number | null) => {
    if (appliance === "washer") {
      setWasherUser(userId);
    } else {
      setDryerUser(userId);
    }
  };

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
          fetch(`${API_URL}/washer/getAgentStatus`, {
            signal: controller.signal,
          }),
          fetch(`${API_URL}/dryer/getAgentStatus`, {
            signal: controller.signal,
          }),
        ]);
        if (!mountedRef.current) return;

        setApiHealthy(washerRes.ok && dryerRes.ok);

        if (washerRes.ok) {
          const washerData = await washerRes.json();
          if (!mountedRef.current) return;
          if (washerData.status === "monitor" && washerData.user) {
            setWasherUser(Number.parseInt(washerData.user, 10));
          } else {
            setWasherUser(null);
          }
        }

        if (dryerRes.ok) {
          const dryerData = await dryerRes.json();
          if (!mountedRef.current) return;
          if (dryerData.status === "monitor" && dryerData.user) {
            setDryerUser(Number.parseInt(dryerData.user, 10));
          } else {
            setDryerUser(null);
          }
        }

        try {
          const healthRes = await fetch(`${API_URL}/health`, {
            signal: controller.signal,
          });
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
        controllersRef.current = controllersRef.current.filter(
          (c) => c !== controller,
        );
      }
    };

    const fetchNames = async () => {
      const controller = new AbortController();
      controllersRef.current.push(controller);
      try {
        const res = await fetch(`${API_URL}/users/names`, {
          signal: controller.signal,
        });
        if (!mountedRef.current) return;

        if (!res.ok) {
          setApiHealthy(false);
          setUserNamesError(true);
          setUsers([]);
          return;
        }

        const data = await res.json();
        if (!mountedRef.current) return;

        const parsed = parseUsersResponse(data);
        if (parsed.length === 0) {
          setUserNamesError(true);
          setUsers([]);
          return;
        }

        setApiHealthy(true);
        setUserNamesError(false);
        setUsers(parsed);
      } catch (e: any) {
        if (e?.name === "AbortError") return;
        setApiHealthy(false);
        setUserNamesError(true);
        setUsers([]);
        console.log("Error fetching user names:", e);
      } finally {
        controllersRef.current = controllersRef.current.filter(
          (c) => c !== controller,
        );
      }
    };

    fetchNames();
    fetchStatus();
    const intervalId = globalThis.setInterval(fetchStatus, 5000);

    return () => {
      mountedRef.current = false;
      globalThis.clearInterval(intervalId);
      abortAllControllers();
      clearAllTimeouts();
    };
  }, []);

  const handleUserClick = async (userId: number, appliance: Appliance) => {
    setLoading(appliance);
    const controller = new AbortController();
    controllersRef.current.push(controller);

    try {
      await postAgentStatus(
        appliance,
        { status: "monitor", user: String(userId) },
        controller,
      );
    } catch (e: any) {
      if (e?.name !== "AbortError") {
        console.log("Error setting status:", e);
      }
    } finally {
      const t = globalThis.setTimeout(() => {
        if (!mountedRef.current) return;
        setMonitoringUser(appliance, userId);
        setLoading(null);
        setStage("main");
        setSelectedAppliance(null);
      }, 300);
      timeoutsRef.current.push(t);
      controllersRef.current = controllersRef.current.filter(
        (c) => c !== controller,
      );
    }
  };

  const handleApplianceClick = (appliance: Appliance) => {
    const currentUser = appliance === "washer" ? washerUser : dryerUser;

    if (currentUser !== null) {
      setLoading(appliance);
      const controller = new AbortController();
      controllersRef.current.push(controller);
      postAgentStatus(appliance, { status: "idle" }, controller)
        .catch((e: any) => {
          if (e?.name === "AbortError") return;
          console.log("Error setting status:", e);
        })
        .finally(() => {
          const t = globalThis.setTimeout(() => {
            if (!mountedRef.current) return;
            setMonitoringUser(appliance, null);
            setLoading(null);
          }, 300);
          timeoutsRef.current.push(t);
          controllersRef.current = controllersRef.current.filter(
            (c) => c !== controller,
          );
        });
      return;
    }

    if (users.length === 1) {
      handleUserClick(users[0].id, appliance);
      return;
    }

    setSelectedAppliance(appliance);
    setStage("select-user");
  };

  return (
    <div className="app-root">
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
          <button
            type="button"
            className="appliance"
            style={{
              backgroundColor: getUserById(washerUser)?.color || "#3b82f6",
            }}
            onClick={() => handleApplianceClick("washer")}
          >
            {washerUser === null ? (
              <>
                <div className="appliance-title">Washer</div>
                <div className="appliance-muted">Tap to use</div>
              </>
            ) : (
              <>
                <div className="appliance-sub">
                  {getUserById(washerUser)?.name || "Unknown"} is using the
                </div>
                <div className="appliance-title">Washer</div>
                <div className="loader" />
              </>
            )}
          </button>

          <button
            type="button"
            className="appliance"
            style={{
              backgroundColor: getUserById(dryerUser)?.color || "#0c3a84",
            }}
            onClick={() => handleApplianceClick("dryer")}
          >
            {dryerUser === null ? (
              <>
                <div className="appliance-title">Dryer</div>
                <div className="appliance-muted">Tap to use</div>
              </>
            ) : (
              <>
                <div className="appliance-sub">
                  {getUserById(dryerUser)?.name || "Unknown"} is using the
                </div>
                <div className="appliance-title">Dryer</div>
                <div className="loader" />
              </>
            )}
          </button>
        </div>
      )}

      {stage === "select-user" && selectedAppliance && users.length > 1 && (
        <div className="select-user-stage">
          <div className="select-header">
            Who is using the {selectedAppliance}?
          </div>
          <div
            className="select-row"
            style={{
              display: "grid",
              gridTemplateColumns: `repeat(${getGridCols()}, minmax(0, 1fr))`,
            }}
          >
            {users.map((user) => (
              <button
                type="button"
                key={user.id}
                className="select-person"
                style={{ backgroundColor: user.color }}
                onClick={() => handleUserClick(user.id, selectedAppliance)}
              >
                {user.name}
              </button>
            ))}
          </div>
          <button
            type="button"
            className="select-cancel"
            onClick={() => {
              setStage("main");
              setSelectedAppliance(null);
            }}
          >
            Cancel
          </button>
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
