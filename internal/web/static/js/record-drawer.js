// Record Details Drawer Component
// Provides detailed record inspection and management interface

class RecordDrawer {
    constructor(appInstance) {
        this.app = appInstance;
        this.isOpen = false;
        this.currentRecord = null;
        this.matchCandidates = [];
        this.selectedCandidate = null;
        this.isLoading = false;
        
        this.init();
    }

    init() {
        this.createDrawer();
        this.setupEventHandlers();
        console.log('Record drawer initialized');
    }

    createDrawer() {
        // Create drawer HTML structure
        const drawerHTML = `
            <div id="record-drawer" class="record-drawer">
                <div class="drawer-overlay" id="drawer-overlay"></div>
                <div class="drawer-content">
                    <div class="drawer-header">
                        <div class="drawer-title">
                            <h2 id="drawer-title">Record Details</h2>
                            <div class="record-id" id="record-id"></div>
                        </div>
                        <button class="drawer-close" id="drawer-close" title="Close">
                            <svg width="24" height="24" viewBox="0 0 24 24">
                                <path d="M18 6L6 18M6 6l12 12" stroke="currentColor" stroke-width="2"/>
                            </svg>
                        </button>
                    </div>

                    <div class="drawer-body" id="drawer-body">
                        <div class="loading-state" id="drawer-loading">
                            <div class="loading-spinner"></div>
                            <p>Loading record details...</p>
                        </div>

                        <div class="drawer-content-wrapper" id="drawer-content" style="display: none;">
                            <!-- Record Information Tab -->
                            <div class="drawer-tabs">
                                <button class="tab-button active" data-tab="details">Details</button>
                                <button class="tab-button" data-tab="candidates">Matches</button>
                                <button class="tab-button" data-tab="history">History</button>
                                <button class="tab-button" data-tab="coordinates">Location</button>
                            </div>

                            <!-- Details Tab -->
                            <div class="tab-content active" id="tab-details">
                                <div class="record-section">
                                    <h3>Basic Information</h3>
                                    <div class="field-grid">
                                        <div class="field-item">
                                            <label>Source Type</label>
                                            <span id="detail-source-type" class="field-value"></span>
                                        </div>
                                        <div class="field-item">
                                            <label>External Reference</label>
                                            <span id="detail-external-ref" class="field-value"></span>
                                        </div>
                                        <div class="field-item">
                                            <label>Document Type</label>
                                            <span id="detail-doc-type" class="field-value"></span>
                                        </div>
                                        <div class="field-item">
                                            <label>Document Date</label>
                                            <span id="detail-doc-date" class="field-value"></span>
                                        </div>
                                    </div>
                                </div>

                                <div class="record-section">
                                    <h3>Address Information</h3>
                                    <div class="address-display">
                                        <div class="address-field">
                                            <label>Original Address</label>
                                            <div id="detail-original-address" class="address-value"></div>
                                        </div>
                                        <div class="address-field">
                                            <label>Canonical Address</label>
                                            <div id="detail-canonical-address" class="address-value"></div>
                                        </div>
                                        <div class="address-field">
                                            <label>Extracted Postcode</label>
                                            <span id="detail-postcode" class="field-value"></span>
                                        </div>
                                    </div>
                                </div>

                                <div class="record-section">
                                    <h3>Match Status</h3>
                                    <div class="match-status-display">
                                        <div class="status-badge-large" id="detail-match-status"></div>
                                        <div class="match-metrics">
                                            <div class="metric-item">
                                                <label>Quality</label>
                                                <span id="detail-address-quality" class="metric-value"></span>
                                            </div>
                                            <div class="metric-item">
                                                <label>Score</label>
                                                <span id="detail-match-score" class="metric-value"></span>
                                            </div>
                                            <div class="metric-item">
                                                <label>Method</label>
                                                <span id="detail-match-method" class="metric-value"></span>
                                            </div>
                                            <div class="metric-item">
                                                <label>Similarity</label>
                                                <span id="detail-address-similarity" class="metric-value"></span>
                                            </div>
                                        </div>
                                    </div>
                                </div>

                                <div class="record-section" id="matched-llpg-section" style="display: none;">
                                    <h3>Matched LLPG Record</h3>
                                    <div class="llpg-display">
                                        <div class="field-grid">
                                            <div class="field-item">
                                                <label>UPRN</label>
                                                <span id="detail-matched-uprn" class="field-value"></span>
                                            </div>
                                            <div class="field-item">
                                                <label>LLPG Address</label>
                                                <div id="detail-llpg-address" class="address-value"></div>
                                            </div>
                                            <div class="field-item">
                                                <label>USRN</label>
                                                <span id="detail-usrn" class="field-value"></span>
                                            </div>
                                            <div class="field-item">
                                                <label>BLPU Class</label>
                                                <span id="detail-blpu-class" class="field-value"></span>
                                            </div>
                                        </div>
                                        <div class="coordinate-distance" id="coordinate-distance-display" style="display: none;">
                                            <label>Coordinate Distance</label>
                                            <span id="detail-coordinate-distance" class="distance-value"></span>
                                        </div>
                                    </div>
                                </div>
                            </div>

                            <!-- Match Candidates Tab -->
                            <div class="tab-content" id="tab-candidates">
                                <div class="candidates-header">
                                    <h3>Potential Matches</h3>
                                    <button class="btn-secondary" id="find-new-matches">Find New Matches</button>
                                </div>
                                
                                <div class="llpg-search-section">
                                    <label>Search LLPG</label>
                                    <div class="search-input-wrapper">
                                        <input type="text" id="llpg-search" placeholder="Search for LLPG addresses...">
                                        <button class="search-btn" id="llpg-search-btn">Search</button>
                                    </div>
                                </div>

                                <div class="candidates-list" id="candidates-list">
                                    <div class="no-candidates">
                                        <p>No match candidates found.</p>
                                        <p>Try searching for LLPG addresses above.</p>
                                    </div>
                                </div>
                            </div>

                            <!-- History Tab -->
                            <div class="tab-content" id="tab-history">
                                <div class="history-header">
                                    <h3>Decision History</h3>
                                    <div class="history-filters">
                                        <select id="history-filter">
                                            <option value="all">All Actions</option>
                                            <option value="matches">Match Decisions</option>
                                            <option value="coordinates">Coordinate Changes</option>
                                            <option value="overrides">Manual Overrides</option>
                                        </select>
                                    </div>
                                </div>
                                
                                <div class="history-timeline" id="history-timeline">
                                    <div class="no-history">
                                        <p>No decision history available for this record.</p>
                                    </div>
                                </div>
                            </div>

                            <!-- Coordinates Tab -->
                            <div class="tab-content" id="tab-coordinates">
                                <div class="coordinates-section">
                                    <h3>Current Coordinates</h3>
                                    <div class="coordinate-display">
                                        <div class="coordinate-group">
                                            <h4>Source Coordinates</h4>
                                            <div class="field-grid">
                                                <div class="field-item">
                                                    <label>Easting</label>
                                                    <span id="detail-source-easting" class="field-value"></span>
                                                </div>
                                                <div class="field-item">
                                                    <label>Northing</label>
                                                    <span id="detail-source-northing" class="field-value"></span>
                                                </div>
                                            </div>
                                        </div>
                                        
                                        <div class="coordinate-group">
                                            <h4>LLPG Coordinates</h4>
                                            <div class="field-grid">
                                                <div class="field-item">
                                                    <label>Easting</label>
                                                    <span id="detail-llpg-easting" class="field-value"></span>
                                                </div>
                                                <div class="field-item">
                                                    <label>Northing</label>
                                                    <span id="detail-llpg-northing" class="field-value"></span>
                                                </div>
                                            </div>
                                        </div>
                                    </div>

                                    <div class="manual-coordinates-section">
                                        <h3>Manual Coordinate Override</h3>
                                        <p>Set precise coordinates for this record manually.</p>
                                        
                                        <div class="coordinate-input-group">
                                            <div class="input-pair">
                                                <label for="manual-easting">Easting (BNG)</label>
                                                <input type="number" id="manual-easting" placeholder="e.g. 445000" step="0.1">
                                            </div>
                                            <div class="input-pair">
                                                <label for="manual-northing">Northing (BNG)</label>
                                                <input type="number" id="manual-northing" placeholder="e.g. 105000" step="0.1">
                                            </div>
                                        </div>
                                        
                                        <div class="coordinate-actions">
                                            <button class="btn-primary" id="set-coordinates">Set Coordinates</button>
                                            <button class="btn-outline" id="pick-from-map">Pick from Map</button>
                                            <button class="btn-outline" id="use-llpg-coordinates">Use LLPG Coordinates</button>
                                        </div>
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>

                    <div class="drawer-footer" id="drawer-footer" style="display: none;">
                        <div class="action-buttons">
                            <button class="btn-success" id="accept-match" style="display: none;">
                                Accept Match
                            </button>
                            <button class="btn-danger" id="reject-match" style="display: none;">
                                Reject All Matches
                            </button>
                            <button class="btn-secondary" id="save-changes">
                                Save Changes
                            </button>
                        </div>
                    </div>
                </div>
            </div>
        `;

        // Add drawer to page
        document.body.insertAdjacentHTML('beforeend', drawerHTML);
    }

    setupEventHandlers() {
        // Close drawer events
        document.getElementById('drawer-close').addEventListener('click', () => {
            this.close();
        });

        document.getElementById('drawer-overlay').addEventListener('click', () => {
            this.close();
        });

        // Tab switching
        document.querySelectorAll('.tab-button').forEach(button => {
            button.addEventListener('click', (e) => {
                this.switchTab(e.target.dataset.tab);
            });
        });

        // Candidate-related events
        document.getElementById('find-new-matches').addEventListener('click', () => {
            this.findNewMatches();
        });

        document.getElementById('llpg-search-btn').addEventListener('click', () => {
            this.searchLLPG();
        });

        document.getElementById('llpg-search').addEventListener('keypress', (e) => {
            if (e.key === 'Enter') {
                this.searchLLPG();
            }
        });

        // Coordinate management
        document.getElementById('set-coordinates').addEventListener('click', () => {
            this.setManualCoordinates();
        });

        document.getElementById('pick-from-map').addEventListener('click', () => {
            this.startCoordinatePicking();
        });

        document.getElementById('use-llpg-coordinates').addEventListener('click', () => {
            this.useLLPGCoordinates();
        });

        // Action buttons
        document.getElementById('accept-match').addEventListener('click', () => {
            this.acceptSelectedMatch();
        });

        document.getElementById('reject-match').addEventListener('click', () => {
            this.rejectAllMatches();
        });

        document.getElementById('save-changes').addEventListener('click', () => {
            this.saveChanges();
        });

        // History filter
        document.getElementById('history-filter').addEventListener('change', (e) => {
            this.filterHistory(e.target.value);
        });

        // Keyboard shortcuts
        document.addEventListener('keydown', (e) => {
            if (!this.isOpen) return;
            
            switch (e.key) {
                case 'Escape':
                    this.close();
                    break;
                case '1':
                case '2':
                case '3':
                case '4':
                    if (e.ctrlKey || e.metaKey) {
                        e.preventDefault();
                        const tabs = ['details', 'candidates', 'history', 'coordinates'];
                        this.switchTab(tabs[parseInt(e.key) - 1]);
                    }
                    break;
            }
        });
    }

    async open(recordId) {
        if (this.isLoading) return;

        this.isLoading = true;
        this.show();
        this.showLoading();

        try {
            // Load record details
            const record = await this.loadRecord(recordId);
            this.currentRecord = record;

            // Load match candidates if record is unmatched or needs review
            if (record.match_status !== 'MATCHED') {
                this.matchCandidates = await this.loadMatchCandidates(recordId);
            }

            // Load decision history
            const history = await this.loadDecisionHistory(recordId);

            // Populate drawer with data
            this.populateRecordDetails(record);
            this.populateMatchCandidates(this.matchCandidates);
            this.populateDecisionHistory(history);

            this.hideLoading();
            this.showContent();

        } catch (error) {
            console.error('Error loading record details:', error);
            this.showError('Failed to load record details');
        } finally {
            this.isLoading = false;
        }
    }

    show() {
        const drawer = document.getElementById('record-drawer');
        drawer.classList.add('open');
        document.body.classList.add('drawer-open');
        this.isOpen = true;

        // Dispatch open event
        this.dispatchEvent('drawerOpened', { recordId: this.currentRecord?.src_id });
    }

    close() {
        const drawer = document.getElementById('record-drawer');
        drawer.classList.remove('open');
        document.body.classList.remove('drawer-open');
        this.isOpen = false;
        this.currentRecord = null;
        this.matchCandidates = [];
        this.selectedCandidate = null;

        // Reset to first tab
        this.switchTab('details');

        // Dispatch close event
        this.dispatchEvent('drawerClosed');
    }

    showLoading() {
        document.getElementById('drawer-loading').style.display = 'flex';
        document.getElementById('drawer-content').style.display = 'none';
        document.getElementById('drawer-footer').style.display = 'none';
    }

    hideLoading() {
        document.getElementById('drawer-loading').style.display = 'none';
    }

    showContent() {
        document.getElementById('drawer-content').style.display = 'block';
        document.getElementById('drawer-footer').style.display = 'block';
    }

    showError(message) {
        const drawerBody = document.getElementById('drawer-body');
        drawerBody.innerHTML = `
            <div class="error-state">
                <div class="error-icon">⚠️</div>
                <h3>Error Loading Record</h3>
                <p>${message}</p>
                <button class="btn-primary" onclick="window.app.recordDrawer.close()">Close</button>
            </div>
        `;
    }

    switchTab(tabName) {
        // Update tab buttons
        document.querySelectorAll('.tab-button').forEach(button => {
            button.classList.remove('active');
        });
        document.querySelector(`[data-tab="${tabName}"]`).classList.add('active');

        // Update tab content
        document.querySelectorAll('.tab-content').forEach(content => {
            content.classList.remove('active');
        });
        document.getElementById(`tab-${tabName}`).classList.add('active');

        // Load tab-specific data if needed
        if (tabName === 'candidates' && this.matchCandidates.length === 0) {
            this.loadMatchCandidates(this.currentRecord.src_id);
        }
    }

    // Data loading methods
    async loadRecord(recordId) {
        const response = await fetch(`/api/records/${recordId}`);
        if (!response.ok) {
            throw new Error(`Failed to load record: ${response.statusText}`);
        }
        return await response.json();
    }

    async loadMatchCandidates(recordId) {
        const response = await fetch(`/api/records/${recordId}/candidates`);
        if (!response.ok) {
            console.error('Failed to load match candidates');
            return [];
        }
        return await response.json();
    }

    async loadDecisionHistory(recordId) {
        try {
            const response = await fetch(`/api/records/${recordId}/history?limit=100`);
            if (!response.ok) {
                console.error('Failed to load decision history');
                return [];
            }
            return await response.json();
        } catch (error) {
            console.error('Error loading decision history:', error);
            return [];
        }
    }

    // Data population methods
    populateRecordDetails(record) {
        document.getElementById('drawer-title').textContent = `${record.source_type.toUpperCase()} Record`;
        document.getElementById('record-id').textContent = `ID: ${record.src_id}`;

        // Basic information
        document.getElementById('detail-source-type').textContent = record.source_type || 'N/A';
        document.getElementById('detail-external-ref').textContent = record.external_ref || 'N/A';
        document.getElementById('detail-doc-type').textContent = record.doc_type || 'N/A';
        document.getElementById('detail-doc-date').textContent = record.doc_date || 'N/A';

        // Address information
        document.getElementById('detail-original-address').textContent = record.original_address || 'N/A';
        document.getElementById('detail-canonical-address').textContent = record.canonical_address || 'N/A';
        document.getElementById('detail-postcode').textContent = record.extracted_postcode || 'N/A';

        // Match status
        this.updateMatchStatusDisplay(record);

        // Coordinates
        document.getElementById('detail-source-easting').textContent = record.source_easting || 'N/A';
        document.getElementById('detail-source-northing').textContent = record.source_northing || 'N/A';
        document.getElementById('detail-llpg-easting').textContent = record.llpg_easting || 'N/A';
        document.getElementById('detail-llpg-northing').textContent = record.llpg_northing || 'N/A';

        // Show/hide sections based on data
        this.updateSectionVisibility(record);
    }

    updateMatchStatusDisplay(record) {
        const statusEl = document.getElementById('detail-match-status');
        statusEl.textContent = record.match_status;
        statusEl.className = `status-badge-large status-${record.match_status}`;

        document.getElementById('detail-address-quality').textContent = record.address_quality || 'N/A';
        document.getElementById('detail-match-score').textContent = record.match_score ? 
            `${(record.match_score * 100).toFixed(1)}%` : 'N/A';
        document.getElementById('detail-match-method').textContent = record.match_method || 'N/A';
        document.getElementById('detail-address-similarity').textContent = record.address_similarity ? 
            `${(record.address_similarity * 100).toFixed(1)}%` : 'N/A';

        if (record.match_status === 'MATCHED') {
            document.getElementById('detail-matched-uprn').textContent = record.matched_uprn || 'N/A';
            document.getElementById('detail-llpg-address').textContent = record.llpg_address || 'N/A';
            document.getElementById('detail-usrn').textContent = record.usrn || 'N/A';
            document.getElementById('detail-blpu-class').textContent = record.blpu_class || 'N/A';
            
            if (record.coordinate_distance) {
                document.getElementById('detail-coordinate-distance').textContent = 
                    `${record.coordinate_distance.toFixed(1)}m`;
                document.getElementById('coordinate-distance-display').style.display = 'block';
            }
        }
    }

    updateSectionVisibility(record) {
        const matchedSection = document.getElementById('matched-llpg-section');
        matchedSection.style.display = record.match_status === 'MATCHED' ? 'block' : 'none';

        // Update action buttons
        const acceptBtn = document.getElementById('accept-match');
        const rejectBtn = document.getElementById('reject-match');
        
        if (record.match_status === 'MATCHED') {
            acceptBtn.style.display = 'none';
            rejectBtn.style.display = 'inline-block';
            rejectBtn.textContent = 'Remove Match';
        } else {
            acceptBtn.style.display = this.selectedCandidate ? 'inline-block' : 'none';
            rejectBtn.style.display = 'inline-block';
            rejectBtn.textContent = 'Mark as Unmatched';
        }
    }

    populateMatchCandidates(candidates) {
        const candidatesList = document.getElementById('candidates-list');
        
        if (candidates.length === 0) {
            candidatesList.innerHTML = `
                <div class="no-candidates">
                    <p>No match candidates found.</p>
                    <p>Try searching for LLPG addresses above.</p>
                </div>
            `;
            return;
        }

        const candidatesHTML = candidates.map((candidate, index) => `
            <div class="candidate-item" data-candidate-index="${index}">
                <div class="candidate-header">
                    <div class="candidate-score">
                        <span class="score-value">${(candidate.score * 100).toFixed(1)}%</span>
                        <span class="score-method">${candidate.method}</span>
                    </div>
                    <div class="candidate-actions">
                        <button class="btn-outline select-candidate" data-candidate-index="${index}">
                            Select
                        </button>
                    </div>
                </div>
                <div class="candidate-details">
                    <div class="candidate-address">
                        <strong>${candidate.address}</strong>
                        <small>UPRN: ${candidate.uprn}</small>
                    </div>
                    <div class="candidate-metadata">
                        <span>Easting: ${candidate.easting}</span>
                        <span>Northing: ${candidate.northing}</span>
                        ${candidate.usrn ? `<span>USRN: ${candidate.usrn}</span>` : ''}
                        ${candidate.blpu_class ? `<span>Class: ${candidate.blpu_class}</span>` : ''}
                    </div>
                    <div class="candidate-features">
                        ${candidate.features.map(feature => `<span class="feature-tag">${feature}</span>`).join('')}
                    </div>
                </div>
            </div>
        `).join('');

        candidatesList.innerHTML = candidatesHTML;

        // Add click handlers for candidate selection
        document.querySelectorAll('.select-candidate').forEach(button => {
            button.addEventListener('click', (e) => {
                const index = parseInt(e.target.dataset.candidateIndex);
                this.selectCandidate(candidates[index], index);
            });
        });
    }

    selectCandidate(candidate, index) {
        this.selectedCandidate = candidate;

        // Update UI to show selection
        document.querySelectorAll('.candidate-item').forEach(item => {
            item.classList.remove('selected');
        });
        document.querySelector(`[data-candidate-index="${index}"]`).classList.add('selected');

        // Update action buttons
        document.getElementById('accept-match').style.display = 'inline-block';

        // Pre-fill coordinate override with candidate coordinates
        document.getElementById('manual-easting').value = candidate.easting;
        document.getElementById('manual-northing').value = candidate.northing;
    }

    populateDecisionHistory(history) {
        const timeline = document.getElementById('history-timeline');
        
        if (history.length === 0) {
            timeline.innerHTML = `
                <div class="no-history">
                    <p>No decision history available for this record.</p>
                </div>
            `;
            return;
        }

        // Group history by date
        const groupedHistory = this.groupHistoryByDate(history);
        
        let timelineHTML = '';
        Object.keys(groupedHistory).forEach(date => {
            timelineHTML += `
                <div class="history-date-group">
                    <div class="history-date-header">
                        <h4>${this.formatHistoryDate(date)}</h4>
                    </div>
                    <div class="history-events">
            `;
            
            groupedHistory[date].forEach(event => {
                timelineHTML += this.createHistoryEventHTML(event);
            });
            
            timelineHTML += `
                    </div>
                </div>
            `;
        });

        timeline.innerHTML = timelineHTML;
    }

    groupHistoryByDate(history) {
        const groups = {};
        history.forEach(event => {
            const date = new Date(event.event_timestamp).toDateString();
            if (!groups[date]) {
                groups[date] = [];
            }
            groups[date].push(event);
        });
        return groups;
    }

    formatHistoryDate(dateString) {
        const date = new Date(dateString);
        const today = new Date();
        const yesterday = new Date(today);
        yesterday.setDate(yesterday.getDate() - 1);
        
        if (date.toDateString() === today.toDateString()) {
            return 'Today';
        } else if (date.toDateString() === yesterday.toDateString()) {
            return 'Yesterday';
        } else {
            return date.toLocaleDateString('en-GB', { 
                weekday: 'long', 
                year: 'numeric', 
                month: 'long', 
                day: 'numeric' 
            });
        }
    }

    createHistoryEventHTML(event) {
        const time = new Date(event.event_timestamp).toLocaleTimeString('en-GB', {
            hour: '2-digit',
            minute: '2-digit'
        });

        const iconMap = {
            'MATCH_DECISION': '🎯',
            'COORDINATE_CHANGE': '📍',
            'ADDRESS_CHANGE': '🏠',
            'INTERACTION': '👁️',
            'NOTE': '📝'
        };

        const icon = iconMap[event.event_type] || '📋';
        const actionText = this.formatHistoryAction(event);

        return `
            <div class="history-event event-${event.event_type.toLowerCase()}">
                <div class="event-timeline-marker">
                    <span class="event-icon">${icon}</span>
                </div>
                <div class="event-content">
                    <div class="event-header">
                        <span class="event-action">${actionText}</span>
                        <span class="event-time">${time}</span>
                    </div>
                    <div class="event-details">
                        ${event.details ? `<p>${event.details}</p>` : ''}
                        ${event.notes ? `<small class="event-notes">${event.notes}</small>` : ''}
                    </div>
                    <div class="event-meta">
                        <small>by ${event.user_name}</small>
                    </div>
                </div>
            </div>
        `;
    }

    formatHistoryAction(event) {
        const actionMap = {
            'ACCEPT_MATCH': 'Match Accepted',
            'REJECT_MATCH': 'Match Rejected',
            'REMOVE_MATCH': 'Match Removed',
            'MANUAL_OVERRIDE': 'Manual Override',
            'MANUAL_INPUT': 'Manual Coordinates Set',
            'MAP_CLICK': 'Coordinates from Map',
            'LLPG_COPY': 'Used LLPG Coordinates',
            'MATCH_ACCEPT': 'Coordinates from Match',
            'CANONICAL_UPDATE': 'Address Updated',
            'POSTCODE_CORRECTION': 'Postcode Corrected',
            'FULL_ADDRESS_EDIT': 'Full Address Edit',
            'VIEW': 'Record Viewed',
            'SEARCH': 'Record Searched',
            'EXPORT': 'Record Exported',
            'FLAG': 'Record Flagged',
            'GENERAL': 'Note Added',
            'WARNING': 'Warning Added',
            'QUALITY_ISSUE': 'Quality Issue Noted',
            'RESEARCH_NEEDED': 'Research Required'
        };

        return actionMap[event.action] || event.action;
    }

    // Action methods
    async findNewMatches() {
        try {
            const response = await fetch(`/api/records/${this.currentRecord.src_id}/candidates`, {
                method: 'POST'
            });
            
            if (response.ok) {
                this.matchCandidates = await response.json();
                this.populateMatchCandidates(this.matchCandidates);
                this.app.showNotification('New matches found', 'success');
            }
        } catch (error) {
            console.error('Error finding new matches:', error);
            this.app.showError('Failed to find new matches');
        }
    }

    async searchLLPG() {
        const query = document.getElementById('llpg-search').value.trim();
        if (!query) return;

        try {
            const response = await fetch(`/api/search/llpg?q=${encodeURIComponent(query)}&limit=10`);
            if (response.ok) {
                const results = await response.json();
                this.displayLLPGSearchResults(results);
            }
        } catch (error) {
            console.error('Error searching LLPG:', error);
            this.app.showError('Failed to search LLPG');
        }
    }

    displayLLPGSearchResults(results) {
        // Convert LLPG search results to candidate format and display
        const candidates = results.map(result => ({
            uprn: result.uprn,
            address: result.address,
            canonical_address: result.canonical_address,
            easting: result.easting,
            northing: result.northing,
            usrn: result.usrn,
            blpu_class: result.blpu_class,
            status: result.status,
            score: 0.5, // Default score for search results
            method: 'manual_search',
            features: ['manual_search']
        }));

        this.populateMatchCandidates(candidates);
    }

    async acceptSelectedMatch() {
        if (!this.selectedCandidate) return;

        try {
            const response = await fetch(`/api/records/${this.currentRecord.src_id}/accept`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    uprn: this.selectedCandidate.uprn,
                    method: this.selectedCandidate.method,
                    score: this.selectedCandidate.score
                })
            });

            if (response.ok) {
                this.app.showNotification('Match accepted successfully', 'success');
                // Refresh record data
                this.open(this.currentRecord.src_id);
                // Trigger map refresh
                this.app.map.loadData(this.app.filters.getAPIFilters());
            }
        } catch (error) {
            console.error('Error accepting match:', error);
            this.app.showError('Failed to accept match');
        }
    }

    async rejectAllMatches() {
        if (!confirm('Are you sure you want to reject all matches for this record?')) {
            return;
        }

        try {
            const response = await fetch(`/api/records/${this.currentRecord.src_id}/reject`, {
                method: 'POST'
            });

            if (response.ok) {
                this.app.showNotification('Matches rejected successfully', 'success');
                this.open(this.currentRecord.src_id);
                this.app.map.loadData(this.app.filters.getAPIFilters());
            }
        } catch (error) {
            console.error('Error rejecting matches:', error);
            this.app.showError('Failed to reject matches');
        }
    }

    async setManualCoordinates() {
        const easting = document.getElementById('manual-easting').value;
        const northing = document.getElementById('manual-northing').value;

        if (!easting || !northing) {
            this.app.showError('Please enter both easting and northing coordinates');
            return;
        }

        try {
            const response = await fetch(`/api/records/${this.currentRecord.src_id}/coordinates`, {
                method: 'PUT',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({
                    easting: parseFloat(easting),
                    northing: parseFloat(northing)
                })
            });

            if (response.ok) {
                this.app.showNotification('Coordinates updated successfully', 'success');
                this.open(this.currentRecord.src_id);
                this.app.map.loadData(this.app.filters.getAPIFilters());
            }
        } catch (error) {
            console.error('Error setting coordinates:', error);
            this.app.showError('Failed to set coordinates');
        }
    }

    startCoordinatePicking() {
        this.close();
        // Enable coordinate picking mode on map
        this.app.map.setCoordinatePickingMode(true, (coordinates) => {
            // Reopen drawer and populate coordinates
            document.getElementById('manual-easting').value = coordinates.easting.toFixed(1);
            document.getElementById('manual-northing').value = coordinates.northing.toFixed(1);
            this.open(this.currentRecord.src_id);
            this.switchTab('coordinates');
        });
    }

    useLLPGCoordinates() {
        if (this.currentRecord.llpg_easting && this.currentRecord.llpg_northing) {
            document.getElementById('manual-easting').value = this.currentRecord.llpg_easting;
            document.getElementById('manual-northing').value = this.currentRecord.llpg_northing;
        } else {
            this.app.showError('No LLPG coordinates available for this record');
        }
    }

    saveChanges() {
        // Placeholder for saving any pending changes
        this.app.showNotification('Changes saved', 'success');
    }

    async filterHistory(filterType) {
        if (!this.currentRecord) return;

        try {
            let url = `/api/records/${this.currentRecord.src_id}/history?limit=100`;
            
            // Add filter parameter if not 'all'
            if (filterType !== 'all') {
                const typeMap = {
                    'matches': 'MATCH_DECISION',
                    'coordinates': 'COORDINATE_CHANGE',
                    'overrides': 'ADDRESS_CHANGE',
                    'notes': 'NOTE',
                    'interactions': 'INTERACTION'
                };
                
                const eventType = typeMap[filterType];
                if (eventType) {
                    url += `&type=${eventType}`;
                }
            }

            const response = await fetch(url);
            if (response.ok) {
                const history = await response.json();
                this.populateDecisionHistory(history);
            }
        } catch (error) {
            console.error('Error filtering history:', error);
        }
    }

    dispatchEvent(eventType, data = {}) {
        const event = new CustomEvent(`drawer:${eventType}`, { detail: data });
        document.dispatchEvent(event);
    }

    // Public methods
    isDrawerOpen() {
        return this.isOpen;
    }

    getCurrentRecord() {
        return this.currentRecord;
    }
}

// Export the class
window.RecordDrawer = RecordDrawer;