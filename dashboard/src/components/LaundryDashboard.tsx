'use client';

import { useState, useEffect } from 'react';



const API_URL = process.env.NEXT_PUBLIC_API_URL;

const LaundryDashboard = () => {
    // Track status for both appliances
    const [washerUser, setWasherUser] = useState<string | null>(null);
    const [dryerUser, setDryerUser] = useState<string | null>(null);
    const [loading, setLoading] = useState<null | 'washer' | 'dryer'>(null);
    type UserInfo = { name: string; color: string };
    const [userInfo, setUserInfo] = useState<{ user1: UserInfo; user2: UserInfo }>({
        user1: { name: 'User1', color: '#3b82f6' }, // blue-500 as hex
        user2: { name: 'User2', color: '#22c55e' }, // green-500 as hex
    });
    const [userNamesError, setUserNamesError] = useState(false);
    const [apiHealthy, setApiHealthy] = useState(true);
    const [washerOnline, setWasherOnline] = useState<boolean | null>(null);
    const [dryerOnline, setDryerOnline] = useState<boolean | null>(null);
    const [washerLastSeen, setWasherLastSeen] = useState<string | null>(null);
    const [dryerLastSeen, setDryerLastSeen] = useState<string | null>(null);

    const formatRelativeTime = (iso: string | null) => {
        if (!iso) return '';
        const then = new Date(iso).getTime();
        if (Number.isNaN(then)) return '';
        const diff = Date.now() - then;
        if (diff < 0) return 'just now';
        const seconds = Math.floor(diff / 1000);
        if (seconds < 10) return 'just now';
        if (seconds < 60) return `${seconds}s ago`;
        const minutes = Math.floor(seconds / 60);
        if (minutes < 60) return `${minutes}m ago`;
        const hours = Math.floor(minutes / 60);
        if (hours < 24) return `${hours}h ago`;
        const days = Math.floor(hours / 24);
        return `${days}d ago`;
    };


    // 2-stage interaction state
    const [stage, setStage] = useState<'main' | 'select-user'>('main');
    const [selectedAppliance, setSelectedAppliance] = useState<null | 'washer' | 'dryer'>(null);

    useEffect(() => {
        const fetchStatus = async () => {
            try {
                const [washerRes, dryerRes] = await Promise.all([
                    fetch(`${API_URL}/washer/getAgentStatus`),
                    fetch(`${API_URL}/dryer/getAgentStatus`),
                ]);
                // If any of the status endpoints fail, mark API as unhealthy
                if (washerRes.ok && dryerRes.ok) {
                    setApiHealthy(true);
                } else {
                    setApiHealthy(false);
                }
                if (washerRes.ok) {
                    const washerData = await washerRes.json();
                    if (washerData.status === 'monitor' && washerData.user) {
                        setWasherUser(washerData.user);
                    } else {
                        setWasherUser(null);
                    }
                }
                if (dryerRes.ok) {
                    const dryerData = await dryerRes.json();
                    if (dryerData.status === 'monitor' && dryerData.user) {
                        setDryerUser(dryerData.user);
                    } else {
                        setDryerUser(null);
                    }
                }
                // Fetch consolidated health info (includes washer/dryer online + lastSeen)
                try {
                    const healthRes = await fetch(`${API_URL}/health`);
                    if (healthRes.ok) {
                        const health = await healthRes.json();
                        if (health?.api && typeof health.api.healthy === 'boolean') {
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
                } catch (e) {
                    console.log('Error fetching health:', e);
                    setApiHealthy(false);
                }
            } catch (e) {
                console.log('Error fetching status:', e);
                setApiHealthy(false);
            }
        };

        const fetchNames = async () => {
            try {
                const res = await fetch(`${API_URL}/users/names`);
                if (!res.ok) {
                    setApiHealthy(false);
                    setUserNamesError(true);
                    setUserInfo({
                        user1: { name: 'User1', color: '#3b82f6' },
                        user2: { name: 'User2', color: '#22c55e' },
                    });
                    return;
                }
                const data = await res.json();
                setApiHealthy(true);
                if (
                    data.user1 && data.user2 &&
                    typeof data.user1.name === 'string' && typeof data.user2.name === 'string' &&
                    typeof data.user1.color === 'string' && typeof data.user2.color === 'string'
                ) {
                    setUserInfo({
                        user1: { name: data.user1.name, color: data.user1.color },
                        user2: { name: data.user2.name, color: data.user2.color },
                    });
                    setUserNamesError(false);
                } else {
                    setUserNamesError(true);
                    setUserInfo({
                        user1: { name: 'User1', color: '#3b82f6' },
                        user2: { name: 'User2', color: '#22c55e' },
                    });
                }
            } catch (e) {
                setApiHealthy(false);
                setUserNamesError(true);
                setUserInfo({
                    user1: { name: 'User1', color: '#3b82f6' },
                    user2: { name: 'User2', color: '#22c55e' },
                });
                console.log('Error fetching user names:', e);
            }
        };

        fetchNames();
        fetchStatus();
        const interval = setInterval(fetchStatus, 5000);
        return () => clearInterval(interval);
    }, []);

    // Stage 1: select appliance, Stage 2: select user
    // Clicking an in-use appliance cancels it (sets to idle)
    const handleApplianceClick = (appliance: 'washer' | 'dryer') => {
        if ((appliance === 'washer' && washerUser) || (appliance === 'dryer' && dryerUser)) {
            setLoading(appliance);
            const apiPath = appliance === 'washer' ? 'washer' : 'dryer';
            fetch(`${API_URL}/${apiPath}/setAgentStatus`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ status: 'idle' }),
            }).finally(() => {
                setTimeout(() => {
                    if (appliance === 'washer') {
                        setWasherUser(null);
                    } else {
                        setDryerUser(null);
                    }
                    setLoading(null);
                }, 300);
            });
            return;
        }
        setSelectedAppliance(appliance);
        setStage('select-user');
    };

    const handleUserClick = async (person: 'user1' | 'user2') => {
        if (!selectedAppliance) return;
        setLoading(selectedAppliance);
        const apiPath = selectedAppliance === 'washer' ? 'washer' : 'dryer';
        try {
            await fetch(`${API_URL}/${apiPath}/setAgentStatus`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ status: 'monitor', user: person }),
            });
        } catch (e) {
            console.log('Error setting status:', e);
        }
        setTimeout(() => {
            if (selectedAppliance === 'washer') {
                setWasherUser(person);
            } else {
                setDryerUser(person);
            }
            setLoading(null);
            setStage('main');
            setSelectedAppliance(null);
        }, 300);
    };

    // handleClear removed; handled by handleApplianceClick


    return (
        <div className="flex flex-col h-screen w-screen">
            {/* Unified error banner: only shown when there are issues */}
            {(() => {
                const issues: string[] = [];
                if (!apiHealthy) issues.push('Cannot reach API server');
                if (userNamesError) issues.push('Could not obtain user names');
                if (washerOnline === false) {
                    issues.push(washerLastSeen ? `Washer sensor offline (${formatRelativeTime(washerLastSeen)})` : 'Washer sensor offline');
                }
                if (dryerOnline === false) {
                    issues.push(dryerLastSeen ? `Dryer sensor offline (${formatRelativeTime(dryerLastSeen)})` : 'Dryer sensor offline');
                }
                if (issues.length === 0) return null;
                return (
                    <div className="w-full bg-red-700 text-white text-center py-2 text-lg font-semibold shadow-md z-30">
                        {issues.join(' • ')}
                    </div>
                );
            })()}
            {stage === 'main' && (
                <div className="flex flex-1 flex-row w-full h-full">
                    {/* Washer splitscreen */}
                    <div
                        className="flex-1 flex flex-col justify-center items-center text-4xl cursor-pointer text-center break-words text-white h-full"
                        style={{ backgroundColor: washerUser ? userInfo[washerUser as 'user1' | 'user2']?.color : '#3b82f6' }}
                        onClick={() => handleApplianceClick('washer')}
                    >
                        {washerUser ? (
                            <>
                                <div className="text-2xl">{userInfo[washerUser as 'user1' | 'user2']?.name} is using the</div>
                                <div className="text-3xl font-bold mb-2">Washer</div>
                                <div className="loader-running mt-4"></div>
                            </>
                        ) : (
                            <>
                                <div className="text-3xl font-bold mb-2">Washer</div>
                                <div className="text-xl opacity-80">Tap to use</div>
                            </>
                        )}
                    </div>
                    {/* Dryer splitscreen */}
                    <div
                        className="flex-1 flex flex-col justify-center items-center text-4xl cursor-pointer text-center break-words text-white h-full"
                        style={{ backgroundColor: dryerUser ? userInfo[dryerUser as 'user1' | 'user2']?.color : '#0c3a84ff' }}
                        onClick={() => handleApplianceClick('dryer')}
                    >
                        
                        {dryerUser ? (
                            <>
                                <div className="text-2xl">{userInfo[dryerUser as 'user1' | 'user2']?.name} is using the</div>
                                <div className="text-3xl font-bold mb-2">Dryer</div>
                                <div className="loader-running mt-4"></div>
                            </>
                        ) : (
                            <>
                                <div className="text-3xl font-bold mb-2">Dryer</div>
                                <div className="text-xl opacity-80">Tap to use</div>
                            </>
                            
                        )}
                    </div>
                </div>
            )}
            {stage === 'select-user' && selectedAppliance && (
                <div className="flex flex-1 flex-col h-full w-full">
                    <div className="w-full bg-gray-900 text-white text-center py-4 text-2xl font-semibold shadow-md z-10">
                        Who is using the {selectedAppliance}?
                    </div>
                    <div className="flex flex-row w-full flex-1">
                        <div
                            className="flex-1 flex flex-col justify-center items-center text-4xl cursor-pointer text-center break-words text-white h-full"
                            style={{ backgroundColor: userInfo.user1.color }}
                            onClick={() => handleUserClick('user1')}
                        >
                            {userInfo.user1.name}
                        </div>
                        <div
                            className="flex-1 flex flex-col justify-center items-center text-4xl cursor-pointer text-center break-words text-white h-full"
                            style={{ backgroundColor: userInfo.user2.color }}
                            onClick={() => handleUserClick('user2')}
                        >
                            {userInfo.user2.name}
                        </div>
                    </div>
                    <div
                        className="w-full bg-gray-700 text-white text-center py-6 text-2xl font-semibold shadow-md cursor-pointer hover:bg-gray-600 transition-colors duration-150"
                        onClick={() => { setStage('main'); setSelectedAppliance(null); }}
                        style={{ borderTop: '1px solid #4b5563' }}
                    >
                        Cancel
                    </div>
                </div>
            )}
            {/* Loading overlay for API actions, but not using loader-running */}
            {loading && (
                <div
                    className="fixed top-0 left-0 w-full h-full bg-black bg-opacity-30 flex justify-center items-center z-50 transition-opacity duration-500">
                    <div
                        className="loader w-16 h-16 border-4 border-t-black border-b-black border-solid rounded-full animate-spin"></div>
                </div>
            )}
        </div>
    );
};

export default LaundryDashboard;