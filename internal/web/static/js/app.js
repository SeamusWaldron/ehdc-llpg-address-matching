// Main Application Controller
// Handles component initialization, state management, and coordination

class EHDCMappingApp {
    constructor() {
        this.map = null;
        this.filters = null;
        this.realtime = null;
        this.currentSelection = null;
        this.stats = {
            total: 0,
            visible: 0,
            matched: 0,
            unmatched: 0
        };
        
        this.init();
    }

    async init() {
        console.log('Initializing EHDC LLPG Mapping Application...');
        
        try {
            // Initialize components
            this.initializeComponents();
            
            // Load URL state
            this.loadURLState();
            
            // Setup event handlers
            this.setupEventHandlers();
            
            // Load initial data
            await this.loadInitialData();
            
            // Initialize real-time updates (after components are ready)
            this.initializeRealtime();
            
            console.log('Application initialized successfully');
        } catch (error) {
            console.error('Failed to initialize application:', error);
            this.showError('Failed to initialize application. Please refresh the page.');
        }
    }

    initializeComponents() {
        // Initialize enhanced map component
        this.map = new MapComponent('map', {
            center: [-1.0, 50.9], // Hampshire, UK
            zoom: 10
        });

        // Initialize advanced filter panel
        this.filters = new FilterPanel('filter-container', this.map);

        // Initialize record drawer
        this.recordDrawer = new RecordDrawer(this);

        console.log('Components initialized');
    }

    initializeRealtime() {
        // Initialize real-time updates if available
        if (typeof RealtimeUpdates !== 'undefined') {
            this.realtime = new RealtimeUpdates(this);
            console.log('Real-time updates enabled');
        } else {
            console.log('Real-time updates not available');
        }
    }

    loadURLState() {
        const urlParams = new URLSearchParams(window.location.search);
        const savedFilters = {};

        // Extract filters from URL
        if (urlParams.has('search')) {
            savedFilters.addressSearch = urlParams.get('search');
        }
        
        if (urlParams.has('source')) {
            savedFilters.sourceTypes = urlParams.get('source').split(',');
        }
        
        if (urlParams.has('status')) {
            savedFilters.matchStatus = urlParams.get('status').split(',');
        }
        
        if (urlParams.has('quality')) {
            savedFilters.addressQuality = urlParams.get('quality').split(',');
        }
        
        if (urlParams.has('score')) {
            const range = urlParams.get('score').split('-');
            if (range.length === 2) {
                savedFilters.matchScoreRange = [parseFloat(range[0]), parseFloat(range[1])];
            }
        }

        // Extract map state
        if (urlParams.has('lat') && urlParams.has('lng') && urlParams.has('zoom')) {
            const lat = parseFloat(urlParams.get('lat'));
            const lng = parseFloat(urlParams.get('lng'));
            const zoom = parseInt(urlParams.get('zoom'));
            
            if (!isNaN(lat) && !isNaN(lng) && !isNaN(zoom)) {
                this.map.map.setCenter([lng, lat]);
                this.map.map.setZoom(zoom);
            }
        }

        // Apply saved filters
        if (Object.keys(savedFilters).length > 0) {
            this.filters.setFilters(savedFilters);
        }

        console.log('URL state loaded:', savedFilters);
    }

    setupEventHandlers() {
        // Map event handlers
        document.addEventListener('map:dataUpdated', (e) => {
            this.handleDataUpdated(e.detail);
        });

        document.addEventListener('map:selectionCompleted', (e) => {
            this.handleSelectionCompleted(e.detail);
        });

        document.addEventListener('map:selectionCleared', () => {
            this.handleSelectionCleared();
        });

        document.addEventListener('map:error', (e) => {
            this.showError(`Map error: ${e.detail.error.message}`);
        });

        // Filter event handlers
        document.addEventListener('filters:filtersChanged', (e) => {
            this.handleFiltersChanged(e.detail);
        });

        // Map view change handler for URL state
        this.map.map.on('moveend', () => {
            this.updateMapURLState();
        });

        // Browser history handler
        window.addEventListener('popstate', () => {
            this.loadURLState();
        });

        // Keyboard shortcuts
        this.setupKeyboardShortcuts();

        // Global error handler
        window.addEventListener('error', (e) => {
            console.error('Global error:', e.error);
        });

        // Visibility change handler (pause/resume updates when tab not visible)
        document.addEventListener('visibilitychange', () => {
            if (document.visibilityState === 'visible') {
                this.resumeUpdates();
            } else {
                this.pauseUpdates();
            }
        });
    }

    setupKeyboardShortcuts() {
        document.addEventListener('keydown', (e) => {
            // Only handle shortcuts when not in input fields
            if (e.target.tagName.toLowerCase() === 'input') return;

            switch (e.key) {
                case 'r':
                    if (e.ctrlKey || e.metaKey) {
                        e.preventDefault();
                        this.map.setSelectionMode('rectangle');
                    }
                    break;
                case 'c':
                    if (e.ctrlKey || e.metaKey) {
                        e.preventDefault();
                        this.map.setSelectionMode('circle');
                    }
                    break;
                case 'Escape':
                    this.map.clearSelection();
                    this.map.setSelectionMode(null);
                    break;
                case 'l':
                    if (e.ctrlKey || e.metaKey) {
                        e.preventDefault();
                        this.map.toggleLLPGLayer();
                    }
                    break;
                case 'f':
                    if (e.ctrlKey || e.metaKey) {
                        e.preventDefault();
                        const searchInput = document.getElementById('address-search');
                        if (searchInput) {
                            searchInput.focus();
                        }
                    }
                    break;
            }
        });
    }

    async loadInitialData() {
        try {
            // Load map data with current filters
            await this.map.loadData(this.filters.getAPIFilters());
            
            // Load filter statistics
            await this.filters.loadFilterStats();
            
        } catch (error) {
            console.error('Error loading initial data:', error);
            this.showError('Failed to load initial data. Please check your connection.');
        }
    }

    // Event handlers
    handleDataUpdated(detail) {
        const { data, filters } = detail;
        
        // Update statistics
        this.updateStatistics(data);
        
        // Update UI elements
        this.updateRecordCount(data.features ? data.features.length : 0);
        
        console.log(`Data updated: ${data.features ? data.features.length : 0} features loaded`);
    }

    handleSelectionCompleted(detail) {
        this.currentSelection = detail;
        
        // Update selection statistics
        this.updateSelectionStats(detail.features);
        
        // Show selection info
        this.showSelectionInfo(detail);
        
        console.log(`Selection completed: ${detail.features.length} features selected (${detail.type})`);
    }

    handleSelectionCleared() {
        this.currentSelection = null;
        this.hideSelectionInfo();
        console.log('Selection cleared');
    }

    handleFiltersChanged(detail) {
        // Update URL state to reflect filter changes
        this.updateFilterURLState(detail.filters);
        
        console.log('Filters changed:', detail.filters);
    }

    // Statistics and UI updates
    updateStatistics(data) {
        if (!data.features) return;

        this.stats.visible = data.features.length;
        this.stats.matched = data.features.filter(f => f.properties.match_status === 'MATCHED').length;
        this.stats.unmatched = data.features.filter(f => f.properties.match_status === 'UNMATCHED').length;

        // Update statistics display elements
        this.updateStatsDisplay();
    }

    updateStatsDisplay() {
        // Update main statistics panel if it exists
        const totalEl = document.getElementById('total-records');
        const matchedEl = document.getElementById('matched-records');
        const unmatchedEl = document.getElementById('unmatched-records');
        const matchRateEl = document.getElementById('match-rate');

        if (totalEl) totalEl.textContent = this.stats.visible.toLocaleString();
        if (matchedEl) matchedEl.textContent = this.stats.matched.toLocaleString();
        if (unmatchedEl) unmatchedEl.textContent = this.stats.unmatched.toLocaleString();
        
        if (matchRateEl && this.stats.visible > 0) {
            const rate = (this.stats.matched / this.stats.visible * 100).toFixed(1);
            matchRateEl.textContent = rate + '%';
        }
    }

    updateRecordCount(count) {
        const countEl = document.getElementById('active-filter-count');
        if (countEl) {
            countEl.textContent = count.toLocaleString();
        }
    }

    updateSelectionStats(features) {
        if (!features) return;

        const selectionStats = {
            total: features.length,
            matched: features.filter(f => f.properties.match_status === 'MATCHED').length,
            unmatched: features.filter(f => f.properties.match_status === 'UNMATCHED').length
        };

        // Update selection info display
        this.displaySelectionStats(selectionStats);
    }

    displaySelectionStats(stats) {
        // This could be expanded to show detailed selection statistics
        console.log('Selection stats:', stats);
    }

    showSelectionInfo(selection) {
        // Show selection information in the UI
        // This could be a toast notification or status bar update
        const message = `Selected ${selection.features.length} records (${selection.type} selection)`;
        this.showNotification(message, 'info', 3000);
    }

    hideSelectionInfo() {
        // Hide selection information
    }

    // URL state management
    updateFilterURLState(filters) {
        if (!window.history.replaceState) return;

        const params = new URLSearchParams();
        
        // Add current map state
        const center = this.map.map.getCenter();
        const zoom = Math.round(this.map.map.getZoom() * 10) / 10;
        
        params.set('lat', center.lat.toFixed(6));
        params.set('lng', center.lng.toFixed(6));
        params.set('zoom', zoom);

        // Add filter state
        if (filters.addressSearch && filters.addressSearch.trim()) {
            params.set('search', filters.addressSearch);
        }
        
        if (filters.sourceTypes && filters.sourceTypes.length < 4) {
            params.set('source', filters.sourceTypes.join(','));
        }
        
        if (filters.matchStatus && filters.matchStatus.length < 3) {
            params.set('status', filters.matchStatus.join(','));
        }
        
        if (filters.addressQuality && filters.addressQuality.length < 3) {
            params.set('quality', filters.addressQuality.join(','));
        }
        
        if (filters.matchScoreRange && 
            (filters.matchScoreRange[0] > 0 || filters.matchScoreRange[1] < 1)) {
            params.set('score', `${filters.matchScoreRange[0]}-${filters.matchScoreRange[1]}`);
        }

        const newURL = params.toString() ? 
            `${window.location.pathname}?${params.toString()}` : 
            window.location.pathname;
        
        window.history.replaceState({ filters }, '', newURL);
    }

    updateMapURLState() {
        // Debounce map state updates
        clearTimeout(this.mapStateTimeout);
        this.mapStateTimeout = setTimeout(() => {
            this.updateFilterURLState(this.filters.getFilters());
        }, 500);
    }

    // Utility methods
    showError(message, duration = 5000) {
        this.showNotification(message, 'error', duration);
    }

    showNotification(message, type = 'info', duration = 3000) {
        // Create notification element
        const notification = document.createElement('div');
        notification.className = `notification notification-${type}`;
        notification.textContent = message;
        
        // Add to DOM
        document.body.appendChild(notification);
        
        // Auto-remove after duration
        setTimeout(() => {
            if (notification.parentNode) {
                notification.parentNode.removeChild(notification);
            }
        }, duration);
        
        console.log(`Notification (${type}):`, message);
    }

    pauseUpdates() {
        // Pause real-time updates when tab is not visible
        this.updatesPaused = true;
    }

    resumeUpdates() {
        // Resume updates when tab becomes visible
        if (this.updatesPaused) {
            this.updatesPaused = false;
            // Optionally refresh data
            this.map.loadData(this.filters.getAPIFilters());
        }
    }

    // Export functionality
    async exportCurrentView(format = 'csv') {
        try {
            const filters = this.filters.getAPIFilters();
            const exportData = {
                format: format,
                filters: filters
            };

            // Add viewport bounds if needed
            const bounds = this.map.map.getBounds();
            exportData.viewport = {
                min_lat: bounds.getSouth(),
                max_lat: bounds.getNorth(),
                min_lng: bounds.getWest(),
                max_lng: bounds.getEast()
            };

            const response = await fetch('/api/export', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(exportData)
            });

            if (!response.ok) {
                throw new Error(`Export failed: ${response.statusText}`);
            }

            const result = await response.json();
            
            if (result.success && result.download_url) {
                // Create download link
                const link = document.createElement('a');
                link.href = result.download_url;
                link.download = `ehdc-export-${new Date().toISOString().split('T')[0]}.${format}`;
                link.click();
                
                this.showNotification(`Export completed: ${result.record_count} records`, 'success');
            } else {
                this.showError(result.message || 'Export failed');
            }
            
        } catch (error) {
            console.error('Export error:', error);
            this.showError('Export failed. Please try again.');
        }
    }

    // Public API for external access
    getMapComponent() {
        return this.map;
    }

    getFilterComponent() {
        return this.filters;
    }

    getCurrentSelection() {
        return this.currentSelection;
    }

    getStats() {
        return { ...this.stats };
    }
}

// Global functions for popup actions
window.viewRecordDetails = function(srcId) {
    // Open record details in the drawer
    console.log('View record details:', srcId);
    
    if (window.app && window.app.recordDrawer) {
        window.app.recordDrawer.open(srcId);
    }
};

window.showMatchCandidates = function(srcId) {
    // Open record drawer and switch to candidates tab
    console.log('Show match candidates for:', srcId);
    
    if (window.app && window.app.recordDrawer) {
        window.app.recordDrawer.open(srcId).then(() => {
            window.app.recordDrawer.switchTab('candidates');
        });
    }
};

window.exportData = function() {
    if (window.app) {
        window.app.exportCurrentView('csv');
    }
};

// Initialize application when DOM is loaded
document.addEventListener('DOMContentLoaded', function() {
    // Wait for MapLibre to be available
    if (typeof maplibregl === 'undefined') {
        console.error('MapLibre GL JS is not loaded');
        return;
    }

    // Initialize the application
    window.app = new EHDCMappingApp();
    
    console.log('EHDC LLPG Mapping Application ready');
});

// Add notification styles to the page
const notificationStyles = `
<style>
.notification {
    position: fixed;
    top: 20px;
    right: 20px;
    padding: 1rem 1.5rem;
    border-radius: 6px;
    color: white;
    z-index: 10000;
    box-shadow: 0 4px 8px rgba(0,0,0,0.15);
    transition: all 0.3s ease;
    max-width: 300px;
    word-wrap: break-word;
}

.notification-info {
    background: #17a2b8;
}

.notification-success {
    background: #28a745;
}

.notification-error {
    background: #dc3545;
}

.notification-warning {
    background: #ffc107;
    color: #212529;
}
</style>`;

document.head.insertAdjacentHTML('beforeend', notificationStyles);