// Real-time Updates Client
// Handles Server-Sent Events for live data updates

class RealtimeUpdates {
    constructor(appInstance) {
        this.app = appInstance;
        this.eventSource = null;
        this.isConnected = false;
        this.reconnectAttempts = 0;
        this.maxReconnectAttempts = 5;
        this.reconnectDelay = 1000; // Start with 1 second
        this.maxReconnectDelay = 30000; // Max 30 seconds
        this.clientId = this.generateClientId();
        this.lastHeartbeat = null;
        
        this.init();
    }

    init() {
        this.connect();
        this.setupVisibilityHandling();
        console.log('Real-time updates initialized');
    }

    connect() {
        if (this.eventSource) {
            this.eventSource.close();
        }

        // Build SSE URL with current viewport
        const params = new URLSearchParams({
            client_id: this.clientId
        });

        // Add viewport bounds if map is available
        if (this.app.map && this.app.map.map) {
            const bounds = this.app.map.map.getBounds();
            params.set('min_lat', bounds.getSouth());
            params.set('max_lat', bounds.getNorth());
            params.set('min_lng', bounds.getWest());
            params.set('max_lng', bounds.getEast());
        }

        const sseUrl = `/api/updates/stream?${params.toString()}`;

        try {
            this.eventSource = new EventSource(sseUrl);
            this.setupEventHandlers();
            console.log('Connecting to real-time updates...');
        } catch (error) {
            console.error('Failed to create EventSource:', error);
            this.scheduleReconnect();
        }
    }

    setupEventHandlers() {
        this.eventSource.onopen = (event) => {
            this.isConnected = true;
            this.reconnectAttempts = 0;
            this.reconnectDelay = 1000;
            this.showConnectionStatus('Connected to live updates', 'success');
            console.log('Real-time updates connected');
        };

        this.eventSource.onerror = (event) => {
            this.isConnected = false;
            console.error('SSE connection error:', event);
            
            if (this.eventSource.readyState === EventSource.CLOSED) {
                this.showConnectionStatus('Connection lost, attempting to reconnect...', 'warning');
                this.scheduleReconnect();
            }
        };

        // Handle specific event types
        this.eventSource.addEventListener('connected', (event) => {
            const data = JSON.parse(event.data);
            console.log('SSE connected:', data.data.message);
        });

        this.eventSource.addEventListener('data_update', (event) => {
            const notification = JSON.parse(event.data);
            this.handleDataUpdate(notification.data);
        });

        this.eventSource.addEventListener('stats_update', (event) => {
            const notification = JSON.parse(event.data);
            this.handleStatsUpdate(notification.data);
        });

        this.eventSource.addEventListener('heartbeat', (event) => {
            const notification = JSON.parse(event.data);
            this.lastHeartbeat = new Date(notification.data.timestamp);
            this.checkConnection();
        });

        // Generic message handler for unknown event types
        this.eventSource.onmessage = (event) => {
            try {
                const notification = JSON.parse(event.data);
                console.log('Received update:', notification);
            } catch (error) {
                console.error('Failed to parse SSE message:', error);
            }
        };
    }

    handleDataUpdate(updateData) {
        console.log('Data update received:', updateData);
        
        // Show notification to user
        if (updateData.records_changed > 0) {
            const message = `${updateData.records_changed} records updated (${updateData.change_type})`;
            this.app.showNotification(message, 'info', 3000);
            
            // Refresh map data if in current viewport
            if (this.shouldRefreshData(updateData)) {
                this.refreshMapData();
            }
        }
    }

    handleStatsUpdate(statsData) {
        // Update statistics in the UI
        this.updateStatsDisplay(statsData);
    }

    shouldRefreshData(updateData) {
        // Logic to determine if current view should be refreshed
        // For now, always refresh if records changed
        return updateData.records_changed > 0;
    }

    async refreshMapData() {
        try {
            // Use debouncing to avoid too frequent refreshes
            clearTimeout(this.refreshTimeout);
            this.refreshTimeout = setTimeout(async () => {
                console.log('Refreshing map data due to real-time update...');
                await this.app.map.loadData(this.app.filters.getAPIFilters());
            }, 2000); // Wait 2 seconds before refreshing
        } catch (error) {
            console.error('Error refreshing map data:', error);
        }
    }

    updateStatsDisplay(stats) {
        // Update main statistics display
        const elements = {
            'total-records': stats.total_records,
            'matched-records': stats.matched_records,
            'match-rate': stats.match_rate.toFixed(1) + '%'
        };

        Object.entries(elements).forEach(([id, value]) => {
            const element = document.getElementById(id);
            if (element) {
                // Animate the change if different
                const currentValue = element.textContent;
                if (currentValue !== value.toString()) {
                    element.style.transition = 'color 0.3s ease';
                    element.style.color = '#007bff';
                    element.textContent = typeof value === 'number' ? value.toLocaleString() : value;
                    
                    // Reset color after animation
                    setTimeout(() => {
                        element.style.color = '';
                    }, 300);
                }
            }
        });
    }

    scheduleReconnect() {
        if (this.reconnectAttempts >= this.maxReconnectAttempts) {
            this.showConnectionStatus('Connection failed. Refresh page to retry.', 'error', 0);
            return;
        }

        this.reconnectAttempts++;
        const delay = Math.min(this.reconnectDelay * Math.pow(2, this.reconnectAttempts - 1), this.maxReconnectDelay);

        console.log(`Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts}/${this.maxReconnectAttempts})`);
        
        setTimeout(() => {
            this.connect();
        }, delay);
    }

    checkConnection() {
        // Check if heartbeats are coming through
        if (this.lastHeartbeat) {
            const timeSinceHeartbeat = Date.now() - this.lastHeartbeat.getTime();
            if (timeSinceHeartbeat > 60000) { // No heartbeat for 1 minute
                console.warn('No heartbeat received, connection may be stale');
                this.showConnectionStatus('Connection issues detected', 'warning');
            }
        }
    }

    showConnectionStatus(message, type = 'info', duration = 3000) {
        // Show connection status notification
        this.app.showNotification(message, type, duration);
        
        // Update connection indicator if it exists
        const indicator = document.getElementById('connection-status');
        if (indicator) {
            indicator.className = `connection-status ${type}`;
            indicator.textContent = message;
            
            if (duration > 0) {
                setTimeout(() => {
                    indicator.textContent = '';
                    indicator.className = 'connection-status';
                }, duration);
            }
        }
    }

    setupVisibilityHandling() {
        // Handle page visibility changes
        document.addEventListener('visibilitychange', () => {
            if (document.visibilityState === 'visible') {
                // Page became visible - reconnect if needed
                if (!this.isConnected) {
                    this.connect();
                }
            } else {
                // Page hidden - we might want to close connection to save resources
                // For now, keep connection open for background updates
            }
        });
    }

    generateClientId() {
        return 'client_' + Date.now() + '_' + Math.random().toString(36).substr(2, 9);
    }

    // Public methods
    disconnect() {
        if (this.eventSource) {
            this.eventSource.close();
            this.eventSource = null;
            this.isConnected = false;
            console.log('Real-time updates disconnected');
        }
    }

    reconnect() {
        this.disconnect();
        this.reconnectAttempts = 0;
        this.connect();
    }

    getConnectionStatus() {
        return {
            connected: this.isConnected,
            clientId: this.clientId,
            lastHeartbeat: this.lastHeartbeat,
            reconnectAttempts: this.reconnectAttempts
        };
    }

    // Trigger manual refresh of all clients (if user has permission)
    async triggerGlobalRefresh() {
        try {
            const response = await fetch('/api/updates/refresh', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                }
            });

            if (response.ok) {
                const result = await response.json();
                this.app.showNotification(result.message, 'success');
            }
        } catch (error) {
            console.error('Failed to trigger global refresh:', error);
            this.app.showError('Failed to trigger refresh');
        }
    }

    // Get matching process status
    async getMatchingStatus() {
        try {
            const response = await fetch('/api/updates/status');
            if (response.ok) {
                return await response.json();
            }
        } catch (error) {
            console.error('Failed to get matching status:', error);
        }
        return null;
    }
}

// Export the class
window.RealtimeUpdates = RealtimeUpdates;