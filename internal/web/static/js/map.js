// Enhanced Map Component for EHDC LLPG Interface
// Handles MapLibre integration, selection tools, and LLPG overlay

class MapComponent {
    constructor(containerId, options = {}) {
        this.containerId = containerId;
        this.options = {
            center: [-1.0, 50.9], // Hampshire, UK
            zoom: 10,
            ...options
        };
        
        this.map = null;
        this.currentData = null;
        this.selectionMode = null; // 'rectangle', 'circle', null
        this.selectionActive = false;
        this.selectionFeatures = [];
        this.llpgLayerVisible = false;
        this.coordinatePickingMode = false;
        this.coordinatePickingCallback = null;
        
        this.init();
    }

    init() {
        // Initialize MapLibre map
        this.map = new maplibregl.Map({
            container: this.containerId,
            style: 'https://api.maptiler.com/maps/basic-v2/style.json?key=demo',
            center: this.options.center,
            zoom: this.options.zoom
        });

        // Add navigation controls
        this.map.addControl(new maplibregl.NavigationControl());
        
        // Add custom controls
        this.addCustomControls();
        
        // Setup event handlers
        this.setupEventHandlers();
        
        console.log('Enhanced map component initialized');
    }

    addCustomControls() {
        // Selection Tools Control
        const selectionControl = new SelectionControl(this);
        this.map.addControl(selectionControl, 'top-left');
        
        // LLPG Layer Toggle Control
        const llpgControl = new LLPGLayerControl(this);
        this.map.addControl(llpgControl, 'top-right');
        
        // Statistics Control
        const statsControl = new StatsControl();
        this.map.addControl(statsControl, 'bottom-left');
    }

    setupEventHandlers() {
        const self = this;
        
        this.map.on('load', function() {
            console.log('Map loaded, setting up layers...');
            self.setupLayers();
            self.setupSelectionLayers();
        });

        // Click handlers for different modes
        this.map.on('click', (e) => {
            if (this.coordinatePickingMode) {
                this.handleCoordinatePicking(e);
            } else if (this.selectionMode === 'rectangle') {
                this.handleRectangleSelection(e);
            } else if (this.selectionMode === 'circle') {
                this.handleCircleSelection(e);
            }
        });

        // Mouse handlers for selection feedback
        this.map.on('mousemove', (e) => {
            if (this.selectionActive) {
                this.updateSelectionPreview(e);
            }
        });
    }

    setupLayers() {
        // Records layer (clustered)
        this.map.addSource('records', {
            type: 'geojson',
            data: { type: 'FeatureCollection', features: [] },
            cluster: true,
            clusterMaxZoom: 14,
            clusterRadius: 50
        });

        // Cluster layers
        this.map.addLayer({
            id: 'clusters',
            type: 'circle',
            source: 'records',
            filter: ['has', 'point_count'],
            paint: {
                'circle-color': [
                    'step',
                    ['get', 'point_count'],
                    '#51bbd6', 50,
                    '#f1f075', 100,
                    '#f28cb1'
                ],
                'circle-radius': [
                    'step',
                    ['get', 'point_count'],
                    20, 50,
                    30, 100,
                    40
                ],
                'circle-stroke-width': 2,
                'circle-stroke-color': '#fff'
            }
        });

        // Cluster count labels
        this.map.addLayer({
            id: 'cluster-count',
            type: 'symbol',
            source: 'records',
            filter: ['has', 'point_count'],
            layout: {
                'text-field': '{point_count_abbreviated}',
                'text-size': 12,
                'text-color': '#fff'
            }
        });

        // Individual points with enhanced styling
        this.map.addLayer({
            id: 'unclustered-point',
            type: 'circle',
            source: 'records',
            filter: ['!', ['has', 'point_count']],
            paint: {
                'circle-color': [
                    'case',
                    ['==', ['get', 'match_status'], 'MATCHED'],
                    [
                        'case',
                        ['>=', ['get', 'match_score'], 0.92], '#28a745', // High confidence
                        ['>=', ['get', 'match_score'], 0.80], '#007bff', // Review needed  
                        '#ffc107' // Low confidence
                    ],
                    ['==', ['get', 'match_status'], 'NEEDS_REVIEW'], '#17a2b8',
                    ['==', ['get', 'match_status'], 'UNMATCHED'], '#dc3545',
                    '#6c757d' // Default
                ],
                'circle-radius': [
                    'case',
                    ['has', 'match_score'],
                    ['interpolate', ['linear'], ['get', 'match_score'], 0.5, 4, 1.0, 8],
                    6
                ],
                'circle-stroke-width': 2,
                'circle-stroke-color': '#fff',
                'circle-opacity': 0.8
            }
        });

        // Hover effect
        this.map.addLayer({
            id: 'unclustered-point-hover',
            type: 'circle',
            source: 'records',
            filter: ['==', 'src_id', ''],
            paint: {
                'circle-color': '#fff',
                'circle-radius': 10,
                'circle-stroke-width': 3,
                'circle-stroke-color': '#007bff',
                'circle-opacity': 0.6
            }
        });

        // Setup click handlers
        this.setupClickHandlers();
        this.setupHoverHandlers();
    }

    setupSelectionLayers() {
        // Selection overlay layer
        this.map.addSource('selection', {
            type: 'geojson',
            data: { type: 'FeatureCollection', features: [] }
        });

        this.map.addLayer({
            id: 'selection-fill',
            type: 'fill',
            source: 'selection',
            paint: {
                'fill-color': '#007bff',
                'fill-opacity': 0.1
            }
        });

        this.map.addLayer({
            id: 'selection-stroke',
            type: 'line',
            source: 'selection',
            paint: {
                'line-color': '#007bff',
                'line-width': 2,
                'line-dasharray': [5, 5]
            }
        });
    }

    setupClickHandlers() {
        // Cluster click - zoom in
        this.map.on('click', 'clusters', (e) => {
            const features = this.map.queryRenderedFeatures(e.point, { layers: ['clusters'] });
            const clusterId = features[0].properties.cluster_id;
            this.map.getSource('records').getClusterExpansionZoom(clusterId, (err, zoom) => {
                if (err) return;
                this.map.easeTo({
                    center: features[0].geometry.coordinates,
                    zoom: zoom
                });
            });
        });

        // Point click - show popup
        this.map.on('click', 'unclustered-point', (e) => {
            if (this.selectionMode) return; // Don't show popup in selection mode
            
            const coordinates = e.features[0].geometry.coordinates.slice();
            const props = e.features[0].properties;
            
            const popup = new RecordPopup(props, this);
            popup.show(coordinates);
        });
    }

    setupHoverHandlers() {
        // Hover effects
        this.map.on('mouseenter', 'clusters', () => {
            this.map.getCanvas().style.cursor = 'pointer';
        });

        this.map.on('mouseleave', 'clusters', () => {
            this.map.getCanvas().style.cursor = '';
        });

        this.map.on('mouseenter', 'unclustered-point', (e) => {
            this.map.getCanvas().style.cursor = 'pointer';
            
            // Highlight hovered point
            if (e.features.length > 0) {
                const srcId = e.features[0].properties.src_id;
                this.map.setFilter('unclustered-point-hover', ['==', 'src_id', srcId]);
            }
        });

        this.map.on('mouseleave', 'unclustered-point', () => {
            this.map.getCanvas().style.cursor = '';
            this.map.setFilter('unclustered-point-hover', ['==', 'src_id', '']);
        });
    }

    // Data loading methods
    async loadData(filters = {}) {
        const loading = document.getElementById('loading');
        if (loading) loading.style.display = 'block';
        
        try {
            // Get viewport bounds for spatial filtering
            const bounds = this.map.getBounds();
            const params = new URLSearchParams({
                ...filters,
                min_lat: bounds.getSouth(),
                max_lat: bounds.getNorth(),
                min_lng: bounds.getWest(),
                max_lng: bounds.getEast(),
                limit: 5000 // Adjust based on zoom level
            });
            
            const response = await fetch(`/api/records/geojson?${params}`);
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }
            
            const data = await response.json();
            this.currentData = data;
            
            // Update map source
            this.map.getSource('records').setData(data);
            
            // Dispatch data updated event
            this.dispatchEvent('dataUpdated', { data, filters });
            
        } catch (error) {
            console.error('Error loading map data:', error);
            this.dispatchEvent('error', { error });
        } finally {
            if (loading) loading.style.display = 'none';
        }
    }

    // Selection tool methods
    setSelectionMode(mode) {
        this.selectionMode = mode;
        this.clearSelection();
        
        // Update cursor
        if (mode) {
            this.map.getCanvas().style.cursor = 'crosshair';
        } else {
            this.map.getCanvas().style.cursor = '';
        }
        
        // Update UI
        this.dispatchEvent('selectionModeChanged', { mode });
    }

    handleRectangleSelection(e) {
        if (!this.selectionActive) {
            // Start rectangle selection
            this.selectionStart = e.lngLat;
            this.selectionActive = true;
            this.map.getCanvas().style.cursor = 'crosshair';
        } else {
            // Complete rectangle selection
            this.completeRectangleSelection(e.lngLat);
        }
    }

    handleCircleSelection(e) {
        if (!this.selectionActive) {
            // Start circle selection
            this.selectionStart = e.lngLat;
            this.selectionActive = true;
            this.map.getCanvas().style.cursor = 'crosshair';
        } else {
            // Complete circle selection
            this.completeCircleSelection(e.lngLat);
        }
    }

    completeRectangleSelection(endPoint) {
        const sw = [
            Math.min(this.selectionStart.lng, endPoint.lng),
            Math.min(this.selectionStart.lat, endPoint.lat)
        ];
        const ne = [
            Math.max(this.selectionStart.lng, endPoint.lng),
            Math.max(this.selectionStart.lat, endPoint.lat)
        ];

        const bounds = new maplibregl.LngLatBounds(sw, ne);
        this.selectFeaturesInBounds(bounds);
        this.drawSelectionRectangle(sw, ne);
        
        this.selectionActive = false;
        this.dispatchEvent('selectionCompleted', { 
            type: 'rectangle', 
            bounds: bounds,
            features: this.selectionFeatures 
        });
    }

    completeCircleSelection(endPoint) {
        const center = this.selectionStart;
        const radius = this.map.project(center).dist(this.map.project(endPoint));
        
        this.selectFeaturesInCircle(center, radius);
        this.drawSelectionCircle(center, radius);
        
        this.selectionActive = false;
        this.dispatchEvent('selectionCompleted', { 
            type: 'circle', 
            center: center,
            radius: radius,
            features: this.selectionFeatures 
        });
    }

    selectFeaturesInBounds(bounds) {
        if (!this.currentData) return;
        
        this.selectionFeatures = this.currentData.features.filter(feature => {
            const [lng, lat] = feature.geometry.coordinates;
            return bounds.contains([lng, lat]);
        });
    }

    selectFeaturesInCircle(center, radiusPixels) {
        if (!this.currentData) return;
        
        const centerPoint = this.map.project(center);
        
        this.selectionFeatures = this.currentData.features.filter(feature => {
            const [lng, lat] = feature.geometry.coordinates;
            const point = this.map.project([lng, lat]);
            const distance = centerPoint.dist(point);
            return distance <= radiusPixels;
        });
    }

    drawSelectionRectangle(sw, ne) {
        const rectangle = {
            type: 'Feature',
            geometry: {
                type: 'Polygon',
                coordinates: [[
                    [sw[0], sw[1]],
                    [ne[0], sw[1]],
                    [ne[0], ne[1]],
                    [sw[0], ne[1]],
                    [sw[0], sw[1]]
                ]]
            }
        };
        
        this.map.getSource('selection').setData({
            type: 'FeatureCollection',
            features: [rectangle]
        });
    }

    drawSelectionCircle(center, radiusPixels) {
        // Convert pixel radius to approximate geographic radius
        const radiusMeters = radiusPixels * 0.5; // Rough approximation
        const points = 64;
        const coords = [];
        
        for (let i = 0; i < points; i++) {
            const angle = (i / points) * 2 * Math.PI;
            const dx = radiusMeters * Math.cos(angle) / 111000; // Rough deg conversion
            const dy = radiusMeters * Math.sin(angle) / 111000;
            coords.push([center.lng + dx, center.lat + dy]);
        }
        coords.push(coords[0]); // Close the circle
        
        const circle = {
            type: 'Feature',
            geometry: {
                type: 'Polygon',
                coordinates: [coords]
            }
        };
        
        this.map.getSource('selection').setData({
            type: 'FeatureCollection',
            features: [circle]
        });
    }

    clearSelection() {
        this.selectionFeatures = [];
        this.selectionActive = false;
        
        if (this.map.getSource('selection')) {
            this.map.getSource('selection').setData({
                type: 'FeatureCollection',
                features: []
            });
        }
        
        this.dispatchEvent('selectionCleared');
    }

    // LLPG layer methods
    async toggleLLPGLayer() {
        if (this.llpgLayerVisible) {
            this.hideLLPGLayer();
        } else {
            await this.showLLPGLayer();
        }
    }

    async showLLPGLayer() {
        try {
            // Load LLPG data from API
            const bounds = this.map.getBounds();
            const params = new URLSearchParams({
                source_type: 'llpg',
                min_lat: bounds.getSouth(),
                max_lat: bounds.getNorth(),
                min_lng: bounds.getWest(),
                max_lng: bounds.getEast(),
                limit: 2000
            });
            
            const response = await fetch(`/api/search/llpg?${params}`);
            if (!response.ok) return;
            
            const llpgData = await response.json();
            
            // Convert to GeoJSON
            const features = llpgData.map(record => ({
                type: 'Feature',
                geometry: {
                    type: 'Point',
                    coordinates: [record.easting, record.northing] // Will need coordinate conversion
                },
                properties: record
            }));
            
            // Add LLPG source and layer
            this.map.addSource('llpg-overlay', {
                type: 'geojson',
                data: { type: 'FeatureCollection', features }
            });

            this.map.addLayer({
                id: 'llpg-points',
                type: 'circle',
                source: 'llpg-overlay',
                paint: {
                    'circle-color': '#17a2b8',
                    'circle-radius': 3,
                    'circle-stroke-width': 1,
                    'circle-stroke-color': '#fff',
                    'circle-opacity': 0.7
                }
            });

            this.llpgLayerVisible = true;
            this.dispatchEvent('llpgLayerToggled', { visible: true });
            
        } catch (error) {
            console.error('Error loading LLPG layer:', error);
        }
    }

    hideLLPGLayer() {
        if (this.map.getLayer('llpg-points')) {
            this.map.removeLayer('llpg-points');
            this.map.removeSource('llpg-overlay');
        }
        
        this.llpgLayerVisible = false;
        this.dispatchEvent('llpgLayerToggled', { visible: false });
    }

    // Utility methods
    fitToSelection() {
        if (this.selectionFeatures.length === 0) return;
        
        const bounds = new maplibregl.LngLatBounds();
        this.selectionFeatures.forEach(feature => {
            bounds.extend(feature.geometry.coordinates);
        });
        
        this.map.fitBounds(bounds, { padding: 50 });
    }

    exportSelection() {
        if (this.selectionFeatures.length === 0) return null;
        
        return {
            type: 'FeatureCollection',
            features: this.selectionFeatures
        };
    }

    // Event system
    // Coordinate picking methods
    setCoordinatePickingMode(enabled, callback = null) {
        this.coordinatePickingMode = enabled;
        this.coordinatePickingCallback = callback;
        
        if (enabled) {
            this.map.getCanvas().style.cursor = 'crosshair';
            this.dispatchEvent('coordinatePickingStarted');
        } else {
            this.map.getCanvas().style.cursor = '';
            this.coordinatePickingCallback = null;
            this.dispatchEvent('coordinatePickingStopped');
        }
    }

    handleCoordinatePicking(e) {
        if (!this.coordinatePickingMode) return;
        
        const lngLat = e.lngLat;
        
        // Convert WGS84 to BNG coordinates (approximate conversion)
        // For accurate conversion, you'd use a proper projection library
        const coordinates = this.convertToBNG(lngLat.lng, lngLat.lat);
        
        // Add temporary marker at clicked location
        this.addCoordinatePickMarker(lngLat);
        
        // Call callback if provided
        if (this.coordinatePickingCallback) {
            this.coordinatePickingCallback(coordinates);
        }
        
        // Disable picking mode
        this.setCoordinatePickingMode(false);
    }

    addCoordinatePickMarker(lngLat) {
        // Remove existing pick marker
        this.removeCoordinatePickMarker();
        
        // Add new marker
        this.coordinatePickMarker = new maplibregl.Marker({
            color: '#ff6b00',
            scale: 1.2
        })
        .setLngLat(lngLat)
        .addTo(this.map);

        // Auto-remove after 3 seconds
        setTimeout(() => {
            this.removeCoordinatePickMarker();
        }, 3000);
    }

    removeCoordinatePickMarker() {
        if (this.coordinatePickMarker) {
            this.coordinatePickMarker.remove();
            this.coordinatePickMarker = null;
        }
    }

    convertToBNG(lng, lat) {
        // Simplified conversion - in production, use a proper projection library
        // This is a rough approximation for Hampshire area
        const eastingApprox = (lng + 1.0) * 50000 + 400000;
        const northingApprox = (lat - 50.9) * 111000 + 105000;
        
        return {
            easting: Math.round(eastingApprox),
            northing: Math.round(northingApprox)
        };
    }

    dispatchEvent(eventType, data = {}) {
        const event = new CustomEvent(`map:${eventType}`, { detail: data });
        document.dispatchEvent(event);
    }
}

// Custom Map Controls
class SelectionControl {
    constructor(mapComponent) {
        this.mapComponent = mapComponent;
    }

    onAdd(map) {
        this.container = document.createElement('div');
        this.container.className = 'maplibregl-ctrl maplibregl-ctrl-group';
        
        this.container.innerHTML = `
            <button id="selection-rectangle" class="selection-btn" title="Rectangle Selection">
                <svg width="16" height="16" viewBox="0 0 16 16">
                    <rect x="2" y="4" width="12" height="8" fill="none" stroke="currentColor" stroke-width="2"/>
                </svg>
            </button>
            <button id="selection-circle" class="selection-btn" title="Circle Selection">
                <svg width="16" height="16" viewBox="0 0 16 16">
                    <circle cx="8" cy="8" r="6" fill="none" stroke="currentColor" stroke-width="2"/>
                </svg>
            </button>
            <button id="selection-clear" class="selection-btn" title="Clear Selection">
                <svg width="16" height="16" viewBox="0 0 16 16">
                    <path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="2"/>
                </svg>
            </button>
        `;

        // Event handlers
        this.container.querySelector('#selection-rectangle').onclick = () => {
            this.toggleMode('rectangle');
        };

        this.container.querySelector('#selection-circle').onclick = () => {
            this.toggleMode('circle');
        };

        this.container.querySelector('#selection-clear').onclick = () => {
            this.mapComponent.clearSelection();
            this.updateButtons();
        };

        return this.container;
    }

    toggleMode(mode) {
        const currentMode = this.mapComponent.selectionMode;
        const newMode = currentMode === mode ? null : mode;
        this.mapComponent.setSelectionMode(newMode);
        this.updateButtons(newMode);
    }

    updateButtons(activeMode = null) {
        this.container.querySelectorAll('.selection-btn').forEach(btn => {
            btn.classList.remove('active');
        });

        if (activeMode) {
            const activeBtn = this.container.querySelector(`#selection-${activeMode}`);
            if (activeBtn) activeBtn.classList.add('active');
        }
    }

    onRemove() {
        this.container.parentNode.removeChild(this.container);
    }
}

class LLPGLayerControl {
    constructor(mapComponent) {
        this.mapComponent = mapComponent;
    }

    onAdd(map) {
        this.container = document.createElement('div');
        this.container.className = 'maplibregl-ctrl maplibregl-ctrl-group';
        
        this.container.innerHTML = `
            <button id="llpg-toggle" class="llpg-btn" title="Toggle LLPG Layer">
                <svg width="16" height="16" viewBox="0 0 16 16">
                    <g fill="none" stroke="currentColor" stroke-width="2">
                        <rect x="1" y="1" width="6" height="6"/>
                        <rect x="9" y="1" width="6" height="6"/>
                        <rect x="1" y="9" width="6" height="6"/>
                        <rect x="9" y="9" width="6" height="6"/>
                    </g>
                </svg>
            </button>
        `;

        this.container.querySelector('#llpg-toggle').onclick = () => {
            this.mapComponent.toggleLLPGLayer();
        };

        // Listen for layer toggle events
        document.addEventListener('map:llpgLayerToggled', (e) => {
            this.updateButton(e.detail.visible);
        });

        return this.container;
    }

    updateButton(visible) {
        const btn = this.container.querySelector('#llpg-toggle');
        if (visible) {
            btn.classList.add('active');
        } else {
            btn.classList.remove('active');
        }
    }

    onRemove() {
        this.container.parentNode.removeChild(this.container);
    }
}

class StatsControl {
    onAdd(map) {
        this.container = document.createElement('div');
        this.container.className = 'maplibregl-ctrl stats-control';
        this.container.innerHTML = `
            <div class="stats-summary">
                <span id="visible-count">0</span> records visible
            </div>
        `;

        // Listen for data updates
        document.addEventListener('map:dataUpdated', (e) => {
            this.updateStats(e.detail.data);
        });

        return this.container;
    }

    updateStats(data) {
        const count = data.features ? data.features.length : 0;
        const countEl = this.container.querySelector('#visible-count');
        if (countEl) {
            countEl.textContent = count.toLocaleString();
        }
    }

    onRemove() {
        this.container.parentNode.removeChild(this.container);
    }
}

// Record Popup Component
class RecordPopup {
    constructor(properties, mapComponent) {
        this.properties = properties;
        this.mapComponent = mapComponent;
    }

    show(coordinates) {
        const props = this.properties;
        
        const popupContent = `
            <div class="record-popup">
                <div class="popup-header">
                    <h4>${props.source_type.toUpperCase()}</h4>
                    <span class="status-badge status-${props.match_status}">${props.match_status}</span>
                </div>
                <div class="popup-content">
                    <div class="popup-field">
                        <label>Address:</label>
                        <span>${props.address || 'N/A'}</span>
                    </div>
                    ${props.match_score ? `
                        <div class="popup-field">
                            <label>Match Score:</label>
                            <span class="score">${(props.match_score * 100).toFixed(1)}%</span>
                        </div>
                    ` : ''}
                    ${props.external_reference ? `
                        <div class="popup-field">
                            <label>Reference:</label>
                            <span>${props.external_reference}</span>
                        </div>
                    ` : ''}
                    <div class="popup-field">
                        <label>Quality:</label>
                        <span class="quality-${props.address_quality}">${props.address_quality}</span>
                    </div>
                    ${props.match_method ? `
                        <div class="popup-field">
                            <label>Method:</label>
                            <span>${props.match_method}</span>
                        </div>
                    ` : ''}
                </div>
                <div class="popup-actions">
                    <button onclick="viewRecordDetails(${props.src_id})" class="btn-primary">
                        View Details
                    </button>
                    ${props.match_status !== 'MATCHED' ? `
                        <button onclick="showMatchCandidates(${props.src_id})" class="btn-secondary">
                            Find Matches
                        </button>
                    ` : ''}
                </div>
            </div>
        `;
        
        new maplibregl.Popup({ closeOnClick: true, maxWidth: '400px' })
            .setLngLat(coordinates)
            .setHTML(popupContent)
            .addTo(this.mapComponent.map);
    }
}

// Export the MapComponent class
window.MapComponent = MapComponent;