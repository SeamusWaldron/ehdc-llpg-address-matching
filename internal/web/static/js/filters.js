// Advanced Filtering System with Range Sliders
// Handles all filter controls and state management

class FilterPanel {
    constructor(containerId, mapComponent) {
        this.containerId = containerId;
        this.mapComponent = mapComponent;
        this.filters = {
            sourceTypes: ['decision', 'land_charge', 'enforcement', 'agreement'],
            matchStatus: ['MATCHED', 'UNMATCHED', 'NEEDS_REVIEW'],
            addressQuality: ['GOOD', 'FAIR', 'POOR'],
            matchScoreRange: [0, 1],
            addressSearch: '',
            dateRange: null,
            coordinateFilter: false
        };
        
        this.debounceTimeout = null;
        this.init();
    }

    init() {
        this.render();
        this.setupEventHandlers();
        this.loadFilterStats();
        console.log('Advanced filter panel initialized');
    }

    render() {
        const container = document.getElementById(this.containerId);
        container.innerHTML = `
            <div class="filter-panel">
                <div class="filter-header">
                    <h3>Filters</h3>
                    <button class="reset-filters-btn" title="Reset All Filters">
                        <svg width="16" height="16" viewBox="0 0 16 16">
                            <path d="M1 1l14 14M15 1L1 15" stroke="currentColor" stroke-width="2"/>
                        </svg>
                    </button>
                </div>

                <!-- Search Filter -->
                <div class="filter-group">
                    <label for="address-search">Address Search</label>
                    <div class="search-input-wrapper">
                        <input type="text" id="address-search" placeholder="Enter address or reference...">
                        <span class="search-icon">🔍</span>
                    </div>
                </div>

                <!-- Match Score Range -->
                <div class="filter-group">
                    <label>Match Score Range</label>
                    <div class="range-slider-wrapper">
                        <div class="range-slider" id="match-score-range">
                            <input type="range" id="match-score-min" min="0" max="100" value="0" step="1">
                            <input type="range" id="match-score-max" min="0" max="100" value="100" step="1">
                        </div>
                        <div class="range-values">
                            <span id="match-score-min-value">0%</span>
                            <span id="match-score-max-value">100%</span>
                        </div>
                    </div>
                    <div class="filter-presets">
                        <button class="preset-btn" data-preset="high">High Confidence</button>
                        <button class="preset-btn" data-preset="review">Needs Review</button>
                        <button class="preset-btn" data-preset="all">All Scores</button>
                    </div>
                </div>

                <!-- Source Types -->
                <div class="filter-group">
                    <label>Source Types</label>
                    <div class="checkbox-grid">
                        <label class="checkbox-item">
                            <input type="checkbox" value="decision" checked>
                            <span class="checkmark"></span>
                            <span class="label-text">Decision Notices</span>
                            <span class="count" id="count-decision">0</span>
                        </label>
                        <label class="checkbox-item">
                            <input type="checkbox" value="land_charge" checked>
                            <span class="checkmark"></span>
                            <span class="label-text">Land Charges</span>
                            <span class="count" id="count-land_charge">0</span>
                        </label>
                        <label class="checkbox-item">
                            <input type="checkbox" value="enforcement" checked>
                            <span class="checkmark"></span>
                            <span class="label-text">Enforcement</span>
                            <span class="count" id="count-enforcement">0</span>
                        </label>
                        <label class="checkbox-item">
                            <input type="checkbox" value="agreement" checked>
                            <span class="checkmark"></span>
                            <span class="label-text">Agreements</span>
                            <span class="count" id="count-agreement">0</span>
                        </label>
                    </div>
                    <div class="select-all-controls">
                        <button class="select-all-btn" data-target="source">Select All</button>
                        <button class="select-none-btn" data-target="source">None</button>
                    </div>
                </div>

                <!-- Match Status -->
                <div class="filter-group">
                    <label>Match Status</label>
                    <div class="status-checkboxes">
                        <label class="checkbox-item status-matched">
                            <input type="checkbox" value="MATCHED" checked>
                            <span class="checkmark"></span>
                            <span class="label-text">Matched</span>
                            <span class="count" id="count-MATCHED">0</span>
                        </label>
                        <label class="checkbox-item status-unmatched">
                            <input type="checkbox" value="UNMATCHED" checked>
                            <span class="checkmark"></span>
                            <span class="label-text">Unmatched</span>
                            <span class="count" id="count-UNMATCHED">0</span>
                        </label>
                        <label class="checkbox-item status-review">
                            <input type="checkbox" value="NEEDS_REVIEW" checked>
                            <span class="checkmark"></span>
                            <span class="label-text">Needs Review</span>
                            <span class="count" id="count-NEEDS_REVIEW">0</span>
                        </label>
                    </div>
                </div>

                <!-- Address Quality -->
                <div class="filter-group">
                    <label>Address Quality</label>
                    <div class="quality-checkboxes">
                        <label class="checkbox-item quality-good">
                            <input type="checkbox" value="GOOD" checked>
                            <span class="checkmark"></span>
                            <span class="label-text">Good</span>
                            <span class="count" id="count-GOOD">0</span>
                        </label>
                        <label class="checkbox-item quality-fair">
                            <input type="checkbox" value="FAIR" checked>
                            <span class="checkmark"></span>
                            <span class="label-text">Fair</span>
                            <span class="count" id="count-FAIR">0</span>
                        </label>
                        <label class="checkbox-item quality-poor">
                            <input type="checkbox" value="POOR" checked>
                            <span class="checkmark"></span>
                            <span class="label-text">Poor</span>
                            <span class="count" id="count-POOR">0</span>
                        </label>
                    </div>
                </div>

                <!-- Advanced Filters (Collapsible) -->
                <div class="filter-group advanced-filters">
                    <button class="collapse-toggle" id="advanced-toggle">
                        <span>Advanced Filters</span>
                        <svg class="chevron" width="12" height="12" viewBox="0 0 12 12">
                            <path d="M3 4.5l3 3 3-3" stroke="currentColor" fill="none" stroke-width="1.5"/>
                        </svg>
                    </button>
                    
                    <div class="collapsible-content" id="advanced-content">
                        <!-- Coordinate Filter -->
                        <div class="filter-subgroup">
                            <label class="checkbox-item">
                                <input type="checkbox" id="has-coordinates">
                                <span class="checkmark"></span>
                                <span class="label-text">Only records with coordinates</span>
                            </label>
                        </div>

                        <!-- Date Range Filter -->
                        <div class="filter-subgroup">
                            <label>Document Date Range</label>
                            <div class="date-range">
                                <input type="date" id="date-from" placeholder="From">
                                <input type="date" id="date-to" placeholder="To">
                            </div>
                        </div>

                        <!-- Spatial Filter (for selections) -->
                        <div class="filter-subgroup spatial-filter" style="display: none;">
                            <label>Current Selection</label>
                            <div class="spatial-controls">
                                <div class="selection-info" id="selection-info">No selection</div>
                                <button class="btn-outline" id="fit-selection">Fit to Selection</button>
                                <button class="btn-outline" id="export-selection">Export Selection</button>
                            </div>
                        </div>
                    </div>
                </div>

                <!-- Filter Summary -->
                <div class="filter-summary">
                    <div class="summary-text" id="filter-summary">
                        All filters active
                    </div>
                    <div class="active-count">
                        Showing <span id="active-filter-count">0</span> records
                    </div>
                </div>
            </div>
        `;
    }

    setupEventHandlers() {
        // Search input with debouncing
        const searchInput = document.getElementById('address-search');
        searchInput.addEventListener('input', (e) => {
            this.debounceFilter(() => {
                this.filters.addressSearch = e.target.value;
                this.applyFilters();
            });
        });

        // Range sliders
        this.setupRangeSliders();

        // Checkboxes
        this.setupCheckboxHandlers();

        // Preset buttons
        this.setupPresetHandlers();

        // Select all/none buttons
        this.setupSelectAllHandlers();

        // Advanced filters toggle
        this.setupAdvancedToggle();

        // Date inputs
        this.setupDateHandlers();

        // Reset button
        document.querySelector('.reset-filters-btn').addEventListener('click', () => {
            this.resetAllFilters();
        });

        // Listen for map selection events
        this.setupSelectionHandlers();
    }

    setupRangeSliders() {
        const minSlider = document.getElementById('match-score-min');
        const maxSlider = document.getElementById('match-score-max');
        const minValue = document.getElementById('match-score-min-value');
        const maxValue = document.getElementById('match-score-max-value');

        const updateRangeValues = () => {
            const min = parseInt(minSlider.value);
            const max = parseInt(maxSlider.value);

            // Ensure min doesn't exceed max
            if (min > max) {
                if (event.target === minSlider) {
                    maxSlider.value = min;
                } else {
                    minSlider.value = max;
                }
            }

            const finalMin = parseInt(minSlider.value);
            const finalMax = parseInt(maxSlider.value);

            minValue.textContent = finalMin + '%';
            maxValue.textContent = finalMax + '%';

            this.filters.matchScoreRange = [finalMin / 100, finalMax / 100];

            // Visual feedback
            this.updateRangeSliderBackground(finalMin, finalMax);
        };

        minSlider.addEventListener('input', updateRangeValues);
        maxSlider.addEventListener('input', updateRangeValues);

        // Apply filters on change (not input for performance)
        minSlider.addEventListener('change', () => this.applyFilters());
        maxSlider.addEventListener('change', () => this.applyFilters());

        // Initial setup
        updateRangeValues();
    }

    updateRangeSliderBackground(min, max) {
        const minSlider = document.getElementById('match-score-min');
        const maxSlider = document.getElementById('match-score-max');
        
        const minPercent = (min / 100) * 100;
        const maxPercent = (max / 100) * 100;
        
        minSlider.style.background = `linear-gradient(to right, #ddd ${minPercent}%, #007bff ${minPercent}%, #007bff ${maxPercent}%, #ddd ${maxPercent}%)`;
        maxSlider.style.background = 'transparent';
    }

    setupCheckboxHandlers() {
        // Source types
        document.querySelectorAll('input[type="checkbox"][value]').forEach(checkbox => {
            checkbox.addEventListener('change', (e) => {
                const value = e.target.value;
                const isChecked = e.target.checked;

                if (['decision', 'land_charge', 'enforcement', 'agreement'].includes(value)) {
                    this.updateArrayFilter('sourceTypes', value, isChecked);
                } else if (['MATCHED', 'UNMATCHED', 'NEEDS_REVIEW'].includes(value)) {
                    this.updateArrayFilter('matchStatus', value, isChecked);
                } else if (['GOOD', 'FAIR', 'POOR'].includes(value)) {
                    this.updateArrayFilter('addressQuality', value, isChecked);
                }

                this.applyFilters();
            });
        });

        // Coordinates filter
        document.getElementById('has-coordinates').addEventListener('change', (e) => {
            this.filters.coordinateFilter = e.target.checked;
            this.applyFilters();
        });
    }

    updateArrayFilter(filterName, value, isChecked) {
        if (isChecked) {
            if (!this.filters[filterName].includes(value)) {
                this.filters[filterName].push(value);
            }
        } else {
            this.filters[filterName] = this.filters[filterName].filter(v => v !== value);
        }
    }

    setupPresetHandlers() {
        document.querySelectorAll('.preset-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const preset = e.target.dataset.preset;
                this.applyScorePreset(preset);
            });
        });
    }

    applyScorePreset(preset) {
        const minSlider = document.getElementById('match-score-min');
        const maxSlider = document.getElementById('match-score-max');

        switch (preset) {
            case 'high':
                minSlider.value = 92;
                maxSlider.value = 100;
                break;
            case 'review':
                minSlider.value = 70;
                maxSlider.value = 91;
                break;
            case 'all':
                minSlider.value = 0;
                maxSlider.value = 100;
                break;
        }

        // Trigger change events
        minSlider.dispatchEvent(new Event('input'));
        maxSlider.dispatchEvent(new Event('change'));

        // Update UI
        document.querySelectorAll('.preset-btn').forEach(btn => btn.classList.remove('active'));
        document.querySelector(`[data-preset="${preset}"]`).classList.add('active');
    }

    setupSelectAllHandlers() {
        document.querySelectorAll('.select-all-btn, .select-none-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const target = e.target.dataset.target;
                const isSelectAll = e.target.classList.contains('select-all-btn');

                if (target === 'source') {
                    const checkboxes = document.querySelectorAll('input[type="checkbox"][value^="decision"], input[type="checkbox"][value="land_charge"], input[type="checkbox"][value="enforcement"], input[type="checkbox"][value="agreement"]');
                    checkboxes.forEach(cb => {
                        cb.checked = isSelectAll;
                        cb.dispatchEvent(new Event('change'));
                    });
                }
            });
        });
    }

    setupAdvancedToggle() {
        const toggle = document.getElementById('advanced-toggle');
        const content = document.getElementById('advanced-content');

        toggle.addEventListener('click', () => {
            const isOpen = content.style.display !== 'none';
            content.style.display = isOpen ? 'none' : 'block';
            toggle.classList.toggle('open', !isOpen);
        });
    }

    setupDateHandlers() {
        const dateFrom = document.getElementById('date-from');
        const dateTo = document.getElementById('date-to');

        const updateDateRange = () => {
            const from = dateFrom.value;
            const to = dateTo.value;

            if (from || to) {
                this.filters.dateRange = { from, to };
            } else {
                this.filters.dateRange = null;
            }

            this.applyFilters();
        };

        dateFrom.addEventListener('change', updateDateRange);
        dateTo.addEventListener('change', updateDateRange);
    }

    setupSelectionHandlers() {
        // Listen for map selection events
        document.addEventListener('map:selectionCompleted', (e) => {
            this.showSpatialFilter(e.detail);
        });

        document.addEventListener('map:selectionCleared', () => {
            this.hideSpatialFilter();
        });

        // Spatial filter controls
        document.getElementById('fit-selection').addEventListener('click', () => {
            this.mapComponent.fitToSelection();
        });

        document.getElementById('export-selection').addEventListener('click', () => {
            this.exportSelection();
        });
    }

    showSpatialFilter(selectionInfo) {
        const spatialFilter = document.querySelector('.spatial-filter');
        const selectionInfoEl = document.getElementById('selection-info');

        spatialFilter.style.display = 'block';
        selectionInfoEl.textContent = `${selectionInfo.features.length} records selected (${selectionInfo.type})`;
    }

    hideSpatialFilter() {
        const spatialFilter = document.querySelector('.spatial-filter');
        spatialFilter.style.display = 'none';
    }

    // Filter application
    applyFilters() {
        // Convert filters to API parameters
        const apiFilters = this.getAPIFilters();
        
        // Update map data
        this.mapComponent.loadData(apiFilters);
        
        // Update UI
        this.updateFilterSummary();
        
        // Update URL state
        this.updateURLState();
        
        // Dispatch filter change event
        this.dispatchEvent('filtersChanged', { filters: this.filters, apiFilters });
    }

    getAPIFilters() {
        const filters = {};

        // Search
        if (this.filters.addressSearch.trim()) {
            filters.address_search = this.filters.addressSearch.trim();
        }

        // Source types (API currently supports single type)
        if (this.filters.sourceTypes.length > 0 && this.filters.sourceTypes.length < 4) {
            filters.source_type = this.filters.sourceTypes[0];
        }

        // Match status (API currently supports single status)
        if (this.filters.matchStatus.length > 0 && this.filters.matchStatus.length < 3) {
            filters.match_status = this.filters.matchStatus[0];
        }

        // Address quality (API currently supports single quality)
        if (this.filters.addressQuality.length > 0 && this.filters.addressQuality.length < 3) {
            filters.address_quality = this.filters.addressQuality[0];
        }

        // Match score range
        if (this.filters.matchScoreRange[0] > 0 || this.filters.matchScoreRange[1] < 1) {
            filters.min_score = this.filters.matchScoreRange[0];
            filters.max_score = this.filters.matchScoreRange[1];
        }

        return filters;
    }

    updateFilterSummary() {
        const summary = document.getElementById('filter-summary');
        const activeFilters = [];

        if (this.filters.addressSearch.trim()) {
            activeFilters.push('Address search');
        }
        if (this.filters.sourceTypes.length < 4) {
            activeFilters.push('Source types');
        }
        if (this.filters.matchStatus.length < 3) {
            activeFilters.push('Match status');
        }
        if (this.filters.addressQuality.length < 3) {
            activeFilters.push('Address quality');
        }
        if (this.filters.matchScoreRange[0] > 0 || this.filters.matchScoreRange[1] < 1) {
            activeFilters.push('Match score range');
        }
        if (this.filters.dateRange) {
            activeFilters.push('Date range');
        }
        if (this.filters.coordinateFilter) {
            activeFilters.push('Has coordinates');
        }

        if (activeFilters.length === 0) {
            summary.textContent = 'All filters active';
        } else {
            summary.textContent = 'Active: ' + activeFilters.join(', ');
        }
    }

    updateURLState() {
        if (!window.history.pushState) return;

        const params = new URLSearchParams();
        
        if (this.filters.addressSearch.trim()) {
            params.set('search', this.filters.addressSearch);
        }
        if (this.filters.sourceTypes.length < 4) {
            params.set('source', this.filters.sourceTypes.join(','));
        }
        if (this.filters.matchStatus.length < 3) {
            params.set('status', this.filters.matchStatus.join(','));
        }
        if (this.filters.addressQuality.length < 3) {
            params.set('quality', this.filters.addressQuality.join(','));
        }
        if (this.filters.matchScoreRange[0] > 0 || this.filters.matchScoreRange[1] < 1) {
            params.set('score', `${this.filters.matchScoreRange[0]}-${this.filters.matchScoreRange[1]}`);
        }

        const newURL = params.toString() ? 
            `${window.location.pathname}?${params.toString()}` : 
            window.location.pathname;
        
        window.history.replaceState({}, '', newURL);
    }

    // Filter statistics
    async loadFilterStats() {
        try {
            const response = await fetch('/api/stats');
            if (!response.ok) return;
            
            const stats = await response.json();
            this.updateFilterCounts(stats);
        } catch (error) {
            console.error('Error loading filter stats:', error);
        }
    }

    updateFilterCounts(stats) {
        // Update source type counts
        if (stats.by_source_type) {
            Object.entries(stats.by_source_type).forEach(([type, data]) => {
                const countEl = document.getElementById(`count-${type}`);
                if (countEl) {
                    countEl.textContent = data.count.toLocaleString();
                }
            });
        }

        // Update match status counts (from overall stats)
        document.getElementById('count-MATCHED').textContent = stats.matched_records.toLocaleString();
        document.getElementById('count-UNMATCHED').textContent = stats.unmatched_records.toLocaleString();
        document.getElementById('count-NEEDS_REVIEW').textContent = stats.needs_review.toLocaleString();

        // Update quality counts
        if (stats.by_address_quality) {
            Object.entries(stats.by_address_quality).forEach(([quality, data]) => {
                const countEl = document.getElementById(`count-${quality}`);
                if (countEl) {
                    countEl.textContent = data.count.toLocaleString();
                }
            });
        }
    }

    // Reset and utility methods
    resetAllFilters() {
        this.filters = {
            sourceTypes: ['decision', 'land_charge', 'enforcement', 'agreement'],
            matchStatus: ['MATCHED', 'UNMATCHED', 'NEEDS_REVIEW'],
            addressQuality: ['GOOD', 'FAIR', 'POOR'],
            matchScoreRange: [0, 1],
            addressSearch: '',
            dateRange: null,
            coordinateFilter: false
        };

        // Reset UI
        document.getElementById('address-search').value = '';
        document.getElementById('match-score-min').value = 0;
        document.getElementById('match-score-max').value = 100;
        document.getElementById('date-from').value = '';
        document.getElementById('date-to').value = '';
        document.getElementById('has-coordinates').checked = false;

        document.querySelectorAll('input[type="checkbox"][value]').forEach(cb => {
            cb.checked = true;
        });

        document.querySelectorAll('.preset-btn').forEach(btn => btn.classList.remove('active'));

        // Apply reset filters
        this.applyFilters();
    }

    exportSelection() {
        const selectionData = this.mapComponent.exportSelection();
        if (!selectionData) {
            alert('No selection to export');
            return;
        }

        // Create and download GeoJSON file
        const dataStr = JSON.stringify(selectionData, null, 2);
        const dataBlob = new Blob([dataStr], { type: 'application/json' });
        
        const link = document.createElement('a');
        link.href = URL.createObjectURL(dataBlob);
        link.download = `ehdc-selection-${new Date().toISOString().split('T')[0]}.geojson`;
        link.click();
    }

    debounceFilter(callback) {
        clearTimeout(this.debounceTimeout);
        this.debounceTimeout = setTimeout(callback, 300);
    }

    dispatchEvent(eventType, data = {}) {
        const event = new CustomEvent(`filters:${eventType}`, { detail: data });
        document.dispatchEvent(event);
    }

    // Public methods for external control
    getFilters() {
        return { ...this.filters };
    }

    setFilters(newFilters) {
        this.filters = { ...this.filters, ...newFilters };
        this.applyFilters();
    }
}

// Export the FilterPanel class
window.FilterPanel = FilterPanel;