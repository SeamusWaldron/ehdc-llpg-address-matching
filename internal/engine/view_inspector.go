package engine

import (
	"database/sql"
	"fmt"
)

// ViewInspector helps examine database views
type ViewInspector struct {
	db *sql.DB
}

// NewViewInspector creates a new view inspector
func NewViewInspector(db *sql.DB) *ViewInspector {
	return &ViewInspector{db: db}
}

// ShowViewStructure shows the structure of a view
func (vi *ViewInspector) ShowViewStructure(viewName string) error {
	fmt.Printf("=== Structure of %s ===\n", viewName)
	
	// Get sample data to understand the columns
	rows, err := vi.db.Query(fmt.Sprintf("SELECT * FROM %s LIMIT 1", viewName))
	if err != nil {
		return fmt.Errorf("failed to query view: %w", err)
	}
	defer rows.Close()
	
	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("failed to get columns: %w", err)
	}
	
	fmt.Printf("Columns (%d total):\n", len(columns))
	for i, col := range columns {
		fmt.Printf("%2d. %s\n", i+1, col)
	}
	
	return nil
}

// InspectMapViews shows all v_map_* views
func (vi *ViewInspector) InspectMapViews() error {
	// Check what v_map_ views exist
	rows, err := vi.db.Query(`
		SELECT table_name 
		FROM information_schema.views 
		WHERE table_schema = 'public' 
		AND table_name LIKE 'v_map_%'
		ORDER BY table_name
	`)
	if err != nil {
		return fmt.Errorf("failed to query views: %w", err)
	}
	defer rows.Close()
	
	fmt.Println("=== Available v_map_ views ===")
	var viewNames []string
	for rows.Next() {
		var viewName string
		if err := rows.Scan(&viewName); err != nil {
			continue
		}
		viewNames = append(viewNames, viewName)
		fmt.Printf("- %s\n", viewName)
	}
	
	// Show structure of each view
	for _, viewName := range viewNames {
		fmt.Println()
		if err := vi.ShowViewStructure(viewName); err != nil {
			fmt.Printf("Error inspecting %s: %v\n", viewName, err)
		}
	}
	
	return nil
}