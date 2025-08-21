# EHDC LLPG Web Interface Design Document

## Project Overview

Transform the static HTML mockup (`@planning-records-ui.html`) into a dynamic web application that visualizes address matching results on an interactive map, integrated with the existing Go backend and PostgreSQL database.

## Technical Architecture

### Backend Components

#### 1. Web Server (Go)
```
internal/web/
├── server.go              # HTTP server setup
├── config.go              # Configuration management
├── handlers/              # API endpoint handlers
│   ├── api.go            # API base handler
│   ├── records.go        # Record CRUD operations
│   ├── maps.go           # GeoJSON endpoints
│   ├── search.go         # LLPG search functionality
│   └── export.go         # Data export endpoints
├── middleware/           # HTTP middleware
│   ├── auth.go          # Authentication
│   ├── cors.go          # CORS handling
│   └── logging.go       # Request logging
└── static/              # Frontend assets
    ├── index.html       # Main application
    ├── js/             # JavaScript modules
    │   ├── app.js      # Main application logic
    │   ├── map.js      # MapLibre integration
    │   ├── filters.js  # Filtering system
    │   └── api.js      # Backend communication
    └── css/
        └── app.css     # Enhanced from existing styles
```

#### 2. Database Integration
- **Existing Views**: Leverage `v_map_*` views for all data access
- **GeoJSON Function**: Use `get_record_geojson()` for map data
- **Real-time Updates**: WebSocket support for live data changes
- **Spatial Queries**: PostGIS integration for geographic filtering

### API Endpoints

```
# Core Data Endpoints
GET    /api/records                    # Filtered records list
GET    /api/records/geojson           # GeoJSON data for map
GET    /api/records/:id               # Individual record details
GET    /api/records/:id/candidates    # Match candidates for record

# Modification Endpoints  
POST   /api/records/:id/accept        # Accept a match candidate
PUT    /api/records/:id/coordinates   # Set manual coordinates
POST   /api/records/:id/reject        # Reject all candidates

# Search & Utility
GET    /api/search/llpg              # LLPG address search
GET    /api/search/records           # Full-text address search
POST   /api/export                   # Generate data exports

# Statistics & Analytics
GET    /api/stats                    # Overall statistics
GET    /api/stats/viewport           # Statistics for map viewport
```

### Frontend Architecture

#### 1. Core Components

```javascript
// Main Application Class
class EHDCMappingApp {
    constructor() {
        this.map = new MapComponent();
        this.filters = new FilterPanel();
        this.drawer = new DetailsDrawer();
        this.exporter = new ExportManager();
    }
}

// Map Integration
class MapComponent {
    // MapLibre GL JS integration
    // Clustered marker rendering
    // Real-time data loading
    // Selection tools (rectangle, circle)
    // LLPG layer overlay
}

// Filtering System
class FilterPanel {
    // Confidence range sliders
    // Source type checkboxes
    // Address text search
    // Live filter application
    // URL state synchronization
}

// Record Details
class DetailsDrawer {
    // Record information display
    // Match candidate presentation
    // Manual acceptance workflow
    // LLPG search integration
    // Coordinate override system
}

// Export Management
class ExportManager {
    // Selection-based exports
    // Viewport exports
    // Multiple format support
    // Download management
}
```

#### 2. Data Flow

```
User Action → Filter Update → API Request → Database Query → 
GeoJSON Response → Map Update → UI Refresh
```

#### 3. State Management

```javascript
// Application State
const AppState = {
    // Map state
    map: {
        center: [lat, lng],
        zoom: level,
        selection: {...}
    },
    
    // Filter state
    filters: {
        sourceTypes: ['decision', 'land_charge', ...],
        confidenceRange: [min, max],
        addressQuery: '',
        matchStatus: ['MATCHED', 'UNMATCHED', ...]
    },
    
    // UI state
    ui: {
        selectedRecord: id,
        drawerOpen: boolean,
        loading: boolean
    }
};
```

## Implementation Phases

### Phase 1: Backend API (Week 1)

**Priority Tasks:**
1. Create Go HTTP server with routing
2. Implement core API endpoints
3. Database integration using existing map views
4. GeoJSON endpoint for map data
5. Basic authentication middleware

**Deliverables:**
- Functional REST API
- Database integration complete
- GeoJSON serving working
- Authentication system

### Phase 2: Interactive Map (Week 2) 

**Priority Tasks:**
1. MapLibre GL JS integration
2. Load real data from API
3. Implement marker clustering
4. Add selection tools
5. LLPG layer overlay

**Deliverables:**
- Interactive map with real data
- Clustered markers by confidence
- Geographic selection tools
- LLPG overlay toggle

### Phase 3: Filtering System (Week 2-3)

**Priority Tasks:**
1. Dynamic filter interface
2. Real-time API communication
3. URL-based state management
4. Performance optimization
5. Live search functionality

**Deliverables:**
- Complete filtering interface
- Real-time data updates
- Shareable URLs
- LLPG search integration

### Phase 4: Record Management (Week 3)

**Priority Tasks:**
1. Details drawer implementation
2. Match candidate display
3. Acceptance/rejection workflow
4. Manual coordinate override
5. Audit trail integration

**Deliverables:**
- Interactive record details
- Match management workflow
- Coordinate override system
- Complete audit trail

### Phase 5: Export & Analytics (Week 4)

**Priority Tasks:**
1. Selection-based exports
2. Multiple format support
3. Analytics dashboard
4. Performance monitoring
5. Bulk operations

**Deliverables:**
- Export functionality
- Analytics interface
- Performance metrics
- Bulk operations

## Database Integration

### Map Views Structure

```sql
-- All map views provide consistent schema:
src_id, source_type, filepath, external_reference, doc_type, 
doc_date, address, uprn, easting, northing, address_quality, 
match_status, match_method, match_score, address_similarity, usrn
```

### Key Views:
- `v_map_decisions` - Planning decisions (76,172 records)
- `v_map_land_charges` - Land charges (49,760 records)  
- `v_map_enforcement` - Enforcement notices (1,172 records)
- `v_map_agreements` - Planning agreements (2,602 records)
- `v_map_all_records` - Combined view (129,706 total records)

### GeoJSON Function:
```sql
SELECT * FROM get_record_geojson(
    p_source_type := 'decision',
    p_match_status := 'MATCHED',
    p_min_score := 0.8,
    p_limit := 1000
);
```

## User Interface Features

### Map Features
- **Color-coded markers** by confidence level:
  - Red: No location/coordinates
  - Orange: Low confidence (< 0.80)
  - Blue: Review needed (0.80-0.92)  
  - Green: High confidence (≥ 0.92)
- **Marker clustering** for performance
- **Selection tools** (rectangle, circle)
- **LLPG layer toggle**
- **Coordinate picker** for manual overrides

### Filter Panel
- **Confidence range** dual sliders
- **Source type** multi-select
- **Match status** toggles
- **Address search** with autocomplete
- **Export options** for filtered data

### Details Drawer
- **Record information** display
- **Match candidates** with scores
- **Accept/reject** workflow
- **Manual coordinate** setting
- **LLPG search** integration
- **Audit trail** display

### Export Options
- **CSV format** (enhanced with map data)
- **GeoJSON format** for GIS systems
- **Selection-based** exports
- **Viewport-based** exports
- **Full dataset** exports

## Performance Considerations

### Backend Performance
- **Database indexing** on filtered columns
- **Spatial indexing** for geographic queries
- **Connection pooling** for database access
- **Caching** for frequently accessed data
- **Pagination** for large datasets

### Frontend Performance  
- **Marker clustering** (10k+ points)
- **Viewport-based** data loading
- **Debounced filtering** (500ms)
- **Virtual scrolling** for large lists
- **Lazy loading** for images/details

### Target Metrics
- **Map load time**: < 3 seconds for 10k+ markers
- **Filter response**: < 500ms for complex queries
- **Export generation**: < 30 seconds for full dataset
- **Search latency**: < 200ms for LLPG queries

## Security & Authentication

### Authentication Strategy
- **Session-based** authentication
- **Role-based** access control
- **CSRF protection** for state changes
- **Rate limiting** for API endpoints

### Data Protection
- **Input validation** for all endpoints
- **SQL injection** prevention (parameterized queries)
- **XSS protection** in frontend
- **Audit logging** for all changes

## Deployment Strategy

### Development Environment
```
# Local development
go run cmd/web/main.go --config dev.json
```

### Production Environment  
```
# Build and deploy
go build -o ehdc-web cmd/web/main.go
./ehdc-web --config production.json
```

### Configuration Management
```json
{
    "server": {
        "port": 8080,
        "host": "0.0.0.0"
    },
    "database": {
        "url": "postgres://...",
        "max_connections": 25
    },
    "auth": {
        "enabled": true,
        "session_key": "..."
    },
    "features": {
        "export_enabled": true,
        "manual_override_enabled": true
    }
}
```

## Success Metrics

### Technical Metrics
- **Uptime**: > 99.5%
- **Response time**: < 1 second average
- **Error rate**: < 1%
- **Data accuracy**: 100% consistency with database

### User Experience Metrics
- **Mobile responsive** design (tested on iOS/Android)
- **Accessibility compliant** (WCAG 2.1 AA)
- **Cross-browser** support (Chrome, Firefox, Safari, Edge)
- **Intuitive workflows** (< 3 clicks for common tasks)

### Business Metrics
- **Coverage improvement**: Track match rate increases
- **Processing efficiency**: Time saved vs manual review
- **Data quality**: Reduction in matching errors
- **User adoption**: Active usage statistics

## Integration Points

### With Existing System
1. **Database views** - Use existing `v_map_*` views
2. **Matching algorithms** - Trigger new matching runs
3. **Export system** - Leverage existing CSV export
4. **Audit trail** - Maintain decision history
5. **Authentication** - Integrate with existing auth

### External Systems
1. **GIS systems** - GeoJSON exports
2. **Business systems** - API integration
3. **Reporting tools** - Data export formats
4. **Monitoring systems** - Health checks and metrics

## Development Guidelines

### Code Standards
- **Go**: Follow standard Go conventions
- **JavaScript**: ES6+ with modules
- **CSS**: BEM methodology with CSS variables
- **SQL**: Consistent naming and formatting

### Testing Strategy
- **Unit tests** for all business logic
- **Integration tests** for API endpoints
- **End-to-end tests** for critical workflows
- **Performance tests** for map operations

### Documentation Requirements
- **API documentation** (OpenAPI/Swagger)
- **Database schema** documentation
- **User guide** for interface
- **Deployment guide** for operations

## Next Steps

1. **Review and approve** this design document
2. **Set up development environment** and project structure
3. **Begin Phase 1 implementation** (Backend API)
4. **Establish CI/CD pipeline** for automated testing
5. **Create initial deployment** for testing

This comprehensive design provides a roadmap for creating a production-ready web interface that leverages all the sophisticated address matching capabilities you've built while providing an intuitive, powerful mapping tool for EHDC users.

---

**Total Implementation Time**: ~4 weeks
**Team Size**: 1-2 developers
**Primary Technologies**: Go, PostgreSQL/PostGIS, MapLibre GL JS, Vanilla JavaScript